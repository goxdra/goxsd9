package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // One graph checks public facts, provenance, copies, and traversal across policies.
func TestPositiveIntegerAttributeConstraintsAcrossPolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:include schemaLocation="cycle.xsd"/>
  <xs:include schemaLocation="cycle.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="boundary" type="xs:positiveInteger" default=" &#x9;+0001&#xA; "/>
  <xs:attribute name="directFixed" type="xs:positiveInteger" fixed="2"/>
  <xs:attribute name="large" type="xs:positiveInteger" default="123456789012345678901234567890"/>
  <xs:attribute name="forward" type="r:Forward" default="0007"/>
  <xs:attribute name="imported" type="o:Remote" fixed="42"/>
  <xs:attribute name="narrowed" type="r:Narrowed" default="5"/>
  <xs:attribute name="typeOnly" type="xs:positiveInteger"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:positiveInteger"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="xs:positiveInteger"><xs:minInclusive value="2"/><xs:maxExclusive value="10"/><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":  {id: "root.xsd", contents: root},
		"cycle.xsd": {id: "cycle.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/></xs:schema>`},
		"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Remote"><xs:restriction base="xs:positiveInteger"><xs:maxInclusive value="42"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
	want := []struct {
		name, typeName, marker, lexical, canonical, facetSource, facetMarker string
		kind                                                                 AttributeValueConstraintKind
	}{
		{"boundary", "xs:positiveInteger", `default=" &#x9;+0001`, "+0001", "1", "", "", AttributeValueConstraintDefault},
		{"directFixed", "xs:positiveInteger", `fixed="2"`, "2", "2", "", "", AttributeValueConstraintFixed},
		{"large", "xs:positiveInteger", `default="123456789012345678901234567890"`, "123456789012345678901234567890", "123456789012345678901234567890", "", "", AttributeValueConstraintDefault},
		{"forward", "r:Forward", `default="0007"`, "0007", "7", "root.xsd", `<xs:enumeration value="7"`, AttributeValueConstraintDefault},
		{"imported", "o:Remote", `fixed="42"`, "42", "42", "other.xsd", `value="42"`, AttributeValueConstraintFixed},
		{"narrowed", "r:Narrowed", `default="5"`, "5", "5", "root.xsd", `value="2"`, AttributeValueConstraintDefault},
	}
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) || len(first.Documents()) != 3 {
				t.Fatal("graph order or visibility changed")
			}
			components := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "boundary"))
			if len(components) != 1 {
				t.Fatal("boundary declaration missing")
			}
			for _, expected := range want {
				found := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", expected.name))
				if len(found) != 1 {
					t.Fatalf("%s declaration count = %d", expected.name, len(found))
				}
				declaration, ok := found[0].AttributeDeclaration()
				if !ok || declaration.ID() != found[0].ID() {
					t.Fatalf("%s declaration identity", expected.name)
				}
				constraint := requireAttributeValueConstraint(t, found[0])
				if constraint.Kind() != expected.kind || constraint.IsDefault() != (expected.kind == AttributeValueConstraintDefault) || constraint.IsFixed() != (expected.kind == AttributeValueConstraintFixed) || constraint.Lexical() != expected.lexical || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, expected.marker) {
					t.Fatalf("%s constraint facts = %q/%q/%s", expected.name, constraint.Kind(), constraint.Lexical(), constraint.Loc())
				}
				value, ok := constraint.IntegerValue()
				if !ok || value.Canonical() != expected.canonical {
					t.Fatalf("%s value = %q/%t", expected.name, value.Canonical(), ok)
				}
				if _, hasDecimal := constraint.DecimalValue(); hasDecimal {
					t.Fatalf("%s has decimal", expected.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Loc() != unsignedLongParticleTokenLocAfter(t, root, `<xs:attribute name="`+expected.name+`"`, `type="`+expected.typeName+`"`) {
					t.Fatalf("%s type reference/location", expected.name)
				}
				if expected.facetMarker == "" && !reference.IsBuiltin() {
					t.Fatalf("%s not built in", expected.name)
				}
				if expected.facetMarker != "" {
					id, present := reference.ComponentID()
					if !reference.IsNamed() || !present || id.IsZero() {
						t.Fatalf("%s named identity", expected.name)
					}
				}
				bounds, ok := reference.IntegerBounds()
				if !ok {
					t.Fatalf("%s bounds missing", expected.name)
				}
				minimum, ok := bounds.MinInclusiveFacet()
				wantMinimum := "1"
				if expected.name == "narrowed" {
					wantMinimum = "2"
				}
				if !ok || minimum.Value().Canonical() != wantMinimum {
					t.Fatalf("%s minimum = %#v", expected.name, minimum)
				}
				if expected.facetMarker != "" {
					wantLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.facetSource), expected.facetMarker, root, fixtures)
					foundLoc := false
					if expected.name == "forward" {
						named := first.FindKind(ComponentKindSimpleTypeDefinition, reference.Name())
						definition, hasDefinition := named[0].SimpleTypeDefinition()
						if !hasDefinition {
							t.Fatal("named definition missing")
						}
						for _, loc := range definition.IntegerEnumerationFacets().Locations() {
							foundLoc = foundLoc || loc == wantLoc
						}
					}
					if expected.name != "forward" {
						for _, bound := range bounds.Bounds() {
							foundLoc = foundLoc || bound.Loc() == wantLoc
						}
					}
					if !foundLoc {
						t.Fatalf("%s facet location %s missing", expected.name, wantLoc)
					}
				}
				value.value.SetInt64(99)
				boundCopy := bounds.Bounds()
				boundCopy[0] = IntegerBoundFacet{}
				again := requireAttributeValueConstraint(t, first.FindKind(ComponentKindAttributeDeclaration, found[0].Name())[0])
				copied, ok := again.IntegerValue()
				if !ok || copied.Canonical() != expected.canonical || again.Lexical() != expected.lexical {
					t.Fatalf("%s copied value leaked", expected.name)
				}
				referenceAgain, _ := declaration.TypeReference()
				boundsAgain, _ := referenceAgain.IntegerBounds()
				if minAgain, ok := boundsAgain.MinInclusiveFacet(); !ok || minAgain.Value().Canonical() != minimum.Value().Canonical() {
					t.Fatalf("%s copied bounds leaked", expected.name)
				}
			}
			only := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "typeOnly"))
			if len(only) != 1 {
				t.Fatal("type-only declaration missing")
			}
			declaration, _ := only[0].AttributeDeclaration()
			if _, ok := declaration.ValueConstraint(); ok {
				t.Fatal("type-only declaration gained constraint")
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
				t.Fatal("Walk order differs from Components")
			}
		})
	}
}

