package goxsd9

import (
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func nestedGroupSequenceSchema(version, choice string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-sequence" targetNamespace="urn:nested-sequence" version="` + version + `">
  <xs:group name="Outer"><xs:choice>` + choice + `</xs:choice></xs:group>
  <xs:group name="Target"><xs:sequence><xs:element ref="g:last"/></xs:sequence></xs:group>
  <xs:element name="first" type="xs:integer"/>
  <xs:element name="last" type="xs:integer"/>
</xs:schema>`
}

func assertNestedSequenceDiagnostic(t *testing.T, err error, root, marker, spec, code string, class FailureClass, relatedMarkers []string, cause error) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	related := make([]Loc, 0, len(relatedMarkers))
	for _, relatedMarker := range relatedMarkers {
		related = append(related, namedGroupChoiceLoc(t, root, relatedMarker))
	}
	if len(related) == 0 {
		related = nil
	}
	if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, marker) || diagnostic.SpecRef() != spec || !reflect.DeepEqual(diagnostic.Related(), related) {
		t.Fatalf("diagnostic = %s, related %v; want %s at %s with %s and %v", diagnostic, diagnostic.Related(), code, marker, spec, related)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic cause = %v, want %v", err, cause)
	}
}

func assertNestedResolutionFailure(t *testing.T, err error, primary Loc, related []Loc, code, spec string, cause error) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != code || diagnostic.Loc() != primary || diagnostic.SpecRef() != spec || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, cause) {
		t.Fatalf("resolution diagnostic = %s, related %v; want %s at %s with %v", diagnostic, diagnostic.Related(), code, primary, related)
	}
}

func TestIssue743UnlockFixturePublicObservables(t *testing.T) { //nolint:gocognit // The fixture checks policy, type, bounds, and target identity together.
	data, err := os.ReadFile("testdata/issue743/nested-sequence.xsd")
	if err != nil {
		t.Fatal(err)
	}
	root := string(data)
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			choice, choiceOK := namedSequenceGroup(t, schema, "urn:issue743", "Choice").Particle().(ChoiceParticle)
			if !choiceOK {
				t.Fatal("fixture group has no choice particle")
			}
			if len(choice.Alternatives()) != 1 {
				t.Fatalf("fixture alternatives = %d, want one", len(choice.Alternatives()))
			}
			sequence, ok := choice.Alternatives()[0].(SequenceParticle)
			if !ok || sequence.Occurrences().String() != "2/3" || sequence.Loc() != namedGroupChoiceLoc(t, root, `<xs:sequence minOccurs="2"`) || len(sequence.Particles()) != 2 {
				t.Fatalf("fixture sequence = %T %s", choice.Alternatives()[0], choice.Alternatives()[0].Occurrences())
			}
			group, ok := sequence.Particles()[1].(ModelGroupReferenceParticle)
			if !ok || group.TargetID() != namedSequenceGroup(t, schema, "urn:issue743", "Tail").ID() || group.Occurrences().String() != "0/1" {
				t.Fatalf("fixture group ref = %T", sequence.Particles()[1])
			}
		})
	}
}

func TestNamedGroupChoiceNestedSequencePublicFacts(t *testing.T) { //nolint:gocognit // Keep order, exact bounds, identities, and copy isolation in one public contract.
	children := `<xs:element ref="g:first"/><xs:sequence minOccurs="2" maxOccurs="18446744073709551616"><xs:element ref="g:last" minOccurs="0" maxOccurs="unbounded"/><xs:group ref="g:Target" minOccurs="3" maxOccurs="5"/><xs:element ref="g:first" minOccurs="0" maxOccurs="0"/></xs:sequence><xs:group ref="g:Target"/>`
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupSequenceSchema(profile.version, children)
			var first []Component
			for repeat := 0; repeat < 3; repeat++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if repeat == 0 {
					first = schema.Components()
				}
				if !reflect.DeepEqual(first, schema.Components()) {
					t.Fatal("component order changed")
				}
				outer := namedSequenceGroup(t, schema, "urn:nested-sequence", "Outer")
				target := namedSequenceGroup(t, schema, "urn:nested-sequence", "Target")
				choice, ok := outer.Particle().(ChoiceParticle)
				if !ok || len(choice.Alternatives()) != 3 {
					t.Fatalf("outer particle = %T", outer.Particle())
				}
				alternatives := choice.Alternatives()
				if _, isElement := alternatives[0].(ElementReferenceParticle); !isElement {
					t.Fatalf("first alternative = %T", alternatives[0])
				}
				sequence, ok := alternatives[1].(SequenceParticle)
				if !ok || sequence.Loc() != namedGroupChoiceLoc(t, root, `<xs:sequence minOccurs="2"`) || sequence.Occurrences().String() != "2/18446744073709551616" || len(sequence.Particles()) != 2 {
					t.Fatalf("nested sequence = %T %s %s", alternatives[1], alternatives[1].Loc(), alternatives[1].Occurrences())
				}
				last, ok := sequence.Particles()[0].(ElementReferenceParticle)
				if !ok || last.Ref() != mustTestQName(t, "urn:nested-sequence", "last") || last.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:last" minOccurs="0"`) || last.Occurrences().String() != "0/unbounded" {
					t.Fatalf("nested first particle = %T", sequence.Particles()[0])
				}
				group, ok := sequence.Particles()[1].(ModelGroupReferenceParticle)
				if !ok || group.Ref() != target.Name() || group.TargetID() != target.ID() || group.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:Target" minOccurs="3"`) || group.Occurrences().String() != "3/5" {
					t.Fatalf("nested second particle = %T", sequence.Particles()[1])
				}
				if _, ok := alternatives[2].(ModelGroupReferenceParticle); !ok {
					t.Fatalf("third alternative = %T", alternatives[2])
				}
				maximum, finite := sequence.Occurrences().Maximum().Finite()
				if !finite {
					t.Fatal("huge maximum became unbounded")
				}
				maximum.value.SetInt64(0)
				owned := sequence.Particles()
				owned[0] = nil
				alternatives[1] = nil
				if sequence.Occurrences().String() != "2/18446744073709551616" || sequence.Particles()[0] == nil || choice.Alternatives()[1] == nil {
					t.Fatal("returned bounds or slices mutated completed schema")
				}
				walked := make([]ComponentID, 0)
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				for index, component := range schema.Components() {
					if walked[index] != component.ID() {
						t.Fatalf("walk %d = %v, want %v", index, walked[index], component.ID())
					}
				}
			}
		})
	}
}

