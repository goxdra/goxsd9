package goxsd9_test

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const (
	validationStringNamespace      = "urn:string-root"
	validationStringOtherNamespace = "urn:string-other"
)

func TestValidateInstanceSupportsGlobalStringScalars(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationStringSchema(t, policy)
			before := schema.Components()
			for _, test := range []struct {
				name, element, value string
				selfClosing          bool
			}{
				{name: "direct Unicode", element: "direct", value: "雪\u00a0été\t"},
				{name: "direct empty", element: "direct", selfClosing: true},
				{name: "preserve", element: "preserve", value: "a  b"},
				{name: "preserve XML whitespace", element: "preserveWhite", value: " \t "},
				{name: "replace", element: "replace", value: "a\tb"},
				{name: "replace whitespace only", element: "replaceSpace", value: "\t"},
				{name: "collapse", element: "collapse", value: " \ta\n  b \r"},
				{name: "collapse empty", element: "empty", selfClosing: true},
				{name: "collapse whitespace only", element: "empty", value: " \t\r\n"},
				{name: "forward", element: "forward", value: "été\u00a0雪"},
				{name: "inherited", element: "inherited", value: "été\u00a0雪"},
				{name: "included chameleon", element: "included", value: "\t included \n"},
				{name: "imported", element: "imported", value: " other "},
				{name: "anonymous", element: "inline", value: "a\nb"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationStringInstance(test.element, test.value, test.selfClosing)
					if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
						t.Fatalf("ValidateInstance(%q): %v", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("string validation mutated the schema")
			}
		})
	}
}

//nolint:gocognit // Diagnostics check location, provenance, cause, edition, and repeatability.
func TestValidateInstanceReportsStringEnumerationViolations(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationStringSchema(t, policy)
			for _, test := range []struct {
				name, element, value, typeName, typeNamespace string
				selfClosing                                   bool
			}{
				{name: "preserve does not collapse", element: "preserve", value: "a b", typeName: "Preserve", typeNamespace: validationStringNamespace},
				{name: "replace does not collapse", element: "replace", value: "a  b", typeName: "Replace", typeNamespace: validationStringNamespace},
				{name: "collapse keeps NBSP", element: "collapse", value: "a\u00a0b", typeName: "Collapse", typeNamespace: validationStringNamespace},
				{name: "whitespace only is not empty in preserve", element: "preserveWhite", selfClosing: true, typeName: "PreserveWhite", typeNamespace: validationStringNamespace},
				{name: "empty is not a replace space", element: "replaceSpace", selfClosing: true, typeName: "ReplaceSpace", typeNamespace: validationStringNamespace},
				{name: "forward", element: "forward", value: "no", typeName: "Forward", typeNamespace: validationStringNamespace},
				{name: "included", element: "included", value: "no", typeName: "Included", typeNamespace: validationStringNamespace},
				{name: "imported", element: "imported", value: "no", typeName: "Imported", typeNamespace: validationStringOtherNamespace},
				{name: "anonymous", element: "inline", value: "no"},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationStringInstance(test.element, test.value, test.selfClosing)
					first := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					diagnostic := validationTestDiagnostic(t, first)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.EnumerationValueViolationCode {
						t.Fatalf("diagnostic = %v, want invalid string enumeration", first)
					}
					wantLoc := validationTestTextLoc(t, input)
					if test.selfClosing {
						wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
					}
					if diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != validationTokenEnumerationSpecRef(policy.version) || diagnostic.Message() != "value is not in the string enumeration" || diagnostic.Unwrap() == nil {
						t.Fatalf("diagnostic = %v, loc=%s, spec=%q, cause=%v", first, diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Unwrap())
					}
					declaration := validationStringElement(t, schema, test.element)
					var definition goxsd9.SimpleTypeDefinition
					wantRelated := []goxsd9.Loc{declaration.Loc()}
					if test.typeName == "" {
						var ok bool
						definition, ok = declaration.InlineSimpleType()
						if !ok {
							t.Fatal("anonymous string type is missing")
						}
						wantRelated = append(wantRelated, definition.Loc())
					}
					if test.typeName != "" {
						component := validationTokenSimpleType(t, schema, test.typeNamespace, test.typeName)
						var ok bool
						definition, ok = component.SimpleTypeDefinition()
						if !ok {
							t.Fatal("named string type is missing")
						}
						wantRelated = append(wantRelated, component.Loc())
					}
					whiteSpace, ok := definition.StringWhiteSpaceFacet()
					if !ok {
						t.Fatal("effective string whiteSpace facet is missing")
					}
					if !whiteSpace.Loc().IsZero() && !validationTestHasRelated(wantRelated, whiteSpace.Loc()) {
						wantRelated = append(wantRelated, whiteSpace.Loc())
					}
					for _, loc := range definition.StringEnumerationFacets().Locations() {
						if !validationTestHasRelated(wantRelated, loc) {
							wantRelated = append(wantRelated, loc)
						}
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
					}
					second := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if second.Error() != diagnostic.Error() || !reflect.DeepEqual(second.Related(), diagnostic.Related()) {
						t.Fatalf("repeated diagnostics differ: first %v, second %v", diagnostic, second)
					}
				})
			}
		})
	}
}

