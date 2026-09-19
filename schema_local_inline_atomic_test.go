package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestSchemaBridgeBuildsLocalInlineAtomicParticles(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element>
      <xs:element name="count" minOccurs="0" maxOccurs="2"><xs:simpleType><xs:restriction base="xs:integer"><xs:minInclusive value="1"/></xs:restriction></xs:simpleType></xs:element>
    </xs:choice>
  </xs:complexType>
  <xs:complexType name="Sequence">
    <xs:sequence>
      <xs:element name="amount"><xs:simpleType><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType></xs:element>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}

	choice := localInlineAtomicComplexType(t, schema, "Choice")
	choiceParticle, ok := choice.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
	}
	if got, want := len(choiceParticle.Alternatives()), 2; got != want {
		t.Fatalf("choice alternatives = %d, want %d", got, want)
	}
	flag := requireLocalInlineElement(t, choiceParticle.Alternatives()[0])
	assertLocalInlineAtomicReference(t, flag, "boolean", "flag")
	count := requireLocalInlineElement(t, choiceParticle.Alternatives()[1])
	assertLocalInlineAtomicReference(t, count, "integer", "count")
	if got := count.Occurrences().String(); got != "0/2" {
		t.Fatalf("count occurrences = %s, want 0/2", got)
	}

	sequence := localInlineAtomicComplexType(t, schema, "Sequence")
	sequenceParticle, ok := sequence.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
	}
	amount := requireLocalInlineElement(t, sequenceParticle.Particles()[0])
	assertLocalInlineAtomicReference(t, amount, "decimal", "amount")
}

func TestSchemaBridgeBuildsLocalInlineAtomicParticlesAcrossPolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Choice"><xs:choice><xs:element name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element></xs:choice></xs:complexType>
</xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			choice := localInlineAtomicComplexType(t, schema, "Choice")
			particle, ok := choice.Particle().(ChoiceParticle)
			if !ok || len(particle.Alternatives()) != 1 {
				t.Fatalf("choice particle = %T/%d, want one alternative", choice.Particle(), len(particle.Alternatives()))
			}
			assertLocalInlineAtomicReference(t, requireLocalInlineElement(t, particle.Alternatives()[0]), "boolean", "flag")
		})
	}
}

func TestSchemaBridgeAllocatesLocalInlineAtomicIDsInLexicalOrder(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Values">
    <xs:sequence>
      <xs:element name="first"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>
      <xs:element name="second"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:element>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	sequence := localInlineAtomicComplexType(t, schema, "Values")
	particles, ok := sequence.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("particle = %T, want SequenceParticle", sequence.Particle())
	}
	first := requireLocalInlineElement(t, particles.Particles()[0])
	second := requireLocalInlineElement(t, particles.Particles()[1])
	firstReference, firstOK := first.TypeReference()
	secondReference, secondOK := second.TypeReference()
	firstID, firstIDOK := firstReference.AnonymousID()
	secondID, secondIDOK := secondReference.AnonymousID()
	if !firstOK || !secondOK || !firstIDOK || !secondIDOK || firstID.Source() != "root.xsd" || secondID.Source() != "root.xsd" || firstID.Ordinal() >= secondID.Ordinal() {
		t.Fatalf("local anonymous IDs = %v/%v, want lexical root.xsd order", firstID, secondID)
	}
	if firstReference.Loc().IsZero() || firstReference.VarietyLoc().IsZero() || firstReference.Name() != (QName{}) {
		t.Fatalf("first reference facts = %#v, want located anonymous reference", firstReference)
	}
	if first.DeclaredType() != (QName{}) {
		t.Fatalf("first declared type = %q, want zero QName", first.DeclaredType())
	}
}

func TestSchemaBridgeBuildsLocalInlineAtomicExtensionParticles(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:decimal"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType></xs:element></xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	derived := localInlineAtomicComplexType(t, schema, "Derived")
	choice, ok := derived.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("extension particle = %T, want ChoiceParticle", derived.Particle())
	}
	value := requireLocalInlineElement(t, choice.Alternatives()[0])
	assertLocalInlineAtomicReference(t, value, "decimal", "value")
}

