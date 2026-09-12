package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the local token-family shape and provenance matrix together.
func TestSchemaBridgeBuildsLocalTokenParticlesAcrossPolicies(t *testing.T) {
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
			root := localTokenParticleRoot(profile.version)
			fixtures := map[string]discoveryFixture{
				"chameleon.xsd": {
					id: "chameleon.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="IncludedNMTOKEN"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="included"/></xs:restriction></xs:simpleType>
</xs:schema>`,
				},
				"other.xsd": {
					id: "other.xsd",
					contents: fmt.Sprintf(`<xs:schema xmlns:xs="%s" targetNamespace="urn:other" version="%s">
  <xs:simpleType name="ImportedToken"><xs:restriction base="xs:token"><xs:enumeration value=" imported "/></xs:restriction></xs:simpleType>
</xs:schema>`, testXSDNamespace, profile.version),
				},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			before := schema.Components()

			choice := localTokenParticleComplexType(t, schema, "Choice")
			choiceParticle, ok := choice.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			if got, want := choiceParticle.Occurrences().String(), "0/2"; got != want {
				t.Fatalf("choice occurrences = %q, want %q", got, want)
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), 6; got != want {
				t.Fatalf("choice alternative count = %d, want %d", got, want)
			}
			wantChoiceNames := []string{"qualifiedToken", "namedToken", "builtinNMTOKEN", "forwardNMTOKEN", "importedToken", "includedNMTOKEN"}
			wantChoiceOccurrences := []string{"0/2", "1/3", "1/1", "1/1", "1/1", "1/1"}
			for index, particle := range choiceAlternatives {
				element, isElement := particle.(ElementParticle)
				if !isElement {
					t.Fatalf("choice alternative %d = %T, want ElementParticle", index, particle)
				}
				if got, want := element.Name().Local(), wantChoiceNames[index]; got != want {
					t.Fatalf("choice alternative %d name = %q, want %q", index, got, want)
				}
				if got, want := element.Occurrences().String(), wantChoiceOccurrences[index]; got != want {
					t.Fatalf("choice alternative %d occurrences = %q, want %q", index, got, want)
				}
				if index == 0 {
					got := element.Name().Namespace()
					if got != "urn:root" {
						t.Fatalf("qualified choice namespace = %q, want urn:root", got)
					}
					continue
				}
				if got := element.Name().Namespace(); got != "" {
					t.Fatalf("unqualified choice namespace %d = %q, want empty", index, got)
				}
			}

			qualifiedToken := requireLocalTokenElementParticle(t, choiceAlternatives[0], 0)
			assertLocalTokenParticleReference(
				t,
				qualifiedToken,
				mustTestQName(t, testXSDNamespace, "token"),
				schemaSimpleTypeAtomicToken,
				mustSchemaTokenLoc(t, "root.xsd", root, 9, `type="xs:token"`),
				ComponentID{},
				false,
			)
			if !qualifiedToken.IsNillable() {
				t.Fatal("qualified token nillable = false, want true")
			}
			if got, want := qualifiedToken.DisallowedSubstitutions(), []string{"substitution"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("qualified token block = %#v, want %#v", got, want)
			}
			if got, want := qualifiedToken.DisallowedSubstitutionsLoc(), mustSchemaTokenLoc(t, "root.xsd", root, 9, `block="substitution"`); got != want {
				t.Fatalf("qualified token block location = %s, want %s", got, want)
			}

			namedToken := requireLocalTokenElementParticle(t, choiceAlternatives[1], 1)
			rootTokenID := componentIDForName(t, schema, mustTestQName(t, "urn:root", "RootToken"))
			assertLocalTokenParticleReference(
				t,
				namedToken,
				mustTestQName(t, "urn:root", "InheritedToken"),
				schemaSimpleTypeAtomicToken,
				mustSchemaTokenLoc(t, "root.xsd", root, 10, `type="r:InheritedToken"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:root", "InheritedToken")),
				true,
			)
			inheritedType := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "InheritedToken"))
			if len(inheritedType) != 1 {
				t.Fatalf("inherited token component count = %d, want one", len(inheritedType))
			}
			inheritedDefinition, ok := inheritedType[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("inherited token has no simple type definition")
			}
			baseReference, ok := inheritedDefinition.BaseReference()
			if !ok {
				t.Fatal("inherited token has no base reference")
			}
			if baseID, hasBaseID := baseReference.ComponentID(); !hasBaseID || baseID != rootTokenID {
				t.Fatalf("inherited token base ID = %v/%t, want RootToken identity %v", baseID, hasBaseID, rootTokenID)
			}
			assertLocalTokenParticleEnumeration(t, namedToken, []string{" root "}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 4, `value=" root "`),
			})

			assertLocalTokenParticleReference(
				t,
				requireLocalTokenElementParticle(t, choiceAlternatives[2], 2),
				mustTestQName(t, testXSDNamespace, "NMTOKEN"),
				schemaSimpleTypeAtomicNMTOKEN,
				mustSchemaTokenLoc(t, "root.xsd", root, 11, `type="xs:NMTOKEN"`),
				ComponentID{},
				false,
			)
			assertLocalTokenParticleReference(
				t,
				requireLocalTokenElementParticle(t, choiceAlternatives[3], 3),
				mustTestQName(t, "urn:root", "ForwardNMTOKEN"),
				schemaSimpleTypeAtomicNMTOKEN,
				mustSchemaTokenLoc(t, "root.xsd", root, 12, `type="r:ForwardNMTOKEN"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:root", "ForwardNMTOKEN")),
				true,
			)
			assertLocalTokenParticleReference(
				t,
				requireLocalTokenElementParticle(t, choiceAlternatives[4], 4),
				mustTestQName(t, "urn:other", "ImportedToken"),
				schemaSimpleTypeAtomicToken,
				mustSchemaTokenLoc(t, "root.xsd", root, 13, `type="o:ImportedToken"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:other", "ImportedToken")),
				true,
			)
			assertLocalTokenParticleReference(
				t,
				requireLocalTokenElementParticle(t, choiceAlternatives[5], 5),
				mustTestQName(t, "urn:root", "IncludedNMTOKEN"),
				schemaSimpleTypeAtomicNMTOKEN,
				mustSchemaTokenLoc(t, "root.xsd", root, 14, `type="r:IncludedNMTOKEN"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:root", "IncludedNMTOKEN")),
				true,
			)

			sequence := localTokenParticleComplexType(t, schema, "Sequence")
			sequenceParticle, ok := sequence.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			if got, want := sequenceParticle.Occurrences().String(), "2/4"; got != want {
				t.Fatalf("sequence occurrences = %q, want %q", got, want)
			}
			sequenceElements := sequenceParticle.Elements()
			if got, want := len(sequenceElements), 3; got != want {
				t.Fatalf("sequence element count = %d, want %d", got, want)
			}
			wantSequenceOccurrences := []string{"2/5", "0/unbounded", "1/1"}
			for index, element := range sequenceElements {
				if got, want := element.Occurrences().String(), wantSequenceOccurrences[index]; got != want {
					t.Fatalf("sequence element %d occurrences = %q, want %q", index, got, want)
				}
				if element.Loc().Source() != "root.xsd" || element.Loc().IsZero() {
					t.Fatalf("sequence element %d location = %s, want root.xsd location", index, element.Loc())
				}
			}
			assertLocalTokenParticleReference(
				t,
				sequenceElements[0],
				mustTestQName(t, testXSDNamespace, "token"),
				schemaSimpleTypeAtomicToken,
				mustSchemaTokenLoc(t, "root.xsd", root, 17, `type="xs:token"`),
				ComponentID{},
				false,
			)
			assertLocalTokenParticleReference(
				t,
				sequenceElements[1],
				mustTestQName(t, "urn:root", "RootNMTOKEN"),
				schemaSimpleTypeAtomicNMTOKEN,
				mustSchemaTokenLoc(t, "root.xsd", root, 18, `type="r:RootNMTOKEN"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:root", "RootNMTOKEN")),
				true,
			)
			assertLocalTokenParticleEnumeration(t, sequenceElements[1], []string{"root-token"}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 5, `value="root-token"`),
			})
			assertLocalTokenParticleReference(
				t,
				sequenceElements[2],
				mustTestQName(t, "urn:root", "IncludedNMTOKEN"),
				schemaSimpleTypeAtomicNMTOKEN,
				mustSchemaTokenLoc(t, "root.xsd", root, 19, `type="r:IncludedNMTOKEN"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:root", "IncludedNMTOKEN")),
				true,
			)

			extended := localTokenParticleComplexType(t, schema, "Extended")
			extendedParticle, ok := extended.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("extension particle = %T, want SequenceParticle", extended.Particle())
			}
			if got, want := extendedParticle.Occurrences().String(), "0/2"; got != want {
				t.Fatalf("extension sequence occurrences = %q, want %q", got, want)
			}
			extendedElements := extendedParticle.Elements()
			if got, want := len(extendedElements), 2; got != want {
				t.Fatalf("extension element count = %d, want %d", got, want)
			}
			if got, want := extendedElements[0].Occurrences().String(), "2/4"; got != want {
				t.Fatalf("extension NMTOKEN occurrences = %q, want %q", got, want)
			}
			if got, want := extendedElements[1].Occurrences().String(), "0/unbounded"; got != want {
				t.Fatalf("extension token occurrences = %q, want %q", got, want)
			}
			assertLocalTokenParticleReference(
				t,
				extendedElements[0],
				mustTestQName(t, testXSDNamespace, "NMTOKEN"),
				schemaSimpleTypeAtomicNMTOKEN,
				mustSchemaTokenLoc(t, "root.xsd", root, 23, `type="xs:NMTOKEN"`),
				ComponentID{},
				false,
			)
			assertLocalTokenParticleReference(
				t,
				extendedElements[1],
				mustTestQName(t, "urn:root", "InheritedToken"),
				schemaSimpleTypeAtomicToken,
				mustSchemaTokenLoc(t, "root.xsd", root, 24, `type="r:InheritedToken"`),
				componentIDForName(t, schema, mustTestQName(t, "urn:root", "InheritedToken")),
				true,
			)

			repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverSchema: %v", err)
			}
			if !reflect.DeepEqual(before, repeated.Components()) {
				t.Fatal("repeated schema builds disagree on local token particle facts")
			}
			choiceAlternatives[0] = nil
			sequenceElements[0] = ElementParticle{}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating particle query results changed the completed schema")
			}
		})
	}
}

func localTokenParticleRoot(version XSDVersion) string {
	return fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" elementFormDefault="unqualified" version="%s">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="RootToken"><xs:restriction base="xs:token"><xs:enumeration value=" root "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="RootNMTOKEN"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="root-token"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="InheritedToken"><xs:restriction base="r:RootToken"/></xs:simpleType>
  <xs:simpleType name="ForwardNMTOKEN"><xs:restriction base="r:LaterNMTOKEN"/></xs:simpleType>
  <xs:complexType name="Choice"><xs:choice minOccurs="0" maxOccurs="2">
    <xs:element name="qualifiedToken" type="xs:token" minOccurs="0" maxOccurs="2" form="qualified" nillable="true" block="substitution"/>
    <xs:element name="namedToken" type="r:InheritedToken" minOccurs="1" maxOccurs="3"/>
    <xs:element name="builtinNMTOKEN" type="xs:NMTOKEN"/>
    <xs:element name="forwardNMTOKEN" type="r:ForwardNMTOKEN"/>
    <xs:element name="importedToken" type="o:ImportedToken"/>
    <xs:element name="includedNMTOKEN" type="r:IncludedNMTOKEN"/>
  </xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence minOccurs="2" maxOccurs="4">
    <xs:element name="sequenceToken" type="xs:token" minOccurs="2" maxOccurs="5"/>
    <xs:element name="sequenceNMTOKEN" type="r:RootNMTOKEN" minOccurs="0" maxOccurs="unbounded"/>
    <xs:element name="sequenceIncluded" type="r:IncludedNMTOKEN"/>
  </xs:sequence></xs:complexType>
  <xs:complexType name="Base"/>
  <xs:complexType name="Extended"><xs:complexContent><xs:extension base="r:Base"><xs:sequence minOccurs="0" maxOccurs="2">
    <xs:element name="extensionNMTOKEN" type="xs:NMTOKEN" minOccurs="2" maxOccurs="4"/>
    <xs:element name="extensionToken" type="r:InheritedToken" minOccurs="0" maxOccurs="unbounded"/>
  </xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:simpleType name="LaterNMTOKEN"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>
</xs:schema>`, testXSDNamespace, version)
}

func localTokenParticleComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", local))
	if len(components) != 1 {
		t.Fatalf("complex type %q matches = %d, want one", local, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("complex type %q has no definition view", local)
	}
	return definition
}

func assertLocalTokenParticleReference(
	t *testing.T,
	element ElementParticle,
	wantName QName,
	wantAtomicKind schemaSimpleTypeAtomicKind,
	wantLoc Loc,
	wantID ComponentID,
	wantNamed bool,
) {
	t.Helper()
	reference, ok := element.TypeReference()
	if !ok {
		t.Fatalf("local element %q has no type reference", element.Name())
	}
	if reference.Name() != wantName || reference.QName() != wantName || reference.Loc() != wantLoc {
		t.Fatalf("local element %q type reference = %q/%q/%s, want %q/%q/%s", element.Name(), reference.Name(), reference.QName(), reference.Loc(), wantName, wantName, wantLoc)
	}
	if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.VarietyLoc().IsZero() {
		t.Fatalf("local element %q variety facts = %q/%s, want located atomic restriction", element.Name(), reference.Variety(), reference.VarietyLoc())
	}
	if reference.facts == nil || reference.facts.atomicKind != wantAtomicKind {
		t.Fatalf("local element %q atomic facts = %#v, want %v", element.Name(), reference.facts, wantAtomicKind)
	}
	if reference.IsNamed() != wantNamed || reference.IsBuiltin() == wantNamed {
		t.Fatalf("local element %q reference kind = named:%t/builtin:%t, want named:%t", element.Name(), reference.IsNamed(), reference.IsBuiltin(), wantNamed)
	}
	gotID, hasID := reference.ComponentID()
	if hasID != wantNamed || gotID != wantID {
		t.Fatalf("local element %q reference ID = %v/%t, want %v/%t", element.Name(), gotID, hasID, wantID, wantNamed)
	}
	if gotID, hasID := element.TypeID(); hasID != wantNamed || gotID != wantID {
		t.Fatalf("local element %q particle ID = %v/%t, want %v/%t", element.Name(), gotID, hasID, wantID, wantNamed)
	}
	facets, ok := reference.facts.facets.(schemaStringFacetVariant)
	if !ok || facets.whiteSpace == nil || facets.whiteSpace.Value() != "collapse" || !facets.whiteSpace.Fixed() || !facets.whiteSpace.Loc().IsZero() {
		t.Fatalf("local element %q whitespace facts = %#v/%t, want fixed unlocated collapse", element.Name(), facets, ok)
	}
}