//nolint:gocognit // Each row exits the same conversion boundary with a distinct cause.
func TestPositiveIntegerAttributeConstraintDiagnostics(t *testing.T) {
	tests := []struct {
		name, typeName, typeDecl, kind, lexical, innerCode, related string
		cause                                                       error
	}{
		{"zero", "xs:positiveInteger", "", "default", "0", BoundValueViolationCode, "", errBoundValueViolation},
		{"negative", "xs:positiveInteger", "", "fixed", "-1", BoundValueViolationCode, "", errBoundValueViolation},
		{"empty", "xs:positiveInteger", "", "fixed", "", InvalidIntegerLexicalCode, "", nil},
		{"malformed", "xs:positiveInteger", "", "default", "1.0", InvalidIntegerLexicalCode, "", nil},
		{"large exact", "r:Digits", `<xs:simpleType name="Digits"><xs:restriction base="xs:positiveInteger"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>`, "fixed", "123456789012345678901234567890", DigitFacetValueViolationCode, `value="2"`, errDigitFacetValueViolation},
		{"named bound", "r:Narrowed", `<xs:simpleType name="Narrowed"><xs:restriction base="xs:positiveInteger"><xs:maxExclusive value="10"/></xs:restriction></xs:simpleType>`, "fixed", "10", BoundValueViolationCode, `value="10"`, errBoundValueViolation},
		{"inherited bound", "r:Child", `<xs:simpleType name="Base"><xs:restriction base="xs:positiveInteger"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="r:Base"/></xs:simpleType>`, "default", "9", BoundValueViolationCode, `value="10"`, errBoundValueViolation},
		{"digits", "r:Digits", `<xs:simpleType name="Digits"><xs:restriction base="xs:positiveInteger"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>`, "fixed", "123", DigitFacetValueViolationCode, `value="2"`, errDigitFacetValueViolation},
		{"enumeration", "r:Enum", `<xs:simpleType name="Enum"><xs:restriction base="xs:positiveInteger"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>`, "default", "8", EnumerationValueViolationCode, `<xs:enumeration value="7"`, errEnumerationValueViolation},
	}
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="value" type="` + test.typeName + `" ` + test.kind + `="` + test.lexical + `"/>` + test.typeDecl + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("invalid constraint returned Schema")
				}
				d := requireDiagnostic(t, err)
				loc := elementReferenceTestAttributeLoc(t, root, test.kind+`="`+test.lexical+`"`)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeValueConstraintCode || d.Loc() != loc || d.SpecRef() != schemaAttributeValueSpecRef(profile.version) {
					t.Fatalf("outer diagnostic = %s", d)
				}
				if !errors.Is(err, errSchemaAttributeValueConstraintInvalid) || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("lost cause: %v", err)
				}
				inner := requireNestedDiagnostic(t, d)
				if inner.Code() != test.innerCode || inner.Loc() != loc {
					t.Fatalf("inner diagnostic = %s", inner)
				}
				if test.related != "" && !schemaLocationListContains(d.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related = %v", d.Related())
				}
			})
		}
	}
}

func TestPositiveIntegerAttributeConstraintConflict(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:attribute name="value" type="xs:positiveInteger" default="bad" fixed="bad"/></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err == nil || schema.storage != nil {
			t.Fatal("conflict returned Schema")
		}
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `fixed="bad"`) || !reflect.DeepEqual(d.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `default="bad"`)}) || d.SpecRef() != schemaAttributeValueConstraintSpecRef(profile.version) || !errors.Is(err, errSchemaAttributeValueConstraintConflict) {
			t.Fatalf("conflict diagnostic = %s", d)
		}
	}
}

//nolint:gocognit // Check each excluded attribute use shape and both public consumers.
func TestPositiveIntegerConstrainedAttributeExclusions(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, test := range []struct{ name, body, marker, related string }{
			{"local direct", `<xs:complexType name="T"><xs:attribute name="a" type="xs:positiveInteger" default="1"/></xs:complexType>`, `default="1"`, ""},
			{"local named", `<xs:simpleType name="L"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:complexType name="T"><xs:attribute name="a" type="r:L" fixed="1"/></xs:complexType>`, `fixed="1"`, ""},
			{"local inline", `<xs:complexType name="T"><xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:attribute></xs:complexType>`, `default="1"`, ""},
			{"global inline", `<xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:attribute>`, "<xs:simpleType>", ""},
			{"ref direct", `<xs:attribute name="a" type="xs:positiveInteger" fixed="1"/><xs:complexType name="T"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, `<xs:attribute name="a"`},
			{"ref named", `<xs:simpleType name="L"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:attribute name="a" type="r:L" default="1"/><xs:complexType name="T"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, `<xs:attribute name="a"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded shape returned Schema")
				}
				d := requireDiagnostic(t, err)
				wantRelated := []Loc(nil)
				if test.related != "" {
					wantRelated = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded diagnostic = %s related=%v", d, d.Related())
				}
				if test.related != "" && (!errors.Is(err, errSchemaAttributeReferenceUnsupported) || d.SpecRef() != schemaAttributeUseSpecRef(profile.version)) {
					t.Fatalf("ref diagnostic = %s", d)
				}
				if test.related == "" && d.SpecRef() != positiveIntegerExcludedAttributeSpecRef(profile.version, test.name) {
					t.Fatalf("excluded SpecRef = %q", d.SpecRef())
				}
			})
		}
		for _, typeName := range []string{"xs:positiveInteger", "r:Named"} {
			name := "direct"
			typeDecl := ""
			if typeName == "r:Named" {
				name, typeDecl = "named", `<xs:simpleType name="Named"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`
			}
			t.Run(profile.name+"/consumers/"+name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="a" type="` + typeName + `" default="1"/><xs:element name="root" type="xs:positiveInteger"/>` + typeDecl + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatal("GenerateGo accepted attribute")
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %s", d)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root">1</root>`)))
				if err == nil {
					t.Fatal("ValidateInstance accepted positiveInteger root")
				}
				d = requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(err, errInstanceUnsupportedType) {
					t.Fatalf("validation diagnostic = %s", d)
				}
			})
		}
	}
}

func positiveIntegerExcludedAttributeSpecRef(version XSDVersion, name string) string {
	if name == "global inline" {
		return "xsd10-structures#schema-document"
	}
	return schemaAttributeUseSpecRef(version)
}