func TestSchemaBridgeResolvesImportedLocalInlineAtomicBase(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" xmlns:r="urn:test" targetNamespace="urn:test">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="o:Cross"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType></xs:element></xs:choice></xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Cross"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchema(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	})
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	choice := localInlineAtomicComplexType(t, schema, "Choice")
	particle, ok := choice.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
	}
	element := requireLocalInlineElement(t, particle.Alternatives()[0])
	reference, ok := element.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("local type reference = %#v/%t, want anonymous", reference, ok)
	}
	definition, ok := reference.AnonymousType()
	base, baseOK := definition.BaseReference()
	if !ok || definition.Base() != mustTestQName(t, "urn:other", "Cross") || !baseOK || base.Kind() != SimpleTypeReferenceNamed {
		t.Fatalf("local anonymous base = %#v/%t, want named imported Cross", definition, ok)
	}
}

//nolint:gocognit // Keep policy and source coverage for each direct owner together.
func TestSchemaBridgeResolvesLocalInlineAtomicBasesAcrossPoliciesAndSources(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" xmlns:o="urn:other" targetNamespace="urn:test">
  <xs:include schemaLocation="child.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element></xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence><xs:element name="count"><xs:simpleType><xs:restriction base="r:Forward"><xs:minInclusive value="1"/></xs:restriction></xs:simpleType></xs:element></xs:sequence></xs:complexType>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:choice><xs:element name="amount"><xs:simpleType><xs:restriction base="o:Imported"><xs:totalDigits value="4"/></xs:restriction></xs:simpleType></xs:element></xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test">
  <xs:simpleType name="Included"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:complexType name="IncludedChoice"><xs:choice><xs:element name="included"><xs:simpleType><xs:restriction base="r:Included"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType></xs:element></xs:choice></xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:decimal"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: child},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}

			choice := localInlineAtomicComplexType(t, schema, "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			assertLocalInlineAtomicReference(t, requireLocalInlineElement(t, choiceParticle.Alternatives()[0]), "boolean", "flag")

			sequence := localInlineAtomicComplexType(t, schema, "Sequence")
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			sequenceElement := requireLocalInlineElement(t, sequenceParticle.Particles()[0])
			sequenceReference, ok := sequenceElement.TypeReference()
			if !ok {
				t.Fatal("forward local inline type reference is missing")
			}
			sequenceDefinition, ok := sequenceReference.AnonymousType()
			if !ok || sequenceDefinition.Base() != mustTestQName(t, "urn:test", "Forward") {
				t.Fatalf("forward local inline base = %#v/%t, want urn:test:Forward", sequenceDefinition, ok)
			}

			derived := localInlineAtomicComplexType(t, schema, "Derived")
			derivedParticle, ok := derived.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("extension particle = %T, want ChoiceParticle", derived.Particle())
			}
			derivedElement := requireLocalInlineElement(t, derivedParticle.Alternatives()[0])
			derivedReference, ok := derivedElement.TypeReference()
			if !ok {
				t.Fatal("imported local inline type reference is missing")
			}
			derivedDefinition, ok := derivedReference.AnonymousType()
			if !ok || derivedDefinition.Base() != mustTestQName(t, "urn:other", "Imported") {
				t.Fatalf("imported local inline base = %#v/%t, want urn:other:Imported", derivedDefinition, ok)
			}

			included := localInlineAtomicComplexType(t, schema, "IncludedChoice")
			includedParticle, ok := included.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("included particle = %T, want ChoiceParticle", included.Particle())
			}
			includedElement := requireLocalInlineElement(t, includedParticle.Alternatives()[0])
			includedReference, ok := includedElement.TypeReference()
			if !ok {
				t.Fatal("chameleon local inline type reference is missing")
			}
			includedDefinition, ok := includedReference.AnonymousType()
			if !ok || includedDefinition.Base() != mustTestQName(t, "urn:test", "Included") {
				t.Fatalf("chameleon local inline base = %#v/%t, want urn:test:Included", includedDefinition, ok)
			}
		})
	}
}

