package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep policy, particle shape, occurrence, and provenance checks together.
func TestSchemaLongLocalParticlesAcrossPolicies(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaLongParticleRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated long particle builds changed component facts or order")
			}

			choice := requireLongParticleComplexType(t, first, "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("Choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			if got, want := choiceParticle.Occurrences().String(), "0/18446744073709551617"; got != want {
				t.Fatalf("Choice occurrences = %q, want %q", got, want)
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), 2; got != want {
				t.Fatalf("Choice alternative count = %d, want %d", got, want)
			}
			qualified := requireLongElementParticle(t, choiceAlternatives[0])
			if qualified.Name() != mustTestQName(t, "urn:root", "qualified") || qualified.DeclaredType() != mustTestQName(t, testXSDNamespace, "long") {
				t.Fatalf("qualified particle name/type = %q/%q, want {urn:root}qualified/{%s}long", qualified.Name(), qualified.DeclaredType(), testXSDNamespace)
			}
			if got, want := qualified.Occurrences().String(), "2/18446744073709551616"; got != want {
				t.Fatalf("qualified occurrences = %q, want %q", got, want)
			}
			if !qualified.IsNillable() || !reflect.DeepEqual(qualified.DisallowedSubstitutions(), []string{"substitution"}) {
				t.Fatalf("qualified element facts = nillable %t/block %v, want true/[substitution]", qualified.IsNillable(), qualified.DisallowedSubstitutions())
			}
			qualifiedReference, ok := qualified.TypeReference()
			if !ok {
				t.Fatal("qualified type reference is missing")
			}
			assertLongBuiltinReference(t, qualifiedReference, elementReferenceTestAttributeLoc(t, root, `type="xs:long"`), profile.version)
			if typeID, hasTypeID := qualified.TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("built-in local type ID = %v/%t, want zero/false", typeID, hasTypeID)
			}

			named := requireLongElementParticle(t, choiceAlternatives[1])
			if named.Name() != mustTestQName(t, "", "named") || named.DeclaredType() != mustTestQName(t, "urn:root", "Tight") {
				t.Fatalf("named particle name/type = %q/%q, want named/r:Tight", named.Name(), named.DeclaredType())
			}
			namedReference, ok := named.TypeReference()
			if !ok || !namedReference.IsNamed() || namedReference.Name() != mustTestQName(t, "urn:root", "Tight") {
				t.Fatalf("named type reference = %#v/%t, want named Tight", namedReference, ok)
			}
			assertLongReferenceFacts(t, namedReference.facts, profile.version, "-100", "7")
			tight := requireLongParticleSimpleType(t, first, "Tight")
			namedID, hasNamedID := namedReference.ComponentID()
			if !hasNamedID || namedID != tight.ID() {
				t.Fatalf("named type ID = %v/%t, want Tight %v/true", namedID, hasNamedID, tight.ID())
			}
			if namedReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:Tight"`) || namedReference.VarietyLoc().IsZero() {
				t.Fatalf("named reference locations = %s/%s, want type and restriction locations", namedReference.Loc(), namedReference.VarietyLoc())
			}
			if base, baseOK := tight.BaseReference(); !baseOK || base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="r:BaseLong"`) {
				t.Fatalf("Tight base location = %s/%t, want located named base", base.Loc(), baseOK)
			}
			bounds, ok := tight.IntegerBounds()
			minFacet, minOK := bounds.MinInclusiveFacet()
			maxFacet, maxOK := bounds.MaxInclusiveFacet()
			if !ok || !minOK || !maxOK || minFacet.Loc() != elementReferenceTestAttributeLoc(t, root, `value="-100"`) || maxFacet.Loc() != elementReferenceTestAttributeLoc(t, root, `value="7"`) {
				t.Fatalf("Tight facet locations = %s/%s/%t, want inherited and local value locations", minFacet.Loc(), maxFacet.Loc(), ok)
			}

			sequence := requireLongParticleComplexType(t, first, "Sequence")
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("Sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			if got, want := sequenceParticle.Occurrences().String(), "0/unbounded"; got != want {
				t.Fatalf("Sequence occurrences = %q, want %q", got, want)
			}
			sequenceElements := sequenceParticle.Elements()
			if got, want := len(sequenceElements), 2; got != want {
				t.Fatalf("Sequence element count = %d, want %d after 0/0 omission", got, want)
			}
			if got, want := sequenceElements[0].Occurrences().String(), "18446744073709551616/18446744073709551617"; got != want {
				t.Fatalf("large sequence occurrences = %q, want %q", got, want)
			}
			if got, want := sequenceElements[1].Occurrences().String(), "1/unbounded"; got != want {
				t.Fatalf("unbounded sequence occurrences = %q, want %q", got, want)
			}
			if sequenceElements[0].Name() != mustTestQName(t, "", "sequenceBuiltin") || sequenceElements[1].Name() != mustTestQName(t, "", "sequenceNamed") {
				t.Fatalf("sequence lexical names = %q/%q, want sequenceBuiltin/sequenceNamed", sequenceElements[0].Name(), sequenceElements[1].Name())
			}

			for _, name := range []string{"ExtendedChoice", "ExtendedSequence"} {
				definition := requireLongParticleComplexType(t, first, name)
				if definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base") {
					t.Fatalf("%s derivation/base = %q/%q, want extension/r:Base", name, definition.Derivation(), definition.Base())
				}
				particle := definition.Particle()
				switch name {
				case "ExtendedChoice":
					model, ok := particle.(ChoiceParticle)
					if !ok || len(model.Alternatives()) != 1 {
						t.Fatalf("%s particle = %T/%d, want one-element choice", name, particle, len(model.Alternatives()))
					}
					if got := requireLongElementParticle(t, model.Alternatives()[0]).Name().Local(); got != "extensionChoice" {
						t.Fatalf("%s child name = %q, want extensionChoice", name, got)
					}
				case "ExtendedSequence":
					model, ok := particle.(SequenceParticle)
					if !ok || len(model.Elements()) != 1 {
						t.Fatalf("%s particle = %T/%d, want one-element sequence", name, particle, len(model.Elements()))
					}
					if got := model.Elements()[0].Name().Local(); got != "extensionSequence" {
						t.Fatalf("%s child name = %q, want extensionSequence", name, got)
					}
				}
			}
		})
	}
}

func schemaLongParticleRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" elementFormDefault="unqualified" version="` + string(version) + `">
  <xs:complexType name="Choice">
    <xs:choice minOccurs="0" maxOccurs="18446744073709551617">
      <xs:element name="qualified" type="xs:long" form="qualified" nillable="true" block="substitution" minOccurs="2" maxOccurs="18446744073709551616"/>
      <xs:element name="named" type="r:Tight"/>
      <xs:element name="omitted" type="r:Tight" minOccurs="0" maxOccurs="0"/>
    </xs:choice>
  </xs:complexType>
  <xs:complexType name="Sequence">
    <xs:sequence minOccurs="0" maxOccurs="unbounded">
      <xs:element name="sequenceBuiltin" type="xs:long" minOccurs="18446744073709551616" maxOccurs="18446744073709551617"/>
      <xs:element name="sequenceNamed" type="r:Tight" maxOccurs="unbounded"/>
      <xs:element name="sequenceOmitted" type="r:Tight" minOccurs="0" maxOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="ExtendedChoice"><xs:complexContent><xs:extension base="r:Base"><xs:choice><xs:element name="extensionChoice" type="r:Tight"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="ExtendedSequence"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="extensionSequence" type="xs:long"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"/>
  <xs:simpleType name="BaseLong"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="r:BaseLong"><xs:maxInclusive value="7"/></xs:restriction></xs:simpleType>
</xs:schema>`
}

func requireLongParticleComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	t.Helper()
	matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", local))
	if len(matches) != 1 {
		t.Fatalf("complex type %q matches = %d, want 1", local, len(matches))
	}
	definition, ok := matches[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("complex type %q has no definition view", local)
	}
	return definition
}

func requireLongParticleSimpleType(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	t.Helper()
	matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", local))
	if len(matches) != 1 {
		t.Fatalf("simple type %q matches = %d, want 1", local, len(matches))
	}
	definition, ok := matches[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("simple type %q has no definition view", local)
	}
	return definition
}

func requireLongElementParticle(t *testing.T, particle Particle) ElementParticle {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("particle = %T, want ElementParticle", particle)
	}
	return element
}

//nolint:gocognit // Keep graph visibility and named identity checks together.
func TestSchemaLongLocalParticlesResolveVisibleGraphs(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := schemaLongParticleGraph(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated graph builds changed long particle facts or order")
			}
			graph := requireLongParticleComplexType(t, first, "Graph")
			sequence, ok := graph.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("Graph particle = %T, want SequenceParticle", graph.Particle())
			}
			elements := sequence.Elements()
			if got, want := len(elements), 4; got != want {
				t.Fatalf("Graph element count = %d, want %d", got, want)
			}
			cases := []struct {
				local  string
				name   QName
				source SourceID
				min    string
				max    string
			}{
				{local: "forward", name: mustTestQName(t, "urn:root", "Forward"), source: "root.xsd", min: "-9223372036854775808", max: "9223372036854775807"},
				{local: "included", name: mustTestQName(t, "urn:root", "Included"), source: "ordinary.xsd", min: "-10", max: "10"},
				{local: "chameleon", name: mustTestQName(t, "urn:root", "Chameleon"), source: "chameleon.xsd", min: "-20", max: "20"},
				{local: "imported", name: mustTestQName(t, "urn:other", "Imported"), source: "other.xsd", min: "-30", max: "30"},
			}
			for index, test := range cases {
				element := elements[index]
				if element.Name() != mustTestQName(t, "", test.local) {
					t.Fatalf("element %d name = %q, want %q", index, element.Name(), test.local)
				}
				reference, ok := element.TypeReference()
				if !ok || !reference.IsNamed() || reference.Name() != test.name {
					t.Fatalf("%s type reference = %#v/%t, want named %q", test.local, reference, ok, test.name)
				}
				assertLongReferenceFacts(t, reference.facts, profile.version, test.min, test.max)
				id, hasID := reference.ComponentID()
				if !hasID || id.Source() != test.source {
					t.Fatalf("%s type ID = %v/%t, want source %q", test.local, id, hasID, test.source)
				}
			}
		})
	}
}

func schemaLongParticleGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Graph"><xs:sequence>
    <xs:element name="forward" type="r:Forward"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="chameleon" type="r:Chameleon"/>
    <xs:element name="imported" type="o:Imported"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="xs:long"/></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"ordinary.xsd":  {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:long"><xs:minInclusive value="-10"/><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:long"><xs:minInclusive value="-20"/><xs:maxInclusive value="20"/></xs:restriction></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:long"><xs:minInclusive value="-30"/><xs:maxInclusive value="30"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
	return root, fixtures
}

func TestSchemaLongLocalParticleFailuresRemainVisibleAtZeroOccurrence(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name  string
			body  string
			defs  string
			cause error
		}{
			{name: "unresolved zero occurrence", body: `<xs:element name="value" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, cause: errSchemaElementTypeUnresolved},
			{name: "wrong kind", body: `<xs:element name="value" type="r:NotType"/>`, cause: errSchemaElementTypeWrongKind},
			{name: "cyclic named type", body: `<xs:element name="value" type="r:One"/>`, defs: `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, cause: errSchemaSimpleTypeBaseCycle},
			{name: "malformed type QName", body: `<xs:element name="value" type="r:bad:q"/>`},
			{name: "invalid long bound", body: `<xs:element name="value" type="r:Bad"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundRestriction},
			{name: "malformed long bound", body: `<xs:element name="value" type="r:Bad"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundValue},
			{name: "invalid occurrence lexical", body: `<xs:element name="value" type="xs:long" minOccurs="not-a-number"/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := schemaLongParticleFailureRoot(profile.version, test.body, test.defs, test.name == "wrong kind")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertLongParticleInvalidNoSchema(t, schema, err, test.cause)
			})
		}
	}
}

func schemaLongParticleFailureRoot(version XSDVersion, body, defs string, wrongKind bool) string {
	declarations := ``
	if wrongKind {
		declarations = `<xs:element name="NotType" type="xs:long"/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(version) + `">` + declarations + `<xs:complexType name="Record"><xs:choice>` + body + `</xs:choice></xs:complexType>` + defs + `</xs:schema>`
}

func assertLongParticleInvalidNoSchema(t *testing.T, schema Schema, err error, cause error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want invalid error and no schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func TestSchemaLongLocalParticleZeroOwnersDoNotHideInvalidTypes(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:` + model + ` minOccurs="0" maxOccurs="0"><xs:element name="value" type="r:Missing"/></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertLongParticleInvalidNoSchema(t, schema, err, errSchemaElementTypeUnresolved)
			})
		}
	}
}

func TestSchemaLongLocalParticleExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		for _, test := range []struct {
			name string
			body string
			defs string
		}{
			{name: "inline long", body: `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:element>`},
			{name: "named int", body: `<xs:element name="value" type="r:Int"/>`, defs: `<xs:simpleType name="Int"><xs:restriction base="xs:int"/></xs:simpleType>`},
			{name: "named unsigned long", body: `<xs:element name="value" type="r:Unsigned"/>`, defs: `<xs:simpleType name="Unsigned"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>`},
			{name: "named non-negative integer", body: `<xs:element name="value" type="r:NonNegative"/>`, defs: `<xs:simpleType name="NonNegative"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`},
			{name: "named non-positive integer", body: `<xs:element name="value" type="r:NonPositive"/>`, defs: `<xs:simpleType name="NonPositive"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`},
			{name: "named list", body: `<xs:element name="value" type="r:List"/>`, defs: `<xs:simpleType name="List"><xs:list itemType="xs:long"/></xs:simpleType>`},
			{name: "named union", body: `<xs:element name="value" type="r:Union"/>`, defs: `<xs:simpleType name="Union"><xs:union memberTypes="xs:long"/></xs:simpleType>`},
			{name: "nested sequence", body: `<xs:sequence><xs:element name="value" type="xs:long"/></xs:sequence>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:choice>` + test.body + `</xs:choice></xs:complexType>` + test.defs + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("schema/error = %v/%#v, want unsupported error and no schema", err, schema)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want schema unsupported with preserved cause", diagnostic)
				}
			})
		}
	}
}

func TestSchemaLongLocalParticleConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range longPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:choice><xs:element name="value" type="xs:long"/></xs:choice></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			assertLongParticleConsumersUnsupported(t, schema)
		})
	}
}

func assertLongParticleConsumersUnsupported(t *testing.T, schema Schema) {
	t.Helper()
	generated, generationErr := GenerateGo(schema, "generated")
	if generated != nil || generationErr == nil {
		t.Fatalf("local long GenerateGo result = (%q, %v), want no output", generated, generationErr)
	}
	assertLongParticleConsumerDiagnostic(t, generationErr, FeatureCodegen)
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
	if validationErr == nil {
		t.Fatal("local long ValidateInstance unexpectedly succeeded")
	}
	assertLongParticleConsumerDiagnostic(t, validationErr, FeatureInstanceValidation)
}

func assertLongParticleConsumerDiagnostic(t *testing.T, err error, feature FeatureID) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != feature || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("consumer diagnostic = %s, want unsupported %s", diagnostic, feature)
	}
}
