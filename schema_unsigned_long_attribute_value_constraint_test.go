package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // One graph checks ordered exact facts and copies at every discovery boundary.
func TestUnsignedLongAttributeValueConstraintsAcrossPolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:include schemaLocation="cycle.xsd"/>
  <xs:include schemaLocation="cycle.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="zero" type="xs:unsignedLong" default=" &#x9;000&#xA; "/>
  <xs:attribute name="maximum" type="xs:unsignedLong" fixed="18446744073709551615"/>
  <xs:attribute name="forward" type="r:Forward" default="0007"/>
  <xs:attribute name="imported" type="o:Remote" fixed="42"/>
  <xs:attribute name="chameleon" type="r:Chameleon" default="11"/>
  <xs:attribute name="narrowed" type="r:Narrowed" fixed="5"/>
  <xs:attribute name="unconstrained" type="xs:unsignedLong"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:unsignedLong"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="1"/><xs:maxExclusive value="10"/><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`},
		"cycle.xsd":     {id: "cycle.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Remote"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="42"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
	want := []struct {
		name, typeLexical, marker, lexical, canonical, facetSource, facetMarker string
		kind                                                                    AttributeValueConstraintKind
	}{
		{"zero", "xs:unsignedLong", `default=" &#x9;000`, "000", "0", "", "", AttributeValueConstraintDefault},
		{"maximum", "xs:unsignedLong", `fixed="18446744073709551615"`, "18446744073709551615", "18446744073709551615", "", "", AttributeValueConstraintFixed},
		{"forward", "r:Forward", `default="0007"`, "0007", "7", "root.xsd", `<xs:enumeration value="7"`, AttributeValueConstraintDefault},
		{"imported", "o:Remote", `fixed="42"`, "42", "42", "other.xsd", `value="42"`, AttributeValueConstraintFixed},
		{"chameleon", "r:Chameleon", `default="11"`, "11", "11", "chameleon.xsd", `value="10"`, AttributeValueConstraintDefault},
		{"narrowed", "r:Narrowed", `fixed="5"`, "5", "5", "root.xsd", `value="1"`, AttributeValueConstraintFixed},
	}
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) || len(first.Documents()) != 4 {
				t.Fatal("repeated/cyclic graph changed components or document count")
			}
			components := unsignedLongGlobalAttributeComponents(first)
			if len(components) != len(want)+1 {
				t.Fatalf("attributes = %d, want %d", len(components), len(want)+1)
			}
			for i, expected := range want {
				component := components[i]
				if component.Name() != mustTestQName(t, "urn:root", expected.name) {
					t.Fatalf("attribute %d = %q", i, component.Name())
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.ID() != component.ID() || declaration.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+expected.name+`"`) {
					t.Fatalf("%s declaration identity/location changed", expected.name)
				}
				constraint := requireAttributeValueConstraint(t, component)
				if constraint.Kind() != expected.kind || constraint.IsDefault() != (expected.kind == AttributeValueConstraintDefault) || constraint.IsFixed() != (expected.kind == AttributeValueConstraintFixed) || constraint.Lexical() != expected.lexical || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, expected.marker) {
					t.Fatalf("%s constraint kind/lexical/location = %q/%q/%s", expected.name, constraint.Kind(), constraint.Lexical(), constraint.Loc())
				}
				value, ok := constraint.IntegerValue()
				if !ok || value.Canonical() != expected.canonical {
					t.Fatalf("%s exact integer = %q/%t", expected.name, value.Canonical(), ok)
				}
				if _, hasDecimal := constraint.DecimalValue(); hasDecimal {
					t.Fatalf("%s has decimal value", expected.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Loc() != unsignedLongParticleTokenLocAfter(t, root, `<xs:attribute name="`+expected.name+`"`, `type="`+expected.typeLexical+`"`) {
					t.Fatalf("%s type reference/location changed", expected.name)
				}
				if i < 2 {
					if !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "unsignedLong") {
						t.Fatalf("%s lost built-in type identity", expected.name)
					}
				}
				if i >= 2 {
					id, hasID := reference.ComponentID()
					if !reference.IsNamed() || !hasID || id.IsZero() || reference.Name().Local() != strings.TrimPrefix(expected.typeLexical, "r:") && reference.Name().Local() != strings.TrimPrefix(expected.typeLexical, "o:") {
						t.Fatalf("%s lost named type identity", expected.name)
					}
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || len(bounds.Bounds()) < 2 {
					t.Fatalf("%s lost exact intrinsic/effective bounds", expected.name)
				}
				if expected.facetMarker != "" {
					wantLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.facetSource), expected.facetMarker, root, fixtures)
					found := false
					if expected.name == "forward" {
						named := first.FindKind(ComponentKindSimpleTypeDefinition, reference.Name())
						if len(named) != 1 {
							t.Fatal("forward named type missing")
						}
						definition, hasDefinition := named[0].SimpleTypeDefinition()
						if !hasDefinition {
							t.Fatal("forward simple type view missing")
						}
						for _, loc := range definition.IntegerEnumerationFacets().Locations() {
							found = found || loc == wantLoc
						}
					}
					if expected.name != "forward" {
						for _, bound := range bounds.Bounds() {
							found = found || bound.Loc() == wantLoc
						}
					}
					if !found {
						t.Fatalf("%s lost facet provenance at %s", expected.name, wantLoc)
					}
				}
				value.value.SetInt64(99)
				again := requireAttributeValueConstraint(t, first.FindKind(ComponentKindAttributeDeclaration, component.Name())[0])
				copied, ok := again.IntegerValue()
				if !ok || copied.Canonical() != expected.canonical || again.Lexical() != expected.lexical {
					t.Fatalf("%s copied value leaked", expected.name)
				}
			}
			unconstrained, ok := components[len(want)].AttributeDeclaration()
			if !ok {
				t.Fatal("unconstrained declaration missing")
			}
			if _, ok := unconstrained.ValueConstraint(); ok {
				t.Fatal("type-only attribute gained a value constraint")
			}
			var walked []ComponentID
			if err := first.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			var ordered []ComponentID
			for _, component := range first.Components() {
				ordered = append(ordered, component.ID())
			}
			if !reflect.DeepEqual(walked, ordered) {
				t.Fatal("walk order changed")
			}
		})
	}
}

