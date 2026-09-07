package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func TestSchemaBridgeModelsLocalStringParticlesAcrossPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
		wantXSD XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0", wantXSD: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: "1.0", wantXSD: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: "1.1", wantXSD: XSDVersion11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := localStringParticleTestSchema(test.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			if schema.LanguagePolicy() != test.policy {
				t.Fatalf("schema policy = %q, want %q", schema.LanguagePolicy(), test.policy)
			}
			assertLocalStringChoiceFacts(t, schema, test.wantXSD, root)
			assertLocalStringSequenceFacts(t, schema, test.wantXSD)
			assertLocalStringExtensionFacts(t, schema)
		})
	}
}

func localStringParticleTestSchema(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:string-particles" targetNamespace="urn:string-particles" elementFormDefault="qualified" version="` + version + `">
  <xs:complexType name="Choice"><xs:choice minOccurs="0" maxOccurs="unbounded">
    <xs:element name="builtin" type="xs:string" nillable="true" block="substitution"/>
    <xs:element name="named" type="r:Text" minOccurs="0" maxOccurs="unbounded"/>
    <xs:element name="absent" type="xs:string" minOccurs="0" maxOccurs="0"/>
  </xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence minOccurs="0" maxOccurs="2">
    <xs:element name="first" type="xs:string" form="unqualified" minOccurs="2" maxOccurs="3"/>
    <xs:element name="absent" type="xs:string" minOccurs="0" maxOccurs="0"/>
    <xs:element name="last" type="r:Inherited" minOccurs="0" maxOccurs="unbounded"/>
  </xs:sequence></xs:complexType>
  <xs:complexType name="Empty"/>
  <xs:complexType name="Extended"><xs:complexContent><xs:extension base="r:Empty"><xs:sequence minOccurs="0" maxOccurs="2">
    <xs:element name="extension" type="xs:string"/>
    <xs:element name="extension-named" type="r:Text"/>
  </xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:simpleType name="Text"><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"/><xs:enumeration value=" first "/><xs:enumeration value=""/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Inherited"><xs:restriction base="r:Text"><xs:enumeration value="first"/></xs:restriction></xs:simpleType>
</xs:schema>`
}

//nolint:gocognit // Keep exact atomic-kind exclusion and no-schema evidence together.
func TestSchemaBridgeRejectsNamedTokenDerivedLocalParticles(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:token-particles" targetNamespace="urn:token-particles" version="` + string(profile.version) + `">
  <xs:complexType name="Container"><xs:` + model + `><xs:element name="item" type="r:Token"/></xs:` + model + `></xs:complexType>
  <xs:simpleType name="Token"><xs:restriction base="xs:token"/></xs:simpleType>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted a named token-derived local particle")
				}
				assertTokenNoSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Class(), diagnostic.Feature())
				}
				wantSpec := "xsd11-structures#cSchemaDocument"
				if profile.version == XSDVersion10 {
					wantSpec = "xsd10-structures#schema-document"
				}
				if diagnostic.SpecRef() != wantSpec || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="r:Token"`) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic evidence = %s/%q, want located %s unsupported", diagnostic.Loc(), diagnostic.SpecRef(), wantSpec)
				}
			})
		}
	}
}

func localStringParticleTestComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	t.Helper()
	found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:string-particles", local))
	if len(found) != 1 {
		t.Fatalf("complex type %s count = %d, want one; components = %#v", local, len(found), schema.Components())
	}
	definition, ok := found[0].ComplexType()
	if !ok {
		t.Fatalf("complex type %s has no view", local)
	}
	return definition
}

