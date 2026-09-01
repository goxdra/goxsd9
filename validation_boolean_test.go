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
	validationBooleanNamespace      = "urn:boolean-root"
	validationBooleanOtherNamespace = "urn:boolean-other"
)

type validationBooleanPolicyCase struct {
	name    string
	policy  goxsd9.LanguagePolicy
	version goxsd9.XSDVersion
	specRef string
}

type validationBooleanElementCase struct {
	name     string
	typeName string
}

func validationBooleanPolicies() []validationBooleanPolicyCase {
	return []validationBooleanPolicyCase{
		{name: "Compatibility", policy: goxsd9.Compatibility, version: goxsd9.XSDVersion11, specRef: "xsd11-datatypes#boolean-lexical-mapping"},
		{name: "Strict10", policy: goxsd9.Strict10, version: goxsd9.XSDVersion10, specRef: "xsd10-datatypes#boolean-lexical-representation"},
		{name: "Strict11", policy: goxsd9.Strict11, version: goxsd9.XSDVersion11, specRef: "xsd11-datatypes#boolean-lexical-mapping"},
	}
}

func validationBooleanElements() []validationBooleanElementCase {
	return []validationBooleanElementCase{
		{name: "direct", typeName: ""},
		{name: "named", typeName: "Named"},
		{name: "cross", typeName: "Cross"},
	}
}

func validationBooleanSchema(t *testing.T, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="` + validationBooleanNamespace + `" xmlns:o="` + validationBooleanOtherNamespace + `" targetNamespace="` + validationBooleanNamespace + `" version="1.1">
  <xs:import namespace="` + validationBooleanOtherNamespace + `" schemaLocation="boolean-other.xsd"/>
  <xs:element name="direct" type="xs:boolean"/>
  <xs:element name="named" type="r:Named"/>
  <xs:element name="cross" type="o:Cross"/>
  <xs:simpleType name="Named"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="` + validationBooleanOtherNamespace + `" version="1.0">
  <xs:simpleType name="Cross"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
	return validationTestSchemaWithPolicy(t, root, map[string]validationTestFixture{
		"boolean-other.xsd": {id: "boolean-other.xsd", contents: other},
	}, policy)
}

func validationBooleanInstance(element, value string) string {
	return `<` + element + ` xmlns="` + validationBooleanNamespace + `">` + value + `</` + element + `>`
}

func validationBooleanQName(t *testing.T, namespace, local string) goxsd9.QName {
	t.Helper()
	name, err := goxsd9.NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	return name
}

func validationBooleanRelated(t *testing.T, schema goxsd9.Schema, element validationBooleanElementCase) []goxsd9.Loc {
	t.Helper()
	declarations := schema.FindKind(
		goxsd9.ComponentKindElementDeclaration,
		validationBooleanQName(t, validationBooleanNamespace, element.name),
	)
	if len(declarations) != 1 {
		t.Fatalf("%s declarations = %d, want 1", element.name, len(declarations))
	}
	related := make([]goxsd9.Loc, 0, 2)
	related = append(related, declarations[0].Loc())
	if element.typeName == "" {
		return related
	}
	types := schema.FindKind(
		goxsd9.ComponentKindSimpleTypeDefinition,
		validationBooleanQName(t, validationBooleanNamespace, element.typeName),
	)
	if element.name == "cross" {
		types = schema.FindKind(
			goxsd9.ComponentKindSimpleTypeDefinition,
			validationBooleanQName(t, validationBooleanOtherNamespace, element.typeName),
		)
	}
	if len(types) != 1 {
		t.Fatalf("%s types = %d, want 1", element.typeName, len(types))
	}
	return append(related, types[0].Loc())
}

func validationBooleanElementDeclaration(t *testing.T, schema goxsd9.Schema, element validationBooleanElementCase) goxsd9.ElementDeclaration {
	t.Helper()
	declarations := schema.FindKind(
		goxsd9.ComponentKindElementDeclaration,
		validationBooleanQName(t, validationBooleanNamespace, element.name),
	)
	if len(declarations) != 1 {
		t.Fatalf("%s declarations = %d, want 1", element.name, len(declarations))
	}
	declaration, ok := declarations[0].ElementDeclaration()
	if !ok {
		t.Fatalf("%s has no element declaration view", element.name)
	}
	return declaration
}

//nolint:gocognit // Keep policy, lexical, element, and whitespace coverage together.
func TestValidateInstanceSupportsGlobalBooleanScalarsAcrossPolicies(t *testing.T) {
	for _, policy := range validationBooleanPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationBooleanSchema(t, policy.policy)
			before := schema.Components()
			for _, element := range validationBooleanElements() {
				for _, lexical := range []string{"true", "false", "1", "0"} {
					for _, test := range []struct {
						name  string
						value string
					}{
						{name: "literal", value: lexical},
						{name: "collapsed whitespace", value: "\t \n" + lexical + "\r\n "},
					} {
						t.Run(element.name+"/"+lexical+"/"+test.name, func(t *testing.T) {
							input := validationBooleanInstance(element.name, test.value)
							if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
								t.Fatalf("ValidateInstance(%q): %v", input, err)
							}
						})
					}
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("boolean validation mutated the completed schema")
			}
		})
	}
}

