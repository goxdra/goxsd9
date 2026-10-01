package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // One graph proves ordered public facts across policies and discovery boundaries.
func TestLongAttributeValueConstraintsAcrossPolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:include schemaLocation="cycle.xsd"/>
  <xs:include schemaLocation="cycle.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="minimum" type="xs:long" default=" &#x9;-9223372036854775808&#xA; "/>
  <xs:attribute name="maximum" type="xs:long" fixed="+9223372036854775807"/>
  <xs:attribute name="forward" type="r:Forward" default="-0007"/>
  <xs:attribute name="imported" type="o:Remote" fixed="42"/>
  <xs:attribute name="chameleon" type="r:Chameleon" default="11"/>
  <xs:attribute name="narrowed" type="r:Narrowed" fixed="5"/>
  <xs:attribute name="unconstrained" type="xs:long"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:long"><xs:enumeration value="-7"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="xs:long"><xs:minInclusive value="0"/><xs:maxExclusive value="10"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:long"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`},
		"cycle.xsd":     {id: "cycle.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Remote"><xs:restriction base="xs:long"><xs:maxInclusive value="42"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
	want := []struct {
		name, typeName, source, marker, lexical, canonical string
		kind                                               AttributeValueConstraintKind
		boundSource, boundMarker, boundValue               string
	}{
		{"minimum", "long", "root.xsd", `default=" &#x9;`, "-9223372036854775808", "-9223372036854775808", AttributeValueConstraintDefault, "", "", ""},
		{"maximum", "long", "root.xsd", `fixed="+9223372036854775807"`, "+9223372036854775807", "9223372036854775807", AttributeValueConstraintFixed, "", "", ""},
		{"forward", "Forward", "root.xsd", `default="-0007"`, "-0007", "-7", AttributeValueConstraintDefault, "", "", ""},
		{"imported", "Remote", "root.xsd", `fixed="42"`, "42", "42", AttributeValueConstraintFixed, "other.xsd", `value="42"`, "42"},
		{"chameleon", "Chameleon", "root.xsd", `default="11"`, "11", "11", AttributeValueConstraintDefault, "chameleon.xsd", `value="10"`, "10"},
		{"narrowed", "Narrowed", "root.xsd", `fixed="5"`, "5", "5", AttributeValueConstraintFixed, "root.xsd", `value="0"`, "0"},
	}
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if len(schema.Documents()) != 4 {
				t.Fatalf("documents = %d, want 4 after repeated/cyclic discovery", len(schema.Documents()))
			}
			components := globalLongAttributeComponents(schema)
			if len(components) != 6+1 {
				t.Fatalf("long attributes = %d, want 7", len(components))
			}
			for i, expected := range want {
				component := components[i]
				if component.Name() != mustTestQName(t, "urn:root", expected.name) {
					t.Fatalf("attribute %d = %s", i, component.Name())
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok {
					t.Fatalf("%s has no declaration", expected.name)
				}
				constraint, ok := declaration.ValueConstraint()
				if !ok {
					t.Fatalf("%s has no constraint", expected.name)
				}
				if constraint.Kind() != expected.kind || constraint.IsDefault() != (expected.kind == AttributeValueConstraintDefault) || constraint.IsFixed() != (expected.kind == AttributeValueConstraintFixed) || constraint.Lexical() != expected.lexical {
					t.Fatalf("%s constraint kind/lexical = %s/%q", expected.name, constraint.Kind(), constraint.Lexical())
				}
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), expected.marker, root, fixtures)
				if constraint.Loc() != wantLoc {
					t.Fatalf("%s constraint Loc = %s, want %s", expected.name, constraint.Loc(), wantLoc)
				}
				value, ok := constraint.IntegerValue()
				if !ok || value.Canonical() != expected.canonical {
					t.Fatalf("%s integer = %q/%t", expected.name, value.Canonical(), ok)
				}
				if _, hasDecimal := constraint.DecimalValue(); hasDecimal {
					t.Fatalf("%s has decimal", expected.name)
				}
				if _, hasBoolean := constraint.BooleanValue(); hasBoolean {
					t.Fatalf("%s has boolean", expected.name)
				}
				if _, hasPrecisionDecimal := constraint.PrecisionDecimalValue(); hasPrecisionDecimal {
					t.Fatalf("%s has precision decimal", expected.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("%s has no type reference", expected.name)
				}
				if i >= 2 {
					if !reference.IsNamed() || reference.Name().Local() != expected.typeName {
						t.Fatalf("%s named type = %s", expected.name, reference.Name())
					}
					if id, hasID := reference.ComponentID(); !hasID || id.IsZero() {
						t.Fatalf("%s has no named type ID", expected.name)
					}
				}
				if i < 2 {
					if !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "long") {
						t.Fatalf("%s direct type = %s", expected.name, reference.Name())
					}
					if id, hasID := reference.ComponentID(); hasID || !id.IsZero() {
						t.Fatalf("%s has built-in type ID", expected.name)
					}
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || len(bounds.Bounds()) != 2 {
					t.Fatalf("%s has no two exact integer bounds", expected.name)
				}
				if expected.boundMarker != "" {
					boundLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.boundSource), expected.boundMarker, root, fixtures)
					found := false
					for _, bound := range bounds.Bounds() {
						if bound.Loc() == boundLoc && bound.Value().Canonical() == expected.boundValue {
							found = true
						}
					}
					if !found {
						t.Fatalf("%s lost named bound provenance: %v", expected.name, bounds.Bounds())
					}
				}
				value.value.SetInt64(0)
				again := requireAttributeValueConstraint(t, schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", expected.name))[0])
				againValue, ok := again.IntegerValue()
				if !ok || againValue.Canonical() != expected.canonical {
					t.Fatalf("%s mutable integer leaked: %q", expected.name, againValue.Canonical())
				}
			}
			declaration, ok := components[6].AttributeDeclaration()
			if !ok {
				t.Fatal("unconstrained declaration absent")
			}
			if _, ok := declaration.ValueConstraint(); ok {
				t.Fatal("type-only long has value constraint")
			}
			walk := make([]ComponentID, 0)
			if err := schema.Walk(func(c Component) error { walk = append(walk, c.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			ordered := make([]ComponentID, 0)
			for _, c := range schema.Components() {
				ordered = append(ordered, c.ID())
			}
			if !reflect.DeepEqual(walk, ordered) {
				t.Fatal("walk changed component order")
			}
		})
	}
}

