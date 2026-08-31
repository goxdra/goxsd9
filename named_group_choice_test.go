package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNamedGroupDirectChoiceRejectsExcludedLocalDeclarations(t *testing.T) {
	tests := []struct {
		name      string
		policy    LanguagePolicy
		version   string
		groupName string
		children  string
		marker    string
	}{
		{
			name:      "XSD 1.0 structures schemaTop",
			policy:    Strict10,
			version:   "1.0",
			groupName: "schemaTop",
			children:  `<xs:element name="schema" type="xs:string"/>`,
			marker:    `<xs:element name="schema"`,
		},
		{
			name:      "XSD 1.1 structures schemaTop",
			policy:    Strict11,
			version:   "1.1",
			groupName: "schemaTop",
			children:  `<xs:element name="schema" type="xs:string"/><xs:element name="component" type="xs:string"/>`,
			marker:    `<xs:element name="schema"`,
		},
		{
			name:      "XSD 1.1 datatypes datatypeTop",
			policy:    Strict11,
			version:   "1.1",
			groupName: "datatypeTop",
			children:  `<xs:element name="decimal" type="xs:string"/>`,
			marker:    `<xs:element name="decimal"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := namedGroupChoiceSchema(test.version, test.groupName, "", "", test.children)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			assertNamedGroupChoiceUnsupported(t, schema, err, root, test.marker)
		})
	}
}

func TestNamedGroupDirectChoiceBuildsImmutableDefinition(t *testing.T) { //nolint:gocognit,funlen // Keep the immutable public component contract together.
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
			root := namedGroupReferenceRoot(profile.version, `<xs:element ref="g:first"/>
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
					t.Fatalf("discover named model-group choice: %v", err)
				}
				if iteration == 0 {
					schema = current
					continue
				}
				if !reflect.DeepEqual(schema.Components(), current.Components()) {
					t.Fatalf("repeat build %d changed ordered components", iteration)
				}
			}
			components := schema.Components()
			if got, want := len(components), 6; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			groupName := mustTestQName(t, "urn:named-group", "G")
			groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
			if len(groups) != 1 {
				t.Fatalf("model-group count = %d, want 1", len(groups))
			}
			group := groups[0]
			if group.ID().Ordinal() != 1 || group.Kind() != ComponentKindModelGroupDefinition {
				t.Fatalf("group identity = %v/%q, want ordinal 1/model group", group.ID(), group.Kind())
			}
			definition, ok := group.ModelGroupDefinition()
			if !ok {
				t.Fatal("model-group definition view is missing")
			}
			alias, ok := group.ModelGroup()
			if !ok || alias.ID() != definition.ID() {
				t.Fatal("model-group accessor aliases disagree")
			}
			if definition.Component() != group || definition.ID() != group.ID() || definition.Name() != groupName || definition.Loc() != group.Loc() {
				t.Fatal("model-group view does not preserve generic component facts")
			}
			if _, complexOK := group.ComplexTypeDefinition(); complexOK {
				t.Fatal("model-group was exposed as a complex type")
			}
			choice, ok := definition.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("model-group particle = %T, want ChoiceParticle", definition.Particle())
			}
			if choice.Loc() != namedGroupChoiceLoc(t, root, "<xs:choice>") {
				t.Fatalf("choice location = %s, want choice location", choice.Loc())
			}
			if got, want := choice.Occurrences().String(), "1/1"; got != want {
				t.Fatalf("choice occurrences = %q, want %q", got, want)
			}
			alternatives := choice.Alternatives()
			if got, want := len(alternatives), 4; got != want {
				t.Fatalf("alternative count = %d, want %d after 0/0 omission", got, want)
			}
			wantNames := []QName{
				mustTestQName(t, "urn:named-group", "first"),
				mustTestQName(t, "urn:named-group", "second"),
				mustTestQName(t, "urn:named-group", "finite"),
				mustTestQName(t, "urn:named-group", "huge"),
			}
			wantOccurrences := []string{"1/1", "0/unbounded", "2/5", "3/18446744073709551616"}
			for index, wantName := range wantNames {
				reference, ok := alternatives[index].(ElementReferenceParticle)
				if !ok {
					t.Fatalf("alternative %d = %T, want ElementReferenceParticle", index, alternatives[index])
				}
				if reference.Name() != wantName || reference.Ref() != wantName {
					t.Fatalf("alternative %d name = %q/%q, want %q", index, reference.Name(), reference.Ref(), wantName)
				}
				if got := reference.Occurrences().String(); got != wantOccurrences[index] {
					t.Fatalf("alternative %d occurrences = %q, want %q", index, got, wantOccurrences[index])
				}
				if reference.Loc().Source() != "root.xsd" || reference.RefLoc().Source() != "root.xsd" {
					t.Fatalf("alternative %d locations = %s/%s, want root.xsd", index, reference.Loc(), reference.RefLoc())
				}
				targets := schema.FindKind(ComponentKindElementDeclaration, wantName)
				if len(targets) != 1 || reference.TargetID() != targets[0].ID() {
					t.Fatalf("alternative %d target ID = %v, want %v", index, reference.TargetID(), targets[0].ID())
				}
			}
			if got, want := schema.Find(groupName)[0].ID(), group.ID(); got != want {
				t.Fatalf("Find returned ID %v, want %v", got, want)
			}
			walked := make([]ComponentID, 0, len(components))
			if err := schema.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if len(walked) != len(components) {
				t.Fatalf("walk count = %d, want %d", len(walked), len(components))
			}
			for index, component := range components {
				if walked[index] != component.ID() || component.ID().Ordinal() != uint64(index+1) {
					t.Fatalf("walk component %d = %v, want %v", index, walked[index], component.ID())
				}
			}
			before := schema.Components()
			alternatives[0] = nil
			if choice.Alternatives()[0] == nil {
				t.Fatal("mutating Alternatives changed the completed choice")
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("model-group queries mutated the completed schema")
			}
		})
	}
}

