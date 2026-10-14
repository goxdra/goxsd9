package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func nestedGroupChoiceSchema(version, sequence string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-choice" targetNamespace="urn:nested-choice" version="` + version + `">
  <xs:group name="Outer"><xs:sequence>` + sequence + `</xs:sequence></xs:group>
  <xs:group name="Target"><xs:sequence><xs:element ref="g:last"/></xs:sequence></xs:group>
  <xs:element name="first" type="xs:integer"/>
  <xs:element name="last" type="xs:integer"/>
</xs:schema>`
}

var nestedGroupChoiceProfiles = []struct {
	name, version string
	policy        LanguagePolicy
	factVersion   XSDVersion
}{
	{"Compatibility", "1.0", Compatibility, XSDVersion11},
	{"Strict10", "1.0", Strict10, XSDVersion10},
	{"Strict11", "1.1", Strict11, XSDVersion11},
}

func TestNamedGroupSequenceNestedChoicePublicFacts(t *testing.T) { //nolint:gocognit // One public contract includes order, exact bounds, identities, locations, and copy isolation.
	sequence := `<xs:element ref="g:first"/><xs:choice minOccurs="2" maxOccurs="18446744073709551616"><xs:element ref="g:last" minOccurs="0" maxOccurs="unbounded"/><xs:group ref="g:Target" minOccurs="3" maxOccurs="5"/><xs:element ref="g:first" minOccurs="0" maxOccurs="0"/></xs:choice><xs:group ref="g:Target"/>`
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupChoiceSchema(profile.version, sequence)
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
					t.Fatal("repeated parse changed component order")
				}
				outer := namedSequenceGroup(t, schema, "urn:nested-choice", "Outer")
				target := namedSequenceGroup(t, schema, "urn:nested-choice", "Target")
				seq, ok := outer.Particle().(SequenceParticle)
				if !ok || len(seq.Particles()) != 3 {
					t.Fatalf("outer particle = %T, terms %d", outer.Particle(), len(seq.Particles()))
				}
				terms := seq.Particles()
				if _, isElement := terms[0].(ElementReferenceParticle); !isElement {
					t.Fatalf("first term = %T", terms[0])
				}
				choice, ok := terms[1].(ChoiceParticle)
				if !ok || choice.Loc() != namedGroupChoiceLoc(t, root, `<xs:choice minOccurs="2"`) || choice.Occurrences().String() != "2/18446744073709551616" || len(choice.Alternatives()) != 2 {
					t.Fatalf("nested choice = %T %s %s", terms[1], choice.Loc(), choice.Occurrences())
				}
				last, ok := choice.Alternatives()[0].(ElementReferenceParticle)
				if !ok || last.Ref() != mustTestQName(t, "urn:nested-choice", "last") || last.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:last" minOccurs="0"`) || last.Occurrences().String() != "0/unbounded" {
					t.Fatalf("first nested alternative = %T %v", choice.Alternatives()[0], choice.Alternatives()[0])
				}
				group, ok := choice.Alternatives()[1].(ModelGroupReferenceParticle)
				if !ok || group.Ref() != target.Name() || group.TargetID() != target.ID() || group.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:Target" minOccurs="3"`) || group.Occurrences().String() != "3/5" {
					t.Fatalf("second nested alternative = %T %v", choice.Alternatives()[1], choice.Alternatives()[1])
				}
				if _, ok := terms[2].(ModelGroupReferenceParticle); !ok {
					t.Fatalf("last term = %T", terms[2])
				}
				bounds := choice.Occurrences().Maximum()
				maximum, finite := bounds.Finite()
				if !finite {
					t.Fatal("large maximum became unbounded")
				}
				maximum.value.SetInt64(0)
				alternatives := choice.Alternatives()
				alternatives[0] = nil
				terms[1] = nil
				if choice.Occurrences().String() != "2/18446744073709551616" || choice.Alternatives()[0] == nil || seq.Particles()[1] == nil {
					t.Fatal("returned bounds or slices mutated schema")
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

func TestNamedGroupSequenceNestedChoiceZeroResolution(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupChoiceSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/><xs:group ref="g:Target"/></xs:choice><xs:element ref="g:last"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			group := namedSequenceGroup(t, schema, "urn:nested-choice", "Outer")
			sequence, ok := group.Particle().(SequenceParticle)
			if !ok || len(sequence.Particles()) != 1 {
				t.Fatalf("zero nested choice was not omitted: %v", group.Particle())
			}
			last, ok := sequence.Particles()[0].(ElementReferenceParticle)
			if !ok || last.Ref() != mustTestQName(t, "urn:nested-choice", "last") {
				t.Fatalf("zero nested choice was not omitted: %v", sequence.Particles())
			}
		})
	}
}

