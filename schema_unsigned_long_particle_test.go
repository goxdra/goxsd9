package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep policy, shape, occurrence, and ownership checks together.
func TestSchemaUnsignedLongLocalParticlesAcrossPolicies(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := schemaUnsignedLongLocalParticleRoot()
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			before := first.Components()
			if !reflect.DeepEqual(before, second.Components()) {
				t.Fatal("repeated unsignedLong particle builds changed component facts or order")
			}

			choice := requireUnsignedLongParticleComplexType(t, first, "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("Choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			if got, want := choiceParticle.Occurrences().String(), "0/18446744073709551617"; got != want {
				t.Fatalf("Choice occurrences = %q, want %q", got, want)
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), 2; got != want {
				t.Fatalf("Choice alternative count = %d, want %d after 0/0 omission", got, want)
			}

			qualified := requireUnsignedLongElementParticle(t, choiceAlternatives[0])
			if qualified.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element name="qualified"`) || qualified.Name() != mustTestQName(t, "urn:root", "qualified") || qualified.DeclaredType() != mustTestQName(t, testXSDNamespace, "unsignedLong") {
				t.Fatalf("qualified facts = %s/%q/%q, want located qualified unsignedLong particle", qualified.Loc(), qualified.Name(), qualified.DeclaredType())
			}
			if got, want := qualified.Occurrences().String(), "2/18446744073709551616"; got != want {
				t.Fatalf("qualified occurrences = %q, want %q", got, want)
			}
			if !qualified.IsNillable() || !reflect.DeepEqual(qualified.DisallowedSubstitutions(), []string{"substitution"}) {
				t.Fatalf("qualified element facts = nillable %t/block %v, want true/[substitution]", qualified.IsNillable(), qualified.DisallowedSubstitutions())
			}
			if qualified.DisallowedSubstitutionsLoc() != elementReferenceTestAttributeLoc(t, root, `block="substitution"`) {
				t.Fatalf("qualified block location = %s, want explicit block location", qualified.DisallowedSubstitutionsLoc())
			}
			qualifiedReference, ok := qualified.TypeReference()
			if !ok {
				t.Fatal("qualified type reference is missing")
			}
			assertUnsignedLongBuiltinReference(t, qualifiedReference, elementReferenceTestAttributeLoc(t, root, `type="xs:unsignedLong"`), profile.version)
			if typeID, hasTypeID := qualified.TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("built-in local type ID = %v/%t, want zero/false", typeID, hasTypeID)
			}

			named := requireUnsignedLongElementParticle(t, choiceAlternatives[1])
			if named.Name() != mustTestQName(t, "", "named") || named.DeclaredType() != mustTestQName(t, "urn:root", "Tight") {
				t.Fatalf("named particle name/type = %q/%q, want named/r:Tight", named.Name(), named.DeclaredType())
			}
			namedReference, ok := named.TypeReference()
			if !ok || !namedReference.IsNamed() || namedReference.Name() != mustTestQName(t, "urn:root", "Tight") {
				t.Fatalf("named type reference = %#v/%t, want named Tight", namedReference, ok)
			}
			assertUnsignedLongReferenceFacts(t, namedReference.facts, profile.version, "7")
			tight := requireUnsignedLongParticleSimpleType(t, first, "Tight")
			namedID, hasNamedID := namedReference.ComponentID()
			particleID, hasParticleID := named.TypeID()
			if !hasNamedID || namedID != tight.ID() || !hasParticleID || particleID != tight.ID() {
				t.Fatalf("named type ID = %v/%t, want Tight %v/true", namedID, hasNamedID, tight.ID())
			}
			if namedReference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:Tight"`) || namedReference.VarietyLoc().IsZero() {
				t.Fatalf("named reference locations = %s/%s, want type and restriction locations", namedReference.Loc(), namedReference.VarietyLoc())
			}
			base, baseOK := tight.BaseReference()
			if !baseOK || !base.IsNamed() || base.Name() != mustTestQName(t, "urn:root", "BaseUnsignedLong") || base.Loc() != elementReferenceTestAttributeLoc(t, root, `base="r:BaseUnsignedLong"`) {
				t.Fatalf("Tight base reference = %#v/%t, want located named BaseUnsignedLong", base, baseOK)
			}
			bounds, boundsOK := tight.IntegerBounds()
			minFacet, minOK := bounds.MinInclusiveFacet()
			maxFacet, maxOK := bounds.MaxInclusiveFacet()
			if !boundsOK || !minOK || !maxOK || minFacet.Loc() != elementReferenceTestAttributeLoc(t, root, `value="0"`) || maxFacet.Loc() != elementReferenceTestAttributeLoc(t, root, `value="7"`) {
				t.Fatalf("Tight facet locations = %s/%s/%t, want inherited and local value locations", minFacet.Loc(), maxFacet.Loc(), boundsOK)
			}

			sequence := requireUnsignedLongParticleComplexType(t, first, "Sequence")
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
			assertUnsignedLongBuiltinReference(t, mustUnsignedLongParticleTypeReference(t, sequenceElements[0]), mustSchemaTokenLoc(t, "root.xsd", root, 8, `type="xs:unsignedLong"`), profile.version)

			for _, name := range []string{"ExtendedChoice", "ExtendedSequence"} {
				definition := requireUnsignedLongParticleComplexType(t, first, name)
				if definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base") {
					t.Fatalf("%s derivation/base = %q/%q, want extension/r:Base", name, definition.Derivation(), definition.Base())
				}
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					if name != "ExtendedChoice" || len(particle.Alternatives()) != 1 {
						t.Fatalf("%s particle = %T/%d, want one-element choice", name, definition.Particle(), len(particle.Alternatives()))
					}
					if got := requireUnsignedLongElementParticle(t, particle.Alternatives()[0]).Name().Local(); got != "extensionChoice" {
						t.Fatalf("%s child name = %q, want extensionChoice", name, got)
					}
				case SequenceParticle:
					if name != "ExtendedSequence" || len(particle.Elements()) != 1 {
						t.Fatalf("%s particle = %T/%d, want one-element sequence", name, definition.Particle(), len(particle.Elements()))
					}
					if got := particle.Elements()[0].Name().Local(); got != "extensionSequence" {
						t.Fatalf("%s child name = %q, want extensionSequence", name, got)
					}
				default:
					t.Fatalf("%s particle = %T, want direct choice or sequence", name, definition.Particle())
				}
			}

			if requireUnsignedLongParticleComplexType(t, first, "ZeroChoice").Particle() != nil || requireUnsignedLongParticleComplexType(t, first, "ZeroSequence").Particle() != nil {
				t.Fatal("0/0 unsignedLong owners were published as particles")
			}

			choiceAlternatives[0] = ElementParticle{}
			sequenceElements[0] = ElementParticle{}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied particle views changed the completed schema")
			}
		})
	}
}