func validationStringSchema(t *testing.T, policy validationTokenPolicyCase) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationStringNamespace + `" xmlns:o="` + validationStringOtherNamespace + `" targetNamespace="` + validationStringNamespace + `" version="` + string(policy.version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:import namespace="` + validationStringOtherNamespace + `" schemaLocation="other.xsd"/>
  <xs:element name="direct" type="xs:string"/>
  <xs:element name="preserve" type="r:Preserve"/>
  <xs:element name="preserveWhite" type="r:PreserveWhite"/>
  <xs:element name="replace" type="r:Replace"/>
  <xs:element name="replaceSpace" type="r:ReplaceSpace"/>
  <xs:element name="collapse" type="r:Collapse"/>
  <xs:element name="empty" type="r:Empty"/>
  <xs:element name="forward" type="r:Forward"/>
  <xs:element name="inherited" type="r:Inherited"/>
  <xs:element name="included" type="r:Included"/>
  <xs:element name="imported" type="o:Imported"/>
  <xs:element name="inline"><xs:simpleType><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType></xs:element>
  <xs:simpleType name="Preserve"><xs:restriction base="xs:string"><xs:enumeration value="a  b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="PreserveWhite"><xs:restriction base="xs:string"><xs:enumeration value=" &#9; "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Replace"><xs:restriction base="xs:string"><xs:whiteSpace value="replace"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ReplaceSpace"><xs:restriction base="xs:string"><xs:whiteSpace value="replace"/><xs:enumeration value=" "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Collapse"><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Empty"><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"/><xs:enumeration value=""/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:string"><xs:enumeration value="été&#160;雪"/></xs:restriction></xs:simpleType>
</xs:schema>`
	fixtures := map[string]validationTestFixture{
		"included.xsd": {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"/><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`},
		"other.xsd":    {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationStringOtherNamespace + `" targetNamespace="` + validationStringOtherNamespace + `" version="` + string(policy.version) + `"><xs:simpleType name="Imported"><xs:restriction base="xs:string"><xs:enumeration value=" other "/></xs:restriction></xs:simpleType></xs:schema>`},
	}
	return validationTestSchemaWithPolicy(t, root, fixtures, policy.policy)
}

func validationStringInstance(element, value string, selfClosing bool) string {
	if selfClosing {
		return `<` + element + ` xmlns="` + validationStringNamespace + `"/>`
	}
	return `<` + element + ` xmlns="` + validationStringNamespace + `">` + value + `</` + element + `>`
}

func validationStringElement(t *testing.T, schema goxsd9.Schema, local string) goxsd9.ElementDeclaration {
	t.Helper()
	name, err := goxsd9.NewQName(validationStringNamespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(components) != 1 {
		t.Fatalf("element %s has %d declarations", local, len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("element %s has no declaration view", local)
	}
	return declaration
}

func TestValidateInstanceKeepsStringOutsideGlobalTextOnlyBoundaryUnsupported(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationStringNamespace + `" targetNamespace="` + validationStringNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="item" type="xs:string"/>
  <xs:element name="box" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:complexType>
</xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, nil, policy.policy)
			for _, input := range []string{
				`<item xmlns="` + validationStringNamespace + `" extra="x">value</item>`,
				`<item xmlns="` + validationStringNamespace + `"><child/></item>`,
				`<box xmlns="` + validationStringNamespace + `"><item>value</item></box>`,
			} {
				diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
				if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Loc().IsZero() {
					t.Fatalf("%q: diagnostic = %v, want located unsupported", input, diagnostic)
				}
			}
		})
	}
}

