package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the cross-policy model, identity, location, and immutability contract together.
func TestSchemaBridgeBuildsLocalInlineAtomicParticlesAcrossPolicies(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := fmt.Sprintf(localInlineAtomicSchema, testXSDNamespace, profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if got, want := len(components), 6; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			gotAnonymous, walkErr := countAnonymousSimpleTypeComponents(schema)
			if walkErr != nil {
				t.Fatalf("walk schema: %v", walkErr)
			}
			if got := gotAnonymous; got != 0 {
				t.Fatalf("anonymous simple type component count = %d, want 0", got)
			}

			choice := localInlineComplexType(t, schema, "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), 3; got != want {
				t.Fatalf("choice alternative count = %d, want %d", got, want)
			}
			choiceTypes := []QName{
				mustTestQName(t, testXSDNamespace, "boolean"),
				mustTestQName(t, "urn:root", "ForwardInteger"),
				mustTestQName(t, testXSDNamespace, "decimal"),
			}
			choiceBaseKinds := []SimpleTypeReferenceKind{
				SimpleTypeReferenceBuiltin,
				SimpleTypeReferenceNamed,
				SimpleTypeReferenceBuiltin,
			}
			var choiceIDs []SimpleTypeID
			for index, rawParticle := range choiceAlternatives {
				element := requireInlineElementParticle(t, rawParticle)
				if got, want := element.Name().Local(), []string{"boolean", "integer", "decimal"}[index]; got != want {
					t.Fatalf("choice element %d name = %q, want %q", index, got, want)
				}
				if got, want := element.DeclaredType(), (QName{}); got != want {
					t.Fatalf("choice element %d declared type = %q, want zero", index, got)
				}
				if got, want := element.Occurrences().String(), []string{"1/1", "2/4", "1/1"}[index]; got != want {
					t.Fatalf("choice element %d occurrences = %q, want %q", index, got, want)
				}
				reference, definition := requireInlineAnonymousReference(t, element)
				if got, want := reference.Loc(), definition.Loc(); got != want {
					t.Fatalf("choice element %d reference location = %s, definition location = %s", index, got, want)
				}
				if got, want := reference.Name(), (QName{}); got != want {
					t.Fatalf("choice element %d anonymous reference name = %q, want zero", index, got)
				}
				if got, want := reference.Variety(), SimpleTypeVarietyAtomicRestriction; got != want {
					t.Fatalf("choice element %d variety = %q, want %q", index, got, want)
				}
				if definition.Name() != (QName{}) || !definition.IsAnonymous() || !definition.Component().ID().IsZero() {
					t.Fatalf("choice element %d leaked anonymous component facts: name=%q anonymous=%t id=%v", index, definition.Name(), definition.IsAnonymous(), definition.Component().ID())
				}
				anonymousID, hasAnonymousID := reference.AnonymousID()
				definitionID, hasDefinitionID := definition.NodeID()
				if !hasAnonymousID || !hasDefinitionID || anonymousID.IsZero() || anonymousID != definitionID {
					t.Fatalf("choice element %d anonymous identity = %v/%t and %v/%t", index, anonymousID, hasAnonymousID, definitionID, hasDefinitionID)
				}
				choiceIDs = append(choiceIDs, anonymousID)
				base, hasBase := definition.BaseReference()
				if !hasBase || base.Kind() != choiceBaseKinds[index] || base.Name() != choiceTypes[index] {
					t.Fatalf("choice element %d base reference = %q/%q/%t, want %q/%q/true", index, base.Kind(), base.Name(), hasBase, choiceBaseKinds[index], choiceTypes[index])
				}
				if base.Kind() == SimpleTypeReferenceNamed {
					baseID, hasBaseID := base.ComponentID()
					wantBaseID := componentIDByLocalName(t, components, "ForwardInteger")
					if !hasBaseID || baseID != wantBaseID {
						t.Fatalf("choice element %d named base ID = %v/%t, want %v/true", index, baseID, hasBaseID, wantBaseID)
					}
				}
				if base.Kind() != SimpleTypeReferenceNamed {
					baseID, hasBaseID := base.ComponentID()
					if hasBaseID || !baseID.IsZero() {
						t.Fatalf("choice element %d built-in base has component ID %v/%t", index, baseID, hasBaseID)
					}
				}
				if base.Loc().IsZero() || definition.BaseLoc().IsZero() || definition.VarietyLoc().IsZero() {
					t.Fatalf("choice element %d lost base or variety locations", index)
				}
				if index == 1 {
					digits := definition.DigitFacets()
					value, hasDigits := digits.TotalDigits()
					if !hasDigits || value.String() != "3" {
						t.Fatalf("integer inline totalDigits = %q/%t, want 3/true", value, hasDigits)
					}
					if loc, hasLocation := digits.TotalDigitsLoc(); !hasLocation || loc != mustSchemaTokenLoc(t, "root.xsd", root, 5, `value="3"`) {
						t.Fatalf("integer inline totalDigits location = %s/%t", loc, hasLocation)
					}
				}
				if index == 2 {
					digits := definition.DigitFacets()
					value, hasDigits := digits.FractionDigits()
					if !hasDigits || value.String() != "2" {
						t.Fatalf("decimal inline fractionDigits = %q/%t, want 2/true", value, hasDigits)
					}
				}
			}
			for index := 1; index < len(choiceIDs); index++ {
				if choiceIDs[index-1].Source() != "root.xsd" || choiceIDs[index].Ordinal() <= choiceIDs[index-1].Ordinal() {
					t.Fatalf("choice anonymous IDs are not lexical and source-local: %v, %v", choiceIDs[index-1], choiceIDs[index])
				}
			}

			sequence := localInlineComplexType(t, schema, "Sequence")
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			if got, want := sequenceParticle.Occurrences().String(), "0/2"; got != want {
				t.Fatalf("sequence occurrences = %q, want %q", got, want)
			}
			sequenceElements := sequenceParticle.Elements()
			if got, want := len(sequenceElements), 1; got != want {
				t.Fatalf("sequence element count = %d, want %d", got, want)
			}
			if got, want := sequenceElements[0].Occurrences().String(), "3/4"; got != want {
				t.Fatalf("sequence element occurrences = %q, want %q", got, want)
			}
			if reference, _ := requireInlineAnonymousReference(t, sequenceElements[0]); reference.Kind() != SimpleTypeReferenceAnonymous {
				t.Fatalf("sequence type reference kind = %q, want anonymous", reference.Kind())
			}

			extension := localInlineComplexType(t, schema, "Extension")
			if extension.Derivation() != ComplexTypeDerivationExtension || extension.Particle() == nil {
				t.Fatalf("extension facts = %q/%T, want extension with particle", extension.Derivation(), extension.Particle())
			}
			extensionParticle, ok := extension.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("extension particle = %T, want ChoiceParticle", extension.Particle())
			}
			if got, want := extensionParticle.Occurrences().String(), "0/3"; got != want {
				t.Fatalf("extension choice occurrences = %q, want %q", got, want)
			}
			extensionElements := extensionParticle.Alternatives()
			if got, want := len(extensionElements), 1; got != want {
				t.Fatalf("extension element count = %d, want %d", got, want)
			}
			if reference, _ := requireInlineAnonymousReference(t, requireInlineElementParticle(t, extensionElements[0])); reference.Kind() != SimpleTypeReferenceAnonymous {
				t.Fatalf("extension type reference kind = %q, want anonymous", reference.Kind())
			}

			before := schema.Components()
			choiceAlternatives[0] = nil
			sequenceElements[0] = ElementParticle{}
			if got := requireInlineElementParticle(t, choiceParticle.Alternatives()[0]).Name().Local(); got != "boolean" {
				t.Fatalf("mutating choice query changed name to %q", got)
			}
			if got := sequenceParticle.Elements()[0].Occurrences().String(); got != "3/4" {
				t.Fatalf("mutating sequence query changed occurrences to %q", got)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating particle query results changed the completed schema")
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeat discoverSchema: %v", err)
			}
			if !reflect.DeepEqual(before, repeated.Components()) {
				t.Fatal("repeated schema builds disagree")
			}
		})
	}
}