func TestNamedGroupChoiceNestedSequenceZeroOmission(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupSequenceSchema(profile.version, `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/><xs:group ref="g:Target"/></xs:sequence><xs:element ref="g:last"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			choice, choiceOK := namedSequenceGroup(t, schema, "urn:nested-sequence", "Outer").Particle().(ChoiceParticle)
			if !choiceOK {
				t.Fatal("outer group has no choice particle")
			}
			if len(choice.Alternatives()) != 1 {
				t.Fatalf("zero nested sequence was published: %v", choice.Alternatives())
			}
			last, ok := choice.Alternatives()[0].(ElementReferenceParticle)
			if !ok || last.Ref() != mustTestQName(t, "urn:nested-sequence", "last") {
				t.Fatalf("remaining alternative = %T", choice.Alternatives()[0])
			}
		})
	}
}

func TestNamedGroupChoiceNestedSequenceGraphSources(t *testing.T) { //nolint:gocognit // Each graph checks both nested reference kinds and source provenance.
	for _, test := range []struct {
		name, root, ownerXML, ownerSource, targetSource, targetNS, written string
		fixtures                                                           map[string]discoveryFixture
	}{
		{
			name:        "include",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-sequence" targetNamespace="urn:nested-sequence"><xs:include schemaLocation="child.xsd"/><xs:group name="Outer"><xs:choice><xs:sequence><xs:element ref="g:item"/><xs:group ref="g:Target"/></xs:sequence></xs:choice></xs:group></xs:schema>`,
			ownerSource: "root.xsd", targetSource: "child.xsd", targetNS: "urn:nested-sequence", written: "g:Target",
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:nested-sequence"><xs:group name="Target"><xs:sequence/></xs:group><xs:element name="item" type="xs:integer"/></xs:schema>`}},
		},
		{
			name:        "import",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:nested-sequence"><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Outer"><xs:choice><xs:sequence><xs:element ref="o:item"/><xs:group ref="o:Target"/></xs:sequence></xs:choice></xs:group></xs:schema>`,
			ownerSource: "root.xsd", targetSource: "other.xsd", targetNS: "urn:other", written: "o:Target",
			fixtures: map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="Target"><xs:sequence/></xs:group><xs:element name="item" type="xs:integer"/></xs:schema>`}},
		},
		{
			name:        "chameleon",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:nested-sequence"><xs:include schemaLocation="child.xsd"/><xs:group name="Target"><xs:sequence/></xs:group><xs:element name="item" type="xs:integer"/></xs:schema>`,
			ownerSource: "child.xsd", targetSource: "root.xsd", targetNS: "urn:nested-sequence", written: "Target",
			ownerXML: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Outer"><xs:choice><xs:sequence><xs:element ref="item"/><xs:group ref="Target"/></xs:sequence></xs:choice></xs:group></xs:schema>`,
		},
	} {
		for _, profile := range nestedGroupChoiceProfiles {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				fixtures := test.fixtures
				if test.name == "chameleon" {
					fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: test.ownerXML}}
				}
				var first []Component
				for repeat := 0; repeat < 3; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if repeat == 0 {
						first = schema.Components()
					}
					if !reflect.DeepEqual(first, schema.Components()) {
						t.Fatal("graph component order changed")
					}
					owner := namedSequenceGroup(t, schema, "urn:nested-sequence", "Outer")
					target := namedSequenceGroup(t, schema, test.targetNS, "Target")
					choice, choiceOK := owner.Particle().(ChoiceParticle)
					if !choiceOK || len(choice.Alternatives()) != 1 {
						t.Fatalf("graph owner particle = %T", owner.Particle())
					}
					sequence, sequenceOK := choice.Alternatives()[0].(SequenceParticle)
					if !sequenceOK {
						t.Fatalf("graph nested particle = %T", choice.Alternatives()[0])
					}
					particles := sequence.Particles()
					if len(particles) != 2 {
						t.Fatalf("nested particles = %v", particles)
					}
					element, elementOK := particles[0].(ElementReferenceParticle)
					group, groupOK := particles[1].(ModelGroupReferenceParticle)
					if !elementOK || !groupOK || owner.ID().Source() != SourceID(test.ownerSource) || target.ID().Source() != SourceID(test.targetSource) || element.Ref().Namespace() != test.targetNS || group.TargetID() != target.ID() || group.Ref() != target.Name() {
						t.Fatalf("graph facts = %v/%v/%v/%v", owner.ID(), target.ID(), particles[0], particles[1])
					}
					ownerXML := test.root
					if test.ownerSource != "root.xsd" {
						ownerXML = test.ownerXML
					}
					wantGroup := namedGroupChoiceLoc(t, ownerXML, `ref="`+test.written+`"`)
					wantGroup.source = SourceID(test.ownerSource)
					if group.RefLoc() != wantGroup || sequence.Loc().source != SourceID(test.ownerSource) || element.RefLoc().source != SourceID(test.ownerSource) {
						t.Fatalf("graph locations = %s/%s/%s, want %s", sequence.Loc(), element.RefLoc(), group.RefLoc(), wantGroup)
					}
				}
			})
		}
	}
}

