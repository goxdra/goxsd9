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
	validationLocalTokenChoiceNamespace      = "urn:local-token-choice"       //nolint:gosec // Test namespace is not a credential.
	validationLocalTokenChoiceOtherNamespace = "urn:local-token-choice-other" //nolint:gosec // Test namespace is not a credential.
)

func TestValidateInstanceSupportsLocalTokenChoicesAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationLocalTokenChoiceSchema(t, policy)
			before := schema.Components()
			for _, test := range []struct {
				name        string
				child       string
				value       string
				selfClosing bool
			}{
				{name: "built-in arbitrary value", child: "direct", value: " \tfree token\r\n"},
				{name: "named restriction collapsed", child: "named", value: "\t allowed \r\n"},
				{name: "chameleon restriction collapsed", child: "included", value: "\r included \t"},
				{name: "imported restriction collapsed", child: "imported", value: "\t imported \n"},
				{name: "empty restriction", child: "empty", selfClosing: true},
				{name: "empty restriction whitespace-only", child: "empty", value: "\t \r\n"},
				{name: "whitespace enumeration", child: "whitespaceEmpty", value: " \t "},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationLocalTokenChoiceInstance(test.child, test.value, test.selfClosing)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("local token-choice validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit // Keep value-space and structural diagnostic matrices together.
func TestValidateInstanceReportsLocalTokenChoiceDiagnosticsAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationLocalTokenChoiceSchema(t, policy)
			for _, test := range []struct {
				name      string
				child     string
				typeName  string
				namespace string
				value     string
			}{
				{name: "named restriction", child: "named", typeName: "Named", namespace: validationLocalTokenChoiceNamespace, value: "not allowed"},
				{name: "imported restriction", child: "imported", typeName: "Imported", namespace: validationLocalTokenChoiceOtherNamespace, value: "not imported"},
			} {
				t.Run("token enumeration/"+test.name, func(t *testing.T) {
					input := validationLocalTokenChoiceInstance(test.child, test.value, false)
					firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, firstErr)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.EnumerationValueViolationCode {
						t.Fatalf("diagnostic = %s/%q, want invalid token enumeration violation", diagnostic, diagnostic.Code())
					}
					if diagnostic.Loc() != validationLocalTokenChoiceTextLoc(t, input, test.child) {
						t.Fatalf("Loc() = %s, want selected token text location", diagnostic.Loc())
					}
					if diagnostic.Message() != "value is not in the token enumeration" {
						t.Fatalf("Message() = %q, want token enumeration message", diagnostic.Message())
					}
					if diagnostic.SpecRef() != validationTokenEnumerationSpecRef(policy.version) {
						t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), validationTokenEnumerationSpecRef(policy.version))
					}
					if diagnostic.Unwrap() == nil || errors.Is(firstErr, goxsd9.ErrUnsupported) {
						t.Fatalf("enumeration diagnostic cause or classification is wrong: %v", firstErr)
					}
					wantRelated := validationLocalTokenChoiceEnumerationRelated(t, schema, test.child, test.typeName, test.namespace)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}

					secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					second := validationTestDiagnostic(t, secondErr)
					if diagnostic.Error() != second.Error() || diagnostic.Loc() != second.Loc() || diagnostic.SpecRef() != second.SpecRef() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
						t.Fatalf("repeated token-choice diagnostics differ: first %v, second %v", firstErr, secondErr)
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
					name:   "unknown child wins before token validation",
					input:  `<choiceRoot xmlns="` + validationLocalTokenChoiceNamespace + `"><unknown xmlns="">not allowed</unknown></choiceRoot>`,
					marker: "<unknown",
				},
				{
					name:   "repeated child wins before token validation",
					input:  `<choiceRoot xmlns="` + validationLocalTokenChoiceNamespace + `"><named xmlns="">allowed</named><named xmlns="">not allowed</named></choiceRoot>`,
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
					if diagnostic.Loc() != validationLocalTokenChoiceMarkerLoc(t, test.input, test.marker, test.last) {
						t.Fatalf("Loc() = %s, want structural marker location", diagnostic.Loc())
					}
					if diagnostic.SpecRef() != validationLocalTokenChoiceStructureSpecRef(policy.version) {
						t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), validationLocalTokenChoiceStructureSpecRef(policy.version))
					}
					if diagnostic.Unwrap() == nil || errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("structural diagnostic cause or classification is wrong: %v", err)
					}
					wantRelated := validationLocalTokenChoiceStructureRelated(t, schema)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}
				})
			}
		})
	}
}

//nolint:gocognit // Keep the excluded direct-choice shape matrix together.
func TestValidateInstanceKeepsExcludedLocalTokenChoiceShapesUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			for _, test := range []struct {
				name         string
				alternatives string
				child        string
			}{
				{
					name:         "mixed token and integer families",
					alternatives: `<xs:element name="token" type="xs:token"/><xs:element name="number" type="xs:integer"/>`,
					child:        "token",
				},
				{
					name:         "local NMTOKEN",
					alternatives: `<xs:element name="value" type="xs:NMTOKEN"/>`,
					child:        "value",
				},
				{
					name:         "non-default token occurrence",
					alternatives: `<xs:element name="value" type="xs:token" minOccurs="0"/>`,
					child:        "value",
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					schema := validationLocalTokenChoiceShapeSchema(t, policy, test.alternatives)
					before := schema.Components()
					input := validationLocalTokenChoiceInstance(test.child, "value", false)
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
						t.Fatalf("diagnostic = %s/%q/%q, want unsupported instance validation", diagnostic, diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.Loc().IsZero() || diagnostic.SpecRef() != validationLocalTokenChoiceStructureSpecRef(policy.version) || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("unsupported diagnostic evidence or cause is wrong: %v", err)
					}
					if !reflect.DeepEqual(before, schema.Components()) {
						t.Fatal("unsupported local token-choice validation mutated the completed schema")
					}
				})
			}
		})
	}
}