func assertLocalTokenParticleEnumeration(t *testing.T, element ElementParticle, wantValues []string, wantLocations []Loc) {
	t.Helper()
	reference, ok := element.TypeReference()
	if !ok || reference.facts == nil {
		t.Fatalf("local element %q has no type reference facts", element.Name())
	}
	facets, ok := reference.facts.facets.(schemaStringFacetVariant)
	if !ok || !facets.enumeration.HasEnumeration() {
		t.Fatalf("local element %q has no string enumeration facts", element.Name())
	}
	if got := facets.enumeration.Values(); !reflect.DeepEqual(got, wantValues) {
		t.Fatalf("local element %q enumeration values = %#v, want %#v", element.Name(), got, wantValues)
	}
	if got := facets.enumeration.Locations(); !reflect.DeepEqual(got, wantLocations) {
		t.Fatalf("local element %q enumeration locations = %#v, want %#v", element.Name(), got, wantLocations)
	}
}

func TestSchemaBridgeLocalTokenParticlesRemainConsumerUnsupported(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.1"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			for _, test := range []struct {
				name      string
				localType string
				local     string
			}{
				{name: "builtin token", localType: `type="xs:token"`, local: "token"},
				{name: "named NMTOKEN", localType: `type="r:NamedNMTOKEN"`, local: "nmtoken"},
			} {
				t.Run(test.name, func(t *testing.T) {
					root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:r="urn:root" targetNamespace="urn:root" version="%s">
  <xs:element name="root" type="r:Container"/>
  <xs:simpleType name="NamedNMTOKEN"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>
  <xs:complexType name="Container"><xs:sequence><xs:element name="%s" %s/></xs:sequence></xs:complexType>
</xs:schema>`, testXSDNamespace, profile.version, test.local, test.localType)
					schema := discoverLocalTokenParticleSchema(t, root, profile.policy)
					body := `<root xmlns="urn:root"><` + test.local + ` xmlns="">value</` + test.local + `></root>`
					assertLocalTokenParticleConsumersUnsupported(t, schema, body, test.name)
				})
			}
		})
	}
}

func requireLocalTokenElementParticle(t *testing.T, particle Particle, index int) ElementParticle {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("choice alternative %d = %T, want ElementParticle", index, particle)
	}
	return element
}

func discoverLocalTokenParticleSchema(t *testing.T, root string, policy LanguagePolicy) Schema {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	return schema
}

func assertLocalTokenParticleConsumersUnsupported(t *testing.T, schema Schema, body string, family string) {
	t.Helper()
	generated, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || generated != nil {
		t.Fatalf("GenerateGo %s result = (%q, %v), want explicit unsupported", family, generated, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || !errors.Is(generationErr, ErrUnsupported) {
		t.Fatalf("%s generation diagnostic = %s, want unsupported: %v", family, generationDiagnostic, generationErr)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(body)))
	if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
		t.Fatalf("%s validation error = %v, want explicit unsupported", family, validationErr)
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported {
		t.Fatalf("%s validation diagnostic = %s, want unsupported", family, validationDiagnostic)
	}
}