func TestNamedGroupSequenceNestedChoiceGraphSources(t *testing.T) { //nolint:gocognit // Each graph checks the same public source and target contract.
	for _, test := range []struct {
		name, root, ownerXML, ownerSource, targetSource, targetNS, written string
		fixtures                                                           map[string]discoveryFixture
	}{
		{
			name:        "forward",
			root:        nestedGroupChoiceSchema("1.1", `<xs:choice><xs:element ref="g:first"/><xs:group ref="g:Target"/></xs:choice>`),
			ownerSource: "root.xsd", targetSource: "root.xsd", targetNS: "urn:nested-choice", written: "g:Target",
		},
		{
			name:        "include",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-choice" targetNamespace="urn:nested-choice"><xs:include schemaLocation="child.xsd"/><xs:group name="Outer"><xs:sequence><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:sequence></xs:group></xs:schema>`,
			ownerSource: "root.xsd", targetSource: "child.xsd", targetNS: "urn:nested-choice", written: "g:Target",
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:nested-choice"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`}},
		},
		{
			name:        "import",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:nested-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Outer"><xs:sequence><xs:choice><xs:group ref="o:Target"/></xs:choice></xs:sequence></xs:group></xs:schema>`,
			ownerSource: "root.xsd", targetSource: "other.xsd", targetNS: "urn:other", written: "o:Target",
			fixtures: map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`}},
		},
		{
			name:        "chameleon",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:nested-choice"><xs:include schemaLocation="child.xsd"/><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`,
			ownerSource: "child.xsd", targetSource: "root.xsd", targetNS: "urn:nested-choice", written: "Target",
			ownerXML: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Outer"><xs:sequence><xs:choice><xs:group ref="Target"/></xs:choice></xs:sequence></xs:group></xs:schema>`,
		},
	} {
		for _, profile := range nestedGroupChoiceProfiles {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				if test.name == "chameleon" {
					test.fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: test.ownerXML}}
				}
				var first []Component
				for repeat := 0; repeat < 3; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, test.fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if repeat == 0 {
						first = schema.Components()
					}
					if !reflect.DeepEqual(first, schema.Components()) {
						t.Fatal("graph component order changed")
					}
					owner := namedSequenceGroup(t, schema, "urn:nested-choice", "Outer")
					target := namedSequenceGroup(t, schema, test.targetNS, "Target")
					sequence, ok := owner.Particle().(SequenceParticle)
					if !ok || len(sequence.Particles()) != 1 {
						t.Fatalf("graph owner particle = %T", owner.Particle())
					}
					choice, ok := sequence.Particles()[0].(ChoiceParticle)
					if !ok {
						t.Fatalf("graph nested term = %T", sequence.Particles()[0])
					}
					alternatives := choice.Alternatives()
					if len(alternatives) == 0 {
						t.Fatal("graph nested choice has no alternatives")
					}
					reference, ok := alternatives[len(alternatives)-1].(ModelGroupReferenceParticle)
					if !ok {
						t.Fatalf("graph last alternative = %T", alternatives[len(alternatives)-1])
					}
					if owner.ID().Source() != SourceID(test.ownerSource) || target.ID().Source() != SourceID(test.targetSource) || reference.TargetID() != target.ID() || reference.Ref() != target.Name() {
						t.Fatalf("graph IDs and QName = %v/%v/%v/%s", owner.ID(), target.ID(), reference.TargetID(), reference.Ref())
					}
					ownerXML := test.root
					if test.ownerSource != "root.xsd" {
						ownerXML = test.ownerXML
					}
					want := namedGroupChoiceLoc(t, ownerXML, `ref="`+test.written+`"`)
					want.source = SourceID(test.ownerSource)
					if reference.RefLoc() != want || choice.Loc().Source() != SourceID(test.ownerSource) {
						t.Fatalf("graph locations = %s/%s, want %s", reference.RefLoc(), choice.Loc(), want)
					}
				}
			})
		}
	}
}