func schemaUnsignedLongLocalParticleRoot() string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" elementFormDefault="unqualified" version="2.0">
  <xs:complexType name="Choice"><xs:choice minOccurs="0" maxOccurs="18446744073709551617">
    <xs:element name="qualified" type="xs:unsignedLong" form="qualified" nillable="true" block="substitution" minOccurs="2" maxOccurs="18446744073709551616"/>
    <xs:element name="named" type="r:Tight"/>
    <xs:element name="omitted" type="r:Tight" minOccurs="0" maxOccurs="0"/>
  </xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence minOccurs="0" maxOccurs="unbounded">
    <xs:element name="sequenceBuiltin" type="xs:unsignedLong" minOccurs="18446744073709551616" maxOccurs="18446744073709551617"/>
    <xs:element name="sequenceNamed" type="r:Tight" maxOccurs="unbounded"/>
    <xs:element name="sequenceOmitted" type="r:Tight" minOccurs="0" maxOccurs="0"/>
  </xs:sequence></xs:complexType>
  <xs:complexType name="Base"/>
  <xs:complexType name="ExtendedChoice"><xs:complexContent><xs:extension base="r:Base"><xs:choice><xs:element name="extensionChoice" type="r:Tight"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="ExtendedSequence"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="extensionSequence" type="xs:unsignedLong"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="ZeroChoice"><xs:choice minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:unsignedLong"/></xs:choice></xs:complexType>
  <xs:complexType name="ZeroSequence"><xs:sequence minOccurs="0" maxOccurs="0"><xs:element name="value" type="r:Tight"/></xs:sequence></xs:complexType>
  <xs:simpleType name="Tight"><xs:restriction base="r:BaseUnsignedLong"><xs:maxInclusive value="7"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="BaseUnsignedLong"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="0"/><xs:maxInclusive value="18446744073709551615"/></xs:restriction></xs:simpleType>