func TestNamedGroupChoiceNestedSequenceReferenceFailures(t *testing.T) { //nolint:gocognit // Alternate exits at one resolution boundary share the same 0/0 fixture.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, child, marker, code string
			cause                     error
			group                     bool
			related                   []string
		}{
			{"missing element", `<xs:element ref="g:missing"/>`, `ref="g:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, errSchemaElementReferenceUnresolved, false, nil},
			{"wrong element kind", `<xs:element ref="g:Target"/>`, `ref="g:Target"`, diagnosticSchemaElementReferenceWrongKindCode, errSchemaElementReferenceWrongKind, false, []string{`<xs:group name="Target"`}},
			{"missing group", `<xs:group ref="g:missing"/>`, `ref="g:missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved, true, nil},
			{"wrong group kind", `<xs:group ref="g:first"/>`, `ref="g:first"`, diagnosticSchemaModelGroupReferenceWrongKindCode, errSchemaModelGroupReferenceWrongKind, true, []string{`<xs:element name="first"`}},
			{"cycle", `<xs:group ref="g:Outer"/>`, `ref="g:Outer"`, diagnosticSchemaModelGroupReferenceCycleCode, errSchemaModelGroupReferenceCycle, true, []string{`<xs:group name="Outer"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupSequenceSchema(profile.version, `<xs:sequence minOccurs="0" maxOccurs="0">`+test.child+`</xs:sequence>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := schemaElementReferenceSpecRef(profile.factVersion)
				if test.group {
					wantSpec = schemaModelGroupReferenceSpecRef(profile.factVersion)
				}
				if test.name == "cycle" {
					wantSpec = "xsd11-structures#mg-props-correct"
					if profile.factVersion == XSDVersion10 {
						wantSpec = "xsd10-structures#mg-props-correct"
					}
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, test.cause) {
					t.Fatalf("reference diagnostic = %s, want %s at %s", diagnostic, test.code, test.marker)
				}
				var related []Loc
				for _, marker := range test.related {
					related = append(related, namedGroupChoiceLoc(t, root, marker))
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), related)
				}
			})
		}
	}
}

