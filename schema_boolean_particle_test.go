package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // Keep the local boolean particle contract together.
func TestSchemaBridgeBuildsBooleanLocalParticles(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" elementFormDefault="unqualified" version="%s">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Choice">
    <xs:choice>
      <xs:element name="qualified" type="xs:boolean" form="qualified"/>
      <xs:element name="named" type="r:Derived"/>
      <xs:element name="forward" type="r:Forward"/>
      <xs:element name="cross" type="o:Cross"/>
    </xs:choice>
  </xs:complexType>
  <xs:complexType name="Sequence">
    <xs:sequence>
      <xs:element name="builtin" type="xs:boolean" minOccurs="18446744073709551616" maxOccurs="18446744073709551617"/>
      <xs:element name="named" type="r:Derived"/>
      <xs:element name="forward" type="r:Forward"/>
      <xs:element name="cross" type="o:Cross" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Derived"><xs:restriction base="r:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:boolean"/></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Base"/></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="%s">
  <xs:simpleType name="Cross"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`

	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			fixtures := map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: formatBooleanParticleSchema(other, profile.version)},
			}
			schema, err := discoverTestSchemaWithPolicy(t, formatBooleanParticleSchema(root, profile.version), fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			components := schema.Components()
			if got, want := len(components), 6; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			for index, component := range components[:5] {
				if got, want := component.ID().Ordinal(), uint64(index+1); got != want {
					t.Fatalf("component %d ordinal = %d, want %d", index, got, want)
				}
			}
			if got, want := components[5].ID().Ordinal(), uint64(1); got != want {
				t.Fatalf("cross-document ordinal = %d, want %d", got, want)
			}

			choice := booleanParticleComplexType(t, schema, "Choice")
			choiceParticle, choiceOK := choice.Particle().(ChoiceParticle)
			if !choiceOK {
				t.Fatalf("choice particle = %T, want ChoiceParticle", choice.Particle())
			}
			if got, want := choiceParticle.Occurrences().String(), "1/1"; got != want {
				t.Fatalf("choice occurrences = %q, want %q", got, want)
			}
			if got, want := choiceParticle.Loc(), mustTestLoc(t, "root.xsd", 4, 5); got != want {
				t.Fatalf("choice location = %s, want %s", got, want)
			}
			choiceAlternatives := choiceParticle.Alternatives()
			if got, want := len(choiceAlternatives), 4; got != want {
				t.Fatalf("choice alternative count = %d, want %d", got, want)
			}
			wantChoiceNames := []string{"qualified", "named", "forward", "cross"}
			wantChoiceNamespaces := []string{"urn:root", "", "", ""}
			wantChoiceTypes := []QName{
				mustTestQName(t, testXSDNamespace, "boolean"),
				mustTestQName(t, "urn:root", "Derived"),
				mustTestQName(t, "urn:root", "Forward"),
				mustTestQName(t, "urn:other", "Cross"),
			}
			for index, alternative := range choiceAlternatives {
				element := requireBooleanElementParticle(t, alternative, index)
				if got := element.Name().Local(); got != wantChoiceNames[index] {
					t.Fatalf("choice alternative %d name = %q, want %q", index, got, wantChoiceNames[index])
				}
				if got := element.Name().Namespace(); got != wantChoiceNamespaces[index] {
					t.Fatalf("choice alternative %d namespace = %q, want %q", index, got, wantChoiceNamespaces[index])
				}
				if got := element.DeclaredType(); got != wantChoiceTypes[index] {
					t.Fatalf("choice alternative %d declared type = %q, want %q", index, got, wantChoiceTypes[index])
				}
				if got := element.Occurrences().String(); got != "1/1" {
					t.Fatalf("choice alternative %d occurrences = %q, want 1/1", index, got)
				}
			}
			if typeID, hasTypeID := requireBooleanElementParticle(t, choiceAlternatives[0], 0).TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("built-in choice type ID = (%v, %t), want zero,false", typeID, hasTypeID)
			}
			for index, componentIndex := range []int{2, 4, 5} {
				element := requireBooleanElementParticle(t, choiceAlternatives[index+1], index+1)
				if typeID, hasTypeID := element.TypeID(); !hasTypeID || typeID != components[componentIndex].ID() {
					t.Fatalf("choice alternative %d type ID = (%v, %t), want (%v, true)", index+1, typeID, hasTypeID, components[componentIndex].ID())
				}
			}

			sequence := booleanParticleComplexType(t, schema, "Sequence")
			sequenceParticle, sequenceOK := sequence.Particle().(SequenceParticle)
			if !sequenceOK {
				t.Fatalf("sequence particle = %T, want SequenceParticle", sequence.Particle())
			}
			sequenceElements := sequenceParticle.Elements()
			if got, want := len(sequenceElements), 4; got != want {
				t.Fatalf("sequence element count = %d, want %d", got, want)
			}
			if got, want := sequenceElements[0].Occurrences().String(), "18446744073709551616/18446744073709551617"; got != want {
				t.Fatalf("big integer occurrence range = %q, want %q", got, want)
			}
			if got, want := sequenceElements[3].Occurrences().String(), "1/unbounded"; got != want {
				t.Fatalf("unbounded occurrence range = %q, want %q", got, want)
			}
			if got, want := sequenceParticle.Loc(), mustTestLoc(t, "root.xsd", 12, 5); got != want {
				t.Fatalf("sequence location = %s, want %s", got, want)
			}
			wantSequenceNames := []string{"builtin", "named", "forward", "cross"}
			wantSequenceTypes := []QName{
				mustTestQName(t, testXSDNamespace, "boolean"),
				mustTestQName(t, "urn:root", "Derived"),
				mustTestQName(t, "urn:root", "Forward"),
				mustTestQName(t, "urn:other", "Cross"),
			}
			for index, element := range sequenceElements {
				if got, want := element.Loc(), mustTestLoc(t, "root.xsd", 13+index, 7); got != want {
					t.Fatalf("sequence element %d location = %s, want %s", index, got, want)
				}
				if got := element.Name(); got != mustTestQName(t, "", wantSequenceNames[index]) {
					t.Fatalf("sequence element %d name = %q, want %q", index, got, wantSequenceNames[index])
				}
				if got := element.DeclaredType(); got != wantSequenceTypes[index] {
					t.Fatalf("sequence element %d declared type = %q, want %q", index, got, wantSequenceTypes[index])
				}
			}
			if typeID, hasTypeID := sequenceElements[0].TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("built-in sequence type ID = (%v, %t), want zero,false", typeID, hasTypeID)
			}
			for index, componentIndex := range []int{2, 4, 5} {
				if typeID, hasTypeID := sequenceElements[index+1].TypeID(); !hasTypeID || typeID != components[componentIndex].ID() {
					t.Fatalf("sequence element %d type ID = (%v, %t), want (%v, true)", index+1, typeID, hasTypeID, components[componentIndex].ID())
				}
			}

			before := schema.Components()
			choiceAlternatives[0] = nil
			sequenceElements[0] = ElementParticle{}
			choiceCopy := choiceParticle.Alternatives()
			choiceElement := requireBooleanElementParticle(t, choiceCopy[0], 0)
			if got := choiceElement.Name().Local(); got != "qualified" {
				t.Fatalf("mutating choice query changed name to %q", got)
			}
			if got := sequenceParticle.Elements()[0].Name().Local(); got != "builtin" {
				t.Fatalf("mutating sequence query changed name to %q", got)
			}
			if got := sequenceParticle.Elements()[0].Occurrences().String(); got != "18446744073709551616/18446744073709551617" {
				t.Fatalf("mutating sequence query changed occurrences to %q", got)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating particle query results changed the completed schema")
			}

			repeated, err := discoverTestSchemaWithPolicy(t, formatBooleanParticleSchema(root, profile.version), fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverSchema: %v", err)
			}
			if !reflect.DeepEqual(before, repeated.Components()) {
				t.Fatal("repeated schema builds disagree")
			}
		})
	}
}

//nolint:gocognit // Keep local target failures, causes, and related locations together.
func TestSchemaBridgeRejectsInvalidBooleanLocalParticleTargets(t *testing.T) {
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	} {
		for _, test := range []struct {
			name        string
			body        string
			code        string
			cause       error
			related     int
			primaryLine int
			primaryCol  int
		}{
			{
				name:        "missing",
				body:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing" targetNamespace="urn:root"><xs:complexType name="Record"><xs:sequence><xs:element name="value" type="m:Missing"/></xs:sequence></xs:complexType></xs:schema>`,
				code:        diagnosticSchemaElementTypeUnresolvedCode,
				cause:       errSchemaElementTypeUnresolved,
				primaryLine: 1,
				primaryCol:  173,
			},
			{
				name:    "duplicate",
				body:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType><xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType><xs:complexType name="Record"><xs:choice><xs:element name="value" type="r:Flag"/></xs:choice></xs:complexType></xs:schema>`,
				code:    diagnosticSchemaGlobalDuplicateCode,
				cause:   errSchemaGlobalDeclarationDuplicate,
				related: 1,
			},
			{
				name:    "cyclic",
				body:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType><xs:complexType name="Record"><xs:sequence><xs:element name="value" type="r:One"/></xs:sequence></xs:complexType></xs:schema>`,
				code:    diagnosticSchemaSimpleTypeCycleCode,
				cause:   errSchemaSimpleTypeBaseCycle,
				related: 1,
			},
			{
				name:        "wrong kind",
				body:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:element name="Target"/><xs:complexType name="Record"><xs:choice><xs:element name="value" type="r:Target"/></xs:choice></xs:complexType></xs:schema>`,
				code:        diagnosticSchemaElementTypeWrongKindCode,
				cause:       errSchemaElementTypeWrongKind,
				related:     1,
				primaryLine: 1,
				primaryCol:  195,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.body, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted an invalid local boolean target")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Code() != test.code || diagnostic.Class() != FailureInvalid {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
				}
				if !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
				}
				if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
					t.Fatalf("diagnostic location = %v, want root.xsd location", diagnostic.Loc())
				}
				if test.primaryLine != 0 && (diagnostic.Loc().Line() != test.primaryLine || diagnostic.Loc().Column() != test.primaryCol) {
					t.Fatalf("diagnostic location = %v, want root.xsd:%d:%d", diagnostic.Loc(), test.primaryLine, test.primaryCol)
				}
				if got := len(diagnostic.Related()); got < test.related {
					t.Fatalf("related locations = %v, want at least %d", diagnostic.Related(), test.related)
				}
			})
		}
	}
}

func formatBooleanParticleSchema(schema string, version XSDVersion) string {
	return fmt.Sprintf(schema, version)
}

func booleanParticleComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	t.Helper()
	name := mustTestQName(t, "urn:root", local)
	components := schema.FindKind(ComponentKindComplexTypeDefinition, name)
	if len(components) != 1 {
		t.Fatalf("%s component count = %d, want 1", name, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s has no complex type definition", name)
	}
	return definition
}

func requireBooleanElementParticle(t *testing.T, particle Particle, index int) ElementParticle {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("particle %d type = %T, want ElementParticle", index, particle)
	}
	return element
}