//nolint:gocognit // Keep the imported/chameleon graph visibility matrix together.
func TestSchemaBridgeResolvesLocalInlineAtomicGraphBases(t *testing.T) {
	graphs := []struct {
		name      string
		root      string
		fixtureID SourceID
		fixture   string
		baseName  QName
	}{
		{
			name:      "imported",
			root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" xmlns:t="urn:root" targetNamespace="urn:root"><xs:import namespace="urn:base" schemaLocation="base.xsd"/><xs:complexType name="Record"><xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="b:Cross"/></xs:simpleType></xs:element></xs:sequence></xs:complexType></xs:schema>`,
			fixtureID: "base.xsd",
			fixture:   `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base"><xs:simpleType name="Cross"><xs:restriction base="xs:decimal"/></xs:simpleType></xs:schema>`,
			baseName:  mustTestQName(t, "urn:base", "Cross"),
		},
		{
			name:      "chameleon",
			root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:include schemaLocation="base.xsd"/><xs:complexType name="Record"><xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="t:Cross"/></xs:simpleType></xs:element></xs:sequence></xs:complexType></xs:schema>`,
			fixtureID: "base.xsd",
			fixture:   `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Cross"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`,
			baseName:  mustTestQName(t, "urn:root", "Cross"),
		},
	}
	for _, graph := range graphs {
		t.Run(graph.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, graph.root, map[string]discoveryFixture{
				string(graph.fixtureID): {id: graph.fixtureID, contents: graph.fixture},
			}, Strict11)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			definition := localInlineComplexType(t, schema, "Record")
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok || len(sequence.Elements()) != 1 {
				t.Fatalf("graph particle = %T/%d, want one-element sequence", definition.Particle(), len(sequence.Elements()))
			}
			reference, anonymous := requireInlineAnonymousReference(t, sequence.Elements()[0])
			base, ok := anonymous.BaseReference()
			if !ok || base.Kind() != SimpleTypeReferenceNamed || base.Name() != graph.baseName {
				t.Fatalf("graph base = %q/%q/%t, want named %q", base.Kind(), base.Name(), ok, graph.baseName)
			}
			if _, hasID := base.ComponentID(); !hasID || base.Loc().IsZero() || reference.Loc().IsZero() {
				t.Fatal("graph anonymous reference lost named base or use-site identity")
			}
		})
	}
}

