package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type byteConstraintWant struct {
	name, source, typeName, lexical, canonical, marker string
	kind                                               AttributeValueConstraintKind
	min, max                                           string
	named                                              bool
}

//nolint:gocognit // Check byte-specific intrinsic bounds and the copied public value.
func assertByteAttributeConstraint(t *testing.T, schema Schema, root string, want byteConstraintWant) {
	t.Helper()
	attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", want.name))
	if len(attribute) != 1 || attribute[0].ID().Source() != SourceID(want.source) {
		t.Fatalf("%s attribute source/count = %v", want.name, attribute)
	}
	declaration, ok := attribute[0].AttributeDeclaration()
	if !ok {
		t.Fatalf("%s has no attribute declaration", want.name)
	}
	reference, ok := declaration.TypeReference()
	if !ok || reference.IsNamed() != want.named || declaration.DeclaredType() != mustTestQName(t, map[bool]string{true: "urn:r", false: testXSDNamespace}[want.named], want.typeName) {
		t.Fatalf("%s type facts = %v/%t", want.name, reference, ok)
	}
	bounds, ok := reference.IntegerBounds()
	if !ok || len(bounds.Bounds()) != 2 || bounds.Bounds()[0].Value().Canonical() != want.min || bounds.Bounds()[1].Value().Canonical() != want.max {
		t.Fatalf("%s byte bounds = %v/%t", want.name, bounds, ok)
	}
	if !want.named && (bounds.Bounds()[0].Loc() != (Loc{}) || bounds.Bounds()[1].Loc() != (Loc{})) {
		t.Fatalf("%s intrinsic bounds have source locations", want.name)
	}
	constraint, ok := declaration.ValueConstraint()
	if !ok || constraint.Kind() != want.kind || constraint.IsDefault() != (want.kind == AttributeValueConstraintDefault) || constraint.IsFixed() != (want.kind == AttributeValueConstraintFixed) || constraint.Lexical() != want.lexical || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, want.marker) {
		t.Fatalf("%s value constraint = %v/%t", want.name, constraint, ok)
	}
	value, ok := constraint.IntegerValue()
	if !ok || value.Canonical() != want.canonical {
		t.Fatalf("%s exact byte value = %s/%t", want.name, value.Canonical(), ok)
	}
	if _, hasDecimal := constraint.DecimalValue(); hasDecimal {
		t.Fatalf("%s exposes a decimal value", want.name)
	}
	value.value.SetInt64(99)
	fresh, ok := declaration.ValueConstraint()
	freshValue, hasInteger := fresh.IntegerValue()
	if !ok || !hasInteger || freshValue.Canonical() != want.canonical {
		t.Fatalf("%s changed through a public value copy", want.name)
	}
}

func TestSchemaByteGlobalAttributeValueConstraintUnlock(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r">
  <xs:attribute name="directDefault" type="xs:byte" default=" +0007 "/>
  <xs:attribute name="directFixed" type="xs:byte" fixed="-128"/>
  <xs:attribute name="upper" type="xs:byte" default="127"/>
  <xs:attribute name="namedFixed" type="r:Small" fixed="+0007"/>
  <xs:attribute name="namedDefault" type="r:Small" default=" 7 "/>
  <xs:attribute name="negativeZero" type="xs:byte" fixed="-0"/>
  <xs:simpleType name="Small"><xs:restriction base="xs:byte"><xs:minInclusive value="0"/><xs:maxInclusive value="10"/><xs:enumeration value="7"/><xs:totalDigits value="1"/></xs:restriction></xs:simpleType>
</xs:schema>`
	for _, profile := range byteAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []byteConstraintWant{
				{"directDefault", "root.xsd", "byte", "+0007", "7", `default=" +0007 "`, AttributeValueConstraintDefault, "-128", "127", false},
				{"directFixed", "root.xsd", "byte", "-128", "-128", `fixed="-128"`, AttributeValueConstraintFixed, "-128", "127", false},
				{"upper", "root.xsd", "byte", "127", "127", `default="127"`, AttributeValueConstraintDefault, "-128", "127", false},
				{"namedFixed", "root.xsd", "Small", "+0007", "7", `fixed="+0007"`, AttributeValueConstraintFixed, "0", "10", true},
				{"namedDefault", "root.xsd", "Small", "7", "7", `default=" 7 "`, AttributeValueConstraintDefault, "0", "10", true},
				{"negativeZero", "root.xsd", "byte", "-0", "0", `fixed="-0"`, AttributeValueConstraintFixed, "-128", "127", false},
			} {
				assertByteAttributeConstraint(t, schema, root, want)
			}
			attribute := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", "namedFixed"))[0]
			declaration, _ := attribute.AttributeDeclaration()
			reference, _ := declaration.TypeReference()
			matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:r", "Small"))
			id, present := reference.ComponentID()
			if len(matches) != 1 || !present || id != matches[0].ID() || reference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:Small"`) {
				t.Fatalf("named byte identity = %v/%t", id, present)
			}
			bounds, _ := reference.IntegerBounds()
			if bounds.Bounds()[0].Loc() != elementReferenceTestAttributeLoc(t, root, `value="0"`) || bounds.Bounds()[1].Loc() != elementReferenceTestAttributeLoc(t, root, `value="10"`) {
				t.Fatalf("named byte facet provenance = %v", bounds.Bounds())
			}
		})
	}
}

