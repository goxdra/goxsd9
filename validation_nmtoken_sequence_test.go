package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const (
	validationNMTOKENSequenceNamespace      = "urn:nmtoken-sequence"       //nolint:gosec // Test namespace is not a credential.
	validationNMTOKENSequenceOtherNamespace = "urn:nmtoken-sequence-other" //nolint:gosec // Test namespace is not a credential.
)

func TestValidateInstanceSupportsLocalNMTOKENSequencesAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNMTOKENSequenceNamespace + `" xmlns:o="` + validationNMTOKENSequenceOtherNamespace + `" targetNamespace="` + validationNMTOKENSequenceNamespace + `" version="` + string(policy.version) + `">
  <xs:include schemaLocation="nmtoken-sequence-chameleon.xsd"/>
  <xs:import namespace="` + validationNMTOKENSequenceOtherNamespace + `" schemaLocation="nmtoken-sequence-other.xsd"/>
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence>
    <xs:element name="direct" type="xs:NMTOKEN"/>
    <xs:element name="forward" type="r:Forward"/>
    <xs:element name="named" type="r:Named"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="imported" type="o:Imported"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" named "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" forward "/></xs:restriction></xs:simpleType>
</xs:schema>`
			fixtures := map[string]validationTestFixture{
				"nmtoken-sequence-chameleon.xsd": {
					id:       "nmtoken-sequence-chameleon.xsd",
					contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" included "/></xs:restriction></xs:simpleType></xs:schema>`,
				},
				"nmtoken-sequence-other.xsd": {
					id:       "nmtoken-sequence-other.xsd",
					contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationNMTOKENSequenceOtherNamespace + `" targetNamespace="` + validationNMTOKENSequenceOtherNamespace + `" version="` + string(policy.version) + `"><xs:simpleType name="ImportedBase"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" imported "/></xs:restriction></xs:simpleType><xs:simpleType name="Imported"><xs:restriction base="o:ImportedBase"/></xs:simpleType></xs:schema>`,
				},
			}
			schema := validationTestSchemaWithPolicy(t, root, fixtures, policy.policy)
			before := schema.Components()
			input := `<root xmlns="` + validationNMTOKENSequenceNamespace + `"><direct xmlns="">&#x9;名-9·́ &#xD;&#xA;</direct><forward xmlns="">&#x9;forward&#xD;&#xA;</forward><named xmlns=""> &#x9;named &#xD;&#xA;</named><included xmlns="">&#xD;included&#x9;</included><imported xmlns="">&#x9;imported&#xA;</imported></root>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("ValidateInstance(%q): %v", input, err)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("local NMTOKEN-sequence validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit,funlen // Keep NMTOKEN lexical, enumeration, location, and provenance assertions together.
func TestValidateInstanceReportsLocalNMTOKENSequenceDiagnosticsAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationNMTOKENSequenceDiagnosticSchema(t, policy)
			before := schema.Components()
			for _, test := range []struct {
				name        string
				directValue string
				namedValue  string
				directEmpty bool
				local       string
				code        string
				message     string
				specRef     string
			}{
				{
					name:        "empty value",
					directEmpty: true,
					local:       "direct",
					code:        goxsd9.InvalidNMTOKENLexicalCode,
					message:     "NMTOKEN value is empty after XML whitespace collapse",
					specRef:     validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:        "whitespace-only value",
					directValue: " \t\r\n ",
					local:       "direct",
					code:        goxsd9.InvalidNMTOKENLexicalCode,
					message:     "NMTOKEN value is whitespace-only after XML whitespace collapse",
					specRef:     validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:        "invalid NameChar",
					directValue: "bad/value",
					local:       "direct",
					code:        goxsd9.InvalidNMTOKENLexicalCode,
					message:     "NMTOKEN value contains an invalid NameChar",
					specRef:     validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:        "invalid NameChar before enumeration",
					directValue: "allowed",
					namedValue:  "bad/value",
					local:       "named",
					code:        goxsd9.InvalidNMTOKENLexicalCode,
					message:     "NMTOKEN value contains an invalid NameChar",
					specRef:     validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:        "enumeration after collapse",
					directValue: "allowed",
					namedValue:  " \tother\r\n ",
					local:       "named",
					code:        goxsd9.EnumerationValueViolationCode,
					message:     "value is not in the NMTOKEN enumeration",
					specRef:     validationNMTOKENEnumerationSpecRef(policy.version),
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationNMTOKENSequenceInstanceWithValues(test.directValue, test.namedValue, test.directEmpty)
					firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, firstErr)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), test.code)
					}
					if diagnostic.Message() != test.message {
						t.Fatalf("Message() = %q, want %q", diagnostic.Message(), test.message)
					}
					wantLoc := validationNMTOKENSequenceTextLoc(t, input, test.local)
					if test.directEmpty {
						wantLoc = validationSequenceMarkerLoc(t, input, "<direct")
					}
					if diagnostic.Loc() != wantLoc {
						t.Fatalf("Loc() = %s, want %s", diagnostic.Loc(), wantLoc)
					}
					if diagnostic.SpecRef() != test.specRef {
						t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), test.specRef)
					}
					if diagnostic.Unwrap() == nil || errors.Is(firstErr, goxsd9.ErrUnsupported) {
						t.Fatalf("diagnostic cause or classification is wrong: %v", firstErr)
					}
					wantRelated := validationNMTOKENSequenceRelated(t, schema, test.local)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}
					secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					second := validationTestDiagnostic(t, secondErr)
					if diagnostic.Error() != second.Error() || diagnostic.Loc() != second.Loc() || diagnostic.SpecRef() != second.SpecRef() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
						t.Fatalf("repeated NMTOKEN-sequence diagnostics differ: first %v, second %v", firstErr, secondErr)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("NMTOKEN-sequence diagnostics mutated the completed schema")
			}
		})
	}
}

func TestValidateInstanceKeepsMixedNMTOKENSequenceUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNMTOKENSequenceNamespace + `" targetNamespace="` + validationNMTOKENSequenceNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence><xs:element name="nmtoken" type="xs:NMTOKEN"/><xs:element name="token" type="xs:token"/></xs:sequence></xs:complexType>
</xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, nil, policy.policy)
			input := `<root xmlns="` + validationNMTOKENSequenceNamespace + `"><nmtoken xmlns="">value</nmtoken><token xmlns="">value</token></root>`
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("diagnostic = %s/%q, want unsupported mixed-family sequence", diagnostic, diagnostic.Code())
			}
			if diagnostic.Loc() != validationTestLoc(t, "instance.xml", 1, 1) || diagnostic.SpecRef() != validationLocalNMTOKENChoiceStructureSpecRef(policy.version) {
				t.Fatalf("diagnostic evidence = %s/%q, want root and edition locations", diagnostic.Loc(), diagnostic.SpecRef())
			}
		})
	}
}

func validationNMTOKENSequenceDiagnosticSchema(t *testing.T, policy validationTokenPolicyCase) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNMTOKENSequenceNamespace + `" targetNamespace="` + validationNMTOKENSequenceNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="root" type="r:Root"/>
  <xs:complexType name="Root"><xs:sequence><xs:element name="direct" type="xs:NMTOKEN"/><xs:element name="named" type="r:Named"/></xs:sequence></xs:complexType>
  <xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" allowed "/></xs:restriction></xs:simpleType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy.policy)
}

func validationNMTOKENSequenceInstanceWithValues(directValue, namedValue string, directEmpty bool) string {
	direct := `<direct xmlns="">` + directValue + `</direct>`
	if directEmpty {
		direct = `<direct xmlns=""/>`
	}
	return `<root xmlns="` + validationNMTOKENSequenceNamespace + `">` + direct + `<named xmlns="">` + namedValue + `</named></root>`
}

func validationNMTOKENSequenceTextLoc(t *testing.T, input, local string) goxsd9.Loc {
	t.Helper()
	start := strings.Index(input, `<`+local)
	if start < 0 {
		t.Fatalf("input has no <%s> element: %q", local, input)
	}
	end := strings.IndexByte(input[start:], '>')
	if end < 0 {
		t.Fatalf("input has no <%s> start-tag end: %q", local, input)
	}
	return validationTestLoc(t, "instance.xml", 1, start+end+2)
}

func validationNMTOKENSequenceRelated(t *testing.T, schema goxsd9.Schema, local string) []goxsd9.Loc {
	t.Helper()
	rootName, err := goxsd9.NewQName(validationNMTOKENSequenceNamespace, "root")
	if err != nil {
		t.Fatalf("NewQName root: %v", err)
	}
	rootComponents := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
	if len(rootComponents) != 1 {
		t.Fatalf("root declarations = %d, want one", len(rootComponents))
	}
	typeName, err := goxsd9.NewQName(validationNMTOKENSequenceNamespace, "Root")
	if err != nil {
		t.Fatalf("NewQName Root: %v", err)
	}
	typeComponents := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, typeName)
	if len(typeComponents) != 1 {
		t.Fatalf("Root definitions = %d, want one", len(typeComponents))
	}
	definition, ok := typeComponents[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Root has no complex type definition view")
	}
	sequence, ok := definition.Particle().(goxsd9.SequenceParticle)
	if !ok {
		t.Fatalf("Root particle = %T, want SequenceParticle", definition.Particle())
	}
	elements := sequence.Elements()
	if len(elements) != 2 {
		t.Fatalf("sequence elements = %d, want two", len(elements))
	}
	index := 0
	if local == "named" {
		index = 1
	}
	related := []goxsd9.Loc{rootComponents[0].Loc(), typeComponents[0].Loc(), sequence.Loc(), elements[index].Loc()}
	if local != "named" {
		return related
	}
	namedName, err := goxsd9.NewQName(validationNMTOKENSequenceNamespace, "Named")
	if err != nil {
		t.Fatalf("NewQName Named: %v", err)
	}
	namedComponents := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, namedName)
	if len(namedComponents) != 1 {
		t.Fatalf("Named definitions = %d, want one", len(namedComponents))
	}
	namedDefinition, ok := namedComponents[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("Named has no simple type definition view")
	}
	related = append(related, namedComponents[0].Loc())
	return append(related, namedDefinition.StringEnumerationFacets().Locations()...)
}