func TestSchemaBridgeSkipsUnsupportedLocalInlineTypeAtZeroOccurrence(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Record"><xs:choice><xs:element name="absent" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:list itemType="xs:integer"/></xs:simpleType></xs:element><xs:element name="kept" type="xs:integer"/></xs:choice></xs:complexType></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	choice, ok := localInlineComplexType(t, schema, "Record").Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("particle = %T, want ChoiceParticle", localInlineComplexType(t, schema, "Record").Particle())
	}
	alternatives := choice.Alternatives()
	if len(alternatives) != 1 || requireInlineElementParticle(t, alternatives[0]).Name().Local() != "kept" {
		t.Fatalf("zero-occurrence alternatives = %#v, want only kept", alternatives)
	}
}

func TestSchemaBridgeRejectsLocalInlineAtomicBoundariesWithoutPartialSchema(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		class FailureClass
		cause error
	}{
		{
			name:  "string base remains unsupported",
			body:  `<xs:complexType name="Record"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`,
			class: FailureUnsupported,
			cause: ErrUnsupported,
		},
		{
			name:  "token restriction remains unsupported",
			body:  `<xs:complexType name="Record"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`,
			class: FailureUnsupported,
			cause: ErrUnsupported,
		},
		{
			name:  "NMTOKEN restriction remains unsupported",
			body:  `<xs:complexType name="Record"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:NMTOKEN"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`,
			class: FailureUnsupported,
			cause: ErrUnsupported,
		},
		{
			name:  "list remains unsupported",
			body:  `<xs:complexType name="Record"><xs:choice><xs:element name="value"><xs:simpleType><xs:list itemType="xs:integer"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`,
			class: FailureUnsupported,
			cause: ErrUnsupported,
		},
		{
			name:  "restriction base is required",
			body:  `<xs:complexType name="Record"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction/></xs:simpleType></xs:element></xs:choice></xs:complexType>`,
			class: FailureInvalid,
			cause: nil,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">` + test.body + `</xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err == nil {
				t.Fatal("discoverSchema accepted an excluded local inline type")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class {
				t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
			}
			if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic location = %s, want root.xsd location", diagnostic.Loc())
			}
		})
	}
}

