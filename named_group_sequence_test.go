package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestNamedGroupDirectSequenceBuildsImmutableDefinition(t *testing.T) { //nolint:gocognit // Keep the public sequence contract together.
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := namedGroupSequenceRoot(profile.version, `<xs:element ref="g:first"/>
      <xs:element ref="g:second" minOccurs="0" maxOccurs="unbounded"/>
      <xs:element ref="g:omitted" minOccurs="0" maxOccurs="0"/>
      <xs:element ref="g:finite" minOccurs="2" maxOccurs="5"/>
      <xs:element ref="g:huge" minOccurs="3" maxOccurs="18446744073709551616"/>`, `<xs:element name="first" type="xs:integer"/>
    <xs:element name="second" type="xs:integer"/>
    <xs:element name="omitted" type="xs:integer"/>
    <xs:element name="finite" type="xs:integer"/>
    <xs:element name="huge" type="xs:integer"/>`)
			var schema Schema
			for iteration := 0; iteration < 3; iteration++ {
				current, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover named model-group sequence: %v", err)
				}
				if iteration == 0 {
					schema = current
					continue
				}
				if !reflect.DeepEqual(schema.Components(), current.Components()) {
					t.Fatalf("repeat build %d changed ordered components", iteration)
				}
			}

			groupName := mustTestQName(t, "urn:named-group", "G")
			groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
			if len(groups) != 1 {
				t.Fatalf("model-group count = %d, want 1", len(groups))
			}
			definition, ok := groups[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("model-group definition view is missing")
			}
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("model-group particle = %T, want SequenceParticle", definition.Particle())
			}
			if sequence.Loc() != namedGroupChoiceLoc(t, root, "<xs:sequence>") {
				t.Fatalf("sequence location = %s, want sequence location", sequence.Loc())
			}
			if got, want := sequence.Occurrences().String(), "1/1"; got != want {
				t.Fatalf("sequence occurrences = %q, want %q", got, want)
			}
			particles := sequence.Particles()
			if got, want := len(particles), 4; got != want {
				t.Fatalf("sequence particle count = %d, want %d after 0/0 omission", got, want)
			}
			wantNames := []QName{
				mustTestQName(t, "urn:named-group", "first"),
				mustTestQName(t, "urn:named-group", "second"),
				mustTestQName(t, "urn:named-group", "finite"),
				mustTestQName(t, "urn:named-group", "huge"),
			}
			wantOccurrences := []string{"1/1", "0/unbounded", "2/5", "3/18446744073709551616"}
			assertNamedGroupElementReferenceParticles(t, schema, particles, wantNames, wantOccurrences, "particle")

			before := schema.Components()
			particles[0] = nil
			if sequence.Particles()[0] == nil {
				t.Fatal("mutating Particles changed the completed sequence")
			}
			minimum := sequence.Particles()[1].Occurrences().Minimum()
			minimum.value.SetInt64(99)
			if got, want := sequence.Particles()[1].Occurrences().String(), "0/unbounded"; got != want {
				t.Fatalf("mutating returned minimum changed the completed sequence: got %q, want %q", got, want)
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("model-group sequence queries mutated the completed schema")
			}
		})
	}
}

func TestNamedGroupDirectSequenceAllowsEmptyDefaultSequence(t *testing.T) {
	root := namedGroupSequenceRoot("1.1", "", "")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover empty named model-group sequence: %v", err)
	}
	groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:named-group", "G"))
	if len(groups) != 1 {
		t.Fatalf("model-group count = %d, want 1", len(groups))
	}
	definition, ok := groups[0].ModelGroupDefinition()
	if !ok {
		t.Fatal("empty model-group has no definition view")
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("empty model-group particle = %T, want SequenceParticle", definition.Particle())
	}
	if particles := sequence.Particles(); len(particles) != 0 {
		t.Fatalf("empty sequence particles = %d, want 0", len(particles))
	}
}