//nolint:gocognit // Observe discovery order and cross-document byte values together.
func TestSchemaByteAttributeConstraintGraphVisibility(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" xmlns:o="urn:o" targetNamespace="urn:r" version="1.0">
  <xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="shared.xsd"/>
  <xs:import namespace="urn:o" schemaLocation="other.xsd"/>
  <xs:attribute name="included" type="r:Included" default="2"/>
  <xs:attribute name="imported" type="o:Imported" fixed="3"/>
</xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" version="1.1"><xs:simpleType name="Included"><xs:restriction base="xs:byte"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:o" version="1.1"><xs:import namespace="urn:r" schemaLocation="root.xsd"/><xs:simpleType name="Imported"><xs:restriction base="xs:byte"><xs:enumeration value="3"/></xs:restriction></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"shared.xsd": {id: "shared.xsd", contents: shared},
		"other.xsd":  {id: "other.xsd", contents: other},
		"root.xsd":   {id: "root.xsd", contents: root},
	}
	for _, profile := range byteAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Documents()) != 3 || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated include/import changed discovery or component order")
			}
			for _, test := range []struct {
				name, source, value, marker string
				kind                        AttributeValueConstraintKind
			}{
				{"included", "shared.xsd", "2", `default="2"`, AttributeValueConstraintDefault},
				{"imported", "other.xsd", "3", `fixed="3"`, AttributeValueConstraintFixed},
			} {
				attribute := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", test.name))
				if len(attribute) != 1 {
					t.Fatalf("%s attribute count = %d", test.name, len(attribute))
				}
				declaration, _ := attribute[0].AttributeDeclaration()
				id, present := declaration.TypeID()
				constraint, hasValue := declaration.ValueConstraint()
				value, hasInteger := constraint.IntegerValue()
				if !present || id.Source() != SourceID(test.source) || !hasValue || !hasInteger || value.Canonical() != test.value || constraint.Kind() != test.kind || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) {
					t.Fatalf("%s composed graph value = %v/%v", test.name, id, constraint)
				}
			}
		})
	}
}