//nolint:gocognit // Each row is an alternate exit at the value-conversion boundary.
func TestUnsignedLongAttributeConstraintDiagnostics(t *testing.T) {
	tests := []struct {
		name, typeName, typeDecl, kind, lexical, innerCode, related string
		cause                                                       error
	}{
		{"negative", "xs:unsignedLong", "", "default", "-1", BoundValueViolationCode, "", errBoundValueViolation},
		{"above maximum", "xs:unsignedLong", "", "fixed", "18446744073709551616", BoundValueViolationCode, "", errBoundValueViolation},
		{"malformed", "xs:unsignedLong", "", "default", "1.0", InvalidIntegerLexicalCode, "", nil},
		{"named bound", "r:Narrowed", `<xs:simpleType name="Narrowed"><xs:restriction base="xs:unsignedLong"><xs:maxExclusive value="10"/></xs:restriction></xs:simpleType>`, "fixed", "10", BoundValueViolationCode, `value="10"`, errBoundValueViolation},
		{"inherited bound", "r:Child", `<xs:simpleType name="Base"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="r:Base"/></xs:simpleType>`, "default", "9", BoundValueViolationCode, `value="10"`, errBoundValueViolation},
		{"digits", "r:Digits", `<xs:simpleType name="Digits"><xs:restriction base="xs:unsignedLong"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>`, "fixed", "123", DigitFacetValueViolationCode, `value="2"`, errDigitFacetValueViolation},
		{"enumeration", "r:Enum", `<xs:simpleType name="Enum"><xs:restriction base="xs:unsignedLong"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>`, "default", "8", EnumerationValueViolationCode, `<xs:enumeration value="7"`, errEnumerationValueViolation},
	}
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				innerCode, cause := test.innerCode, test.cause
				if profile.version == XSDVersion10 && test.name == "negative" {
					innerCode, cause = InvalidIntegerLexicalCode, nil
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="value" type="` + test.typeName + `" ` + test.kind + `="` + test.lexical + `"/>` + test.typeDecl + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("invalid unsignedLong constraint returned schema")
				}
				diagnostic := requireDiagnostic(t, err)
				loc := elementReferenceTestAttributeLoc(t, root, test.kind+`="`+test.lexical+`"`)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeValueConstraintCode || diagnostic.Loc() != loc || diagnostic.SpecRef() != schemaAttributeValueSpecRef(profile.version) {
					t.Fatalf("diagnostic = %s/%q", diagnostic, diagnostic.SpecRef())
				}
				if !errors.Is(err, errSchemaAttributeValueConstraintInvalid) || cause != nil && !errors.Is(err, cause) {
					t.Fatalf("lost invalid/facet cause: %v", err)
				}
				inner := requireNestedDiagnostic(t, diagnostic)
				if inner.Code() != innerCode || inner.Loc() != loc {
					t.Fatalf("nested diagnostic = %s", inner)
				}
				if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related = %v, want %s", diagnostic.Related(), test.related)
				}
			})
		}
	}
}

//nolint:gocognit // Compare the same signed spellings under every policy.
func TestUnsignedLongAttributeConstraintEditionLexical(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, lexical := range []string{"+0", "-0", "+18446744073709551615", "-000"} {
			t.Run(profile.name+"/"+lexical, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:attribute name="value" type="xs:unsignedLong" default="` + lexical + `"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.version == XSDVersion10 {
					if err == nil || schema.storage != nil {
						t.Fatal("XSD 1.0 accepted signed unsignedLong")
					}
					d := requireDiagnostic(t, err)
					if d.Code() != diagnosticSchemaAttributeValueConstraintCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `default="`+lexical+`"`) || d.SpecRef() != schemaAttributeValueSpecRef(profile.version) || requireNestedDiagnostic(t, d).Code() != InvalidIntegerLexicalCode {
						t.Fatalf("XSD 1.0 diagnostic = %s", d)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				constraint := requireAttributeValueConstraint(t, schema.Components()[0])
				value, ok := constraint.IntegerValue()
				canonical := "0"
				if lexical == "+18446744073709551615" {
					canonical = "18446744073709551615"
				}
				if !ok || constraint.Lexical() != lexical || value.Canonical() != canonical {
					t.Fatalf("signed value = %q/%q/%t", constraint.Lexical(), value.Canonical(), ok)
				}
			})
		}
	}
}

func TestUnsignedLongAttributeConstraintConflict(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:attribute name="value" type="xs:unsignedLong" default="bad" fixed="bad"/></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err == nil || schema.storage != nil {
			t.Fatal("conflict returned schema")
		}
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `fixed="bad"`) || d.SpecRef() != schemaAttributeValueConstraintSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `default="bad"`)}) || !errors.Is(err, errSchemaAttributeValueConstraintConflict) {
			t.Fatalf("conflict diagnostic = %s", d)
		}
	}
}

func TestUnsignedLongAttributeConstraintExcludedShapes(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct{ name, body, marker string }{
			{"local direct", `<xs:complexType name="T"><xs:attribute name="a" type="xs:unsignedLong" default="1"/></xs:complexType>`, `default="1"`},
			{"local named", `<xs:simpleType name="L"><xs:restriction base="xs:unsignedLong"/></xs:simpleType><xs:complexType name="T"><xs:attribute name="a" type="r:L" fixed="1"/></xs:complexType>`, `fixed="1"`},
			{"local inline", `<xs:complexType name="T"><xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:attribute></xs:complexType>`, `default="1"`},
			{"global inline", `<xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:attribute>`, `fixed="1"`},
			{"element direct", `<xs:element name="e" type="xs:unsignedLong" default="1"/>`, `default="1"`},
			{"element named", `<xs:element name="e" type="r:L" fixed="1"/><xs:simpleType name="L"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`, `fixed="1"`},
			{"element inline", `<xs:element name="e" default="1"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:element>`, `default="1"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded shape returned schema")
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded shape diagnostic = %s", d)
				}
			})
		}
	}
}