func TestNamedGroupDirectSequenceResolvesForwardIncludeImportAndChameleonTargets(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:named-group" xmlns:o="urn:named-other" targetNamespace="urn:named-group" version="1.1">
  <xs:include schemaLocation="child.xsd"/>
  <xs:import namespace="urn:named-other" schemaLocation="other.xsd"/>
  <xs:group name="G"><xs:sequence>
    <xs:element ref="g:included"/>
    <xs:element ref="g:forward"/>
    <xs:element ref="o:foreign"/>
  </xs:sequence></xs:group>
  <xs:element name="forward" type="xs:integer"/>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {
			id: "child.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="included" type="xs:integer"/>
</xs:schema>`,
		},
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-other"><xs:element name="foreign" type="xs:integer"/></xs:schema>`,
		},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err != nil {
		t.Fatalf("discover composed named-group sequence: %v", err)
	}
	assertNamedGroupComposedSequence(t, schema)
}

func assertNamedGroupComposedSequence(t *testing.T, schema Schema) {
	t.Helper()
	groupName := mustTestQName(t, "urn:named-group", "G")
	groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
	if len(groups) != 1 {
		t.Fatalf("model-group count = %d, want 1", len(groups))
	}
	definition, ok := groups[0].ModelGroupDefinition()
	if !ok {
		t.Fatal("composed group has no definition view")
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("composed group particle = %T, want SequenceParticle", definition.Particle())
	}
	particles := sequence.Particles()
	if got, want := len(particles), 3; got != want {
		t.Fatalf("composed sequence particle count = %d, want %d", got, want)
	}
	assertNamedGroupComposedSequenceMembers(t, schema, particles)
	targets := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:named-group", "included"))
	if len(targets) != 1 || targets[0].ID().Source() != "child.xsd" {
		t.Fatalf("chameleon target = %#v, want one child.xsd declaration", targets)
	}
}

func assertNamedGroupComposedSequenceMembers(t *testing.T, schema Schema, particles []Particle) {
	t.Helper()
	want := []struct {
		namespace string
		local     string
	}{
		{namespace: "urn:named-group", local: "included"},
		{namespace: "urn:named-group", local: "forward"},
		{namespace: "urn:named-other", local: "foreign"},
	}
	for index, expected := range want {
		reference, ok := particles[index].(ElementReferenceParticle)
		if !ok {
			t.Fatalf("particle %d = %T, want ElementReferenceParticle", index, particles[index])
		}
		name := mustTestQName(t, expected.namespace, expected.local)
		if reference.Name() != name || reference.Ref() != name {
			t.Fatalf("particle %d name = %q/%q, want %q", index, reference.Name(), reference.Ref(), name)
		}
		if reference.TargetID().IsZero() || reference.RefLoc().Source() != "root.xsd" {
			t.Fatalf("particle %d target/location = %v/%s, want allocated/root.xsd", index, reference.TargetID(), reference.RefLoc())
		}
		targets := schema.FindKind(ComponentKindElementDeclaration, name)
		if len(targets) != 1 || reference.TargetID() != targets[0].ID() {
			t.Fatalf("particle %d target ID = %v, want %v", index, reference.TargetID(), targets[0].ID())
		}
	}
}

func assertNamedGroupElementReferenceParticles(t *testing.T, schema Schema, particles []Particle, wantNames []QName, wantOccurrences []string, label string) {
	t.Helper()
	for index, wantName := range wantNames {
		reference, ok := particles[index].(ElementReferenceParticle)
		if !ok {
			t.Fatalf("%s %d = %T, want ElementReferenceParticle", label, index, particles[index])
		}
		if reference.Name() != wantName || reference.Ref() != wantName {
			t.Fatalf("%s %d name = %q/%q, want %q", label, index, reference.Name(), reference.Ref(), wantName)
		}
		if got := reference.Occurrences().String(); got != wantOccurrences[index] {
			t.Fatalf("%s %d occurrences = %q, want %q", label, index, got, wantOccurrences[index])
		}
		if reference.Loc().Source() != "root.xsd" || reference.RefLoc().Source() != "root.xsd" {
			t.Fatalf("%s %d locations = %s/%s, want root.xsd", label, index, reference.Loc(), reference.RefLoc())
		}
		targets := schema.FindKind(ComponentKindElementDeclaration, wantName)
		if len(targets) != 1 || reference.TargetID() != targets[0].ID() {
			t.Fatalf("%s %d target ID = %v, want %v", label, index, reference.TargetID(), targets[0].ID())
		}
	}
}