func TestNamedGroupDirectChoiceAllowsEmptyDefaultChoice(t *testing.T) { //nolint:gocognit // Keep the cross-policy empty-choice contract together.
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
			root := namedGroupReferenceRoot(profile.version, "", "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover empty named model-group choice: %v", err)
			}
			groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:named-group", "G"))
			if len(groups) != 1 {
				t.Fatalf("model-group count = %d, want 1", len(groups))
			}
			definition, ok := groups[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("empty model-group has no definition view")
			}
			choice, ok := definition.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("empty model-group particle = %T, want ChoiceParticle", definition.Particle())
			}
			if alternatives := choice.Alternatives(); len(alternatives) != 0 {
				t.Fatalf("empty choice alternatives = %d, want 0", len(alternatives))
			}
		})
	}
}

func TestNamedGroupChoiceZeroRangeMapsToNoParticle(t *testing.T) {
	zero := namedGroupTestOccurrenceRange(t, "0", "0")
	groupLoc := mustTestLoc(t, "root.xsd", 2, 3)
	choiceLoc := mustTestLoc(t, "root.xsd", 3, 5)
	schema, err := newSchemaWithPolicy([]schemaDocumentInput{{
		source:          "root.xsd",
		rootLoc:         mustTestLoc(t, "root.xsd", 1, 1),
		targetNamespace: "urn:named-group",
		declarations: []schemaComponentInput{{
			kind: ComponentKindModelGroupDefinition,
			name: mustTestQName(t, "urn:named-group", "G"),
			loc:  groupLoc,
			modelGroup: &schemaModelGroupInput{particle: &schemaChoiceParticleInput{
				loc:          choiceLoc,
				occurrences:  zero,
				alternatives: nil,
			}},
		}},
	}}, Strict11)
	if err != nil {
		t.Fatalf("newSchema with zero choice range: %v", err)
	}
	groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:named-group", "G"))
	if len(groups) != 1 {
		t.Fatalf("model-group count = %d, want 1", len(groups))
	}
	definition, ok := groups[0].ModelGroupDefinition()
	if !ok {
		t.Fatal("zero-range model-group has no definition view")
	}
	if definition.Particle() != nil {
		t.Fatal("zero-range group choice returned a particle")
	}
}