func TestNamedGroupSequenceNestedChoiceReferenceFailures(t *testing.T) { //nolint:gocognit // Alternate exits are checked at the same nested boundary.
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
			{"self cycle", `<xs:group ref="g:Outer"/>`, `ref="g:Outer"`, diagnosticSchemaModelGroupReferenceCycleCode, errSchemaModelGroupReferenceCycle, true, []string{`<xs:group name="Outer"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupChoiceSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0">`+test.child+`</xs:choice>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := schemaElementReferenceSpecRef(profile.factVersion)
				if test.group {
					wantSpec = schemaModelGroupReferenceSpecRef(profile.factVersion)
				}
				if test.name == "self cycle" {
					wantSpec = "xsd11-structures#mg-props-correct"
					if profile.factVersion == XSDVersion10 {
						wantSpec = "xsd10-structures#mg-props-correct"
					}
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with %s", diagnostic, test.code, test.marker, wantSpec)
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

func TestNamedGroupSequenceNestedChoiceInvisibleReferences(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, target, code, spec string
			cause                    error
		}{
			{"group", `<xs:group name="Target"><xs:sequence/></xs:group>`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.factVersion), errSchemaModelGroupReferenceNamespace},
			{"element", `<xs:element name="Target" type="xs:integer"/>`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				kind := test.name
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:nested-choice"><xs:include schemaLocation="child.xsd"/><xs:group name="Outer"><xs:sequence><xs:choice minOccurs="0" maxOccurs="0"><xs:` + kind + ` ref="o:Target"/></xs:choice></xs:sequence></xs:group></xs:schema>`
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + test.target + `</xs:schema>`
				fixtures := map[string]discoveryFixture{
					"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:nested-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
					"other.xsd": {id: "other.xsd", contents: other},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := namedGroupChoiceLoc(t, other, `<xs:`+kind+` name="Target"`)
				wantRelated.source = "other.xsd"
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:Target"`) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, test.cause) {
					t.Fatalf("invisible diagnostic = %s, related %v", diagnostic, diagnostic.Related())
				}
			})
		}
	}
}