func TestNamedGroupChoiceNestedSequenceInvisibleReferences(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, target, code, spec string
			cause                    error
		}{
			{"group", `<xs:group name="Target"><xs:sequence/></xs:group>`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.factVersion), errSchemaModelGroupReferenceNamespace},
			{"element", `<xs:element name="Target" type="xs:integer"/>`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:nested-sequence"><xs:include schemaLocation="child.xsd"/><xs:group name="Outer"><xs:choice><xs:sequence minOccurs="0" maxOccurs="0"><xs:` + test.name + ` ref="o:Target"/></xs:sequence></xs:choice></xs:group></xs:schema>`
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + test.target + `</xs:schema>`
				fixtures := map[string]discoveryFixture{
					"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:nested-sequence"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
					"other.xsd": {id: "other.xsd", contents: other},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := namedGroupChoiceLoc(t, other, `<xs:`+test.name+` name="Target"`)
				wantRelated.source = "other.xsd"
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:Target"`) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, test.cause) {
					t.Fatalf("invisible diagnostic = %s, related %v", diagnostic, diagnostic.Related())
				}
			})
		}
	}
}

func TestNamedGroupChoiceNestedSequenceAmbiguousResolutionBoundary(t *testing.T) {
	name := mustTestQName(t, "urn:nested-sequence", "Target")
	owner := schemaComponentRecord{id: ComponentID{source: "root.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: mustTestQName(t, "urn:nested-sequence", "Outer"), loc: mustTestLoc(t, "root.xsd", 2, 3)}
	first := schemaComponentRecord{id: ComponentID{source: "one.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: mustTestLoc(t, "one.xsd", 2, 3), modelGroup: &schemaModelGroupInput{particle: &schemaSequenceParticleInput{}}}
	second := schemaComponentRecord{id: ComponentID{source: "two.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: mustTestLoc(t, "two.xsd", 2, 3), modelGroup: &schemaModelGroupInput{particle: &schemaSequenceParticleInput{}}}
	refLoc := mustTestLoc(t, "root.xsd", 3, 28)
	zero := namedGroupTestOccurrenceRange(t, "0", "0")
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			input := &schemaChoiceParticleInput{loc: owner.loc, occurrences: namedGroupTestOccurrenceRange(t, "1", "1"), alternatives: []schemaParticleTermInput{
				&schemaSequenceParticleInput{loc: refLoc, occurrences: zero, particles: []schemaParticleTermInput{
					&schemaModelGroupReferenceParticleInput{loc: refLoc, occurrences: zero, reference: &schemaModelGroupReferenceInput{name: name, loc: refLoc}},
				}},
			}}
			resolver := schemaModelGroupResolver{
				records:        []schemaComponentRecord{owner, first, second},
				byName:         map[QName][]int{name: {1, 2}},
				visibleSources: map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
				version:        profile.factVersion,
			}
			particle, err := resolver.resolveChoice(input, owner)
			if particle != nil {
				t.Fatalf("ambiguous nested sequence returned %T", particle)
			}
			assertNestedResolutionFailure(t, err, refLoc, []Loc{first.loc, second.loc}, diagnosticSchemaModelGroupReferenceAmbiguousCode, schemaModelGroupReferenceSpecRef(profile.factVersion), errSchemaModelGroupReferenceAmbiguous)
		})
	}
}

func TestNamedGroupChoiceNestedSequenceAmbiguousElementBoundary(t *testing.T) {
	name := mustTestQName(t, "urn:nested-sequence", "item")
	owner := schemaComponentRecord{id: ComponentID{source: "root.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: mustTestQName(t, "urn:nested-sequence", "Outer"), loc: mustTestLoc(t, "root.xsd", 2, 3)}
	first := schemaComponentRecord{id: ComponentID{source: "one.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, name: name, loc: mustTestLoc(t, "one.xsd", 2, 3)}
	second := schemaComponentRecord{id: ComponentID{source: "two.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, name: name, loc: mustTestLoc(t, "two.xsd", 2, 3)}
	refLoc := mustTestLoc(t, "root.xsd", 3, 28)
	zero := namedGroupTestOccurrenceRange(t, "0", "0")
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			input := &schemaChoiceParticleInput{loc: owner.loc, occurrences: namedGroupTestOccurrenceRange(t, "1", "1"), alternatives: []schemaParticleTermInput{
				&schemaSequenceParticleInput{loc: refLoc, occurrences: zero, particles: []schemaParticleTermInput{
					schemaElementParticleInput{loc: refLoc, occurrences: zero, reference: &schemaElementReferenceInput{name: name, loc: refLoc}},
				}},
			}}
			resolver := schemaModelGroupResolver{
				records:        []schemaComponentRecord{owner, first, second},
				byName:         map[QName][]int{name: {1, 2}},
				visibleSources: map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
				version:        profile.factVersion,
			}
			particle, err := resolver.resolveChoice(input, owner)
			if particle != nil {
				t.Fatalf("ambiguous nested element returned %T", particle)
			}
			assertNestedResolutionFailure(t, err, refLoc, []Loc{first.loc, second.loc}, diagnosticSchemaElementReferenceAmbiguousCode, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceAmbiguous)
		})
	}
}

func TestNamedGroupChoiceNestedSequenceIndirectCycle(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-sequence" targetNamespace="urn:nested-sequence">
  <xs:group name="Outer"><xs:choice><xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="g:Target" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:choice></xs:group>
  <xs:group name="Target"><xs:sequence><xs:group ref="g:Outer" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:group>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantSpec := "xsd11-structures#mg-props-correct"
			if profile.factVersion == XSDVersion10 {
				wantSpec = "xsd10-structures#mg-props-correct"
			}
			related := []Loc{
				namedGroupChoiceLoc(t, root, `<xs:group name="Outer"`),
				namedGroupChoiceLoc(t, root, `<xs:group name="Target"`),
				namedGroupChoiceLoc(t, root, `ref="g:Target"`),
			}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:Outer"`) || diagnostic.SpecRef() != wantSpec || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
				t.Fatalf("indirect cycle = %s, related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

func TestNamedGroupChoiceNestedSequenceDuplicateElementReference(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupSequenceSchema(profile.version, `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/><xs:element ref="g:first" minOccurs="0" maxOccurs="0"/></xs:sequence>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			first := namedGroupChoiceLoc(t, root, `ref="g:first"`)
			second := namedGroupLastLoc(t, root, `ref="g:first"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceDuplicateCode || diagnostic.Loc() != second || diagnostic.SpecRef() != schemaElementReferenceDuplicateSpecRef(profile.factVersion) || !reflect.DeepEqual(diagnostic.Related(), []Loc{first}) || !errors.Is(err, errSchemaElementReferenceDuplicate) {
				t.Fatalf("duplicate nested reference = %s, related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

func TestNamedGroupChoiceNestedSequenceOccurrenceFailures(t *testing.T) { //nolint:gocognit // Lexical and interval exits precede reference resolution, including under 0/0.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, term, marker, spec string
			cause                    error
			related                  []string
			lexical                  bool
		}{
			{"malformed minimum", `<xs:sequence minOccurs="bad" maxOccurs="0"><xs:group ref="g:missing"/></xs:sequence>`, `minOccurs="bad"`, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil, nil, true},
			{"negative minimum", `<xs:sequence minOccurs="-1" maxOccurs="0"><xs:group ref="g:missing"/></xs:sequence>`, `minOccurs="-1"`, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), errParticleOccurrenceNegative, nil, false},
			{"unbounded minimum", `<xs:sequence minOccurs="unbounded" maxOccurs="0"><xs:group ref="g:missing"/></xs:sequence>`, `minOccurs="unbounded"`, schemaParticleMinimumSpecRef(profile.factVersion), nil, nil, true},
			{"reversed bounds", `<xs:sequence minOccurs="2" maxOccurs="1"><xs:group ref="g:missing"/></xs:sequence>`, `<xs:sequence minOccurs="2"`, schemaParticleCorrectSpecRef(profile.factVersion), errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}, false},
			{"malformed child under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="g:first" maxOccurs="bad"/></xs:sequence>`, `maxOccurs="bad"`, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil, nil, true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupSequenceSchema(profile.version, test.term)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || diagnostic.SpecRef() != test.spec {
					t.Fatalf("occurrence diagnostic = %s, want invalid at %s with %s", diagnostic, test.marker, test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("occurrence cause = %v, want %v", err, test.cause)
				}
				if test.lexical {
					inner := requireDiagnostic(t, errors.Unwrap(err))
					if inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != diagnostic.Loc() {
						t.Fatalf("lexical cause = %s, want XSD2001 at %s", inner, diagnostic.Loc())
					}
				}
				var related []Loc
				for _, marker := range test.related {
					related = append(related, namedGroupChoiceLoc(t, root, marker))
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), related)
				}
			})
		}
	}
}

func TestNamedGroupChoiceNestedSequenceExcludedChildren(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, child, marker string
			class               FailureClass
			code                string
		}{
			{"deeper choice", `<xs:choice><xs:element ref="g:first"/></xs:choice>`, `<xs:choice><xs:element`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"deeper sequence", `<xs:sequence><xs:element ref="g:first"/></xs:sequence>`, `<xs:sequence><xs:element`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"local declaration", `<xs:element name="local" type="xs:integer"/>`, `<xs:element name="local"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"wildcard", `<xs:any/>`, `<xs:any`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"all", `<xs:all/>`, `<xs:all`, FailureInvalid, invalidSchemaCompositionCode},
			{"mixed group and deeper choice", `<xs:group ref="g:Target"/><xs:choice><xs:element ref="g:first"/></xs:choice>`, `<xs:choice><xs:element`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"mixed element and wildcard", `<xs:element ref="g:first"/><xs:any/>`, `<xs:any`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"mixed group and all", `<xs:group ref="g:Target"/><xs:all/>`, `<xs:all`, FailureInvalid, invalidSchemaCompositionCode},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupSequenceSchema(profile.version, `<xs:sequence minOccurs="0" maxOccurs="0">`+test.child+`</xs:sequence>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				wantSpec := schemaSyntaxSpecRefForVersion(profile.factVersion)
				if test.name == "all" || test.name == "mixed group and all" {
					wantSpec = schemaGroupParticleSpecRef(profile.factVersion)
				}
				var cause error
				if test.class == FailureUnsupported {
					cause = ErrUnsupported
				}
				assertNestedSequenceDiagnostic(t, err, root, test.marker, wantSpec, test.code, test.class, nil, cause)
			})
		}
	}
}

func TestNamedGroupChoiceNestedSequenceExcludedPlacements(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, body, marker string
		}{
			{"named sequence", `<xs:group name="Outer"><xs:sequence><xs:sequence><xs:element ref="g:first"/></xs:sequence></xs:sequence></xs:group>`, `<xs:sequence><xs:element`},
			{"direct complex choice", `<xs:complexType name="Container"><xs:choice><xs:sequence><xs:element ref="g:first"/></xs:sequence></xs:choice></xs:complexType>`, `<xs:sequence><xs:element`},
			{"inline complex choice", `<xs:element name="root"><xs:complexType><xs:choice><xs:sequence><xs:element ref="g:first"/></xs:sequence></xs:choice></xs:complexType></xs:element>`, `<xs:sequence><xs:element`},
			{"named ref target sequence", `<xs:group name="Outer"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:group><xs:group name="Target"><xs:sequence><xs:sequence><xs:element ref="g:first"/></xs:sequence></xs:sequence></xs:group>`, `<xs:sequence><xs:element`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-sequence" targetNamespace="urn:nested-sequence">` + test.body + `<xs:element name="first" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded placement = %s, want unsupported at %s", diagnostic, test.marker)
				}
			})
		}
	}
}