</xs:schema>`
}

func requireUnsignedLongParticleComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
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

func requireUnsignedLongParticleSimpleType(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
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

func requireUnsignedLongElementParticle(t *testing.T, particle Particle) ElementParticle {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("particle = %T, want ElementParticle", particle)
	}
	return element
}

func mustUnsignedLongParticleTypeReference(t *testing.T, element ElementParticle) SimpleTypeReference {
	t.Helper()
	reference, ok := element.TypeReference()
	if !ok {
		t.Fatalf("element %q has no type reference", element.Name())
	}
	return reference
}

//nolint:gocognit // Keep graph visibility and named identity checks together.
func TestSchemaUnsignedLongLocalParticlesResolveVisibleGraphs(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := schemaUnsignedLongLocalParticleGraph()
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated graph builds changed unsignedLong particle facts or order")
			}
			if got, want := len(first.Documents()), 4; got != want {
				t.Fatalf("document count = %d, want %d", got, want)
			}

			graph := requireUnsignedLongParticleComplexType(t, first, "Graph")
			sequence, ok := graph.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("Graph particle = %T, want SequenceParticle", graph.Particle())
			}
			elements := sequence.Elements()
			cases := []struct {
				local      string
				name       QName
				source     SourceID
				minimum    string
				maximum    string
				typeNeedle string
			}{
				{local: "forward", name: mustTestQName(t, "urn:root", "Forward"), source: "root.xsd", minimum: unsignedLongMinimum, maximum: unsignedLongMaximum, typeNeedle: `type="r:Forward"`},
				{local: "included", name: mustTestQName(t, "urn:root", "Included"), source: "ordinary.xsd", minimum: "1", maximum: "10", typeNeedle: `type="r:Included"`},
				{local: "chameleon", name: mustTestQName(t, "urn:root", "Chameleon"), source: "chameleon.xsd", minimum: "2", maximum: "20", typeNeedle: `type="r:Chameleon"`},
				{local: "imported", name: mustTestQName(t, "urn:other", "Imported"), source: "other.xsd", minimum: "3", maximum: "30", typeNeedle: `type="o:Imported"`},
			}
			if len(elements) != len(cases) {
				t.Fatalf("Graph element count = %d, want %d", len(elements), len(cases))
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
				assertIntegerReferenceFacts(t, reference.facts, profile.version, schemaSimpleTypeAtomicUnsignedLong, "unsignedLong", test.minimum, test.maximum)
				if reference.Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", test.typeNeedle, root, fixtures) {
					t.Fatalf("%s type location = %s, want use-site type location", test.local, reference.Loc())
				}
				id, hasID := reference.ComponentID()
				if !hasID || id.Source() != test.source {
					t.Fatalf("%s type ID = %v/%t, want source %q", test.local, id, hasID, test.source)
				}
			}
		})
	}
}

func schemaUnsignedLongLocalParticleGraph() (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="2.0">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Graph"><xs:sequence>
    <xs:element name="forward" type="r:Forward"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="chameleon" type="r:Chameleon"/>
    <xs:element name="imported" type="o:Imported"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"ordinary.xsd": {
			id:       "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="1"/><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="2"/><xs:maxInclusive value="20"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="3"/><xs:maxInclusive value="30"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
	}
	return root, fixtures
}

func TestSchemaUnsignedLongLocalParticleFailuresRemainLocated(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct {
			name        string
			body        string
			defs        string
			cause       error
			wantSpecRef bool
		}{
			{name: "unresolved", body: `<xs:element name="value" type="r:Missing"/>`, cause: errSchemaElementTypeUnresolved, wantSpecRef: true},
			{name: "wrong kind", body: `<xs:element name="value" type="r:NotType"/>`, cause: errSchemaElementTypeWrongKind, wantSpecRef: true},
			{name: "cyclic named type", body: `<xs:element name="value" type="r:One"/>`, defs: `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, cause: errSchemaSimpleTypeBaseCycle, wantSpecRef: true},
			{name: "malformed type QName", body: `<xs:element name="value" type="r:bad:q"/>`},
			{name: "invalid unsignedLong bound", body: `<xs:element name="value" type="r:Bad"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="18446744073709551616"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundRestriction, wantSpecRef: true},
			{name: "malformed unsignedLong bound", body: `<xs:element name="value" type="r:Bad"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:unsignedLong"><xs:maxInclusive value="not-an-integer"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundValue, wantSpecRef: true},
			{name: "invalid occurrence lexical", body: `<xs:element name="value" type="xs:unsignedLong" minOccurs="not-a-number"/>`, wantSpecRef: true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := schemaUnsignedLongLocalParticleFailureRoot(profile.version, test.body, test.defs, test.name == "wrong kind")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertUnsignedLongLocalParticleInvalidNoSchema(t, schema, err, test.cause, test.wantSpecRef)
			})
		}
	}
}

