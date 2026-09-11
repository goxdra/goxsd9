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
	validationTokenNamespace      = "urn:token-root" //nolint:gosec // Test namespace is not a credential.
	validationTokenOtherNamespace = "urn:token-other"
)

type validationTokenPolicyCase struct {
	name    string
	policy  goxsd9.LanguagePolicy
	version goxsd9.XSDVersion
}

func validationTokenPolicies() []validationTokenPolicyCase {
	return []validationTokenPolicyCase{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: goxsd9.XSDVersion11},
		{name: "Strict10", policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
		{name: "Strict11", policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
	}
}

//nolint:gocognit // Keep graph shape, value-space, location, and immutability coverage together.
func TestValidateInstanceSupportsGlobalTokenScalarsAcrossPolicies(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationTokenSchema(t, policy.version, policy.policy)
			before := schema.Components()
			for _, test := range []struct {
				name        string
				element     string
				value       string
				selfClosing bool
			}{
				{name: "direct arbitrary value", element: "direct", value: "\tany token value\r\n"},
				{name: "direct no text", element: "direct", selfClosing: true},
				{name: "named collapsed", element: "named", value: "\tallowed\n"},
				{name: "forward collapsed", element: "forward", value: " \tforward\r"},
				{name: "inherited collapsed", element: "inherited", value: "\r forward \t"},
				{name: "included chameleon", element: "included", value: " \tincluded\n"},
				{name: "imported inherited", element: "imported", value: "\rimported\t"},
				{name: "explicit empty member", element: "empty", selfClosing: true},
				{name: "explicit empty member whitespace", element: "empty", value: "\t \r\n"},
				{name: "whitespace-only member", element: "whitespaceEmpty", selfClosing: true},
				{name: "whitespace-only member whitespace", element: "whitespaceEmpty", value: " \t "},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationTokenInstance(test.element, test.value, test.selfClosing)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}

			for _, test := range []struct {
				name      string
				values    []string
				typeName  string
				namespace string
			}{
				{name: "named", typeName: "Named", namespace: validationTokenNamespace, values: []string{"  allowed  "}},
				{name: "forward", typeName: "Forward", namespace: validationTokenNamespace, values: []string{" forward "}},
				{name: "inherited", typeName: "Inherited", namespace: validationTokenNamespace, values: []string{" forward "}},
				{name: "included", typeName: "Included", namespace: validationTokenNamespace, values: []string{" included "}},
				{name: "imported", typeName: "Imported", namespace: validationTokenOtherNamespace, values: []string{" imported "}},
				{name: "empty", typeName: "Empty", namespace: validationTokenNamespace, values: []string{""}},
				{name: "whitespaceEmpty", typeName: "WhitespaceEmpty", namespace: validationTokenNamespace, values: []string{"   "}},
			} {
				t.Run("raw facts/"+test.name, func(t *testing.T) {
					component := validationTokenSimpleType(t, schema, test.namespace, test.typeName)
					definition, ok := component.SimpleTypeDefinition()
					if !ok {
						t.Fatalf("%s has no simple type definition view", test.typeName)
					}
					facets := definition.StringEnumerationFacets()
					if !facets.HasEnumeration() || !reflect.DeepEqual(facets.Values(), test.values) {
						t.Fatalf("%s enumeration = (has=%t, values=%#v), want true,%#v", test.typeName, facets.HasEnumeration(), facets.Values(), test.values)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("token validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit // Keep token enumeration diagnostics and repeatability assertions together.
func TestValidateInstanceReportsCollapsedTokenEnumerationViolations(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationTokenSchema(t, policy.version, policy.policy)
			before := schema.Components()
			for _, test := range []struct {
				name        string
				element     string
				typeName    string
				namespace   string
				value       string
				selfClosing bool
			}{
				{name: "named non-member", element: "named", typeName: "Named", namespace: validationTokenNamespace, value: "other"},
				{name: "forward non-member", element: "forward", typeName: "Forward", namespace: validationTokenNamespace, value: "other"},
				{name: "inherited non-member", element: "inherited", typeName: "Inherited", namespace: validationTokenNamespace, value: "other"},
				{name: "included non-member", element: "included", typeName: "Included", namespace: validationTokenNamespace, value: "other"},
				{name: "imported non-member", element: "imported", typeName: "Imported", namespace: validationTokenOtherNamespace, value: "other"},
				{name: "empty non-member", element: "empty", typeName: "Empty", namespace: validationTokenNamespace, value: "other"},
				{name: "whitespaceEmpty non-member", element: "whitespaceEmpty", typeName: "WhitespaceEmpty", namespace: validationTokenNamespace, value: "other"},
				{name: "no text uses root location", element: "named", typeName: "Named", namespace: validationTokenNamespace, selfClosing: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationTokenInstance(test.element, test.value, test.selfClosing)
					firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, firstErr)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.EnumerationValueViolationCode {
						t.Fatalf("diagnostic = %s/%q, want invalid token enumeration violation", diagnostic, diagnostic.Code())
					}
					wantLoc := validationTestTextLoc(t, input)
					if test.selfClosing {
						wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
					}
					if diagnostic.Loc() != wantLoc {
						t.Fatalf("Loc() = %s, want %s", diagnostic.Loc(), wantLoc)
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
					wantRelated := validationTokenRelated(t, schema, test.element, test.typeName, test.namespace)
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}

					secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					second := validationTestDiagnostic(t, secondErr)
					if diagnostic.Error() != second.Error() || diagnostic.Loc() != second.Loc() || diagnostic.SpecRef() != second.SpecRef() || !reflect.DeepEqual(diagnostic.Related(), second.Related()) {
						t.Fatalf("repeated token diagnostics differ: first %v, second %v", firstErr, secondErr)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("token enumeration validation mutated the completed schema")
			}
		})
	}
}

func TestValidateInstanceKeepsTokenOutsideRootScalarBoundaryUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := validationTokenChoiceSchemaRoot(policy.version)
			schema := validationTestSchemaWithPolicy(t, root, nil, policy.policy)
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(
				`<box xmlns="`+validationTokenNamespace+`"><token>value</token></box>`,
			)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
				t.Fatalf("choice token diagnostic = %s/%q/%q/%q, want instance-validation unsupported", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
			}
			wantSpecRef := "xsd11-structures#cvc-elt"
			if policy.version == goxsd9.XSDVersion10 {
				wantSpecRef = "xsd10-structures#cvc-elt"
			}
			if diagnostic.SpecRef() != wantSpecRef {
				t.Fatalf("choice token SpecRef() = %q, want %q", diagnostic.SpecRef(), wantSpecRef)
			}
			if !errors.Is(err, goxsd9.ErrUnsupported) || diagnostic.Loc().IsZero() {
				t.Fatalf("choice token diagnostic lost unsupported classification or location: %v", err)
			}
		})
	}
}

func TestValidateInstanceKeepsExcludedGlobalStringFamiliesUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			for _, datatype := range []string{"string", "NMTOKEN"} {
				t.Run(datatype, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="` + validationTokenNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="item" type="xs:` + datatype + `"/>
</xs:schema>`
					schema := validationTestSchemaWithPolicy(t, root, nil, policy.policy)
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(
						`<item xmlns="`+validationTokenNamespace+`">value</item>`,
					)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("%s diagnostic = %s/%q/%q, want instance-validation unsupported", datatype, diagnostic, diagnostic.Class(), diagnostic.Code())
					}
				})
			}
		})
	}
}

func validationTokenSchema(t *testing.T, version goxsd9.XSDVersion, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	root := validationTokenSchemaRoot(version)
	fixtures := map[string]validationTestFixture{
		"token-chameleon.xsd": {
			id: "token-chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `">
  <xs:simpleType name="Included">
    <xs:restriction base="xs:token">
      <xs:enumeration value=" included "/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`,
		},
		"token-other.xsd": {
			id: "token-other.xsd",
			contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationTokenOtherNamespace + `" targetNamespace="` + validationTokenOtherNamespace + `" version="` + string(version) + `">
  <xs:simpleType name="ImportedBase">
    <xs:restriction base="xs:token">
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

func validationTokenSchemaRoot(version goxsd9.XSDVersion) string {
	return `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationTokenNamespace + `" xmlns:o="` + validationTokenOtherNamespace + `" targetNamespace="` + validationTokenNamespace + `" version="` + string(version) + `">
  <xs:include schemaLocation="token-chameleon.xsd"/>
  <xs:import namespace="` + validationTokenOtherNamespace + `" schemaLocation="token-other.xsd"/>
  <xs:element name="direct" type="xs:token"/>
  <xs:element name="named" type="r:Named"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="inherited" type="r:Inherited"/>
  <xs:element name="included" type="r:Included"/>
  <xs:element name="imported" type="o:Imported"/>
  <xs:element name="empty" type="r:Empty"/>
  <xs:element name="whitespaceEmpty" type="r:WhitespaceEmpty"/>
  <xs:simpleType name="Named">
    <xs:restriction base="xs:token">
      <xs:enumeration value="  allowed  "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Forward">
    <xs:restriction base="r:Later"/>
  </xs:simpleType>
  <xs:simpleType name="Inherited">
    <xs:restriction base="r:Later"/>
  </xs:simpleType>
  <xs:simpleType name="Later">
    <xs:restriction base="xs:token">
      <xs:enumeration value=" forward "/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Empty">
    <xs:restriction base="xs:token">
      <xs:enumeration value=""/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="WhitespaceEmpty">
    <xs:restriction base="xs:token">
      <xs:enumeration value="   "/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

func validationTokenChoiceSchemaRoot(version goxsd9.XSDVersion) string {
	return `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationTokenNamespace + `" targetNamespace="` + validationTokenNamespace + `" version="` + string(version) + `">
  <xs:element name="token" type="xs:token"/>
  <xs:element name="box" type="r:Choice"/>
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element ref="r:token"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`
}

func validationTokenInstance(element, value string, selfClosing bool) string {
	if selfClosing {
		return `<` + element + ` xmlns="` + validationTokenNamespace + `"/>`
	}
	return `<` + element + ` xmlns="` + validationTokenNamespace + `">` + value + `</` + element + `>`
}

func validationTokenSimpleType(t *testing.T, schema goxsd9.Schema, namespace, local string) goxsd9.Component {
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

func validationTokenElement(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName(validationTokenNamespace, local)
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

func validationTokenRelated(t *testing.T, schema goxsd9.Schema, element, typeName, namespace string) []goxsd9.Loc {
	t.Helper()
	declaration := validationTokenElement(t, schema, element)
	component := validationTokenSimpleType(t, schema, namespace, typeName)
	definition, ok := component.SimpleTypeDefinition()
	if !ok {
		t.Fatalf("%s has no simple type definition view", typeName)
	}
	enumerationLocations := definition.StringEnumerationFacets().Locations()
	related := make([]goxsd9.Loc, 0, 2+len(enumerationLocations))
	related = append(related, declaration.Loc(), component.Loc())
	return append(related, enumerationLocations...)
}

func validationTokenEnumerationSpecRef(version goxsd9.XSDVersion) string {
	if version == goxsd9.XSDVersion10 {
		return "xsd10-datatypes#cvc-enumeration-valid"
	}
	return "xsd11-datatypes#cvc-enumeration-valid"
}