//nolint:gocognit,funlen // Keep the complete choice particle fact contract together.
func assertLocalStringChoiceFacts(t *testing.T, schema Schema, wantXSD XSDVersion, root string) {
	t.Helper()
	definition := localStringParticleTestComplexType(t, schema, "Choice")
	choice, ok := definition.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("Choice particle = %T, want ChoiceParticle", definition.Particle())
	}
	if choice.Occurrences().String() != "0/unbounded" {
		t.Fatalf("Choice occurrences = %s, want 0/unbounded", choice.Occurrences())
	}
	alternatives := choice.Alternatives()
	if len(alternatives) != 2 {
		t.Fatalf("Choice alternatives = %d, want 2 after 0/0 omission", len(alternatives))
	}
	builtin := localStringParticleTestElement(t, alternatives[0])
	named := localStringParticleTestElement(t, alternatives[1])
	if builtin.Name() != mustTestQName(t, "urn:string-particles", "builtin") || builtin.DeclaredType() != mustTestQName(t, testXSDNamespace, "string") {
		t.Fatalf("builtin facts = %s/%s, want qualified builtin and xs:string", builtin.Name(), builtin.DeclaredType())
	}
	if builtin.Occurrences().String() != "1/1" || !builtin.IsNillable() || !reflect.DeepEqual(builtin.DisallowedSubstitutions(), []string{"substitution"}) {
		t.Fatalf("builtin particle facts = occurrence %s, nillable %t, block %v", builtin.Occurrences(), builtin.IsNillable(), builtin.DisallowedSubstitutions())
	}
	builtinReference, ok := builtin.TypeReference()
	if !ok || !builtinReference.IsBuiltin() || builtinReference.Name() != builtin.DeclaredType() {
		t.Fatalf("builtin type reference = %#v/%t, want xs:string builtin", builtinReference, ok)
	}
	if _, hasID := builtin.TypeID(); hasID {
		t.Fatal("builtin local string has a synthetic type identity")
	}
	if builtinReference.StringEnumerationFacets().HasEnumeration() || builtinReference.StringEnumerationFacets().Values() != nil {
		t.Fatal("builtin xs:string unexpectedly has an enumeration facet")
	}
	whiteSpace, ok := builtinReference.StringWhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "preserve" || !whiteSpace.Loc().IsZero() || whiteSpace.Fixed() {
		t.Fatalf("builtin whiteSpace = (%q, %s, fixed=%t), want preserve/default", whiteSpace.Value(), whiteSpace.Loc(), whiteSpace.Fixed())
	}
	if builtinReference.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, "type") {
		t.Fatalf("builtin type location = %s, want type attribute", builtinReference.Loc())
	}

	if named.Name() != mustTestQName(t, "urn:string-particles", "named") || named.Occurrences().String() != "0/unbounded" {
		t.Fatalf("named particle facts = %s/%s, want named and 0/unbounded", named.Name(), named.Occurrences())
	}
	wantType := mustTestQName(t, "urn:string-particles", "Text")
	if named.DeclaredType() != wantType {
		t.Fatalf("named declared type = %s, want %s", named.DeclaredType(), wantType)
	}
	namedReference, ok := named.TypeReference()
	if !ok || !namedReference.IsNamed() || namedReference.Name() != wantType {
		t.Fatalf("named type reference = %#v/%t, want %s", namedReference, ok, wantType)
	}
	typeID, hasTypeID := named.TypeID()
	referenceID, hasReferenceID := namedReference.ComponentID()
	if !hasTypeID || !hasReferenceID || typeID != referenceID || typeID.Source() != "root.xsd" {
		t.Fatalf("named type identities = %v/%t and %v/%t, want matching root.xsd identity", typeID, hasTypeID, referenceID, hasReferenceID)
	}
	enumeration := namedReference.StringEnumerationFacets()
	if !enumeration.HasEnumeration() || !reflect.DeepEqual(enumeration.Values(), []string{" first ", ""}) || enumeration.Version() != wantXSD {
		t.Fatalf("named enumeration = has=%t values=%v version=%s, want true,[ spaced-first,empty],%s", enumeration.HasEnumeration(), enumeration.Values(), enumeration.Version(), wantXSD)
	}
	if len(enumeration.Locations()) != 2 || len(enumeration.Declarations()) != 2 || enumeration.Locations()[0].IsZero() || enumeration.Locations()[1].IsZero() {
		t.Fatalf("named enumeration locations/provenance = %v/%v, want two located declarations", enumeration.Locations(), enumeration.Declarations())
	}
	whiteSpace, ok = namedReference.StringWhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "collapse" || whiteSpace.Loc().IsZero() || whiteSpace.Fixed() {
		t.Fatalf("named whiteSpace = (%q, %s, fixed=%t), want located collapse", whiteSpace.Value(), whiteSpace.Loc(), whiteSpace.Fixed())
	}
	values := enumeration.Values()
	values[0] = "changed"
	if namedReference.StringEnumerationFacets().Values()[0] != " first " {
		t.Fatal("mutating enumeration values changed the immutable particle facts")
	}
}

func assertLocalStringSequenceFacts(t *testing.T, schema Schema, wantXSD XSDVersion) {
	t.Helper()
	definition := localStringParticleTestComplexType(t, schema, "Sequence")
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("Sequence particle = %T, want SequenceParticle", definition.Particle())
	}
	if sequence.Occurrences().String() != "0/2" {
		t.Fatalf("Sequence occurrences = %s, want 0/2", sequence.Occurrences())
	}
	particles := sequence.Particles()
	if len(particles) != 2 || len(sequence.Elements()) != 2 {
		t.Fatalf("Sequence particles/elements = %d/%d, want 2/2 after 0/0 omission", len(particles), len(sequence.Elements()))
	}
	first := localStringParticleTestElement(t, particles[0])
	last := localStringParticleTestElement(t, particles[1])
	if first.Name() != mustTestQName(t, "", "first") || first.Occurrences().String() != "2/3" {
		t.Fatalf("first sequence particle = %s/%s, want unqualified first/2/3", first.Name(), first.Occurrences())
	}
	if last.Name() != mustTestQName(t, "urn:string-particles", "last") || last.Occurrences().String() != "0/unbounded" {
		t.Fatalf("last sequence particle = %s/%s, want qualified last/0/unbounded", last.Name(), last.Occurrences())
	}
	lastReference, ok := last.TypeReference()
	if !ok || !lastReference.IsNamed() || lastReference.Name() != mustTestQName(t, "urn:string-particles", "Inherited") {
		t.Fatalf("last sequence type reference = %#v/%t, want Inherited", lastReference, ok)
	}
	if lastReference.StringEnumerationFacets().Version() != wantXSD || !reflect.DeepEqual(lastReference.StringEnumerationFacets().Values(), []string{"first"}) {
		t.Fatalf("inherited sequence enumeration = %v/%s, want [first]/%s", lastReference.StringEnumerationFacets().Values(), lastReference.StringEnumerationFacets().Version(), wantXSD)
	}
	particles[0] = nil
	if sequence.Particles()[0] == nil {
		t.Fatal("mutating Particles changed the immutable sequence")
	}
}