//nolint:gocognit // Keep direct, forward, and cross-document identity checks together.
func TestValidateInstanceUsesCompletedBooleanTypeIDs(t *testing.T) {
	for _, policy := range validationBooleanPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationBooleanSchema(t, policy.policy)
			for _, element := range validationBooleanElements() {
				declaration := validationBooleanElementDeclaration(t, schema, element)
				typeID, hasTypeID := declaration.TypeID()
				if element.typeName == "" {
					if hasTypeID || !typeID.IsZero() {
						t.Fatalf("%s type ID = %v/%t, want zero,false for built-in boolean", element.name, typeID, hasTypeID)
					}
					continue
				}
				if !hasTypeID || typeID.IsZero() {
					t.Fatalf("%s type ID = %v/%t, want a completed named type identity", element.name, typeID, hasTypeID)
				}
				target, ok := schema.Lookup(typeID)
				if !ok {
					t.Fatalf("%s type ID %v is not in the completed schema", element.name, typeID)
				}
				if target.Kind() != goxsd9.ComponentKindSimpleTypeDefinition {
					t.Fatalf("%s target kind = %q, want simple type definition", element.name, target.Kind())
				}
				definition, ok := target.SimpleTypeDefinition()
				if !ok || !definition.IsBoolean() {
					t.Fatalf("%s target does not expose completed boolean facts", element.name)
				}
				if element.name == "cross" && typeID.Source() != "boolean-other.xsd" {
					t.Fatalf("cross type ID source = %q, want boolean-other.xsd", typeID.Source())
				}
			}
		})
	}
}

//nolint:gocognit // Keep lexical, location, provenance, and repeatability assertions together.
func TestValidateInstanceReportsGlobalBooleanLexicalDiagnostics(t *testing.T) {
	invalid := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "XML whitespace only", value: " \t\r\n"},
		{name: "uppercase", value: "True"},
		{name: "trailing data", value: "truex"},
		{name: "two tokens", value: "true false"},
		{name: "numeric near miss", value: "2"},
		{name: "signed value", value: "+1"},
		{name: "internal whitespace", value: "tr\tue"},
		{name: "Unicode whitespace", value: "\u00a0true\u00a0"},
	}
	for _, policy := range validationBooleanPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			schema := validationBooleanSchema(t, policy.policy)
			before := schema.Components()
			for _, element := range validationBooleanElements() {
				wantRelated := validationBooleanRelated(t, schema, element)
				for _, test := range invalid {
					t.Run(element.name+"/"+test.name, func(t *testing.T) {
						input := validationBooleanInstance(element.name, test.value)
						err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
						diagnostic := validationTestDiagnostic(t, err)
						if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.InvalidBooleanLexicalCode {
							t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), goxsd9.InvalidBooleanLexicalCode)
						}
						wantLoc := validationTestTextLoc(t, input)
						if test.value == "" {
							wantLoc = validationTestLoc(t, "instance.xml", 1, 1)
						}
						if diagnostic.Loc() != wantLoc {
							t.Fatalf("Loc() = %s, want %s", diagnostic.Loc(), wantLoc)
						}
						if diagnostic.Related() == nil || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
							t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
						}
						if diagnostic.SpecRef() != policy.specRef {
							t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), policy.specRef)
						}
						if diagnostic.Message() != "invalid xs:boolean lexical representation" {
							t.Fatalf("Message() = %q, want datatype parser message", diagnostic.Message())
						}
						if diagnostic.Unwrap() != nil {
							t.Fatalf("invalid lexical diagnostic has a cause %v, want parser cause", diagnostic.Unwrap())
						}
						if errors.Is(err, goxsd9.ErrUnsupported) {
							t.Fatal("invalid boolean lexical diagnostic was classified as unsupported")
						}
					})
				}
			}

			repeatInput := validationBooleanInstance("named", "truex")
			firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(repeatInput)))
			secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(repeatInput)))
			first := validationTestDiagnostic(t, firstErr)
			second := validationTestDiagnostic(t, secondErr)
			if first.Code() != second.Code() || first.Loc() != second.Loc() || first.SpecRef() != second.SpecRef() || first.Error() != second.Error() || !reflect.DeepEqual(first.Related(), second.Related()) {
				t.Fatalf("repeated boolean diagnostics differ: first %v, second %v", firstErr, secondErr)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("boolean lexical validation mutated the completed schema")
			}
		})
	}
}