func TestNamedGroupDirectSequenceReferenceDiagnostics(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
		specRef string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0", specRef: schemaElementReferenceXSD11SpecRef},
		{name: "Strict10", policy: Strict10, version: "1.0", specRef: schemaElementReferenceXSD10SpecRef},
		{name: "Strict11", policy: Strict11, version: "1.1", specRef: schemaElementReferenceXSD11SpecRef},
	}
	for _, profile := range profiles {
		t.Run(profile.name+"/missing", func(t *testing.T) {
			root := namedGroupSequenceRoot(profile.version, `<xs:element ref="g:missing"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertNamedGroupSequenceReferenceDiagnostic(t, schema, err, profile.specRef, diagnosticSchemaElementReferenceUnresolvedCode, errSchemaElementReferenceUnresolved, namedGroupChoiceLoc(t, root, `ref="g:missing"`), nil)
		})
		t.Run(profile.name+"/missing zero range", func(t *testing.T) {
			root := namedGroupSequenceRoot(profile.version, `<xs:element ref="g:missing" minOccurs="0" maxOccurs="0"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertNamedGroupSequenceReferenceDiagnostic(t, schema, err, profile.specRef, diagnosticSchemaElementReferenceUnresolvedCode, errSchemaElementReferenceUnresolved, namedGroupChoiceLoc(t, root, `ref="g:missing"`), nil)
		})
		t.Run(profile.name+"/wrong kind", func(t *testing.T) {
			root := namedGroupSequenceRoot(profile.version, `<xs:element ref="g:notElement"/>`, `<xs:simpleType name="notElement"><xs:restriction base="xs:integer"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			related := []Loc{namedGroupChoiceLoc(t, root, `<xs:simpleType name="notElement"`)}
			assertNamedGroupSequenceReferenceDiagnostic(t, schema, err, profile.specRef, diagnosticSchemaElementReferenceWrongKindCode, errSchemaElementReferenceWrongKind, namedGroupChoiceLoc(t, root, `ref="g:notElement"`), related)
		})
	}

	root := namedGroupSequenceRoot("1.1", `<xs:element ref="g:item"/><xs:element ref="g:item" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="item" type="xs:integer"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	assertNamedGroupSequenceReferenceDiagnostic(t, schema, err, schemaElementReferenceDuplicateXSD11SpecRef, diagnosticSchemaElementReferenceDuplicateCode, errSchemaElementReferenceDuplicate, namedGroupLastLoc(t, root, `ref="g:item"`), []Loc{namedGroupChoiceLoc(t, root, `ref="g:item"`)})
}

func assertNamedGroupSequenceReferenceDiagnostic(t *testing.T, schema Schema, err error, specRef, code string, cause error, loc Loc, related []Loc) {
	t.Helper()
	if err == nil {
		t.Fatal("named-group sequence reference failure returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != code || diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic = %s/%q, want invalid/%q/%q", diagnostic, diagnostic.SpecRef(), code, specRef)
	}
	if diagnostic.Loc() != loc || !reflect.DeepEqual(diagnostic.Related(), related) {
		t.Fatalf("diagnostic locations = %s/%v, want %s/%v", diagnostic.Loc(), diagnostic.Related(), loc, related)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("diagnostic cause does not match %v: %v", cause, err)
	}
}

func TestNamedGroupDirectSequenceRejectsInvisibleAndAmbiguousTargets(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:named-group" xmlns:o="urn:named-other" targetNamespace="urn:named-group" version="1.1">
  <xs:include schemaLocation="child-import.xsd"/>
  <xs:group name="G"><xs:sequence><xs:element ref="o:foreign"/></xs:sequence></xs:group>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child-import.xsd": {
			id:       "child-import.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-group"><xs:import namespace="urn:named-other" schemaLocation="other.xsd"/></xs:schema>`,
		},
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-other"><xs:element name="foreign" type="xs:integer"/></xs:schema>`,
		},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	otherElementIndex := strings.Index(fixtures["other.xsd"].contents, `<xs:element name="foreign"`)
	if otherElementIndex < 0 {
		t.Fatal("other.xsd fixture is missing the foreign element marker")
	}
	otherElementLoc := mustTestLoc(t, "other.xsd", 1, otherElementIndex+1)
	assertNamedGroupSequenceReferenceDiagnostic(t, schema, err, schemaElementReferenceImportXSD11SpecRef, diagnosticSchemaElementReferenceNamespaceCode, errSchemaElementReferenceNamespace, namedGroupChoiceLoc(t, root, `ref="o:foreign"`), []Loc{otherElementLoc})

	name := mustTestQName(t, "urn:named-group", "item")
	owner := schemaComponentRecord{
		id:   ComponentID{source: "root.xsd", ordinal: 1},
		kind: ComponentKindModelGroupDefinition,
		name: mustTestQName(t, "urn:named-group", "G"),
		loc:  mustTestLoc(t, "root.xsd", 2, 3),
	}
	first := schemaComponentRecord{
		id:   ComponentID{source: "one.xsd", ordinal: 1},
		kind: ComponentKindElementDeclaration,
		name: name,
		loc:  mustTestLoc(t, "one.xsd", 2, 3),
	}
	second := schemaComponentRecord{
		id:   ComponentID{source: "two.xsd", ordinal: 1},
		kind: ComponentKindElementDeclaration,
		name: name,
		loc:  mustTestLoc(t, "two.xsd", 2, 3),
	}
	referenceLoc := mustTestLoc(t, "root.xsd", 3, 25)
	occurrences := namedGroupTestOccurrenceRange(t, "0", "0")
	sequenceInput := &schemaSequenceParticleInput{
		loc:         mustTestLoc(t, "root.xsd", 3, 5),
		occurrences: occurrences,
		particles: []schemaParticleTermInput{schemaElementParticleInput{
			loc:         mustTestLoc(t, "root.xsd", 3, 5),
			reference:   &schemaElementReferenceInput{name: name, loc: referenceLoc},
			occurrences: occurrences,
		}},
	}
	_, err = resolveSchemaModelGroupSequenceParticle(sequenceInput, owner, []schemaComponentRecord{owner, first, second}, map[QName][]int{name: {1, 2}}, map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}}, XSDVersion11)
	if err == nil {
		t.Fatal("ambiguous zero-range sequence reference returned a particle")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaElementReferenceAmbiguousCode || diagnostic.SpecRef() != schemaElementReferenceXSD11SpecRef {
		t.Fatalf("ambiguous sequence diagnostic = %s/%q, want ambiguous/XSD 1.1 reference", diagnostic, diagnostic.SpecRef())
	}
	if diagnostic.Loc() != referenceLoc || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) {
		t.Fatalf("ambiguous sequence locations = %s/%v", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaElementReferenceAmbiguous) {
		t.Fatalf("ambiguous sequence cause is not preserved: %v", err)
	}
}

func TestNamedGroupDirectSequenceRejectsUnsupportedShapes(t *testing.T) {
	for _, test := range []struct {
		name   string
		model  string
		marker string
	}{
		{name: "local element", model: `<xs:sequence><xs:element name="local" type="xs:integer"/></xs:sequence>`, marker: `<xs:element name="local"`},
		{name: "nested choice", model: `<xs:sequence><xs:choice/></xs:sequence>`, marker: `<xs:choice`},
		{name: "group reference", model: `<xs:sequence><xs:group ref="g:Other"/></xs:sequence>`, marker: `ref="g:Other"`},
		{name: "wildcard", model: `<xs:sequence><xs:any/></xs:sequence>`, marker: `<xs:any`},
		{name: "all model", model: `<xs:all/>`, marker: `<xs:all`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := namedGroupModelRoot("1.1", test.model)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertNamedGroupChoiceUnsupported(t, schema, err, root, test.marker)
		})
	}

	root := namedGroupModelRoot("1.1", `<xs:sequence minOccurs="0"><xs:element ref="g:item"/></xs:sequence>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("named-group sequence compositor occurrence returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `minOccurs="0"`) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("compositor occurrence diagnostic = %s, want located unsupported", diagnostic)
	}
}

func TestNamedGroupSequenceConsumersRejectGroupReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-ref" targetNamespace="urn:group-ref" version="1.1">
  <xs:element name="root" type="g:Container"/>
  <xs:complexType name="Container"><xs:group ref="g:G"/></xs:complexType>
  <xs:group name="G"><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover sequence-backed consumer-gate schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:group-ref"><item>1</item></root>`)))
	if validationErr == nil || !errors.Is(validationErr, errInstanceModelGroupReference) || !errors.Is(validationErr, ErrUnsupported) {
		t.Fatalf("validation error = %v, want explicit unsupported model-group reference", validationErr)
	}
	output, generationErr := GenerateGo(schema, "generated")
	if output != nil || generationErr == nil || !errors.Is(generationErr, errCodegenDirectModelGroupReference) || !errors.Is(generationErr, ErrUnsupported) {
		t.Fatalf("generation result = (%q, %v), want explicit unsupported model-group reference", output, generationErr)
	}
}