func assertLocalStringExtensionFacts(t *testing.T, schema Schema) {
	t.Helper()
	definition := localStringParticleTestComplexType(t, schema, "Extended")
	if definition.Base() != mustTestQName(t, "urn:string-particles", "Empty") {
		t.Fatalf("extension base = %s, want Empty", definition.Base())
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("Extended particle = %T, want SequenceParticle", definition.Particle())
	}
	if sequence.Occurrences().String() != "0/2" || len(sequence.Elements()) != 2 {
		t.Fatalf("Extended particle facts = %s/%d, want 0/2 and two elements", sequence.Occurrences(), len(sequence.Elements()))
	}
	if sequence.Elements()[0].Name() != mustTestQName(t, "urn:string-particles", "extension") || sequence.Elements()[1].Name() != mustTestQName(t, "urn:string-particles", "extension-named") {
		t.Fatalf("Extended element order = %v, want extension then extension-named", sequence.Elements())
	}
}

//nolint:gocognit // Keep graph visibility, identity, and facet provenance together.
func TestSchemaBridgeResolvesGraphVisibleLocalStringTypes(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:string-graph" xmlns:o="urn:string-other" targetNamespace="urn:string-graph" version="` + test.version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:string-other" schemaLocation="other.xsd"/>
  <xs:complexType name="Graph"><xs:sequence>
    <xs:element name="included" type="r:IncludedText"/>
    <xs:element name="imported" type="o:ImportedText"/>
  </xs:sequence></xs:complexType>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"chameleon.xsd": {
					id:       "chameleon.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="IncludedText"><xs:restriction base="xs:string"><xs:whiteSpace value="replace"/></xs:restriction></xs:simpleType></xs:schema>`,
				},
				"other.xsd": {
					id:       "other.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:string-other"><xs:simpleType name="ImportedText"><xs:restriction base="xs:string"><xs:enumeration value="remote"/></xs:restriction></xs:simpleType></xs:schema>`,
				},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:string-graph", "Graph"))
			if len(found) != 1 {
				t.Fatalf("complex type Graph count = %d, want one", len(found))
			}
			definition, ok := found[0].ComplexType()
			if !ok {
				t.Fatal("Graph has no complex type view")
			}
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok || len(sequence.Elements()) != 2 {
				t.Fatalf("Graph particle = %#v, want two-element sequence", definition.Particle())
			}
			included := sequence.Elements()[0]
			imported := sequence.Elements()[1]
			includedID, includedOK := included.TypeID()
			importedID, importedOK := imported.TypeID()
			if !includedOK || includedID.Source() != "chameleon.xsd" || !importedOK || importedID.Source() != "other.xsd" {
				t.Fatalf("graph type identities = %v/%t and %v/%t, want chameleon/imported sources", includedID, includedOK, importedID, importedOK)
			}
			includedReference, ok := included.TypeReference()
			if !ok || includedReference.Name() != mustTestQName(t, "urn:string-graph", "IncludedText") {
				t.Fatalf("included type reference = %#v/%t, want chameleon-expanded QName", includedReference, ok)
			}
			includedWhiteSpace, ok := includedReference.StringWhiteSpaceFacet()
			if !ok || includedWhiteSpace.Value() != "replace" || includedWhiteSpace.Loc().Source() != "chameleon.xsd" {
				t.Fatalf("included whiteSpace = (%q, %s), want replace from chameleon.xsd", includedWhiteSpace.Value(), includedWhiteSpace.Loc())
			}
			importedReference, ok := imported.TypeReference()
			if !ok || importedReference.Name() != mustTestQName(t, "urn:string-other", "ImportedText") {
				t.Fatalf("imported type reference = %#v/%t, want imported QName", importedReference, ok)
			}
			if !reflect.DeepEqual(importedReference.StringEnumerationFacets().Values(), []string{"remote"}) {
				t.Fatalf("imported enumeration = %v, want [remote]", importedReference.StringEnumerationFacets().Values())
			}
		})
	}
}

func localStringParticleTestElement(t *testing.T, particle Particle) ElementParticle {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("particle = %T, want ElementParticle", particle)
	}
	return element
}