func TestNamedGroupSequenceNestedChoiceAmbiguousResolutionBoundary(t *testing.T) {
	name := mustTestQName(t, "urn:nested-choice", "Target")
	ownerName := mustTestQName(t, "urn:nested-choice", "Outer")
	owner := schemaComponentRecord{id: ComponentID{source: "root.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: ownerName, loc: mustTestLoc(t, "root.xsd", 2, 3)}
	first := schemaComponentRecord{id: ComponentID{source: "one.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: mustTestLoc(t, "one.xsd", 2, 3), modelGroup: &schemaModelGroupInput{particle: &schemaSequenceParticleInput{}}}
	second := schemaComponentRecord{id: ComponentID{source: "two.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: mustTestLoc(t, "two.xsd", 2, 3), modelGroup: &schemaModelGroupInput{particle: &schemaSequenceParticleInput{}}}
	refLoc := mustTestLoc(t, "root.xsd", 3, 28)
	zero := namedGroupTestOccurrenceRange(t, "0", "0")
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			input := &schemaSequenceParticleInput{loc: owner.loc, occurrences: namedGroupTestOccurrenceRange(t, "1", "1"), particles: []schemaParticleTermInput{
				&schemaChoiceParticleInput{loc: refLoc, occurrences: zero, alternatives: []schemaParticleTermInput{
					&schemaModelGroupReferenceParticleInput{loc: refLoc, occurrences: zero, reference: &schemaModelGroupReferenceInput{name: name, loc: refLoc}},
				}},
			}}
			resolver := schemaModelGroupResolver{
				records:        []schemaComponentRecord{owner, first, second},
				byName:         map[QName][]int{name: {1, 2}},
				visibleSources: map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
				version:        profile.factVersion,
			}
			particle, err := resolver.resolveSequence(input, owner)
			if particle != nil {
				t.Fatalf("ambiguous nested choice returned %T", particle)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceAmbiguousCode || diagnostic.Loc() != refLoc || diagnostic.SpecRef() != schemaModelGroupReferenceSpecRef(profile.factVersion) || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) || !errors.Is(err, errSchemaModelGroupReferenceAmbiguous) {
				t.Fatalf("ambiguous diagnostic = %s, related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

func TestNamedGroupSequenceNestedChoiceDuplicateElementReference(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupChoiceSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="g:first"/><xs:element ref="g:first" minOccurs="0" maxOccurs="0"/></xs:choice>`)
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

func TestNamedGroupSequenceNestedChoiceOccurrenceFailures(t *testing.T) { //nolint:gocognit // Lexical and interval exits precede reference resolution, including under 0/0.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, term, marker, code, spec string
			cause                          error
			related                        []string
		}{
			{"malformed minimum", `<xs:choice minOccurs="bad" maxOccurs="0"><xs:group ref="g:missing"/></xs:choice>`, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil, nil},
			{"reversed bounds", `<xs:choice minOccurs="2" maxOccurs="1"><xs:group ref="g:missing"/></xs:choice>`, `<xs:choice minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"malformed child bound", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="g:first" maxOccurs="bad"/></xs:choice>`, `maxOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupChoiceSchema(profile.version, test.term)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || diagnostic.SpecRef() != test.spec {
					t.Fatalf("occurrence diagnostic = %s, want %s at %s with %s", diagnostic, test.code, test.marker, test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("occurrence cause = %v, want %v", err, test.cause)
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

func TestNamedGroupSequenceNestedChoiceExcludedShapes(t *testing.T) { //nolint:gocognit // Every excluded child uses the same located policy matrix.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, child, marker string
			code                string
			class               FailureClass
		}{
			{"deeper choice", `<xs:choice><xs:element ref="g:first"/></xs:choice>`, `<xs:choice><xs:element`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"deeper sequence", `<xs:sequence><xs:element ref="g:first"/></xs:sequence>`, `<xs:sequence><xs:element`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"local declaration", `<xs:element name="local" type="xs:integer"/>`, `<xs:element name="local"`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"wildcard", `<xs:any/>`, `<xs:any`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"all", `<xs:all/>`, `<xs:all`, invalidSchemaCompositionCode, FailureInvalid},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := nestedGroupChoiceSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0">`+test.child+`</xs:choice>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpec := schemaSyntaxSpecRefForVersion(profile.factVersion)
				if test.name == "all" {
					wantSpec = schemaGroupParticleSpecRef(profile.factVersion)
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 {
					t.Fatalf("diagnostic = %s, want %s at %s", diagnostic, test.code, test.marker)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported cause was lost: %v", err)
				}
			})
		}
	}
}

func TestNamedGroupSequenceRejectsSecondNestedChoice(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := nestedGroupChoiceSchema(profile.version, `<xs:choice><xs:element ref="g:first"/></xs:choice><xs:choice><xs:group ref="g:Target"/></xs:choice>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			first := namedGroupChoiceLoc(t, root, `<xs:choice>`)
			second := namedGroupLastLoc(t, root, `<xs:choice>`)
			if first == second || diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != second || diagnostic.SpecRef() != schemaGroupParticleSpecRef(profile.factVersion) || len(diagnostic.Related()) != 0 {
				t.Fatalf("second choice diagnostic = %s, want invalid at %s", diagnostic, second)
			}
		})
	}
}

func TestNamedGroupSequenceNestedChoiceConsumers(t *testing.T) { //nolint:gocognit // Both consumer gates share the same named-group owner.
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:nested-choice" targetNamespace="urn:nested-choice" version="` + profile.version + `">
  <xs:element name="root" type="g:Container"/><xs:complexType name="Container"><xs:group ref="g:Outer"/></xs:complexType>
  <xs:group name="Outer"><xs:sequence><xs:choice><xs:element ref="g:item"/></xs:choice></xs:sequence></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Outer"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:nested-choice"><item>1</item></root>`)))
			validation := requireDiagnostic(t, validationErr)
			wantValidationSpec := instanceValidationXSD11SpecRef
			if profile.factVersion == XSDVersion10 {
				wantValidationSpec = instanceValidationXSD10SpecRef
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantLoc || validation.SpecRef() != wantValidationSpec || len(validation.Related()) == 0 || !errors.Is(validationErr, errInstanceModelGroupReference) {
				t.Fatalf("validation = %s, want unsupported at %s", validation, wantLoc)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			wantCodegenSpec := codegenDirectSequenceXSD11ParticlesSpecRef
			if profile.factVersion == XSDVersion10 {
				wantCodegenSpec = codegenDirectSequenceXSD10ParticlesSpecRef
			}
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantLoc || generation.SpecRef() != wantCodegenSpec || len(generation.Related()) == 0 || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
				t.Fatalf("generation = (%q, %s), want nil unsupported at %s", output, generation, wantLoc)
			}
		})
	}
}
