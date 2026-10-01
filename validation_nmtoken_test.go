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
	validationNMTOKENNamespace  = "urn:nmtoken-root" //nolint:gosec // Test namespace is not a credential.
	validationImportedNamespace = "urn:external-types"
)

func TestValidateInstanceSupportsGlobalNMTOKENScalarsAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationNMTOKENSchema(t, policy.version, policy.policy)
			before := schema.Components()
			for _, test := range []struct {
				name    string
				element string
				value   string
			}{
				{name: "direct colon", element: "direct", value: ":"},
				{name: "direct digit", element: "direct", value: "9"},
				{name: "direct hyphen", element: "direct", value: "-"},
				{name: "direct period", element: "direct", value: "."},
				{name: "direct punctuation and collapsed whitespace", element: "direct", value: "\t :9-. \r\n"},
				{name: "direct concatenated decoded text", element: "direct", value: "9<!-- ignored -->-"},
				{name: "named collapsed", element: "named", value: "\t allowed\n"},
				{name: "forward inherited", element: "forward", value: " \tforward\r"},
				{name: "inherited through named base", element: "inherited", value: "\r forward \t"},
				{name: "included chameleon", element: "included", value: " \tincluded\n"},
				{name: "imported inherited", element: "imported", value: "\rimported\t"},
				{name: "named without enumeration", element: "plain", value: "value"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationNMTOKENInstance(test.element, test.value, false)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("NMTOKEN validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit,funlen // Keep NMTOKEN failure ordering, provenance, and repeatability assertions together.
func TestValidateInstanceReportsGlobalNMTOKENDiagnosticsAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationNMTOKENSchema(t, policy.version, policy.policy)
			before := schema.Components()
			for _, test := range []struct {
				name        string
				element     string
				typeName    string
				namespace   string
				value       string
				selfClosing bool
				code        string
				message     string
				specRef     string
			}{
				{
					name:        "empty self-closing value",
					element:     "direct",
					selfClosing: true,
					code:        goxsd9.InvalidNMTOKENLexicalCode,
					message:     "NMTOKEN value is empty after XML whitespace collapse",
					specRef:     validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:    "whitespace-only value",
					element: "direct",
					value:   " \t\r\n ",
					code:    goxsd9.InvalidNMTOKENLexicalCode,
					message: "NMTOKEN value is whitespace-only after XML whitespace collapse",
					specRef: validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:    "invalid NameChar",
					element: "direct",
					value:   "bad/value",
					code:    goxsd9.InvalidNMTOKENLexicalCode,
					message: "NMTOKEN value contains an invalid NameChar",
					specRef: validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:      "invalid NameChar is checked before enumeration",
					element:   "named",
					typeName:  "Named",
					namespace: validationNMTOKENNamespace,
					value:     "other value",
					code:      goxsd9.InvalidNMTOKENLexicalCode,
					message:   "NMTOKEN value contains an invalid NameChar",
					specRef:   validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:      "enumeration after collapse",
					element:   "named",
					typeName:  "Named",
					namespace: validationNMTOKENNamespace,
					value:     " \tother\r\n ",
					code:      goxsd9.EnumerationValueViolationCode,
					message:   "value is not in the NMTOKEN enumeration",
					specRef:   validationNMTOKENEnumerationSpecRef(policy.version),
				},
				{
					name:      "named invalid NameChar",
					element:   "plain",
					typeName:  "Plain",
					namespace: validationNMTOKENNamespace,
					value:     "bad/value",
					code:      goxsd9.InvalidNMTOKENLexicalCode,
					message:   "NMTOKEN value contains an invalid NameChar",
					specRef:   validationNMTOKENDatatypeSpecRef(policy.version),
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationNMTOKENInstance(test.element, test.value, test.selfClosing)
					firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, firstErr)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), test.code)
					}
					if diagnostic.Message() != test.message {
						t.Fatalf("Message() = %q, want %q", diagnostic.Message(), test.message)
					}
					wantLoc := validationTestTextLoc(t, input)
					if test.selfClosing {
						wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
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
					wantRelated := validationNMTOKENRelated(t, schema, test.element, test.typeName, test.namespace)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}

					secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					second := validationTestDiagnostic(t, secondErr)
					if diagnostic.Error() != second.Error() || diagnostic.Loc() != second.Loc() || diagnostic.SpecRef() != second.SpecRef() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
						t.Fatalf("repeated NMTOKEN diagnostics differ: first %v, second %v", firstErr, secondErr)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("NMTOKEN diagnostic validation mutated the completed schema")
			}
		})
	}
}

func TestValidateInstanceKeepsNMTOKENOutsideRootScalarBoundaryUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, validationNMTOKENChoiceSchemaRoot(policy.version), nil, policy.policy)
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(
				`<box xmlns="`+validationNMTOKENNamespace+`"><nmtoken>value</nmtoken></box>`,
			)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
				t.Fatalf("choice NMTOKEN diagnostic = %s/%q/%q/%q, want instance-validation unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
			}
			wantSpecRef := "xsd11-structures#cvc-elt"
			if policy.version == goxsd9.XSDVersion10 {
				wantSpecRef = "xsd10-structures#cvc-elt"
			}
			if diagnostic.SpecRef() != wantSpecRef {
				t.Fatalf("choice NMTOKEN SpecRef() = %q, want %q", diagnostic.SpecRef(), wantSpecRef)
			}
			if !errors.Is(err, goxsd9.ErrUnsupported) || diagnostic.Loc().IsZero() {
				t.Fatalf("choice NMTOKEN diagnostic lost unsupported classification or location: %v", err)
			}
		})
	}
}