func TestNamedGroupChoiceSecondNestedSequenceUnsupported(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, first, second string
		}{
			{"authored", `<xs:sequence><xs:element ref="g:first"/></xs:sequence>`, `<xs:sequence><xs:group ref="g:Target"/></xs:sequence>`},
			{"first omitted", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/></xs:sequence>`, `<xs:sequence><xs:group ref="g:Target"/></xs:sequence>`},
			{"second omitted", `<xs:sequence><xs:element ref="g:first"/></xs:sequence>`, `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="g:Target"/></xs:sequence>`},
			{"both omitted", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/></xs:sequence>`, `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="g:Target"/></xs:sequence>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupSequenceSchema(profile.version, test.first+test.second)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				want := namedGroupChoiceLoc(t, root, test.second)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != want || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(profile.factVersion) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("second sequence = %s, want unsupported at %s", diagnostic, want)
				}
			})
		}
	}
}

func TestNamedGroupChoiceSecondNestedSequenceInvalidAlternates(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, second, marker, spec string
			cause                      error
			related                    []string
		}{
			{"bad bound", `<xs:sequence minOccurs="bad"><xs:group ref="g:Target"/></xs:sequence>`, `minOccurs="bad"`, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil, nil},
			{"reversed bounds", `<xs:sequence minOccurs="2" maxOccurs="1"><xs:group ref="g:Target"/></xs:sequence>`, `<xs:sequence minOccurs="2"`, schemaParticleCorrectSpecRef(profile.factVersion), errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"bad child under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="g:Target" maxOccurs="bad"/></xs:sequence>`, `maxOccurs="bad"`, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil, nil},
			{"all under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:all/></xs:sequence>`, `<xs:all`, schemaGroupParticleSpecRef(profile.factVersion), nil, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupSequenceSchema(profile.version, `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/></xs:sequence>`+test.second)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				assertNestedSequenceDiagnostic(t, err, root, test.marker, test.spec, invalidSchemaCompositionCode, FailureInvalid, test.related, test.cause)
				if errors.Is(err, ErrUnsupported) {
					t.Fatalf("invalid second sequence became unsupported: %v", err)
				}
			})
		}
	}
}

