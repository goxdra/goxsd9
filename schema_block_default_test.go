package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Keep the policy, projection, location, and order matrix together.
func TestSchemaBlockDefaultBuildsCanonicalFacts(t *testing.T) {
	policies := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.1"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
	cases := []struct {
		name    string
		present bool
		value   string
		element []string
		complex []string
	}{
		{name: "omitted"},
		{name: "empty", present: true},
		{name: "whitespace", present: true, value: " \t\n "},
		{name: "extension", present: true, value: "extension", element: []string{"extension"}, complex: []string{"extension"}},
		{name: "restriction", present: true, value: "restriction", element: []string{"restriction"}, complex: []string{"restriction"}},
		{name: "substitution", present: true, value: "substitution", element: []string{"substitution"}},
		{
			name:    "collapsed duplicate list",
			present: true,
			value:   " \t restriction\n extension restriction ",
			element: []string{"extension", "restriction"},
			complex: []string{"extension", "restriction"},
		},
		{
			name:    "all",
			present: true,
			value:   "#all",
			element: []string{"extension", "restriction", "substitution"},
			complex: []string{"extension", "restriction"},
		},
	}
	for _, policy := range policies {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := schemaBlockDefaultTestRoot(test.present, test.value, policy.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}
				components := schema.Components()
				if got, want := len(components), 2; got != want {
					t.Fatalf("component count = %d, want %d", got, want)
				}
				element, ok := components[0].ElementDeclaration()
				if !ok {
					t.Fatal("global element view is missing")
				}
				complexType, ok := components[1].ComplexTypeDefinition()
				if !ok {
					t.Fatal("complex type view is missing")
				}
				if got := element.DisallowedSubstitutions(); !reflect.DeepEqual(got, test.element) {
					t.Fatalf("element disallowed substitutions = %#v, want %#v", got, test.element)
				}
				if got := complexType.ProhibitedSubstitutions(); !reflect.DeepEqual(got, test.complex) {
					t.Fatalf("complex type prohibited substitutions = %#v, want %#v", got, test.complex)
				}
				wantLoc := Loc{}
				if test.present {
					wantLoc = schemaBlockTestAttributeLoc(t, "root.xsd", root, "blockDefault")
				}
				if got := element.DisallowedSubstitutionsLoc(); got != wantLoc {
					t.Fatalf("element block fact location = %s, want %s", got, wantLoc)
				}
				if got := complexType.ProhibitedSubstitutionsLoc(); got != wantLoc {
					t.Fatalf("complex type block fact location = %s, want %s", got, wantLoc)
				}
				if components[0].ID().Ordinal() != 1 || components[1].ID().Ordinal() != 2 {
					t.Fatalf("component ordinals = %d/%d, want 1/2", components[0].ID().Ordinal(), components[1].ID().Ordinal())
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Keep explicit override and immutable query assertions together.
func TestSchemaBlockExplicitValuesOverrideDocumentDefault(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="#all">
  <xs:element name="inherited" type="xs:integer"/>
  <xs:element name="emptyElement" type="xs:integer" block=""/>
  <xs:element name="substitutionElement" type="xs:integer" block="substitution"/>
  <xs:complexType name="emptyType" block=""><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:complexType>
  <xs:complexType name="extensionType" block="extension"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 5; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	for index, component := range components {
		if component.ID().Ordinal() != uint64(index+1) || component.Document() != "root.xsd" {
			t.Fatalf("component %d identity = %s/%d, want root.xsd/%d", index, component.Document(), component.ID().Ordinal(), index+1)
		}
	}

	inherited, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatal("inherited element view is missing")
	}
	if got, want := inherited.DisallowedSubstitutions(), []string{"extension", "restriction", "substitution"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inherited block = %#v, want %#v", got, want)
	}
	rootLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, "blockDefault")
	if got := inherited.DisallowedSubstitutionsLoc(); got != rootLoc {
		t.Fatalf("inherited block location = %s, want %s", got, rootLoc)
	}

	emptyElement, ok := components[1].ElementDeclaration()
	if !ok {
		t.Fatal("empty element view is missing")
	}
	if got := emptyElement.DisallowedSubstitutions(); got != nil {
		t.Fatalf("empty explicit block = %#v, want nil", got)
	}
	emptyElementLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, `block=""`)
	if got := emptyElement.DisallowedSubstitutionsLoc(); got != emptyElementLoc {
		t.Fatalf("empty explicit block location = %s, want %s", got, emptyElementLoc)
	}

	substitutionElement, ok := components[2].ElementDeclaration()
	if !ok {
		t.Fatal("substitution element view is missing")
	}
	if got, want := substitutionElement.DisallowedSubstitutions(), []string{"substitution"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit substitution block = %#v, want %#v", got, want)
	}
	substitutionElementLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, `block="substitution"`)
	if got := substitutionElement.DisallowedSubstitutionsLoc(); got != substitutionElementLoc {
		t.Fatalf("explicit substitution block location = %s, want %s", got, substitutionElementLoc)
	}

	emptyType, ok := components[3].ComplexTypeDefinition()
	if !ok {
		t.Fatal("empty complex type view is missing")
	}
	if got := emptyType.ProhibitedSubstitutions(); got != nil {
		t.Fatalf("empty complex block = %#v, want nil", got)
	}
	firstEmptyBlock := strings.Index(root, `block=""`)
	emptyTypeLoc := schemaBlockTestAttributeLocAfter(t, "root.xsd", root, `block=""`, firstEmptyBlock+len(`block=""`))
	if got := emptyType.ProhibitedSubstitutionsLoc(); got != emptyTypeLoc {
		t.Fatalf("empty complex block location = %s, want %s", got, emptyTypeLoc)
	}

	extensionType, ok := components[4].ComplexTypeDefinition()
	if !ok {
		t.Fatal("extension complex type view is missing")
	}
	if got, want := extensionType.ProhibitedSubstitutions(), []string{"extension"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit extension complex block = %#v, want %#v", got, want)
	}
	extensionTypeLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, `block="extension"`)
	if got := extensionType.ProhibitedSubstitutionsLoc(); got != extensionTypeLoc {
		t.Fatalf("explicit extension complex block location = %s, want %s", got, extensionTypeLoc)
	}

	values := inherited.DisallowedSubstitutions()
	values[0] = "mutated"
	if got, want := inherited.DisallowedSubstitutions(), []string{"extension", "restriction", "substitution"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mutating returned block values changed component = %#v, want %#v", got, want)
	}
	documentComponents := schema.Documents()[0].Components()
	documentComponents[0] = Component{}
	if schema.Components()[0].Name() != mustTestQName(t, "", "inherited") {
		t.Fatal("mutating document component copy changed schema order or identity")
	}
}