//nolint:gocognit // Check all value conversion exits and their nested causes across policies.
func TestSchemaByteAttributeValueConstraintDiagnostics(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		for _, test := range []struct {
			name, typeName, value, declaration, primary, related, innerCode string
			kind                                                            AttributeValueConstraintKind
			cause                                                           error
			conflict                                                        bool
		}{
			{name: "malformed", typeName: "xs:byte", value: "7.0", kind: AttributeValueConstraintDefault, innerCode: InvalidIntegerLexicalCode},
			{name: "exponent", typeName: "xs:byte", value: "1e1", kind: AttributeValueConstraintFixed, innerCode: InvalidIntegerLexicalCode},
			{name: "sign alone", typeName: "xs:byte", value: "+", kind: AttributeValueConstraintDefault, innerCode: InvalidIntegerLexicalCode},
			{name: "non ASCII digit", typeName: "xs:byte", value: "７", kind: AttributeValueConstraintFixed, innerCode: InvalidIntegerLexicalCode},
			{name: "below intrinsic", typeName: "xs:byte", value: "-129", kind: AttributeValueConstraintFixed, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
			{name: "above intrinsic", typeName: "xs:byte", value: "128", kind: AttributeValueConstraintDefault, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
			{name: "named lower", typeName: "r:Narrowed", value: "-1", kind: AttributeValueConstraintFixed, declaration: `<xs:simpleType name="Narrowed"><xs:restriction base="xs:byte"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>`, innerCode: BoundValueViolationCode, cause: errBoundValueViolation, related: `value="0"`},
			{name: "named upper", typeName: "r:Narrowed", value: "11", kind: AttributeValueConstraintDefault, declaration: `<xs:simpleType name="Narrowed"><xs:restriction base="xs:byte"><xs:maxExclusive value="11"/></xs:restriction></xs:simpleType>`, innerCode: BoundValueViolationCode, cause: errBoundValueViolation, related: `value="11"`},
			{name: "named enumeration", typeName: "r:Enumerated", value: "8", kind: AttributeValueConstraintDefault, declaration: `<xs:simpleType name="Enumerated"><xs:restriction base="xs:byte"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>`, innerCode: EnumerationValueViolationCode, cause: errEnumerationValueViolation, related: `<xs:enumeration value="7"`},
			{name: "named digits", typeName: "r:Digits", value: "12", kind: AttributeValueConstraintFixed, declaration: `<xs:simpleType name="Digits"><xs:restriction base="xs:byte"><xs:totalDigits value="1"/></xs:restriction></xs:simpleType>`, innerCode: DigitFacetValueViolationCode, cause: errDigitFacetValueViolation, related: `value="1"`},
			{name: "conflicting values", typeName: "xs:byte", value: "0", kind: AttributeValueConstraintFixed, conflict: true, primary: `fixed="0"`, related: `default="0"`, cause: errSchemaAttributeValueConstraintConflict},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				version := profile.version
				if profile.policy == Compatibility {
					version = XSDVersion11
				}
				valueText := string(test.kind) + `="` + test.value + `"`
				if test.conflict {
					valueText = `default="0" ` + valueText
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `"><xs:attribute name="value" type="` + test.typeName + `" ` + valueText + `/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("invalid byte value returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				primary := valueText
				if test.conflict {
					primary = test.primary
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, primary) || !errors.Is(err, test.cause) && test.cause != nil {
					t.Fatalf("diagnostic = %s, want invalid at %s with %v", diagnostic, primary, test.cause)
				}
				if test.conflict {
					if diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(version) || !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
						t.Fatalf("conflict diagnostic = %s/%v", diagnostic, diagnostic.Related())
					}
					return
				}
				if diagnostic.Code() != diagnosticSchemaAttributeValueConstraintCode || diagnostic.SpecRef() != schemaAttributeValueSpecRef(version) || !errors.Is(err, errSchemaAttributeValueConstraintInvalid) {
					t.Fatalf("value diagnostic = %s", diagnostic)
				}
				inner := requireNestedDiagnostic(t, diagnostic)
				if inner.Code() != test.innerCode || inner.Loc() != diagnostic.Loc() {
					t.Fatalf("nested diagnostic = %s", inner)
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations = %v", diagnostic.Related())
				}
				if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related locations = %v, want %s", diagnostic.Related(), test.related)
				}
			})
		}
	}
}

//nolint:gocognit // Keep excluded attribute shapes under the same policy matrix.
func TestSchemaByteAttributeConstraintExcludedShapes(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		for _, test := range []struct {
			name, body, marker string
			cause              error
		}{
			{"local direct", `<xs:complexType name="C"><xs:attribute name="a" type="xs:byte" default="1"/></xs:complexType>`, `default="1"`, errSchemaAttributeUseUnsupported},
			{"local named", `<xs:complexType name="C"><xs:attribute name="a" type="r:Byte" fixed="1"/></xs:complexType><xs:simpleType name="Byte"><xs:restriction base="xs:byte"/></xs:simpleType>`, `fixed="1"`, errSchemaAttributeUseUnsupported},
			{"global inline", `<xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, ErrUnsupported},
			{"local inline", `<xs:complexType name="C"><xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:attribute></xs:complexType>`, `fixed="1"`, errSchemaAttributeUseUnsupported},
			{"local ref", `<xs:attribute name="a" type="xs:byte" default="1"/><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("excluded byte attribute returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || diagnostic.SpecRef() == "" || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded shape diagnostic = %s, want unsupported at %s with %v", diagnostic, test.marker, test.cause)
				}
				if test.name == "local ref" && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}) {
					t.Fatalf("local ref related = %v", diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Exercise generation and validation independently for both admitted shapes and constraint kinds.
func TestSchemaByteConstrainedAttributeConsumersRemainExcluded(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		for _, test := range []struct{ name, typeName, constraint, declaration string }{
			{"direct default", "xs:byte", `default="1"`, ""},
			{"direct fixed", "xs:byte", `fixed="1"`, ""},
			{"named default", "r:Byte", `default="1"`, `<xs:simpleType name="Byte"><xs:restriction base="xs:byte"/></xs:simpleType>`},
			{"named fixed", "r:Byte", `fixed="1"`, `<xs:simpleType name="Byte"><xs:restriction base="xs:byte"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="` + test.typeName + `" ` + test.constraint + `/>` + test.declaration + `<xs:element name="root" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatal("constrained byte attribute generated output")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("byte generation diagnostic = %s", diagnostic)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:r" a="1">1</root>`)))
				if err == nil {
					t.Fatal("instance attribute was accepted")
				}
				diagnostic = requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 21) || !errors.Is(err, errInstanceAttributes) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("byte validation diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

func TestSchemaByteConstraintKeepsTypeBoundaryPrecedence(t *testing.T) {
	for _, profile := range byteAttributePolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration, marker, code string
			cause                                     error
			class                                     FailureClass
		}{
			{"unresolved type", "r:Missing", "", `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, FailureInvalid},
			{"unsupported named facet", "r:Pattern", `<xs:simpleType name="Pattern"><xs:restriction base="xs:byte"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, ErrUnsupported, FailureUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="` + test.typeName + `" default="not-an-integer"/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("type failure returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !errors.Is(err, test.cause) {
					t.Fatalf("type boundary diagnostic = %s", diagnostic)
				}
			})
		}
	}
}