func assertUnsignedLongLocalParticleInvalidNoSchema(t *testing.T, schema Schema, err error, cause error, wantSpecRef bool) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("schema/error = %v/%#v, want located invalid error and no schema", err, schema)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
	if wantSpecRef && diagnostic.SpecRef() == "" {
		t.Fatalf("diagnostic = %s, want a specification reference", diagnostic)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func schemaUnsignedLongLocalParticleFailureRoot(version XSDVersion, body, defs string, wrongKind bool) string {
	declarations := ""
	if wrongKind {
		declarations = `<xs:element name="NotType" type="xs:unsignedLong"/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(version) + `"><xs:complexType name="Record"><xs:choice>` + body + `</xs:choice></xs:complexType>` + declarations + defs + `</xs:schema>`
}

//nolint:gocognit // Keep both consumer rejection boundaries together.
func TestSchemaUnsignedLongLocalParticleConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="2.0"><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:choice><xs:element name="value" type="xs:unsignedLong"/></xs:choice></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}

			generated, generationErr := GenerateGo(schema, "generated")
			if generated != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", generated, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Feature() != FeatureCodegen || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want unsupported with preserved cause", generationDiagnostic)
			}

			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">0</value></root>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted a local unsignedLong particle")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("ValidateInstance diagnostic = %s, want unsupported with preserved cause", validationDiagnostic)
			}
		})
	}
}

