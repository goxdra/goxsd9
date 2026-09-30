package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Verify the existing cyclic int graph with value constraints added at declarations.
func TestSchemaIntConstraintsInCyclicGraph(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		root, fixtures := intGlobalAttributeGraphFixtures(profile.version)
		values := []struct{ name, typeName, sourceValue, lexical, canonical, typeSource string }{
			{"direct", "xs:int", `default=" -2147483648 "`, "-2147483648", "-2147483648", ""},
			{"forward", "r:ForwardInt", `fixed="+2147483647"`, "+2147483647", "2147483647", "root.xsd"},
			{"imported", "o:ImportedInt", `default="0003"`, "0003", "3", "other.xsd"},
			{"chameleon", "r:ChameleonInt", `fixed="-0"`, "-0", "0", "chameleon.xsd"},
			{"narrowed", "r:NarrowedInt", `default="-0002"`, "-0002", "-2", "root.xsd"},
		}
		for _, item := range values {
			from := `name="` + item.name + `" type="` + item.typeName + `"`
			root = strings.Replace(root, from, from+` `+item.sourceValue, 1)
		}
		fixtures["root.xsd"] = discoveryFixture{id: "root.xsd", contents: root}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		if len(schema.Documents()) != 4 || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
			t.Fatalf("%s repeated cyclic discovery changed documents, facts, or order", profile.name)
		}
		for _, item := range values {
			attribute := mustTestQName(t, "urn:root", item.name)
			t.Run(profile.name+"/"+item.name, func(t *testing.T) {
				matches := schema.FindKind(ComponentKindAttributeDeclaration, attribute)
				if len(matches) != 1 {
					t.Fatalf("%s matches = %d", item.name, len(matches))
				}
				declaration, _ := matches[0].AttributeDeclaration()
				id, named := declaration.TypeID()
				if named != (item.typeSource != "") || named && id.Source() != SourceID(item.typeSource) {
					t.Fatalf("%s type identity = %v/%t, want source %s", item.name, id, named, item.typeSource)
				}
				reference, _ := declaration.TypeReference()
				if item.name == "direct" {
					assertIntBuiltinReference(t, reference, elementReferenceTestAttributeLoc(t, root, `type="xs:int"`), profile.version)
				}
				if item.name == "narrowed" {
					assertNarrowedIntAttributeFacts(t, reference, profile.version, root, fixtures)
				}
				constraint, ok := declaration.ValueConstraint()
				integer, hasInteger := constraint.IntegerValue()
				if !ok || !hasInteger || integer.Canonical() != item.canonical || constraint.Lexical() != item.lexical || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, item.sourceValue) {
					t.Fatalf("%s constraint = %v/%t, integer %s/%t", item.name, constraint, ok, integer.Canonical(), hasInteger)
				}
				integer.value.SetInt64(99)
				again, _ := declaration.ValueConstraint()
				stored, _ := again.IntegerValue()
				if stored.Canonical() != item.canonical {
					t.Fatalf("%s value changed through query copy", item.name)
				}
			})
		}
	}
}

type intConstraintFailure struct {
	name, typeName, value, declaration, related, innerCode string
	kind                                                   AttributeValueConstraintKind
	cause                                                  error
}

