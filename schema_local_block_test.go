package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Keep the edition, policy, location, and order matrix together.
func TestSchemaLocalElementBlockFactsAcrossPolicies(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaLocalBlockTestRoot(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			rootLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, "blockDefault")

			choice := schemaLocalBlockTestComplexType(t, schema, "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			choiceWant := []schemaLocalBlockExpectation{
				{name: "choiceAbsent", values: []string{"restriction"}, marker: ""},
				{name: "choiceEmpty", marker: `block="  "`},
				{name: "choiceExtension", values: []string{"extension"}, marker: `block="extension"`},
				{name: "choiceRepeated", values: []string{"extension", "restriction"}, marker: `block="restriction extension restriction"`},
				{name: "choiceCombined", values: []string{"restriction", "substitution"}, marker: `block="substitution restriction"`},
				{name: "choiceAll", values: []string{"extension", "restriction", "substitution"}, marker: `block="#all"`},
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), len(choiceWant); got != want {
				t.Fatalf("choice alternative count = %d, want %d", got, want)
			}
			for index, want := range choiceWant {
				assertSchemaLocalBlockParticle(t, choiceAlternatives[index], want, root, rootLoc)
			}

			sequence := schemaLocalBlockTestComplexType(t, schema, "Sequence")
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			sequenceWant := []schemaLocalBlockExpectation{
				{name: "sequenceAbsent", values: []string{"restriction"}, marker: ""},
				{name: "sequenceEmpty", marker: `block=" "`},
				{name: "sequenceSubstitution", values: []string{"substitution"}, marker: `block="substitution"`},
				{name: "sequenceCombined", values: []string{"extension", "restriction"}, marker: `block="extension restriction"`},
				{name: "sequenceAll", values: []string{"extension", "restriction", "substitution"}, marker: `block=" #all "`},
			}
			sequenceElements := sequenceParticle.Elements()
			if got, want := len(sequenceElements), len(sequenceWant); got != want {
				t.Fatalf("sequence element count = %d, want %d", got, want)
			}
			for index, want := range sequenceWant {
				assertSchemaLocalBlockParticle(t, sequenceElements[index], want, root, rootLoc)
			}

			choiceElement, ok := choiceAlternatives[0].(ElementParticle)
			if !ok {
				t.Fatalf("choice alternative = %T, want ElementParticle", choiceAlternatives[0])
			}
			values := choiceElement.DisallowedSubstitutions()
			values[0] = "mutated"
			if got, want := choiceElement.DisallowedSubstitutions(), []string{"restriction"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("mutating local block values changed facts = %#v, want %#v", got, want)
			}
			if got := (ElementParticle{}).DisallowedSubstitutions(); got != nil {
				t.Fatalf("zero ElementParticle block values = %#v, want nil", got)
			}
			if got := (ElementParticle{}).DisallowedSubstitutionsLoc(); !got.IsZero() {
				t.Fatalf("zero ElementParticle block location = %s, want zero", got)
			}
		})
	}
}

type schemaLocalBlockExpectation struct {
	name   string
	values []string
	marker string
}

func assertSchemaLocalBlockParticle(t *testing.T, particle Particle, want schemaLocalBlockExpectation, root string, rootLoc Loc) {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("%s particle = %T, want ElementParticle", want.name, particle)
	}
	if got := element.Name().Local(); got != want.name {
		t.Fatalf("local particle name = %q, want %q", got, want.name)
	}
	if got := element.DisallowedSubstitutions(); !reflect.DeepEqual(got, want.values) {
		t.Fatalf("%s block values = %#v, want %#v", want.name, got, want.values)
	}
	wantLoc := rootLoc
	if want.marker != "" {
		wantLoc = schemaBlockTestAttributeLoc(t, "root.xsd", root, want.marker)
	}
	if got := element.DisallowedSubstitutionsLoc(); got != wantLoc {
		t.Fatalf("%s block location = %s, want %s", want.name, got, wantLoc)
	}
}

func schemaLocalBlockTestRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" blockDefault="restriction" version="` + string(version) + `">
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="choiceAbsent" type="xs:integer"/>
    <xs:element name="choiceEmpty" type="xs:boolean" block="  "/>
    <xs:element name="choiceExtension" type="xs:integer" block="extension"/>
    <xs:element name="choiceRepeated" type="xs:boolean" block="restriction extension restriction"/>
    <xs:element name="choiceCombined" type="xs:integer" block="substitution restriction"/>
    <xs:element name="choiceAll" type="xs:boolean" block="#all"/>
  </xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence>
    <xs:element name="sequenceAbsent" type="xs:integer"/>
    <xs:element name="sequenceEmpty" type="xs:boolean" block=" "/>
    <xs:element name="sequenceSubstitution" type="xs:integer" block="substitution"/>
    <xs:element name="sequenceCombined" type="xs:boolean" block="extension restriction"/>
    <xs:element name="sequenceAll" type="xs:integer" block=" #all "/>
  </xs:sequence></xs:complexType>