func TestNamedGroupSequenceResolutionPreservesExactOuterRange(t *testing.T) {
	name := mustTestQName(t, "urn:named-group", "item")
	owner := schemaComponentRecord{
		id:   ComponentID{source: "root.xsd", ordinal: 1},
		kind: ComponentKindModelGroupDefinition,
		name: mustTestQName(t, "urn:named-group", "G"),
		loc:  mustTestLoc(t, "root.xsd", 2, 3),
	}
	target := schemaComponentRecord{
		id:   ComponentID{source: "root.xsd", ordinal: 2},
		kind: ComponentKindElementDeclaration,
		name: name,
		loc:  mustTestLoc(t, "root.xsd", 4, 3),
	}
	outer := namedGroupTestOccurrenceRange(t, "2", "unbounded")
	member := namedGroupTestOccurrenceRange(t, "0", "18446744073709551616")
	input := &schemaSequenceParticleInput{
		loc:         mustTestLoc(t, "root.xsd", 3, 5),
		occurrences: outer,
		particles: []schemaParticleTermInput{schemaElementParticleInput{
			loc:         mustTestLoc(t, "root.xsd", 3, 7),
			reference:   &schemaElementReferenceInput{name: name, loc: mustTestLoc(t, "root.xsd", 3, 25)},
			occurrences: member,
		}},
	}
	particle, err := resolveSchemaModelGroupSequenceParticle(input, owner, []schemaComponentRecord{owner, target}, map[QName][]int{name: {1}}, map[SourceID][]SourceID{"root.xsd": {"root.xsd"}}, XSDVersion11)
	if err != nil {
		t.Fatalf("resolve exact outer sequence range: %v", err)
	}
	sequence, ok := particle.(SequenceParticle)
	if !ok {
		t.Fatalf("resolved particle = %T, want SequenceParticle", particle)
	}
	if got, want := sequence.Occurrences().String(), "2/unbounded"; got != want {
		t.Fatalf("outer occurrences = %q, want %q", got, want)
	}
	children := sequence.Particles()
	if len(children) != 1 || children[0].Occurrences().String() != "0/18446744073709551616" {
		t.Fatalf("member particles = %#v, want exact member occurrence", children)
	}
}

func namedGroupSequenceRoot(version, members, declarations string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:named-group" targetNamespace="urn:named-group" version="` + version + `">
  <xs:group name="G"><xs:sequence>` + members + `</xs:sequence></xs:group>` + declarations + `
</xs:schema>`
}
