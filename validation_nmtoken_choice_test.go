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
	validationLocalNMTOKENChoiceNamespace      = "urn:local-nmtoken-choice"       //nolint:gosec // Test namespace is not a credential.
	validationLocalNMTOKENChoiceOtherNamespace = "urn:local-nmtoken-choice-other" //nolint:gosec // Test namespace is not a credential.
)

func TestValidateInstanceSupportsLocalNMTOKENChoicesAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationLocalNMTOKENChoiceSchema(t, policy)
			before := schema.Components()
			for _, test := range []struct {
				name  string
				child string
				value string
			}{
				{name: "built-in Unicode NameChar with collapsed whitespace", child: "direct", value: "\t名-9·́ \r\n"},
				{name: "named restriction with collapsed whitespace", child: "named", value: "\t allowed \r\n"},
				{name: "chameleon restriction with collapsed whitespace", child: "included", value: "\r included \t"},
				{name: "imported inherited restriction with collapsed whitespace", child: "imported", value: "\t imported \n"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationLocalNMTOKENChoiceInstance(test.child, test.value)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("local NMTOKEN-choice validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit,funlen // Keep NMTOKEN lexical, enumeration, structure, and provenance assertions together.
func TestValidateInstanceReportsLocalNMTOKENChoiceDiagnosticsAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationLocalNMTOKENChoiceSchema(t, policy)
			before := schema.Components()
			for _, test := range []struct {
				name        string
				child       string
				typeName    string
				namespace   string
				value       string
				selfClosing bool
				code        string
				message     string
				specRef     string
			}{
				{
					name:        "empty value",
					child:       "direct",
					selfClosing: true,
					code:        goxsd9.InvalidNMTOKENLexicalCode,
					message:     "NMTOKEN value is empty after XML whitespace collapse",
					specRef:     validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:    "whitespace-only value",
					child:   "direct",
					value:   " \t\r\n ",
					code:    goxsd9.InvalidNMTOKENLexicalCode,
					message: "NMTOKEN value is whitespace-only after XML whitespace collapse",
					specRef: validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:    "invalid NameChar",
					child:   "direct",
					value:   "bad/value",
					code:    goxsd9.InvalidNMTOKENLexicalCode,
					message: "NMTOKEN value contains an invalid NameChar",
					specRef: validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:      "invalid NameChar before enumeration",
					child:     "named",
					typeName:  "Named",
					namespace: validationLocalNMTOKENChoiceNamespace,
					value:     "bad/value",
					code:      goxsd9.InvalidNMTOKENLexicalCode,
					message:   "NMTOKEN value contains an invalid NameChar",
					specRef:   validationNMTOKENDatatypeSpecRef(policy.version),
				},
				{
					name:      "enumeration after collapse",
					child:     "named",
					typeName:  "Named",
					namespace: validationLocalNMTOKENChoiceNamespace,
					value:     " \tother\r\n ",
					code:      goxsd9.EnumerationValueViolationCode,
					message:   "value is not in the NMTOKEN enumeration",
					specRef:   validationNMTOKENEnumerationSpecRef(policy.version),
				},
			} {
				t.Run("value/"+test.name, func(t *testing.T) {
					input := validationLocalNMTOKENChoiceInstance(test.child, test.value)
					if test.selfClosing {
						input = validationLocalNMTOKENChoiceEmptyInstance(test.child)
					}
					firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, firstErr)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), test.code)
					}
					if diagnostic.Message() != test.message {
						t.Fatalf("Message() = %q, want %q", diagnostic.Message(), test.message)
					}
					wantLoc := validationLocalNMTOKENChoiceTextLoc(t, input, test.child)
					if test.selfClosing {
						wantLoc = validationLocalNMTOKENChoiceMarkerLoc(t, input, "<"+test.child, false)
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
					wantRelated := validationLocalNMTOKENChoiceRelated(t, schema, test.child, test.typeName, test.namespace)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}

					secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					second := validationTestDiagnostic(t, secondErr)
					if diagnostic.Error() != second.Error() || diagnostic.Loc() != second.Loc() || diagnostic.SpecRef() != second.SpecRef() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
						t.Fatalf("repeated NMTOKEN-choice diagnostics differ: first %v, second %v", firstErr, secondErr)
					}
				})
			}

			for _, test := range []struct {
				name   string
				input  string
				marker string
				last   bool
			}{
				{
					name:   "unknown child wins before lexical validation",
					input:  `<choiceRoot xmlns="` + validationLocalNMTOKENChoiceNamespace + `"><unknown xmlns="">bad/value</unknown></choiceRoot>`,
					marker: "<unknown",
				},
				{
					name:   "repeated child wins before lexical validation",
					input:  `<choiceRoot xmlns="` + validationLocalNMTOKENChoiceNamespace + `"><named xmlns="">allowed</named><named xmlns="">bad/value</named></choiceRoot>`,
					marker: "<named",
					last:   true,
				},
			} {
				t.Run("structure/"+test.name, func(t *testing.T) {
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidInstanceChoiceCode {
						t.Fatalf("diagnostic = %s/%q, want invalid direct-choice structure", diagnostic, diagnostic.Code())
					}
					if diagnostic.Loc() != validationLocalNMTOKENChoiceMarkerLoc(t, test.input, test.marker, test.last) {
						t.Fatalf("Loc() = %s, want structural marker location", diagnostic.Loc())
					}
					if diagnostic.SpecRef() != validationLocalNMTOKENChoiceStructureSpecRef(policy.version) {
						t.Fatalf("SpecRef() = %q, want choice structure reference", diagnostic.SpecRef())
					}
					if diagnostic.Unwrap() == nil || errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("structural diagnostic cause or classification is wrong: %v", err)
					}
					wantRelated := validationLocalNMTOKENChoiceStructureRelated(t, schema)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("local NMTOKEN-choice diagnostics mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit // Keep excluded local NMTOKEN choice shapes explicit.
func TestValidateInstanceKeepsExcludedLocalNMTOKENChoiceShapesUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			for _, test := range []struct {
				name         string
				alternatives string
				child        string
			}{
				{
					name:         "mixed NMTOKEN and integer families",
					alternatives: `<xs:element name="nmtoken" type="xs:NMTOKEN"/><xs:element name="number" type="xs:integer"/>`,
					child:        "nmtoken",
				},
				{
					name:         "non-default NMTOKEN occurrence",
					alternatives: `<xs:element name="value" type="xs:NMTOKEN" minOccurs="0"/>`,
					child:        "value",
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					schema := validationLocalNMTOKENChoiceShapeSchema(t, policy, test.alternatives)
					before := schema.Components()
					input := validationLocalNMTOKENChoiceInstance(test.child, "value")
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
						t.Fatalf("diagnostic = %s/%q/%q, want unsupported instance validation", diagnostic, diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.Loc().IsZero() || diagnostic.SpecRef() != validationLocalNMTOKENChoiceStructureSpecRef(policy.version) || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("unsupported diagnostic evidence or cause is wrong: %v", err)
					}
					if !reflect.DeepEqual(before, schema.Components()) {
						t.Fatal("unsupported local NMTOKEN-choice validation mutated the completed schema")
					}
				})
			}
		})
	}
}