func TestNamedGroupDirectChoiceResolvesIncludedChameleonAndImportedTargets(t *testing.T) { //nolint:gocognit // Keep graph visibility and provenance assertions together.
	t.Run("included chameleon", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-group"><xs:include schemaLocation="child.xsd"/></xs:schema>`
		fixtures := map[string]discoveryFixture{
			"child.xsd": {
				id: "child.xsd",
				contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:group name="G"><xs:choice><xs:element ref="target"/></xs:choice></xs:group>
  <xs:element name="target" type="xs:integer"/>
</xs:schema>`,
			},
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err != nil {
			t.Fatalf("discover included chameleon group: %v", err)
		}
		groupName := mustTestQName(t, "urn:named-group", "G")
		groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
		if len(groups) != 1 || groups[0].ID().Source() != "child.xsd" {
			t.Fatalf("chameleon group = %#v, want one child.xsd group", groups)
		}
		definition, ok := groups[0].ModelGroupDefinition()
		if !ok {
			t.Fatal("chameleon group has no definition view")
		}
		choice, ok := definition.Particle().(ChoiceParticle)
		if !ok {
			t.Fatal("chameleon group particle is not a choice")
		}
		reference, ok := choice.Alternatives()[0].(ElementReferenceParticle)
		if !ok {
			t.Fatal("chameleon group alternative is not an element reference")
		}
		target := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:named-group", "target"))
		if len(target) != 1 || reference.TargetID() != target[0].ID() || reference.RefLoc().Source() != "child.xsd" {
			t.Fatalf("chameleon target = %v/%s, want child target/ref location", reference.TargetID(), reference.RefLoc())
		}
	})

	t.Run("direct import", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:named-other" targetNamespace="urn:named-group">
  <xs:import namespace="urn:named-other" schemaLocation="other.xsd"/>
  <xs:group name="G"><xs:choice><xs:element ref="o:foreign"/></xs:choice></xs:group>
</xs:schema>`
		fixtures := map[string]discoveryFixture{
			"other.xsd": {
				id:       "other.xsd",
				contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-other"><xs:element name="foreign" type="xs:integer"/></xs:schema>`,
			},
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err != nil {
			t.Fatalf("discover imported group: %v", err)
		}
		groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:named-group", "G"))
		if len(groups) != 1 {
			t.Fatalf("imported model-group count = %d, want 1", len(groups))
		}
		definition, ok := groups[0].ModelGroupDefinition()
		if !ok {
			t.Fatal("imported group has no definition view")
		}
		choice, ok := definition.Particle().(ChoiceParticle)
		if !ok {
			t.Fatal("imported group particle is not a choice")
		}
		reference, ok := choice.Alternatives()[0].(ElementReferenceParticle)
		if !ok {
			t.Fatal("imported group alternative is not an element reference")
		}
		target := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:named-other", "foreign"))
		if len(target) != 1 || reference.TargetID() != target[0].ID() {
			t.Fatalf("imported target ID = %v, want %v", reference.TargetID(), target[0].ID())
		}
	})
}

func TestNamedGroupDirectChoiceSimpleDerivationShape(t *testing.T) { //nolint:gocognit // Keep the bounded XSD 1.1 fixture and order assertions together.
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="` + testXSDNamespace + `" version="1.1">
  <xs:group name="simpleDerivation"><xs:choice>
    <xs:element ref="xs:restriction"/>
    <xs:element ref="xs:list"/>
    <xs:element ref="xs:union"/>
  </xs:choice></xs:group>
  <xs:element name="restriction" type="xs:integer"/>
  <xs:element name="list" type="xs:integer"/>
  <xs:element name="union" type="xs:integer"/>
</xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("discover simpleDerivation group: %v", err)
			}
			groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, testXSDNamespace, "simpleDerivation"))
			if len(groups) != 1 {
				t.Fatalf("simpleDerivation group count = %d, want 1", len(groups))
			}
			definition, ok := groups[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("simpleDerivation group has no definition view")
			}
			choice, ok := definition.Particle().(ChoiceParticle)
			if !ok {
				t.Fatal("simpleDerivation group particle is not a choice")
			}
			if got, want := len(choice.Alternatives()), 3; got != want {
				t.Fatalf("simpleDerivation alternative count = %d, want %d", got, want)
			}
			for index, local := range []string{"restriction", "list", "union"} {
				reference, ok := choice.Alternatives()[index].(ElementReferenceParticle)
				if !ok {
					t.Fatalf("simpleDerivation alternative %d is not an element reference", index)
				}
				if reference.Name() != mustTestQName(t, testXSDNamespace, local) {
					t.Fatalf("simpleDerivation alternative %d = %q, want %s", index, reference.Name(), local)
				}
			}
		})
	}
}