func TestUnsignedLongConstrainedAttributeReferencesRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct{ name, typeName, typeDecl string }{
			{"built-in", "xs:unsignedLong", ""},
			{"named", "r:Value", `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="a" type="` + test.typeName + `" fixed="1"/><xs:complexType name="T"><xs:attribute ref="r:a"/></xs:complexType>` + test.typeDecl + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("referenced constrained unsignedLong attribute returned schema")
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="r:a"`) || !reflect.DeepEqual(d.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
					t.Fatalf("reference diagnostic = %s related %v", d, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Exercise both consumers for built-in and named attribute types.
func TestUnsignedLongAttributeConstraintConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct{ name, typeName, typeDecl string }{
			{"built-in", "xs:unsignedLong", ""},
			{"named", "r:Value", `<xs:simpleType name="Value"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="a" type="` + test.typeName + `" default="1"/><xs:element name="root" type="` + test.typeName + `"/>` + test.typeDecl + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatal("GenerateGo accepted global unsignedLong attribute")
				}
				attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "a"))[0]
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != attribute.Loc() || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("GenerateGo diagnostic = %s", d)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root">1</root>`)))
				if err == nil {
					t.Fatal("ValidateInstance accepted unsignedLong element")
				}
				d = requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("ValidateInstance diagnostic = %s", d)
				}
			})
		}
	}
}