func TestSchemaUnsignedLongLocalParticleExcludedShapesRemainUnsupported(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		for _, test := range []struct {
			name string
			body string
			defs string
		}{
			{name: "inline unsignedLong", body: `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:unsignedLong"/></xs:simpleType></xs:element>`},
			{name: "named long", body: `<xs:element name="value" type="r:Long"/>`, defs: `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`},
			{name: "named int", body: `<xs:element name="value" type="r:Int"/>`, defs: `<xs:simpleType name="Int"><xs:restriction base="xs:int"/></xs:simpleType>`},
			{name: "named non-negative integer", body: `<xs:element name="value" type="r:NonNegative"/>`, defs: `<xs:simpleType name="NonNegative"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType>`},
			{name: "named non-positive integer", body: `<xs:element name="value" type="r:NonPositive"/>`, defs: `<xs:simpleType name="NonPositive"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`},
			{name: "named list", body: `<xs:element name="value" type="r:List"/>`, defs: `<xs:simpleType name="List"><xs:list itemType="xs:unsignedLong"/></xs:simpleType>`},
			{name: "named union", body: `<xs:element name="value" type="r:Union"/>`, defs: `<xs:simpleType name="Union"><xs:union memberTypes="xs:unsignedLong"/></xs:simpleType>`},
			{name: "nested sequence", body: `<xs:sequence><xs:element name="value" type="xs:unsignedLong"/></xs:sequence>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="2.0"><xs:complexType name="Record"><xs:choice>` + test.body + `</xs:choice></xs:complexType>` + test.defs + `</xs:schema>`
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

//nolint:gocognit,funlen // Keep extension shape, provenance, occurrence, and consumer gates together.
func TestSchemaUnsignedLongExtensionParticlesAcrossPolicies(t *testing.T) {
	for _, profile := range unsignedLongPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := schemaUnsignedLongExtensionParticleGraph()
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			before := first.Components()
			if !reflect.DeepEqual(before, second.Components()) {
				t.Fatal("repeated extension builds changed unsignedLong particle facts or order")
			}

			choice := requireUnsignedLongParticleComplexType(t, first, "ExtensionChoice")
			assertUnsignedLongExtensionBase(t, choice, root, 10)
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("ExtensionChoice particle = %T, want ChoiceParticle", choice.Particle())
			}
			if got, want := choiceParticle.Loc(), mustSchemaTokenLoc(t, "root.xsd", root, 11, `<xs:choice`); got != want {
				t.Fatalf("ExtensionChoice particle location = %s, want %s", got, want)
			}
			if got, want := choiceParticle.Occurrences().String(), "0/18446744073709551617"; got != want {
				t.Fatalf("ExtensionChoice occurrences = %q, want %q", got, want)
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), 5; got != want {
				t.Fatalf("ExtensionChoice alternatives = %d, want %d after child 0/0 omission", got, want)
			}
			choiceCases := unsignedLongExtensionParticleCases(t, root, "choice", 12, "2", "18446744073709551616")
			for index, test := range choiceCases {
				element := requireUnsignedLongElementParticle(t, choiceAlternatives[index])
				assertUnsignedLongExtensionElement(t, element, test, profile.version, root, 12+index)
			}

			sequence := requireUnsignedLongParticleComplexType(t, first, "ExtensionSequence")
			assertUnsignedLongExtensionBase(t, sequence, root, 24)
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("ExtensionSequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			if got, want := sequenceParticle.Loc(), mustSchemaTokenLoc(t, "root.xsd", root, 25, `<xs:sequence`); got != want {
				t.Fatalf("ExtensionSequence particle location = %s, want %s", got, want)
			}
			if got, want := sequenceParticle.Occurrences().String(), "0/unbounded"; got != want {
				t.Fatalf("ExtensionSequence occurrences = %q, want %q", got, want)
			}
			sequenceElements := sequenceParticle.Elements()
			if got, want := len(sequenceElements), 5; got != want {
				t.Fatalf("ExtensionSequence elements = %d, want %d after child 0/0 omission", got, want)
			}
			sequenceCases := unsignedLongExtensionParticleCases(t, root, "sequence", 26, "18446744073709551616", "18446744073709551617")
			for index, test := range sequenceCases {
				assertUnsignedLongExtensionElement(t, sequenceElements[index], test, profile.version, root, 26+index)
			}

			for _, local := range []string{"ExtensionChoiceZero", "ExtensionSequenceZero"} {
				definition := requireUnsignedLongParticleComplexType(t, first, local)
				if definition.Particle() != nil {
					t.Fatalf("%s published an extension particle for owner 0/0", local)
				}
			}

			generated, generationErr := GenerateGo(first, "generated")
			if generated != nil || generationErr == nil {
				t.Fatalf("GenerateGo extension result = (%q, %v), want unsupported with no output", generated, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Feature() != FeatureCodegen || !errors.Is(generationErr, ErrUnsupported) || generationDiagnostic.Loc().IsZero() {
				t.Fatalf("GenerateGo extension diagnostic = %s, want located unsupported with preserved cause", generationDiagnostic)
			}

			for _, rootName := range []string{"choiceRoot", "sequenceRoot"} {
				instance := `<` + rootName + ` xmlns="urn:root"/>`
				validationErr := ValidateInstance(first, "instance.xml", io.NopCloser(strings.NewReader(instance)))
				if validationErr == nil {
					t.Fatalf("ValidateInstance accepted extension consumer %q", rootName)
				}
				validationDiagnostic := requireDiagnostic(t, validationErr)
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) || validationDiagnostic.Loc().IsZero() {
					t.Fatalf("ValidateInstance %s diagnostic = %s, want located unsupported with preserved cause", rootName, validationDiagnostic)
				}
			}

			choiceAlternatives[0] = ElementParticle{}
			sequenceElements[0] = ElementParticle{}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied extension particle views changed the completed schema")
			}
		})
	}
}