func validationLocalNMTOKENChoiceSchema(t *testing.T, policy validationTokenPolicyCase) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationLocalNMTOKENChoiceNamespace + `" xmlns:o="` + validationLocalNMTOKENChoiceOtherNamespace + `" targetNamespace="` + validationLocalNMTOKENChoiceNamespace + `" version="` + string(policy.version) + `">
  <xs:include schemaLocation="local-nmtoken-choice-chameleon.xsd"/>
  <xs:import namespace="` + validationLocalNMTOKENChoiceOtherNamespace + `" schemaLocation="local-nmtoken-choice-other.xsd"/>
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="direct" type="xs:NMTOKEN"/>
    <xs:element name="named" type="r:Named"/>
    <xs:element name="unicode" type="r:Unicode"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="imported" type="o:Imported"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Named"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" allowed "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Unicode"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="名-9·́"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]validationTestFixture{
		"local-nmtoken-choice-chameleon.xsd": {
			id: "local-nmtoken-choice-chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `">
  <xs:simpleType name="Included"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" included "/></xs:restriction></xs:simpleType>
</xs:schema>`,
		},
		"local-nmtoken-choice-other.xsd": {
			id: "local-nmtoken-choice-other.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationLocalNMTOKENChoiceOtherNamespace + `" targetNamespace="` + validationLocalNMTOKENChoiceOtherNamespace + `" version="` + string(policy.version) + `">
  <xs:simpleType name="ImportedBase"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value=" imported "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Imported"><xs:restriction base="o:ImportedBase"/></xs:simpleType>