func TestNamedGroupExcludesOtherParticleShapes(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
	cases := []struct {
		name   string
		model  string
		marker string
		last   bool
	}{
		{name: "sequence", model: "<xs:sequence/>", marker: "<xs:sequence"},
		{name: "all", model: "<xs:all/>", marker: "<xs:all"},
		{name: "group reference", model: `<xs:choice><xs:group ref="g:Other"/></xs:choice>`, marker: `ref="g:Other"`},
		{name: "nested choice", model: "<xs:choice><xs:choice/></xs:choice>", marker: "<xs:choice", last: true},
		{name: "wildcard", model: "<xs:choice><xs:any/></xs:choice>", marker: "<xs:any"},
	}
	for _, profile := range profiles {
		for _, test := range cases {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedGroupModelRoot(profile.version, test.model)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if test.last {
					assertNamedGroupChoiceUnsupportedAt(t, schema, err, namedGroupLastLoc(t, root, test.marker))
					return
				}
				assertNamedGroupChoiceUnsupported(t, schema, err, root, test.marker)
			})
		}
	}
}

func TestNamedGroupDirectChoiceReferenceDiagnostics(t *testing.T) { //nolint:gocognit // Keep the cross-policy diagnostic matrix together.
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
			root := namedGroupReferenceRoot(profile.version, `<xs:element ref="g:missing"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("missing group reference returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceUnresolvedCode || diagnostic.SpecRef() != profile.specRef {
				t.Fatalf("missing reference diagnostic = %s/%q, want invalid/unresolved/%q", diagnostic, diagnostic.SpecRef(), profile.specRef)
			}
			if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:missing"`) || len(diagnostic.Related()) != 0 {
				t.Fatalf("missing reference location/related = %s/%v", diagnostic.Loc(), diagnostic.Related())
			}
			if !errors.Is(err, errSchemaElementReferenceUnresolved) {
				t.Fatalf("missing reference cause is not preserved: %v", err)
			}
		})

		t.Run(profile.name+"/wrong kind", func(t *testing.T) {
			root := namedGroupReferenceRoot(profile.version, `<xs:element ref="g:notElement"/>`, `<xs:simpleType name="notElement"><xs:restriction base="xs:integer"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("wrong-kind group reference returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceWrongKindCode || diagnostic.SpecRef() != profile.specRef {
				t.Fatalf("wrong-kind reference diagnostic = %s/%q, want invalid/wrong-kind/%q", diagnostic, diagnostic.SpecRef(), profile.specRef)
			}
			if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:notElement"`) || len(diagnostic.Related()) != 1 {
				t.Fatalf("wrong-kind reference location/related = %s/%v", diagnostic.Loc(), diagnostic.Related())
			}
			if diagnostic.Related()[0] != namedGroupChoiceLoc(t, root, `<xs:simpleType name="notElement"`) {
				t.Fatalf("wrong-kind related location = %s", diagnostic.Related()[0])
			}
			if !errors.Is(err, errSchemaElementReferenceWrongKind) {
				t.Fatalf("wrong-kind reference cause is not preserved: %v", err)
			}
		})
	}
}