func TestValidateInstanceUsesEffectiveStringFacetEditionInMixedGraph(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:o="` + validationStringOtherNamespace + `" targetNamespace="` + validationStringNamespace + `" version="1.1">
  <xs:import namespace="` + validationStringOtherNamespace + `" schemaLocation="other.xsd"/>
  <xs:element name="item" type="o:Imported"/>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="` + validationStringOtherNamespace + `" version="1.0">
  <xs:simpleType name="Imported"><xs:restriction base="xs:string"><xs:enumeration value="valid"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchemaWithPolicy(t, root, map[string]validationTestFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, goxsd9.Compatibility)
	input := validationStringInstance("item", "invalid", false)
	diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.EnumerationValueViolationCode || diagnostic.SpecRef() != validationTokenEnumerationSpecRef(goxsd9.XSDVersion11) {
		t.Fatalf("mixed graph diagnostic = %v, spec=%q, want XSD 1.1 enumeration violation", diagnostic, diagnostic.SpecRef())
	}
}

//nolint:gocognit // Keep policy, inherited values, local declarations, and diagnostics in one matrix.
func TestValidateInstancePreservesInheritedStringEnumerationValues(t *testing.T) {
	for _, policy := range validationTokenPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationStringNamespace + `" targetNamespace="` + validationStringNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="baseDouble" type="r:BaseDouble"/>
  <xs:element name="collapsedInherited" type="r:CollapsedInherited"/>
  <xs:element name="collapsedLocal" type="r:CollapsedLocal"/>
  <xs:element name="baseTab" type="r:BaseTab"/>
  <xs:element name="replacedInherited" type="r:ReplacedInherited"/>
  <xs:element name="replaceBase" type="r:ReplaceBase"/>
  <xs:element name="replaceLocal" type="r:ReplaceLocal"/>
  <xs:simpleType name="BaseDouble"><xs:restriction base="xs:string"><xs:enumeration value="a  b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="CollapsedInherited"><xs:restriction base="r:BaseDouble"><xs:whiteSpace value="collapse"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="CollapsedLocal"><xs:restriction base="r:BaseDouble"><xs:whiteSpace value="collapse"/><xs:enumeration value="a  b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="BaseTab"><xs:restriction base="xs:string"><xs:enumeration value="a&#9;b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ReplacedInherited"><xs:restriction base="r:BaseTab"><xs:whiteSpace value="replace"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ReplaceBase"><xs:restriction base="xs:string"><xs:whiteSpace value="replace"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ReplaceLocal"><xs:restriction base="r:ReplaceBase"><xs:enumeration value="a&#9;b"/></xs:restriction></xs:simpleType>
</xs:schema>`
			schema := validationTestSchemaWithPolicy(t, root, nil, policy.policy)
			before := schema.Components()
			for _, test := range []struct {
				name, element, value string
				valid                bool
			}{
				{name: "preserve base double", element: "baseDouble", value: "a  b", valid: true},
				{name: "preserve base tab", element: "baseTab", value: "a\tb", valid: true},
				{name: "collapse inherited must not rewrite double space", element: "collapsedInherited", value: "a b"},
				{name: "collapse local must use base preserve", element: "collapsedLocal", value: "a  b"},
				{name: "replace inherited must not rewrite tab", element: "replacedInherited", value: "a\tb"},
				{name: "replace base normalizes its member", element: "replaceBase", value: "a\tb", valid: true},
				{name: "replace local uses base replace", element: "replaceLocal", value: "a\tb", valid: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := validationStringInstance(test.element, test.value, false)
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					if test.valid {
						if err != nil {
							t.Fatalf("ValidateInstance(%q): %v", input, err)
						}
						return
					}
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.EnumerationValueViolationCode || diagnostic.Loc() != validationTestTextLoc(t, input) || diagnostic.SpecRef() != validationTokenEnumerationSpecRef(policy.version) || diagnostic.Unwrap() == nil {
						t.Fatalf("ValidateInstance(%q) = %v, want located enumeration violation", input, err)
					}
				})
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("string validation mutated the schema")
			}
			definition, ok := validationTokenSimpleType(t, schema, validationStringNamespace, "ReplaceLocal").SimpleTypeDefinition()
			if !ok || !reflect.DeepEqual(definition.StringEnumerationFacets().Values(), []string{"a\tb"}) {
				t.Fatalf("ReplaceLocal lexical enumeration changed: %#v", definition.StringEnumerationFacets().Values())
			}
		})
	}
}