//nolint:gocognit // Keep generation and validation consumer rejection coverage together.
func TestSchemaBridgeRejectsLocalInlineAtomicConsumerUse(t *testing.T) {
	rootTemplate := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Choice">
    <xs:choice><xs:element name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:element></xs:choice>
  </xs:complexType>
  <xs:complexType name="Sequence">
    <xs:sequence><xs:element name="count"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:sequence>
  </xs:complexType>
  <xs:element name="root" type="r:%s"/>
</xs:schema>`
	for _, target := range []string{"Choice", "Sequence"} {
		t.Run("generation/"+target, func(t *testing.T) {
			root := fmt.Sprintf(rootTemplate, target)
			schema, err := discoverTestSchema(t, root, nil)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			_, generateErr := GenerateGo(schema, "test")
			if generateErr == nil {
				t.Fatalf("GenerateGo accepted local anonymous %s type", target)
			}
			if !errors.Is(generateErr, errCodegenUnsupported) {
				t.Fatalf("GenerateGo error = %v, want unsupported cause", generateErr)
			}
		})
		t.Run("validation/"+target, func(t *testing.T) {
			root := fmt.Sprintf(rootTemplate, target)
			schema, err := discoverTestSchema(t, root, nil)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			input := `<root xmlns="urn:test"><flag xmlns="">true</flag></root>`
			if target == "Sequence" {
				input = `<root xmlns="urn:test"><count xmlns="">1</count></root>`
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			if validationErr == nil {
				t.Fatalf("ValidateInstance accepted local anonymous %s type", target)
			}
			wantCause := errInstanceChoiceTarget
			if target == "Sequence" {
				wantCause = errInstanceSequenceTarget
			}
			if !errors.Is(validationErr, wantCause) {
				t.Fatalf("ValidateInstance error = %v, want %v cause", validationErr, wantCause)
			}
		})
	}
}

func TestSchemaBridgeRejectsInvalidOrUnsupportedLocalInlineTypes(t *testing.T) {
	cases := []struct {
		name       string
		child      string
		class      FailureClass
		cause      error
		wantSource SourceID
	}{
		{
			name:       "unresolved base",
			child:      `<xs:simpleType><xs:restriction base="r:Missing"/></xs:simpleType>`,
			class:      FailureInvalid,
			cause:      errSchemaSimpleTypeBaseUnresolved,
			wantSource: "root.xsd",
		},
		{
			name:       "list variety",
			child:      `<xs:simpleType><xs:list itemType="xs:integer"/></xs:simpleType>`,
			class:      FailureUnsupported,
			cause:      ErrUnsupported,
			wantSource: "root.xsd",
		},
		{
			name:       "token scalar",
			child:      `<xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType>`,
			class:      FailureUnsupported,
			cause:      ErrUnsupported,
			wantSource: "root.xsd",
		},
		{
			name:       "malformed facet",
			child:      `<xs:simpleType><xs:restriction base="xs:decimal"><xs:fractionDigits value="-1"/></xs:restriction></xs:simpleType>`,
			class:      FailureInvalid,
			wantSource: "root.xsd",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test"><xs:complexType name="Choice"><xs:choice><xs:element name="value">` + test.child + `</xs:element></xs:choice></xs:complexType></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err == nil {
				t.Fatal("discoverTestSchema accepted invalid or unsupported local inline type")
			}
			if schema.storage != nil {
				t.Fatal("discoverTestSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Loc().Source() != test.wantSource {
				t.Fatalf("diagnostic = %s, want class %s at %s", diagnostic, test.class, test.wantSource)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
		})
	}
}

func localInlineAtomicComplexType(t *testing.T, schema Schema, name string) ComplexTypeDefinition {
	t.Helper()
	component := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", name))
	if len(component) != 1 {
		t.Fatalf("complex type %q matches = %d, want 1", name, len(component))
	}
	definition, ok := component[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("complex type %q view is missing", name)
	}
	return definition
}

func requireLocalInlineElement(t *testing.T, particle Particle) ElementParticle {
	t.Helper()
	element, ok := elementParticleValue(particle)
	if !ok {
		t.Fatalf("particle = %T, want ElementParticle", particle)
	}
	return element
}

func assertLocalInlineAtomicReference(t *testing.T, element ElementParticle, base, name string) {
	t.Helper()
	if element.Name().Local() != name {
		t.Fatalf("element name = %q, want %q", element.Name(), name)
	}
	if got, ok := element.TypeReference(); !ok || !got.IsAnonymous() {
		t.Fatalf("element type reference = %#v/%t, want anonymous", got, ok)
	}
	reference, _ := element.TypeReference()
	if reference.Name() != (QName{}) {
		t.Fatalf("anonymous reference identity = %q/%v, want zero QName/no component ID", reference.Name(), reference)
	}
	if componentID, ok := reference.ComponentID(); ok || !componentID.IsZero() {
		t.Fatalf("anonymous reference component ID = %v/%t, want zero,false", componentID, ok)
	}
	definition, ok := reference.AnonymousType()
	if !ok || !definition.IsAnonymous() || definition.Base().Local() != base || definition.BaseLoc().IsZero() || definition.VarietyLoc().IsZero() {
		t.Fatalf("anonymous type = %#v/%t, want located %s restriction", definition, ok, base)
	}
}