func TestNamedGroupDirectChoiceRejectsDuplicateReferenceParticles(t *testing.T) {
	root := namedGroupReferenceRoot("1.1", `<xs:element ref="g:item"/><xs:element ref="g:item"/>`, `<xs:element name="item" type="xs:integer"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("duplicate group reference particles returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceDuplicateCode || diagnostic.SpecRef() != schemaElementReferenceDuplicateXSD11SpecRef {
		t.Fatalf("duplicate reference diagnostic = %s/%q, want invalid/duplicate/%q", diagnostic, diagnostic.SpecRef(), schemaElementReferenceDuplicateXSD11SpecRef)
	}
	if diagnostic.Loc() != namedGroupLastLoc(t, root, `ref="g:item"`) {
		t.Fatalf("duplicate reference location = %s, want second ref", diagnostic.Loc())
	}
	if got, want := diagnostic.Related(), []Loc{namedGroupChoiceLoc(t, root, `ref="g:item"`)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate reference related locations = %v, want %v", got, want)
	}
	if !errors.Is(err, errSchemaElementReferenceDuplicate) {
		t.Fatalf("duplicate reference cause is not preserved: %v", err)
	}
}

func TestNamedGroupDirectChoiceRejectsInvisibleReferenceTarget(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:named-other" targetNamespace="urn:named-group">
  <xs:include schemaLocation="child-import.xsd"/>
  <xs:group name="G"><xs:choice><xs:element ref="o:foreign"/></xs:choice></xs:group>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child-import.xsd": {
			id: "child-import.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-group">
  <xs:import namespace="urn:named-other" schemaLocation="other.xsd"/>
</xs:schema>`,
		},
		"other.xsd": {
			id:       "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-other"><xs:element name="foreign" type="xs:integer"/></xs:schema>`,
		},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err == nil {
		t.Fatal("invisible group reference returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceNamespaceCode || diagnostic.SpecRef() != schemaElementReferenceImportXSD11SpecRef {
		t.Fatalf("invisible reference diagnostic = %s/%q, want invalid/namespace/%q", diagnostic, diagnostic.SpecRef(), schemaElementReferenceImportXSD11SpecRef)
	}
	if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:foreign"`) || len(diagnostic.Related()) != 1 {
		t.Fatalf("invisible reference location/related = %s/%v", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaElementReferenceNamespace) {
		t.Fatalf("invisible reference cause is not preserved: %v", err)
	}
}

func TestNamedGroupDirectChoiceAmbiguousReferenceDiagnostic(t *testing.T) {
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
	particle := schemaElementParticleInput{
		loc:         mustTestLoc(t, "root.xsd", 3, 5),
		name:        name,
		reference:   &schemaElementReferenceInput{name: name, loc: referenceLoc},
		occurrences: namedGroupTestOccurrenceRange(t, "1", "1"),
	}
	_, err := resolveSchemaElementReferenceParticle(
		particle,
		owner,
		[]schemaComponentRecord{owner, first, second},
		map[QName][]int{name: {1, 2}},
		map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
		XSDVersion11,
	)
	if err == nil {
		t.Fatal("ambiguous group reference returned a particle")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceAmbiguousCode || diagnostic.SpecRef() != schemaElementReferenceXSD11SpecRef {
		t.Fatalf("ambiguous reference diagnostic = %s/%q, want invalid/ambiguous/%q", diagnostic, diagnostic.SpecRef(), schemaElementReferenceXSD11SpecRef)
	}
	if diagnostic.Loc() != referenceLoc || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) {
		t.Fatalf("ambiguous reference locations = %s/%v", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaElementReferenceAmbiguous) {
		t.Fatalf("ambiguous reference cause is not preserved: %v", err)
	}
}

func namedGroupReferenceRoot(version, alternatives, declarations string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:named-group" targetNamespace="urn:named-group" version="` + version + `">
  <xs:group name="G"><xs:choice>` + alternatives + `</xs:choice></xs:group>` + declarations + `
</xs:schema>`
}

func namedGroupModelRoot(version, model string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:named-group" targetNamespace="urn:named-group" version="` + version + `">
  <xs:group name="G">` + model + `</xs:group>
</xs:schema>`
}

func namedGroupTestOccurrenceRange(t *testing.T, minimum, maximum string) particleOccurrenceRange {
	t.Helper()
	minimumValue, err := parseParticleOccurrence(minimum, false, mustTestLoc(t, "root.xsd", 3, 5))
	if err != nil {
		t.Fatalf("parse minimum occurrence: %v", err)
	}
	maximumValue, err := parseParticleOccurrence(maximum, true, mustTestLoc(t, "root.xsd", 3, 5))
	if err != nil {
		t.Fatalf("parse maximum occurrence: %v", err)
	}
	rangeValue, err := newParticleOccurrenceRange(minimumValue, maximumValue)
	if err != nil {
		t.Fatalf("new particle occurrence range: %v", err)
	}
	return rangeValue
}

func namedGroupLastLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.LastIndex(root, marker)
	if index < 0 {
		t.Fatalf("fixture does not contain location marker %q", marker)
	}
	return namedGroupLocAt(t, root, index)
}

//nolint:gocognit,funlen // Keep the named-group invalid diagnostic matrix together.
func TestNamedGroupDirectChoicePreservesInvalidDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		groupName   string
		groupAttrs  string
		choiceAttrs string
		children    string
		marker      string
		code        string
		message     string
	}{
		{
			name:       "invalid group attribute",
			groupAttrs: ` bogus="value"`,
			children:   `<xs:element name="value" type="xs:string"/>`,
			marker:     `bogus="value"`,
			code:       invalidSchemaCompositionCode,
			message:    "not permitted",
		},
		{
			name:      "invalid group name",
			groupName: "bad:name",
			children:  `<xs:element name="value" type="xs:string"/>`,
			marker:    `name="bad:name"`,
			code:      invalidSchemaDeclarationNameCode,
			message:   "valid NCName",
		},
		{
			name:        "invalid choice attribute",
			choiceAttrs: ` bogus="value"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      `bogus="value"`,
			code:        invalidSchemaCompositionCode,
			message:     "forbidden attribute",
		},
		{
			name:        "malformed occurrence lexical",
			choiceAttrs: ` minOccurs="many"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      `minOccurs="many"`,
			code:        invalidSchemaCompositionCode,
			message:     "invalid occurrence value",
		},
		{
			name:        "occurrence range",
			choiceAttrs: ` minOccurs="2" maxOccurs="1"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      "<xs:choice",
			code:        invalidSchemaCompositionCode,
			message:     "cannot exceed",
		},
		{
			name:        "duplicate occurrence lexical",
			choiceAttrs: ` minOccurs="0" minOccurs="1"`,
			children:    `<xs:element name="value" type="xs:string"/>`,
			marker:      `minOccurs="1"`,
			code:        InvalidXMLSyntaxCode,
			message:     `duplicate attribute "minOccurs"`,
		},
		{
			name:     "annotation follows content",
			children: `<xs:element name="value" type="xs:string"/><xs:annotation/>`,
			marker:   "<xs:annotation",
			code:     invalidSchemaCompositionCode,
			message:  "annotation must be first",
		},
		{
			name:     "all is forbidden in choice",
			children: `<xs:all/>`,
			marker:   "<xs:all",
			code:     invalidSchemaCompositionCode,
			message:  "cannot contain an all particle",
		},
		{
			name:     "local element requires a declaration name",
			children: `<xs:element/>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
		{
			name:     "local element QName is malformed",
			children: `<xs:element name="value" type="bad:q:name"/>`,
			marker:   `type="bad:q:name"`,
			code:     invalidSchemaConditionalCode,
			message:  "malformed QName",
		},
		{
			name:     "local element QName is unbound",
			children: `<xs:element name="value" type="bad:Type"/>`,
			marker:   `type="bad:Type"`,
			code:     invalidSchemaConditionalCode,
			message:  "unbound QName prefix",
		},
		{
			name:     "invalid child is retained",
			children: `<xs:element/>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
		{
			name:     "staged namespace policy yields to invalid child",
			children: `<xs:element name="value" type="xs:string" form="qualified"/><xs:element/>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
		{
			name:     "nested choice validates its subtree",
			children: `<xs:choice><xs:element/></xs:choice>`,
			marker:   "<xs:element/>",
			code:     invalidSchemaDeclarationNameCode,
			message:  "requires a name or ref",
		},
	}
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "XSD 1.0", policy: Strict10, version: "1.0"},
		{name: "XSD 1.1", policy: Strict11, version: "1.1"},
	} {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedGroupChoiceSchema(profile.version, test.groupName, test.groupAttrs, test.choiceAttrs, test.children)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("discoverSchema accepted malformed named-group choice")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
				}
				if diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) {
					t.Fatalf("diagnostic location = %s, want marker %q", diagnostic.Loc(), test.marker)
				}
				if test.message != "" && !strings.Contains(diagnostic.Message(), test.message) {
					t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.message)
				}
				if errors.Is(err, ErrUnsupported) {
					t.Fatalf("invalid named-group choice matches ErrUnsupported: %v", err)
				}
				if errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("invalid named-group choice retained a policy mismatch: %v", err)
				}
			})
		}
	}
}