type unsignedLongExtensionParticleCase struct {
	local        string
	typeName     QName
	source       SourceID
	minimum      string
	maximum      string
	boundMinimum string
	boundMaximum string
	typeLoc      Loc
}

func unsignedLongExtensionParticleCases(t *testing.T, root, prefix string, startLine int, builtinMinimum, builtinMaximum string) []unsignedLongExtensionParticleCase {
	t.Helper()
	return []unsignedLongExtensionParticleCase{
		{local: prefix + "Builtin", typeName: mustTestQName(t, testXSDNamespace, "unsignedLong"), minimum: builtinMinimum, maximum: builtinMaximum, typeLoc: mustSchemaTokenLoc(t, "root.xsd", root, startLine, `type="xs:unsignedLong"`)},
		{local: prefix + "Forward", typeName: mustTestQName(t, "urn:root", "Forward"), source: "root.xsd", minimum: "1", maximum: "1", boundMinimum: unsignedLongMinimum, boundMaximum: unsignedLongMaximum, typeLoc: mustSchemaTokenLoc(t, "root.xsd", root, startLine+1, `type="r:Forward"`)},
		{local: prefix + "Included", typeName: mustTestQName(t, "urn:root", "Included"), source: "ordinary.xsd", minimum: "0", maximum: "10", boundMinimum: "1", boundMaximum: "10", typeLoc: mustSchemaTokenLoc(t, "root.xsd", root, startLine+2, `type="r:Included"`)},
		{local: prefix + "Chameleon", typeName: mustTestQName(t, "urn:root", "Chameleon"), source: "chameleon.xsd", minimum: "1", maximum: "unbounded", boundMinimum: "2", boundMaximum: "20", typeLoc: mustSchemaTokenLoc(t, "root.xsd", root, startLine+3, `type="r:Chameleon"`)},
		{local: prefix + "Imported", typeName: mustTestQName(t, "urn:other", "Imported"), source: "other.xsd", minimum: "3", maximum: "30", boundMinimum: "3", boundMaximum: "30", typeLoc: mustSchemaTokenLoc(t, "root.xsd", root, startLine+4, `type="o:Imported"`)},
	}
}

func assertUnsignedLongExtensionBase(t *testing.T, definition ComplexTypeDefinition, root string, line int) {
	t.Helper()
	if definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base") {
		t.Fatalf("%s derivation/base = %q/%q, want extension/r:Base", definition.Name(), definition.Derivation(), definition.Base())
	}
	base, ok := definition.BaseReference()
	if !ok || base.Name() != mustTestQName(t, "urn:root", "Base") || base.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, line, `base="r:Base"`) {
		t.Fatalf("%s base reference = %#v/%t, want located r:Base", definition.Name(), base, ok)
	}
	if definition.DerivationLoc() != mustSchemaTokenLoc(t, "root.xsd", root, line, `<xs:extension`) {
		t.Fatalf("%s derivation location = %s, want extension location", definition.Name(), definition.DerivationLoc())
	}
	if id, ok := base.ComponentID(); !ok || id.Source() != "root.xsd" {
		t.Fatalf("%s base ID = %v/%t, want root.xsd ownership", definition.Name(), id, ok)
	}
}