func TestSchemaIntConstraintValueFailures(t *testing.T) {
	cases := []intConstraintFailure{
		{name: "decimal lexical", typeName: "xs:int", value: "7.0", kind: AttributeValueConstraintDefault, innerCode: InvalidIntegerLexicalCode},
		{name: "exponent lexical", typeName: "xs:int", value: "1e2", kind: AttributeValueConstraintFixed, innerCode: InvalidIntegerLexicalCode},
		{name: "unicode digit", typeName: "xs:int", value: "١", kind: AttributeValueConstraintDefault, innerCode: InvalidIntegerLexicalCode},
		{name: "sign only", typeName: "xs:int", value: "+", kind: AttributeValueConstraintFixed, innerCode: InvalidIntegerLexicalCode},
		{name: "below built-in", typeName: "xs:int", value: "-2147483649", kind: AttributeValueConstraintDefault, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
		{name: "above built-in", typeName: "xs:int", value: "2147483648", kind: AttributeValueConstraintFixed, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
		{name: "named bound", typeName: "r:Lower", value: "-1", kind: AttributeValueConstraintDefault, declaration: `<xs:simpleType name="Lower"><xs:restriction base="xs:int"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>`, related: `value="0"`, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
		{name: "named enumeration", typeName: "r:Choices", value: "8", kind: AttributeValueConstraintFixed, declaration: `<xs:simpleType name="Choices"><xs:restriction base="xs:int"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>`, related: `<xs:enumeration value="7"`, innerCode: EnumerationValueViolationCode, cause: errEnumerationValueViolation},
		{name: "named digits", typeName: "r:OneDigit", value: "12", kind: AttributeValueConstraintDefault, declaration: `<xs:simpleType name="OneDigit"><xs:restriction base="xs:int"><xs:totalDigits value="1"/></xs:restriction></xs:simpleType>`, related: `value="1"`, innerCode: DigitFacetValueViolationCode, cause: errDigitFacetValueViolation},
	}
	for _, profile := range longPolicyProfiles() {
		for _, test := range cases {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) { assertIntConstraintFailure(t, profile, test) })
		}
	}
}

func assertIntConstraintFailure(t *testing.T, profile longPolicyProfile, test intConstraintFailure) {
	t.Helper()
	valueAttribute := string(test.kind) + `="` + test.value + `"`
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="value" type="` + test.typeName + `" ` + valueAttribute + `/>` + test.declaration + `</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err == nil || schema.storage != nil {
		t.Fatalf("%s returned schema or no error", test.name)
	}
	outer := requireDiagnostic(t, err)
	location := elementReferenceTestAttributeLoc(t, root, valueAttribute)
	version := profile.version
	if profile.policy == Compatibility {
		version = XSDVersion11
	}
	if outer.Class() != FailureInvalid || outer.Code() != diagnosticSchemaAttributeValueConstraintCode || outer.Loc() != location || outer.SpecRef() != schemaAttributeValueSpecRef(version) || !errors.Is(err, errSchemaAttributeValueConstraintInvalid) {
		t.Fatalf("outer diagnostic = %s", outer)
	}
	inner := requireNestedDiagnostic(t, outer)
	if inner.Code() != test.innerCode || inner.Loc() != location {
		t.Fatalf("inner diagnostic = %s", inner)
	}
	if test.cause != nil && !errors.Is(err, test.cause) {
		t.Fatalf("diagnostic lost %v: %v", test.cause, err)
	}
	if test.related == "" && len(outer.Related()) != 0 {
		t.Fatalf("unexpected related locations: %v", outer.Related())
	}
	if test.related != "" && !schemaLocationListContains(outer.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
		t.Fatalf("related locations = %v, want %s", outer.Related(), test.related)
	}
}

//nolint:gocognit // Check both direct and named conflicts under every policy.
func TestSchemaIntConstraintConflict(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct{ name, typeName, declaration string }{
			{"direct", "xs:int", ""},
			{"named", "r:Int", `<xs:simpleType name="Int"><xs:restriction base="xs:int"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="value" type="` + test.typeName + `" default="bad" fixed="also-bad"/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("conflict returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				version := profile.version
				if profile.policy == Compatibility {
					version = XSDVersion11
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `fixed="also-bad"`) || diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(version) || !errors.Is(err, errSchemaAttributeValueConstraintConflict) {
					t.Fatalf("conflict diagnostic = %s", diagnostic)
				}
				if !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `default="bad"`)}) {
					t.Fatalf("conflict related = %v", diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Verify reference and facet failures retain their own diagnostics before value conversion.
func TestSchemaIntConstraintTypeFailuresPrecedeValue(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		version := profile.version
		if profile.policy == Compatibility {
			version = XSDVersion11
		}
		for _, test := range []struct {
			name, typeName, declaration, marker, related, code, specRef string
			class                                                       FailureClass
			cause                                                       error
		}{
			{"unresolved", "r:Missing", "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"wrong kind", "r:Element", `<xs:element name="Element" type="xs:int"/>`, `type="r:Element"`, `<xs:element name="Element"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeWrongKind},
			{"unsupported facet", "r:Pattern", `<xs:simpleType name="Pattern"><xs:restriction base="xs:int"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, map[XSDVersion]string{XSDVersion10: "xsd10-datatypes#decimal", XSDVersion11: "xsd11-datatypes#decimal"}[version], FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="value" type="` + test.typeName + `" default="bad"/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatal("type failure returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.specRef || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !errors.Is(err, test.cause) {
					t.Fatalf("type diagnostic = %s, SpecRef %q, cause %v", diagnostic, diagnostic.SpecRef(), err)
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations: %v", diagnostic.Related())
				}
				if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related locations = %v", diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Keep every excluded attribute shape under the same policy matrix.
func TestSchemaIntAttributeConstraintExcludedShapes(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker string
			cause              error
		}{
			{"local direct", `<xs:complexType name="C"><xs:attribute name="a" type="xs:int" default="1"/></xs:complexType>`, `default="1"`, errSchemaAttributeUseUnsupported},
			{"local named", `<xs:complexType name="C"><xs:attribute name="a" type="r:Int" fixed="1"/></xs:complexType><xs:simpleType name="Int"><xs:restriction base="xs:int"/></xs:simpleType>`, `fixed="1"`, errSchemaAttributeUseUnsupported},
			{"global inline", `<xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, ErrUnsupported},
			{"local inline", `<xs:complexType name="C"><xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:attribute></xs:complexType>`, `fixed="1"`, errSchemaAttributeUseUnsupported},
			{"local ref", `<xs:attribute name="a" type="xs:int" default="1"/><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, errSchemaAttributeReferenceUnsupported},
			{"local named ref", `<xs:attribute name="a" type="r:Int" fixed="1"/><xs:simpleType name="Int"><xs:restriction base="xs:int"/></xs:simpleType><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("excluded int attribute returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				wantSpecRef := schemaAttributeUseSpecRef(profile.version)
				if test.name == "global inline" {
					wantSpecRef = "xsd10-structures#schema-document"
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || diagnostic.SpecRef() != wantSpecRef || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded shape diagnostic = %s with SpecRef %q, want unsupported at %s with %v and SpecRef %q", diagnostic, diagnostic.SpecRef(), test.marker, test.cause, wantSpecRef)
				}
				if (test.name == "local ref" || test.name == "local named ref") && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}) {
					t.Fatalf("local ref related = %v", diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Exercise generation and validation separately for both admitted type shapes.
func TestSchemaIntConstrainedAttributeConsumersRemainExcluded(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration string
		}{
			{"direct", "xs:int", ""},
			{"named", "r:Int", `<xs:simpleType name="Int"><xs:restriction base="xs:int"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="` + test.typeName + `" fixed="1"/>` + test.declaration + `<xs:element name="root" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatal("constrained int attribute generated output")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("named int generation diagnostic = %s", diagnostic)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:r" a="1">1</root>`)))
				if err == nil {
					t.Fatal("instance attribute was accepted")
				}
				diagnostic = requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 21) || !errors.Is(err, errInstanceAttributes) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("int attribute validation diagnostic = %s", diagnostic)
				}
			})
		}
	}
}