//nolint:gocognit // Keep invalid lexical values and diagnostic metadata together.
func TestSchemaBlockDefaultRejectsInvalidLexicalValues(t *testing.T) {
	policies := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	cases := []struct {
		name   string
		root   string
		needle string
		scope  schemaBlockPolicyScope
	}{
		{name: "unknown root token", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="unknown"/>`, needle: "blockDefault", scope: schemaBlockDocumentDefault},
		{name: "wrong case root token", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="Extension"/>`, needle: "blockDefault", scope: schemaBlockDocumentDefault},
		{name: "all mixed with token root", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="#all extension"/>`, needle: "blockDefault", scope: schemaBlockDocumentDefault},
		{name: "all repeated root", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="#all #all"/>`, needle: "blockDefault", scope: schemaBlockDocumentDefault},
		{
			name:   "all mixed with token element",
			root:   `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" block="#all extension"/></xs:schema>`,
			needle: `block="#all extension"`,
			scope:  schemaBlockElement,
		},
		{
			name:   "wrong case element token",
			root:   `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" block="Restriction"/></xs:schema>`,
			needle: `block="Restriction"`,
			scope:  schemaBlockElement,
		},
		{
			name:   "substitution complex token",
			root:   `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item" block="substitution"><xs:choice><xs:element name="member" type="xs:integer"/></xs:choice></xs:complexType></xs:schema>`,
			needle: `block="substitution"`,
			scope:  schemaBlockComplex,
		},
	}
	for _, policy := range policies {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted invalid block syntax")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaBlockCode {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaBlockCode)
				}
				if diagnostic.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", test.root, test.needle) {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), schemaBlockTestAttributeLoc(t, "root.xsd", test.root, test.needle))
				}
				if diagnostic.SpecRef() != schemaBlockSpecRef(policy.version, test.scope) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaBlockSpecRef(policy.version, test.scope))
				}
				if diagnostic.Unwrap() == nil || !errors.Is(err, errSchemaBlock) {
					t.Fatalf("diagnostic does not preserve block cause: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Keep include provenance, ownership, and ordinal assertions together.
func TestSchemaBlockDefaultStaysWithIncludedDocument(t *testing.T) {
	for _, test := range []struct {
		name           string
		childNamespace string
	}{
		{name: "ordinary include", childNamespace: ` targetNamespace="urn:root"`},
		{name: "chameleon include", childNamespace: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" blockDefault="extension">
  <xs:include schemaLocation="child.xsd"/>
  <xs:element name="rootItem" type="xs:integer"/>
</xs:schema>`
			child := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + test.childNamespace + ` blockDefault="restriction">
  <xs:element name="childItem" type="xs:integer"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: child},
			}, Strict11)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			documents := schema.Documents()
			if got, want := len(documents), 2; got != want {
				t.Fatalf("document count = %d, want %d", got, want)
			}
			if documents[0].Source() != "root.xsd" || documents[1].Source() != "child.xsd" {
				t.Fatalf("document order = %q/%q, want root.xsd/child.xsd", documents[0].Source(), documents[1].Source())
			}
			components := schema.Components()
			if got, want := len(components), 2; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			rootElement, ok := components[0].ElementDeclaration()
			if !ok {
				t.Fatal("root element view is missing")
			}
			childElement, ok := components[1].ElementDeclaration()
			if !ok {
				t.Fatal("child element view is missing")
			}
			if components[0].Document() != "root.xsd" || components[1].Document() != "child.xsd" {
				t.Fatalf("component sources = %q/%q, want root.xsd/child.xsd", components[0].Document(), components[1].Document())
			}
			if got, want := rootElement.DisallowedSubstitutions(), []string{"extension"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("root policy = %#v, want %#v", got, want)
			}
			if got, want := childElement.DisallowedSubstitutions(), []string{"restriction"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("included policy = %#v, want %#v", got, want)
			}
			rootLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, "blockDefault")
			childLoc := schemaBlockTestAttributeLoc(t, "child.xsd", child, "blockDefault")
			if rootElement.DisallowedSubstitutionsLoc() != rootLoc || childElement.DisallowedSubstitutionsLoc() != childLoc {
				t.Fatalf("block locations = %s/%s, want %s/%s", rootElement.DisallowedSubstitutionsLoc(), childElement.DisallowedSubstitutionsLoc(), rootLoc, childLoc)
			}
			if childElement.Name() != mustTestQName(t, "urn:root", "childItem") {
				t.Fatalf("included element name = %q, want {urn:root}childItem", childElement.Name())
			}
			if components[0].ID().Ordinal() != 1 || components[1].ID().Ordinal() != 1 {
				t.Fatalf("component ordinals = %d/%d, want 1/1 per source", components[0].ID().Ordinal(), components[1].ID().Ordinal())
			}
		})
	}
}

func schemaBlockDefaultTestRoot(present bool, value, version string) string {
	attribute := ""
	if present {
		attribute = ` blockDefault="` + value + `"`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + version + `"` + attribute + `>
  <xs:element name="item" type="xs:integer"/>
  <xs:complexType name="record"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice></xs:complexType>
</xs:schema>`
}

func schemaBlockTestAttributeLoc(t *testing.T, source SourceID, input, needle string) Loc {
	return schemaBlockTestAttributeLocAfter(t, source, input, needle, 0)
}

func schemaBlockTestAttributeLocAfter(t *testing.T, source SourceID, input, needle string, start int) Loc {
	t.Helper()
	if start < 0 || start > len(input) {
		t.Fatalf("invalid search start %d for %q", start, needle)
	}
	relativeOffset := strings.Index(input[start:], needle)
	if relativeOffset < 0 {
		t.Fatalf("input does not contain %q", needle)
	}
	offset := start + relativeOffset
	line := 1
	column := 1
	for _, character := range input[:offset] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, source, line, column)
}