</xs:schema>`
}

func schemaLocalBlockTestComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	return schemaLocalBlockTestComplexTypeInNamespace(t, schema, "", local)
}

func schemaLocalBlockTestComplexTypeInNamespace(t *testing.T, schema Schema, namespace, local string) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("%s complex type count = %d, want 1", local, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s complex type view is missing", local)
	}
	return definition
}

//nolint:gocognit // Keep ordinary and chameleon provenance assertions together.
func TestSchemaLocalElementBlockUsesIncludedDocumentDefaults(t *testing.T) {
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
</xs:schema>`
			child := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + test.childNamespace + ` blockDefault="restriction">
  <xs:complexType name="Child"><xs:sequence><xs:element name="value" type="xs:boolean"/></xs:sequence></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: child},
			}, Strict11)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Child"))
			if len(components) != 1 {
				t.Fatalf("included Child count = %d, want 1", len(components))
			}
			definition, ok := components[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("included complex type view is missing")
			}
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("included particle = %T, want SequenceParticle", definition.Particle())
			}
			elements := sequence.Elements()
			if len(elements) != 1 {
				t.Fatalf("included local element count = %d, want 1", len(elements))
			}
			local := elements[0]
			if got, want := local.DisallowedSubstitutions(), []string{"restriction"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("included local block = %#v, want %#v", got, want)
			}
			wantLoc := schemaBlockTestAttributeLoc(t, "child.xsd", child, "blockDefault")
			if got := local.DisallowedSubstitutionsLoc(); got != wantLoc {
				t.Fatalf("included local block location = %s, want %s", got, wantLoc)
			}
			if got := local.Name(); got != mustTestQName(t, "", "value") {
				t.Fatalf("chameleon/local name = %q, want unqualified value", got)
			}
		})
	}
}

//nolint:gocognit // Keep reference and unsupported-shape boundary assertions together.
func TestSchemaLocalElementBlockReferenceAndUnsupportedBoundaries(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name+"/reference", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" blockDefault="#all" version="` + string(profile.version) + `">
  <xs:element name="target" type="xs:integer" block="#all"/>
  <xs:complexType name="Choice"><xs:choice><xs:element ref="r:target"/></xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence><xs:element ref="r:target"/></xs:sequence></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			choice := schemaLocalBlockTestComplexTypeInNamespace(t, schema, "urn:root", "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			if _, referenceOK := choiceParticle.Alternatives()[0].(ElementReferenceParticle); !referenceOK {
				t.Fatalf("choice reference particle = %T, want ElementReferenceParticle", choiceParticle.Alternatives()[0])
			}
			sequence := schemaLocalBlockTestComplexTypeInNamespace(t, schema, "urn:root", "Sequence")
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			if _, referenceOK := sequenceParticle.Particles()[0].(ElementReferenceParticle); !referenceOK {
				t.Fatalf("sequence reference particle = %T, want ElementReferenceParticle", sequenceParticle.Particles()[0])
			}
		})

		t.Run(profile.name+"/reference block", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="target" type="xs:integer"/><xs:complexType name="Record"><xs:choice><xs:element ref="r:target" block="extension"/></xs:choice></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertSchemaLocalBlockError(t, schema, err, root, diagnosticSchemaElementReferenceBlockCode, schemaElementReferenceBlockSpecRef(profile.version), errSchemaElementReferenceBlock, `block="extension"`, "local element ref cannot combine with \"block\"")
		})

		t.Run(profile.name+"/named group unsupported", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:group name="Group"><xs:choice><xs:element name="value" type="xs:integer" block="extension"/></xs:choice></xs:group></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("discoverSchema accepted a valid block policy on an unsupported named-group shape")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("unsupported named-group shape returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
				t.Fatalf("diagnostic = %s, want unsupported/%s", diagnostic, UnsupportedSchemaSyntaxCode)
			}
			if diagnostic.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, `block="extension"`) {
				t.Fatalf("diagnostic location = %s, want block attribute", diagnostic.Loc())
			}
		})
	}
}

//nolint:gocognit // Keep malformed-policy cases and diagnostic assertions together.
func TestSchemaLocalElementBlockRejectsMalformedValuesWithoutSchema(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	for _, profile := range profiles {
		for _, test := range []struct {
			name  string
			value string
		}{
			{name: "unknown token", value: "extension bogus"},
			{name: "all combination", value: "#all extension"},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:choice><xs:element name="value" type="xs:integer" block="` + test.value + `"/></xs:choice></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted malformed local block")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("malformed local block returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaBlockCode {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaBlockCode)
				}
				if diagnostic.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, `block="`+test.value+`"`) {
					t.Fatalf("diagnostic location = %s, want block attribute", diagnostic.Loc())
				}
				if diagnostic.SpecRef() != schemaBlockSpecRef(profile.version, schemaBlockElement) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaBlockSpecRef(profile.version, schemaBlockElement))
				}
				if !errors.Is(err, errSchemaBlock) {
					t.Fatalf("diagnostic does not preserve block cause: %v", err)
				}
			})
		}
	}
}

func assertSchemaLocalBlockError(t *testing.T, schema Schema, err error, root string, code, specRef string, cause error, marker, message string) {
	t.Helper()
	if err == nil {
		t.Fatal("discoverSchema accepted invalid local element block input")
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatal("invalid local element block returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != code {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, code)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if diagnostic.Unwrap() == nil || !errors.Is(diagnostic.Unwrap(), cause) || !errors.Is(err, cause) {
		t.Fatalf("diagnostic cause = %v, want preserved %v", diagnostic.Unwrap(), cause)
	}
	if diagnostic.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, marker) {
		t.Fatalf("diagnostic location = %s, want block attribute", diagnostic.Loc())
	}
	if !strings.Contains(diagnostic.Message(), message) {
		t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), message)
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("invalid local element block matched ErrUnsupported: %v", err)
	}
}