func TestNamedGroupDirectChoiceRejectsOccurrenceAttributes(t *testing.T) {
	root := namedGroupChoiceSchema("1.1", "G", "", ` minOccurs="0" maxOccurs="1"`, `<xs:element name="value" type="xs:string"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("discoverSchema accepted named-group choice occurrence attributes")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, invalidSchemaCompositionCode)
	}
	if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `minOccurs="0"`) {
		t.Fatalf("diagnostic location = %s, want minOccurs location", diagnostic.Loc())
	}
	if !strings.Contains(diagnostic.Message(), "does not permit occurrence attributes") {
		t.Fatalf("diagnostic message = %q, want invalid occurrence attributes", diagnostic.Message())
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("invalid occurrence attributes match ErrUnsupported: %v", err)
	}
}

func TestNamedGroupDirectChoiceParsingIsDeterministic(t *testing.T) {
	root := namedGroupChoiceSchema("1.1", "G", ` minOccurs="0"`, "", `<xs:element/>`)
	var first Diagnostic
	var firstErr string
	for iteration := 0; iteration < 16; iteration++ {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if iteration == 0 {
			first = diagnostic
			firstErr = err.Error()
			continue
		}
		if err.Error() != firstErr || diagnostic.Class() != first.Class() || diagnostic.Code() != first.Code() ||
			diagnostic.Feature() != first.Feature() || diagnostic.Loc() != first.Loc() ||
			diagnostic.Message() != first.Message() || diagnostic.SpecRef() != first.SpecRef() ||
			!reflect.DeepEqual(diagnostic.Related(), first.Related()) {
			t.Fatalf("iteration %d diagnostic changed: got %s, want %s", iteration, diagnostic, firstErr)
		}
	}
}

func namedGroupChoiceSchema(version, groupName, groupAttrs, choiceAttrs, children string) string {
	if groupName == "" {
		groupName = "G"
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + version + `">
  <xs:group name="` + groupName + `"` + groupAttrs + `>
    <xs:choice` + choiceAttrs + `>` + children + `</xs:choice>
  </xs:group>
</xs:schema>`
}

func assertNamedGroupChoiceUnsupported(t *testing.T, schema Schema, err error, root, marker string) {
	t.Helper()
	assertNamedGroupChoiceUnsupportedAt(t, schema, err, namedGroupChoiceLoc(t, root, marker))
}

func assertNamedGroupChoiceUnsupportedAt(t *testing.T, schema Schema, err error, wantLoc Loc) {
	t.Helper()
	if err == nil {
		t.Fatal("discoverSchema accepted unsupported named-group choice")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want unsupported/%q/%q", diagnostic, FeatureSchemaSyntax, UnsupportedSchemaSyntaxCode)
	}
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
	}
	if errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("valid named-group choice retained a policy mismatch: %v", err)
	}
}

func assertZeroSchema(t *testing.T, schema Schema) {
	t.Helper()
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("discoverSchema returned a partial schema")
	}
}

func namedGroupChoiceLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.Index(root, marker)
	if index < 0 {
		t.Fatalf("fixture does not contain location marker %q", marker)
	}
	return namedGroupLocAt(t, root, index)
}

func namedGroupLocAt(t *testing.T, root string, index int) Loc {
	t.Helper()
	line := 1
	column := 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	if column != utf8.RuneCountInString(root[strings.LastIndex(root[:index], "\n")+1:index])+1 {
		t.Fatalf("fixture location at byte offset %d has inconsistent column calculation", index)
	}
	return mustTestLoc(t, "root.xsd", line, column)
}