func TestNamedGroupChoiceNestedSequenceConsumers(t *testing.T) { //nolint:gocognit // Both consumers share one located named-group reference.
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-sequence" targetNamespace="urn:nested-sequence" version="` + profile.version + `">
  <xs:element name="root" type="g:Container"/><xs:complexType name="Container"><xs:group ref="g:Outer"/></xs:complexType>
  <xs:group name="Outer"><xs:choice><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Outer"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:nested-sequence"><item>1</item></root>`)))
			validation := requireDiagnostic(t, validationErr)
			wantValidationSpec := instanceValidationXSD11SpecRef
			if profile.factVersion == XSDVersion10 {
				wantValidationSpec = instanceValidationXSD10SpecRef
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantLoc || validation.SpecRef() != wantValidationSpec || len(validation.Related()) == 0 || !errors.Is(validationErr, errInstanceModelGroupReference) {
				t.Fatalf("validation = %s, want located unsupported", validation)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			wantCodegenSpec := codegenDirectSequenceXSD11ParticlesSpecRef
			if profile.factVersion == XSDVersion10 {
				wantCodegenSpec = codegenDirectSequenceXSD10ParticlesSpecRef
			}
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantLoc || generation.SpecRef() != wantCodegenSpec || len(generation.Related()) == 0 || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
				t.Fatalf("generation = (%q, %s), want nil located unsupported", output, generation)
			}
		})
	}
}