func assertUnsignedLongExtensionElement(t *testing.T, element ElementParticle, test unsignedLongExtensionParticleCase, version XSDVersion, root string, line int) {
	t.Helper()
	if element.Name() != mustTestQName(t, "", test.local) || element.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, line, `<xs:element name="`+test.local+`"`) {
		t.Fatalf("%s element facts = %q/%s, want retained name/location", test.local, element.Name(), element.Loc())
	}
	if got, want := element.Occurrences().String(), test.minimum+"/"+test.maximum; got != want {
		t.Fatalf("%s occurrences = %q, want %q", test.local, got, want)
	}
	reference, ok := element.TypeReference()
	if !ok || reference.Name() != test.typeName || reference.Loc() != test.typeLoc {
		t.Fatalf("%s type reference = %#v/%t, want %q at %s", test.local, reference, ok, test.typeName, test.typeLoc)
	}
	if test.source == "" {
		assertUnsignedLongBuiltinReference(t, reference, test.typeLoc, version)
		if id, hasID := element.TypeID(); hasID || !id.IsZero() {
			t.Fatalf("%s built-in type ID = %v/%t, want zero/false", test.local, id, hasID)
		}
		return
	}
	if !reference.IsNamed() {
		t.Fatalf("%s type reference = %#v, want named reference", test.local, reference)
	}
	assertIntegerReferenceFacts(t, reference.facts, version, schemaSimpleTypeAtomicUnsignedLong, "unsignedLong", test.boundMinimum, test.boundMaximum)
	id, ok := reference.ComponentID()
	particleID, particleOK := element.TypeID()
	if !ok || id.Source() != test.source || !particleOK || particleID != id {
		t.Fatalf("%s ownership = %v/%t and %v/%t, want matching %q IDs", test.local, id, ok, particleID, particleOK, test.source)
	}
}

func schemaUnsignedLongExtensionParticleGraph() (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="2.0">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="choiceRoot" type="r:ExtensionChoice"/>
  <xs:element name="sequenceRoot" type="r:ExtensionSequence"/>
  <xs:complexType name="Base"/>
  <xs:complexType name="ExtensionChoice">
    <xs:complexContent>
      <xs:extension base="r:Base">
        <xs:choice minOccurs="0" maxOccurs="18446744073709551617">
          <xs:element name="choiceBuiltin" type="xs:unsignedLong" minOccurs="2" maxOccurs="18446744073709551616"/>
          <xs:element name="choiceForward" type="r:Forward"/>
          <xs:element name="choiceIncluded" type="r:Included" minOccurs="0" maxOccurs="10"/>
          <xs:element name="choiceChameleon" type="r:Chameleon" maxOccurs="unbounded"/>
          <xs:element name="choiceImported" type="o:Imported" minOccurs="3" maxOccurs="30"/>
          <xs:element name="choiceZero" type="r:Included" minOccurs="0" maxOccurs="0"/>
        </xs:choice>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="ExtensionSequence">
    <xs:complexContent>
      <xs:extension base="r:Base">
        <xs:sequence minOccurs="0" maxOccurs="unbounded">
          <xs:element name="sequenceBuiltin" type="xs:unsignedLong" minOccurs="18446744073709551616" maxOccurs="18446744073709551617"/>
          <xs:element name="sequenceForward" type="r:Forward"/>
          <xs:element name="sequenceIncluded" type="r:Included" minOccurs="0" maxOccurs="10"/>
          <xs:element name="sequenceChameleon" type="r:Chameleon" maxOccurs="unbounded"/>
          <xs:element name="sequenceImported" type="o:Imported" minOccurs="3" maxOccurs="30"/>
          <xs:element name="sequenceZero" type="r:Included" minOccurs="0" maxOccurs="0"/>
        </xs:sequence>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="ExtensionChoiceZero">
    <xs:complexContent><xs:extension base="r:Base"><xs:choice minOccurs="0" maxOccurs="0"><xs:element name="zeroChoice" type="r:Included"/></xs:choice></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="ExtensionSequenceZero">
    <xs:complexContent><xs:extension base="r:Base"><xs:sequence minOccurs="0" maxOccurs="0"><xs:element name="zeroSequence" type="r:Included"/></xs:sequence></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:unsignedLong"/></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"ordinary.xsd": {
			id:       "ordinary.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="1"/><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
		"chameleon.xsd": {
			id:       "chameleon.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="2"/><xs:maxInclusive value="20"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:unsignedLong"><xs:minInclusive value="3"/><xs:maxInclusive value="30"/></xs:restriction></xs:simpleType></xs:schema>`,
		},
	}
	return root, fixtures
}