func validationLocalTokenChoiceSchema(t *testing.T, policy validationTokenPolicyCase) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationLocalTokenChoiceNamespace + `" xmlns:o="` + validationLocalTokenChoiceOtherNamespace + `" targetNamespace="` + validationLocalTokenChoiceNamespace + `" version="` + string(policy.version) + `">
  <xs:include schemaLocation="local-token-choice-chameleon.xsd"/>
  <xs:import namespace="` + validationLocalTokenChoiceOtherNamespace + `" schemaLocation="local-token-choice-other.xsd"/>
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="direct" type="xs:token"/>
    <xs:element name="named" type="r:Named"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="imported" type="o:Imported"/>
    <xs:element name="empty" type="r:Empty"/>
    <xs:element name="whitespaceEmpty" type="r:WhitespaceEmpty"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Named"><xs:restriction base="xs:token"><xs:enumeration value=" allowed "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Empty"><xs:restriction base="xs:token"><xs:enumeration value=""/></xs:restriction></xs:simpleType>
  <xs:simpleType name="WhitespaceEmpty"><xs:restriction base="xs:token"><xs:enumeration value="   "/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]validationTestFixture{
		"local-token-choice-chameleon.xsd": {
			id: "local-token-choice-chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `">
  <xs:simpleType name="Included"><xs:restriction base="xs:token"><xs:enumeration value=" included "/></xs:restriction></xs:simpleType>
</xs:schema>`,
		},
		"local-token-choice-other.xsd": {
			id: "local-token-choice-other.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="` + validationLocalTokenChoiceOtherNamespace + `" version="` + string(policy.version) + `">
  <xs:simpleType name="Imported"><xs:restriction base="xs:token"><xs:enumeration value=" imported "/></xs:restriction></xs:simpleType>
</xs:schema>`,
		},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, policy.policy)
}

func validationLocalTokenChoiceShapeSchema(t *testing.T, policy validationTokenPolicyCase, alternatives string) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationLocalTokenChoiceNamespace + `" targetNamespace="` + validationLocalTokenChoiceNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>` + alternatives + `</xs:choice></xs:complexType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, nil, policy.policy)
}

func validationLocalTokenChoiceInstance(child, value string, selfClosing bool) string {
	if selfClosing {
		return `<choiceRoot xmlns="` + validationLocalTokenChoiceNamespace + `"><` + child + ` xmlns=""/></choiceRoot>`
	}
	return `<choiceRoot xmlns="` + validationLocalTokenChoiceNamespace + `"><` + child + ` xmlns="">` + value + `</` + child + `></choiceRoot>`
}

func validationLocalTokenChoiceDefinition(t *testing.T, schema goxsd9.Schema) (goxsd9.Component, goxsd9.Component, goxsd9.ChoiceParticle) {
	t.Helper()
	rootName, err := goxsd9.NewQName(validationLocalTokenChoiceNamespace, "choiceRoot")
	if err != nil {
		t.Fatalf("NewQName choiceRoot: %v", err)
	}
	rootComponents := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
	if len(rootComponents) != 1 {
		t.Fatalf("choiceRoot declarations = %d, want one", len(rootComponents))
	}
	choiceName, err := goxsd9.NewQName(validationLocalTokenChoiceNamespace, "Choice")
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

func validationLocalTokenChoiceElement(t *testing.T, choice goxsd9.ChoiceParticle, local string) goxsd9.ElementParticle {
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

func validationLocalTokenChoiceType(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.Component {
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

func validationLocalTokenChoiceEnumerationRelated(t *testing.T, schema goxsd9.Schema, child, typeName, namespace string) []goxsd9.Loc {
	t.Helper()
	rootComponent, choiceComponent, choice := validationLocalTokenChoiceDefinition(t, schema)
	element := validationLocalTokenChoiceElement(t, choice, child)
	typeComponent := validationLocalTokenChoiceType(t, schema, namespace, typeName)
	typeDefinition, ok := typeComponent.SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type definition view", typeName)
	}
	enumerationLocations := typeDefinition.StringEnumerationFacets().Locations()
	related := make([]goxsd9.Loc, 0, 5+len(enumerationLocations))
	related = append(related, rootComponent.Loc(), choiceComponent.Loc(), choice.Loc(), element.Loc(), typeComponent.Loc())
	return append(related, enumerationLocations...)
}

func validationLocalTokenChoiceStructureRelated(t *testing.T, schema goxsd9.Schema) []goxsd9.Loc {
	t.Helper()
	rootComponent, choiceComponent, choice := validationLocalTokenChoiceDefinition(t, schema)
	alternatives := choice.Alternatives()
	related := make([]goxsd9.Loc, 0, 3+len(alternatives))
	related = append(related, rootComponent.Loc(), choiceComponent.Loc(), choice.Loc())
	for _, particle := range alternatives {
		element, ok := particle.(goxsd9.ElementParticle)
		if !ok {
			t.Fatalf("choice alternative = %T, want element particle", particle)
		}
		related = append(related, element.Loc())
	}
	return related
}

func validationLocalTokenChoiceTextLoc(t *testing.T, input, local string) goxsd9.Loc {
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

func validationLocalTokenChoiceMarkerLoc(t *testing.T, input, marker string, last bool) goxsd9.Loc {
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

func validationLocalTokenChoiceStructureSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-structures#cvc-elt"
	}
	return "xsd11-structures#cvc-elt"
}