//nolint:gocognit // Each diagnostic is an alternate exit from long constraint conversion.
func TestLongAttributeValueConstraintDiagnostics(t *testing.T) {
	tests := []struct {
		name, typeName, typeDecl, kind, lexical, innerCode, related string
		cause                                                       error
	}{
		{"below minimum", "xs:long", "", "default", "-9223372036854775809", BoundValueViolationCode, "", errBoundValueViolation},
		{"above maximum", "xs:long", "", "fixed", "9223372036854775808", BoundValueViolationCode, "", errBoundValueViolation},
		{"malformed", "xs:long", "", "default", "--2", InvalidIntegerLexicalCode, "", nil},
		{"named bound", "r:Narrowed", `<xs:simpleType name="Narrowed"><xs:restriction base="xs:long"><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType>`, "fixed", "11", BoundValueViolationCode, `value="10"`, errBoundValueViolation},
		{"inherited bound", "r:Child", `<xs:simpleType name="Base"><xs:restriction base="xs:long"><xs:maxExclusive value="10"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="r:Base"/></xs:simpleType>`, "default", "10", BoundValueViolationCode, `value="10"`, errBoundValueViolation},
		{"digits", "r:Digits", `<xs:simpleType name="Digits"><xs:restriction base="xs:long"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>`, "fixed", "123", DigitFacetValueViolationCode, `value="2"`, errDigitFacetValueViolation},
		{"enumeration", "r:Enum", `<xs:simpleType name="Enum"><xs:restriction base="xs:long"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>`, "default", "8", EnumerationValueViolationCode, `<xs:enumeration value="7"`, errEnumerationValueViolation},
	}
	for _, profile := range longPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="value" type="` + test.typeName + `" ` + test.kind + `="` + test.lexical + `"/>` + test.typeDecl + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("invalid long constraint returned schema")
				}
				diagnostic := requireDiagnostic(t, err)
				loc := elementReferenceTestAttributeLoc(t, root, test.kind+`="`+test.lexical+`"`)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeValueConstraintCode || diagnostic.Loc() != loc || diagnostic.SpecRef() != schemaAttributeValueSpecRef(profile.version) {
					t.Fatalf("diagnostic = %s", diagnostic)
				}
				if !errors.Is(err, errSchemaAttributeValueConstraintInvalid) || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("lost invalid/facet cause: %v", err)
				}
				inner := requireNestedDiagnostic(t, diagnostic)
				if inner.Code() != test.innerCode || inner.Loc() != loc {
					t.Fatalf("nested diagnostic = %s", inner)
				}
				if test.related != "" {
					wantRelated := elementReferenceTestAttributeLoc(t, root, test.related)
					if !schemaLocationListContains(diagnostic.Related(), wantRelated) {
						t.Fatalf("related = %v, want %s", diagnostic.Related(), wantRelated)
					}
				}
			})
		}
	}
}

func TestLongAttributeConstraintConflictAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:attribute name="value" type="xs:long" default="bad" fixed="bad"/></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err == nil || schema.storage != nil {
			t.Fatalf("%s conflict returned schema", profile.name)
		}
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `fixed="bad"`) || diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(profile.version) || !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `default="bad"`)}) || !errors.Is(err, errSchemaAttributeValueConstraintConflict) {
			t.Fatalf("%s conflict = %s", profile.name, diagnostic)
		}
	}
}

func TestLongAttributeConstraintExcludedShapes(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker string
			code               string
			cause              error
		}{
			{"local direct", `<xs:complexType name="T"><xs:attribute name="a" type="xs:long" default="1"/></xs:complexType>`, `default="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
			{"local named", `<xs:simpleType name="L"><xs:restriction base="xs:long"/></xs:simpleType><xs:complexType name="T"><xs:attribute name="a" type="r:L" fixed="1"/></xs:complexType>`, `fixed="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
			{"local inline", `<xs:complexType name="T"><xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute></xs:complexType>`, `default="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
			{"global inline", `<xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, `fixed="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
			{"element direct", `<xs:element name="e" type="xs:long" default="1"/>`, `default="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
			{"element named", `<xs:element name="e" type="r:L" fixed="1"/><xs:simpleType name="L"><xs:restriction base="xs:long"/></xs:simpleType>`, `fixed="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
			{"element inline", `<xs:element name="e" default="1"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element>`, `default="1"`, UnsupportedSchemaSyntaxCode, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("excluded shape returned schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

func TestLongAttributeConstraintConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="a" type="xs:long" default="1"/><xs:element name="root" type="xs:long"/></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		output, err := GenerateGo(schema, "generated")
		if output != nil || err == nil {
			t.Fatalf("%s GenerateGo returned output", profile.name)
		}
		attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "a"))[0]
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != attribute.Loc() || !errors.Is(err, errCodegenUnsupported) {
			t.Fatalf("GenerateGo = %s", diagnostic)
		}
		err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root">1</root>`)))
		if err == nil {
			t.Fatalf("%s ValidateInstance accepted long", profile.name)
		}
		diagnostic = requireDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("ValidateInstance = %s", diagnostic)
		}
	}
}

func TestLongAttributeConstraintReferencedUseRemainsUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:attribute name="a" type="xs:long" fixed="1"/><xs:complexType name="T"><xs:attribute ref="r:a"/></xs:complexType></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
			t.Fatalf("%s referenced long attribute returned a schema", profile.name)
		}
		diagnostic := requireDiagnostic(t, err)
		wantLoc := elementReferenceTestAttributeLoc(t, root, `ref="r:a"`)
		wantRelated := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
			t.Fatalf("%s reference diagnostic = %s, related %v", profile.name, diagnostic, diagnostic.Related())
		}
	}
}