func TestValidateInstanceKeepsNMTOKENInstanceShapeBoundariesUnsupported(t *testing.T) {
	schema := validationNMTOKENSchema(t, goxsd9.XSDVersion11, goxsd9.Strict11)
	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "attribute", input: `<direct xmlns="` + validationNMTOKENNamespace + `" label="x">value</direct>`},
		{name: "xsi nil", input: `<direct xmlns="` + validationNMTOKENNamespace + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:nil="true">value</direct>`},
		{name: "xsi type", input: `<direct xmlns="` + validationNMTOKENNamespace + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="xs:string">value</direct>`},
		{name: "child element", input: `<direct xmlns="` + validationNMTOKENNamespace + `">value<child/></direct>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("NMTOKEN shape diagnostic = %s/%q/%q, want instance-validation unsupported", diagnostic, diagnostic.Class(), diagnostic.Feature())
			}
		})
	}
}

func validationNMTOKENSchema(t *testing.T, version goxsd9.XSDVersion, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	root := validationNMTOKENSchemaRoot(version)
	fixtures := map[string]validationTestFixture{
		"nmtoken-chameleon.xsd": {
			id: "nmtoken-chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `">
  <xs:simpleType name="Included">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value=" included "/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
		},
		"nmtoken-other.xsd": {
			id: "nmtoken-other.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationImportedNamespace + `" targetNamespace="` + validationImportedNamespace + `" version="` + string(version) + `">
  <xs:simpleType name="ImportedBase">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value=" imported "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Imported">
    <xs:restriction base="o:ImportedBase"/>
  </xs:simpleType>
</xs:schema>`,
		},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, policy)
}

func validationNMTOKENSchemaRoot(version goxsd9.XSDVersion) string {
	return `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNMTOKENNamespace + `" xmlns:o="` + validationImportedNamespace + `" targetNamespace="` + validationNMTOKENNamespace + `" version="` + string(version) + `">
  <xs:include schemaLocation="nmtoken-chameleon.xsd"/>
  <xs:import namespace="` + validationImportedNamespace + `" schemaLocation="nmtoken-other.xsd"/>
  <xs:element name="direct" type="xs:NMTOKEN"/>
  <xs:element name="named" type="r:Named"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="inherited" type="r:Inherited"/>
  <xs:element name="included" type="r:Included"/>
  <xs:element name="imported" type="o:Imported"/>
  <xs:element name="plain" type="r:Plain"/>
  <xs:simpleType name="Named">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value="  allowed  "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Forward">
    <xs:restriction base="r:Later"/>
  </xs:simpleType>
  <xs:simpleType name="Inherited">
    <xs:restriction base="r:Forward"/>
  </xs:simpleType>
  <xs:simpleType name="Later">
    <xs:restriction base="xs:NMTOKEN">
      <xs:enumeration value=" forward "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Plain">
    <xs:restriction base="xs:NMTOKEN"/>
  </xs:simpleType>
</xs:schema>`
}

func validationNMTOKENChoiceSchemaRoot(version goxsd9.XSDVersion) string {
	return `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationNMTOKENNamespace + `" targetNamespace="` + validationNMTOKENNamespace + `" version="` + string(version) + `">
  <xs:element name="nmtoken" type="xs:NMTOKEN"/>
  <xs:element name="box" type="r:Choice"/>
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element ref="r:nmtoken"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
}

func validationNMTOKENInstance(element, value string, selfClosing bool) string {
	if selfClosing {
		return `<` + element + ` xmlns="` + validationNMTOKENNamespace + `"/>`
	}
	return `<` + element + ` xmlns="` + validationNMTOKENNamespace + `">` + value + `</` + element + `>`
}

func validationNMTOKENRelated(t *testing.T, schema goxsd9.Schema, element, typeName, namespace string) []goxsd9.Loc {
	t.Helper()
	declaration := validationNMTOKENElement(t, schema, element)
	if typeName == "" {
		return []goxsd9.Loc{declaration.Loc()}
	}
	component := validationNMTOKENSimpleType(t, schema, namespace, typeName)
	definition, ok := component.SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type definition view", typeName)
	}
	enumerationLocations := definition.StringEnumerationFacets().Locations()
	related := make([]goxsd9.Loc, 2, 2+len(enumerationLocations))
	related[0] = declaration.Loc()
	related[1] = component.Loc()
	return append(related, enumerationLocations...)
}

func validationNMTOKENDatatypeSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#dt-NMTOKEN"
	}
	return "xsd11-datatypes#dt-NMTOKEN"
}

func validationNMTOKENEnumerationSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#NMTOKEN-facets"
	}
	return "xsd11-datatypes#NMTOKEN-facets"
}

func validationNMTOKENElement(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName(validationNMTOKENNamespace, local)
	if err != nil {
		t.Fatalf("NewQName(%q): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(components) != 1 {
		t.Fatalf("%s element declarations = %d, want one", local, len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("%s has no element declaration view", local)
	}
	return declaration
}

func validationNMTOKENSimpleType(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName(%q): %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s simple type definitions = %d, want one", name, len(components))
	}
	return components[0]
}