//nolint:gocognit // Keep validation and generation consumer gates paired by particle shape.
func TestSchemaBridgeRejectsAnonymousLocalsInValidationAndGeneration(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			particle := `<xs:` + model + `><xs:element name="value"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:` + model + `>`
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:element name="root" type="t:Record"/><xs:complexType name="Record">` + particle + `</xs:complexType></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>1</value></root>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted an anonymous local type")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %s, want explicit unsupported: %v", validationDiagnostic, validationErr)
			}
			if len(validationDiagnostic.Related()) == 0 {
				t.Fatal("validation diagnostic lost the anonymous type related location")
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", output, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Feature() != FeatureCodegen || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("generation diagnostic = %s, want explicit unsupported: %v", generationDiagnostic, generationErr)
			}
			if len(generationDiagnostic.Related()) == 0 {
				t.Fatal("generation diagnostic lost the anonymous type related location")
			}
		})
	}
}

func localInlineComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	t.Helper()
	for _, component := range schema.Components() {
		if component.Name().Local() != local {
			continue
		}
		definition, ok := component.ComplexTypeDefinition()
		if !ok {
			t.Fatalf("component %q is not a complex type", local)
		}
		return definition
	}
	t.Fatalf("complex type %q not found", local)
	return ComplexTypeDefinition{}
}

func requireInlineElementParticle(t *testing.T, particle Particle) ElementParticle {
	t.Helper()
	switch typed := particle.(type) {
	case ElementParticle:
		return typed
	case *ElementParticle:
		if typed != nil {
			return *typed
		}
	}
	t.Fatalf("particle = %T, want ElementParticle", particle)
	return ElementParticle{}
}

func requireInlineAnonymousReference(t *testing.T, element ElementParticle) (SimpleTypeReference, SimpleTypeDefinition) {
	t.Helper()
	reference, ok := element.TypeReference()
	if !ok || reference.Kind() != SimpleTypeReferenceAnonymous || !reference.Name().IsZero() {
		t.Fatalf("element type reference = %q/%q/%t, want anonymous zero-QName reference", reference.Kind(), reference.Name(), ok)
	}
	definition, ok := reference.AnonymousType()
	if !ok || !definition.IsAnonymous() {
		t.Fatalf("anonymous type definition = %#v/%t, want immutable anonymous definition", definition, ok)
	}
	return reference, definition
}

func componentIDByLocalName(t *testing.T, components []Component, local string) ComponentID {
	t.Helper()
	for _, component := range components {
		if component.Name().Local() == local {
			return component.ID()
		}
	}
	t.Fatalf("component %q not found", local)
	return ComponentID{}
}

func countAnonymousSimpleTypeComponents(schema Schema) (int, error) {
	count := 0
	err := schema.Walk(func(component Component) error {
		if definition, ok := component.SimpleTypeDefinition(); ok && definition.IsAnonymous() {
			count++
		}
		return nil
	})
	return count, err
}

const localInlineAtomicSchema = `<xs:schema xmlns:xs="%s" xmlns:t="urn:root" targetNamespace="urn:root" version="%s">
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element name="boolean"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element>
      <xs:element name="integer" minOccurs="2" maxOccurs="4"><xs:simpleType><xs:restriction base="t:ForwardInteger"><xs:totalDigits value="3"/></xs:restriction></xs:simpleType></xs:element>
      <xs:element name="decimal"><xs:simpleType><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType></xs:element>
    </xs:choice>
  </xs:complexType>
  <xs:complexType name="Sequence">
    <xs:sequence minOccurs="0" maxOccurs="2">
      <xs:element name="value" minOccurs="3" maxOccurs="4"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:element>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Base"/>
  <xs:complexType name="Extension"><xs:complexContent><xs:extension base="t:Base"><xs:choice minOccurs="0" maxOccurs="3">
    <xs:element name="extended"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element>
  </xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:simpleType name="ForwardInteger"><xs:restriction base="t:IntegerBase"/></xs:simpleType>
  <xs:simpleType name="IntegerBase"><xs:restriction base="xs:integer"><xs:totalDigits value="9"/></xs:restriction></xs:simpleType>
</xs:schema>`