</xs:schema>`,
		},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, policy.policy)
}

func validationLocalNMTOKENChoiceShapeSchema(t *testing.T, policy validationTokenPolicyCase, alternatives string) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationLocalNMTOKENChoiceNamespace + `" targetNamespace="` + validationLocalNMTOKENChoiceNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>` + alternatives + `</xs:choice></xs:complexType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy.policy)
}

func validationLocalNMTOKENChoiceInstance(child, value string) string {
	return `<choiceRoot xmlns="` + validationLocalNMTOKENChoiceNamespace + `"><` + child + ` xmlns="">` + value + `</` + child + `></choiceRoot>`
}

func validationLocalNMTOKENChoiceEmptyInstance(child string) string {
	return `<choiceRoot xmlns="` + validationLocalNMTOKENChoiceNamespace + `"><` + child + ` xmlns=""/></choiceRoot>`
}

func validationLocalNMTOKENChoiceRelated(t *testing.T, schema goxsd9.Schema, child, typeName, namespace string) []goxsd9.Loc {
	t.Helper()
	rootComponent, choiceComponent, choice := validationLocalNMTOKENChoiceDefinition(t, schema)
	element := validationLocalNMTOKENChoiceElement(t, choice, child)
	related := []goxsd9.Loc{rootComponent.Loc(), choiceComponent.Loc(), choice.Loc(), element.Loc()}
	if typeName == "" {
		return related
	}
	typeComponent := validationLocalNMTOKENChoiceType(t, schema, namespace, typeName)
	typeDefinition, ok := typeComponent.SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type definition view", typeName)
	}
	related = append(related, typeComponent.Loc())
	return append(related, typeDefinition.StringEnumerationFacets().Locations()...)
}

func validationLocalNMTOKENChoiceStructureRelated(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	rootComponent, choiceComponent, choice := validationLocalNMTOKENChoiceDefinition(t, schema)
	related := make([]goxsd9.Loc, 0, 3+len(choice.Alternatives()))
	related = append(related, rootComponent.Loc(), choiceComponent.Loc(), choice.Loc())
	for _, particle := range choice.Alternatives() {
		element, ok := particle.(goxsd9.ElementParticle)
		if !ok {
			t.Fatalf("choice alternative = %T, want element particle", particle)
		}
		related = append(related, element.Loc())
	}
	return related
}

func validationLocalNMTOKENChoiceDefinition(t *testing.T, schema goxsd9.Schema) (goxsd9.Component, goxsd9.Component, goxsd9.ChoiceParticle) {
	t.Helper()
	return validationLocalChoiceDefinitionFor(t, schema, validationLocalNMTOKENChoiceNamespace)
}

func validationLocalChoiceDefinitionFor(t *testing.T, schema goxsd9.Schema, namespace string) (goxsd9.Component, goxsd9.Component, goxsd9.ChoiceParticle) {
	t.Helper()
	rootName, err := goxsd9.NewQName(namespace, "choiceRoot")
	if err != nil {
		t.Fatalf("NewQName choiceRoot: %v", err)
	}
	rootComponents := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
	if len(rootComponents) != 1 {
		t.Fatalf("choiceRoot declarations = %d, want one", len(rootComponents))
	}
	choiceName, err := goxsd9.NewQName(namespace, "Choice")
	if err != nil {
		t.Fatalf("NewQName Choice: %v", err)
	}
	choiceComponents := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, choiceName)
	if len(choiceComponents) != 1 {
		t.Fatalf("Choice definitions = %d, want one", len(choiceComponents))
	}
	definition, ok := choiceComponents[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice has no complex type definition view")
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		t.Fatal("Choice has no choice particle")
	}
	return rootComponents[0], choiceComponents[0], choice
}

func validationLocalNMTOKENChoiceElement(t *testing.T, choice goxsd9.ChoiceParticle, local string) goxsd9.ElementParticle {
	t.Helper()
	for _, particle := range choice.Alternatives() {
		element, ok := particle.(goxsd9.ElementParticle)
		if ok && element.Name().Local() == local {
			return element
		}
	}
	t.Fatalf("choice has no %q alternative", local)
	return goxsd9.ElementParticle{}
}

func validationLocalNMTOKENChoiceType(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.Component {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName %s: %v", local, err)
	}
	components := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s simple type definitions = %d, want one", local, len(components))
	}
	return components[0]
}

func validationLocalNMTOKENChoiceTextLoc(t *testing.T, input, local string) goxsd9.Loc {
	t.Helper()
	start := strings.Index(input, "<"+local)
	if start < 0 {
		t.Fatalf("input has no <%s> element: %q", local, input)
	}
	end := strings.IndexByte(input[start:], '>')
	if end < 0 {
		t.Fatalf("input has no <%s> start-tag end: %q", local, input)
	}
	return validationTestLoc(t, "instance.xml", 1, start+end+2)
}

func validationLocalNMTOKENChoiceMarkerLoc(t *testing.T, input, marker string, last bool) goxsd9.Loc {
	t.Helper()
	index := strings.Index(input, marker)
	if last {
		index = strings.LastIndex(input, marker)
	}
	if index < 0 {
		t.Fatalf("input has no marker %q: %q", marker, input)
	}
	return validationTestLoc(t, "instance.xml", 1, index+1)
}

func validationLocalNMTOKENChoiceStructureSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-structures#cvc-elt"
	}
	return "xsd11-structures#cvc-elt"
}
