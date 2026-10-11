package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func namedGroupSequenceReferenceRoot(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="` + version + `">
  <xs:group name="Outer"><xs:sequence>
    <xs:element ref="g:first"/>
    <xs:group ref="g:ChoiceTarget" minOccurs="0"/>
    <xs:group ref="g:SequenceTarget" minOccurs="2" maxOccurs="18446744073709551616"/>
    <xs:group ref="g:ChoiceTarget" minOccurs="0" maxOccurs="0"/>
    <xs:element ref="g:last"/>
  </xs:sequence></xs:group>
  <xs:group name="ChoiceTarget"><xs:choice><xs:element ref="g:first"/></xs:choice></xs:group>
  <xs:group name="SequenceTarget"><xs:sequence><xs:element ref="g:last"/></xs:sequence></xs:group>
  <xs:element name="first" type="xs:integer"/><xs:element name="last" type="xs:integer"/>
</xs:schema>`
}

func namedSequenceGroup(t *testing.T, schema Schema, namespace, local string) ModelGroupDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("group %s count = %d, want one", local, len(components))
	}
	definition, ok := components[0].ModelGroupDefinition()
	if !ok {
		t.Fatalf("group %s has no definition view", local)
	}
	return definition
}

func TestNamedGroupSequenceRetainsOpaqueReferencesAcrossPolicies(t *testing.T) { //nolint:gocognit,funlen // One public fact matrix covers exact bounds, order, copies, and editions.
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := namedGroupSequenceReferenceRoot(profile.version)
			var schema Schema
			for repeat := 0; repeat < 3; repeat++ {
				current, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if repeat == 0 {
					schema = current
					continue
				}
				if !reflect.DeepEqual(schema.Components(), current.Components()) {
					t.Fatal("repeated discovery changed ordered components")
				}
			}
			outer := namedSequenceGroup(t, schema, "urn:sequence-ref", "Outer")
			sequence, ok := outer.Particle().(SequenceParticle)
			if !ok {
				t.Fatalf("outer particle = %T, want sequence", outer.Particle())
			}
			particles := sequence.Particles()
			if len(particles) != 4 {
				t.Fatalf("particles = %d, want four in lexical order after 0/0 omission", len(particles))
			}
			if _, ok := particles[0].(ElementReferenceParticle); !ok {
				t.Fatalf("first term = %T", particles[0])
			}
			if _, ok := particles[3].(ElementReferenceParticle); !ok {
				t.Fatalf("last term = %T", particles[3])
			}
			if len(sequence.Elements()) != 0 {
				t.Fatal("element-only view included reference terms")
			}
			for _, check := range []struct {
				index        int
				name, bounds string
			}{
				{1, "ChoiceTarget", "0/1"},
				{2, "SequenceTarget", "2/18446744073709551616"},
			} {
				reference, ok := particles[check.index].(ModelGroupReferenceParticle)
				if !ok {
					t.Fatalf("term %d = %T, want group reference", check.index, particles[check.index])
				}
				name := mustTestQName(t, "urn:sequence-ref", check.name)
				target := namedSequenceGroup(t, schema, "urn:sequence-ref", check.name)
				marker := `ref="g:` + check.name + `"`
				if reference.Name() != name || reference.Ref() != name || reference.TargetID() != target.ID() || reference.TargetID().IsZero() {
					t.Fatalf("term %d name/target = %q/%v, want %q/%v", check.index, reference.Name(), reference.TargetID(), name, target.ID())
				}
				if reference.Occurrences().String() != check.bounds || reference.Loc() != namedGroupChoiceLoc(t, root, `<xs:group `+marker) || reference.RefLoc() != namedGroupChoiceLoc(t, root, marker) {
					t.Fatalf("term %d bounds/locations = %s/%s/%s", check.index, reference.Occurrences(), reference.Loc(), reference.RefLoc())
				}
				if reference.MinOccurs() != 0 || reference.MaxOccurs() != 0 {
					t.Fatalf("term %d legacy bounds suggest an exact range", check.index)
				}
			}
			minimum := particles[2].Occurrences().Minimum()
			maximum, finite := particles[2].Occurrences().Maximum().Finite()
			if !finite {
				t.Fatal("large finite maximum became unbounded")
			}
			minimum.value.SetInt64(99)
			maximum.value.SetInt64(99)
			if particles[2].Occurrences().String() != "2/18446744073709551616" {
				t.Fatal("mutating returned bounds changed completed facts")
			}
			before := schema.Components()
			particles[1] = nil
			if sequence.Particles()[1] == nil || !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("mutating returned particles changed completed facts")
			}
			walked := make([]ComponentID, 0)
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			for index, component := range schema.Components() {
				if walked[index] != component.ID() {
					t.Fatalf("walk index %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

func TestNamedGroupSequenceReferenceConsumersRejectOpaqueOwner(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="1.1">
  <xs:element name="root" type="g:Container"/>
  <xs:complexType name="Container"><xs:group ref="g:Outer"/></xs:complexType>
  <xs:group name="Outer"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:group>
  <xs:group name="Target"><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatal(err)
	}
	outer := namedSequenceGroup(t, schema, "urn:sequence-ref", "Outer")
	sequence, ok := outer.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("outer particle = %T, want sequence", outer.Particle())
	}
	if _, ok := sequence.Particles()[0].(ModelGroupReferenceParticle); !ok {
		t.Fatal("owning group lost nested reference term")
	}
	wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Outer"`)
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:sequence-ref"><item>1</item></root>`)))
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.SpecRef() != "xsd11-structures#cvc-elt" || validationDiagnostic.Loc() != wantLoc || len(validationDiagnostic.Related()) == 0 || !errors.Is(validationErr, errInstanceModelGroupReference) {
		t.Fatalf("validation = %s, want unsupported at %s", validationDiagnostic, wantLoc)
	}
	output, generationErr := GenerateGo(schema, "generated")
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if output != nil || generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.SpecRef() != codegenDirectSequenceXSD11ParticlesSpecRef || generationDiagnostic.Loc() != wantLoc || len(generationDiagnostic.Related()) == 0 || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
		t.Fatalf("generation = (%q, %s), want nil unsupported at %s", output, generationDiagnostic, wantLoc)
	}
}

func TestNamedGroupSequenceReferenceInvisibleAndAmbiguous(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:sequence-other" targetNamespace="urn:sequence-ref" version="1.1">
  <xs:include schemaLocation="child.xsd"/>
  <xs:group name="Outer"><xs:sequence><xs:group ref="o:Target"/></xs:sequence></xs:group>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:sequence-other"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:sequence-ref"><xs:import namespace="urn:sequence-other" schemaLocation="other.xsd"/></xs:schema>`},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantRelated := namedGroupChoiceLoc(t, other, `<xs:group name="Target"`)
	wantRelated.source = "other.xsd"
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || diagnostic.SpecRef() != schemaModelGroupReferenceImportXSD11SpecRef || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:Target"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
		t.Fatalf("invisible diagnostic = %s, related %v", diagnostic, diagnostic.Related())
	}

	name := mustTestQName(t, "urn:sequence-ref", "Target")
	owner := schemaComponentRecord{id: ComponentID{source: "root.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: mustTestQName(t, "urn:sequence-ref", "Outer"), loc: mustTestLoc(t, "root.xsd", 2, 3)}
	first := schemaComponentRecord{id: ComponentID{source: "one.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: mustTestLoc(t, "one.xsd", 2, 3), modelGroup: &schemaModelGroupInput{particle: &schemaSequenceParticleInput{}}}
	second := schemaComponentRecord{id: ComponentID{source: "two.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: mustTestLoc(t, "two.xsd", 2, 3), modelGroup: &schemaModelGroupInput{particle: &schemaSequenceParticleInput{}}}
	refLoc := mustTestLoc(t, "root.xsd", 3, 25)
	zero := namedGroupTestOccurrenceRange(t, "0", "0")
	input := &schemaSequenceParticleInput{loc: owner.loc, occurrences: namedGroupTestOccurrenceRange(t, "1", "1"), particles: []schemaParticleTermInput{&schemaModelGroupReferenceParticleInput{loc: owner.loc, occurrences: zero, reference: &schemaModelGroupReferenceInput{name: name, loc: refLoc}}}}
	resolver := schemaModelGroupResolver{
		records:        []schemaComponentRecord{owner, first, second},
		byName:         map[QName][]int{name: {1, 2}},
		visibleSources: map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
		version:        XSDVersion11,
	}
	_, err = resolver.resolveSequence(input, owner)
	diagnostic = requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceAmbiguousCode || diagnostic.SpecRef() != schemaModelGroupReferenceXSD11SpecRef || diagnostic.Loc() != refLoc || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) || !errors.Is(err, errSchemaModelGroupReferenceAmbiguous) {
		t.Fatalf("ambiguous 0/0 diagnostic = %s, related %v", diagnostic, diagnostic.Related())
	}
}

func TestNamedGroupSequenceReferenceIndirectMixedCycle(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="1.1">
  <xs:group name="A"><xs:sequence><xs:group ref="g:B"/></xs:sequence></xs:group>
  <xs:group name="B"><xs:choice><xs:group ref="g:C" minOccurs="0" maxOccurs="0"/></xs:choice></xs:group>
  <xs:group name="C"><xs:sequence><xs:group ref="g:A"/></xs:sequence></xs:group>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	related := []Loc{
		namedGroupChoiceLoc(t, root, `<xs:group name="A"`),
		namedGroupChoiceLoc(t, root, `<xs:group name="B"`),
		namedGroupChoiceLoc(t, root, `ref="g:B"`),
		namedGroupChoiceLoc(t, root, `<xs:group name="C"`),
		namedGroupChoiceLoc(t, root, `ref="g:C"`),
	}
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:A"`) || diagnostic.SpecRef() != "xsd11-structures#mg-props-correct" || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
		t.Fatalf("mixed cycle diagnostic = %s, related %v", diagnostic, diagnostic.Related())
	}
}

func TestNamedGroupSequenceReferenceGraphVisibility(t *testing.T) { //nolint:gocognit // Each graph asserts the same public reference identity contract.
	for _, test := range []struct {
		name, root, ownerNS, targetNS, ownerSource, targetSource, written string
		fixtures                                                          map[string]discoveryFixture
	}{
		{
			name: "included forward target",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="1.1">
  <xs:include schemaLocation="child.xsd"/>
  <xs:group name="Outer"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:group>
</xs:schema>`,
			ownerNS: "urn:sequence-ref", targetNS: "urn:sequence-ref", ownerSource: "root.xsd", targetSource: "child.xsd", written: "g:Target",
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:sequence-ref"><xs:group name="Target"><xs:choice/></xs:group></xs:schema>`},
			},
		},
		{
			name: "imported target",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:sequence-other" targetNamespace="urn:sequence-ref" version="1.1">
  <xs:import namespace="urn:sequence-other" schemaLocation="other.xsd"/>
  <xs:group name="Outer"><xs:sequence><xs:group ref="o:Target"/></xs:sequence></xs:group>
</xs:schema>`,
			ownerNS: "urn:sequence-ref", targetNS: "urn:sequence-other", ownerSource: "root.xsd", targetSource: "other.xsd", written: "o:Target",
			fixtures: map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:sequence-other"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`},
			},
		},
		{
			name: "repeated chameleon include",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:sequence-ref" version="1.1">
  <xs:include schemaLocation="child.xsd"/><xs:include schemaLocation="child.xsd"/>
  <xs:group name="Target"><xs:sequence/></xs:group>
</xs:schema>`,
			ownerNS: "urn:sequence-ref", targetNS: "urn:sequence-ref", ownerSource: "child.xsd", targetSource: "root.xsd", written: "Target",
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Outer"><xs:sequence><xs:group ref="Target"/></xs:sequence></xs:group></xs:schema>`},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var first []Component
			for repeat := 0; repeat < 3; repeat++ {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, test.fixtures, Strict11)
				if err != nil {
					t.Fatal(err)
				}
				if repeat == 0 {
					first = schema.Components()
				}
				if !reflect.DeepEqual(first, schema.Components()) {
					t.Fatal("graph discovery order changed")
				}
				outer := namedSequenceGroup(t, schema, test.ownerNS, "Outer")
				target := namedSequenceGroup(t, schema, test.targetNS, "Target")
				sequence, ok := outer.Particle().(SequenceParticle)
				if !ok || len(sequence.Particles()) != 1 {
					t.Fatalf("outer particle = %T", outer.Particle())
				}
				reference, ok := sequence.Particles()[0].(ModelGroupReferenceParticle)
				if !ok {
					t.Fatalf("sequence term = %T", sequence.Particles()[0])
				}
				if outer.ID().Source() != SourceID(test.ownerSource) || target.ID().Source() != SourceID(test.targetSource) || reference.TargetID() != target.ID() || reference.Name() != target.Name() || reference.Ref() != target.Name() {
					t.Fatalf("graph owner/target/ref = %v/%v/%v", outer.ID(), target.ID(), reference.TargetID())
				}
				ownerXML := test.root
				if test.ownerSource != "root.xsd" {
					ownerXML = test.fixtures[test.ownerSource].contents
				}
				wantRefLoc := namedGroupChoiceLoc(t, ownerXML, `ref="`+test.written+`"`)
				wantRefLoc.source = SourceID(test.ownerSource)
				if reference.RefLoc() != wantRefLoc || reference.Loc().Source() != SourceID(test.ownerSource) {
					t.Fatalf("reference locations = %s/%s, want source %s", reference.RefLoc(), reference.Loc(), test.ownerSource)
				}
			}
		})
	}
}

func TestNamedGroupSequenceReferenceDiagnostics(t *testing.T) { //nolint:gocognit // The matrix checks alternate exits at one reference boundary.
	for _, profile := range []struct {
		name, version, spec, cycleSpec string
		policy                         LanguagePolicy
	}{
		{"Compatibility", "1.0", schemaModelGroupReferenceXSD11SpecRef, "xsd11-structures#mg-props-correct", Compatibility},
		{"Strict10", "1.0", schemaModelGroupReferenceXSD10SpecRef, "xsd10-structures#mg-props-correct", Strict10},
		{"Strict11", "1.1", schemaModelGroupReferenceXSD11SpecRef, "xsd11-structures#mg-props-correct", Strict11},
	} {
		for _, test := range []struct {
			name, target, code string
			cause              error
			related            []string
		}{
			{"unresolved", "", diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved, nil},
			{"wrong kind", `<xs:element name="Target" type="xs:integer"/>`, diagnosticSchemaModelGroupReferenceWrongKindCode, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="Target"`}},
			{"cycle through choice", `<xs:group name="Target"><xs:choice><xs:group ref="g:Outer"/></xs:choice></xs:group>`, diagnosticSchemaModelGroupReferenceCycleCode, errSchemaModelGroupReferenceCycle, []string{`<xs:group name="Outer"`, `<xs:group name="Target"`, `ref="g:Target"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="` + profile.version + `">
  <xs:group name="Outer"><xs:sequence><xs:group ref="g:Target" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:group>
  ` + test.target + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Target"`)
				wantSpec := profile.spec
				if test.name == "cycle through choice" {
					wantLoc = namedGroupChoiceLoc(t, root, `ref="g:Outer"`)
					wantSpec = profile.cycleSpec
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s with %s and %v", diagnostic, test.code, wantLoc, wantSpec, test.cause)
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

func TestNamedGroupSequenceReferenceExclusionsStayLocated(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	} {
		for _, test := range []struct {
			name, body, marker string
		}{
			{"named nested choice", `<xs:group name="Outer"><xs:sequence><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:sequence></xs:group>`, `ref="g:Target"`},
			{"named nested sequence", `<xs:group name="Outer"><xs:sequence><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:sequence></xs:group>`, `ref="g:Target"`},
			{"named all target", `<xs:group name="Outer"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:group><xs:group name="Target"><xs:all/></xs:group>`, `<xs:all`},
			{"direct complex choice", `<xs:complexType name="Container"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType>`, `ref="g:Target"`},
			{"direct complex sequence", `<xs:complexType name="Container"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:complexType>`, `ref="g:Target"`},
			{"inline complex choice", `<xs:element name="root"><xs:complexType><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType></xs:element>`, `ref="g:Target"`},
			{"inline complex sequence", `<xs:element name="root"><xs:complexType><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:complexType></xs:element>`, `ref="g:Target"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				body := test.body
				if test.name != "named all target" {
					body += `<xs:group name="Target"><xs:sequence/></xs:group>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="` + profile.version + `">` + body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := namedGroupChoiceLoc(t, root, test.marker)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want unsupported at %s", diagnostic, wantLoc)
				}
			})
		}
	}
}

func TestNamedGroupSequenceReferenceInvalidOccurrenceAndSyntax(t *testing.T) { //nolint:gocognit // Alternate exits must precede the new resolution path in every policy.
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
		factVersion   XSDVersion
	}{
		{"Compatibility", "1.0", Compatibility, XSDVersion11},
		{"Strict10", "1.0", Strict10, XSDVersion10},
		{"Strict11", "1.1", Strict11, XSDVersion11},
	} {
		for _, test := range []struct {
			name, terms, marker, code string
			cause                     error
			spec                      func(XSDVersion) string
			related                   []string
			lexical                   bool
		}{
			{"malformed group minimum", `<xs:group ref="g:Missing" minOccurs="bad"/>`, `minOccurs="bad"`, invalidSchemaCompositionCode, nil, schemaParticleOccurrenceDatatypeSpecRef, nil, true},
			{"reversed group bounds", `<xs:group ref="g:Missing" minOccurs="2" maxOccurs="1"/>`, `<xs:group ref="g:Missing"`, invalidSchemaCompositionCode, errParticleOccurrenceMinimumExceedsMaximum, schemaParticleCorrectSpecRef, []string{`minOccurs="2"`, `maxOccurs="1"`}, false},
			{"missing ref", `<xs:group/>`, `<xs:group/>`, invalidSchemaCompositionCode, nil, schemaGroupParticleSpecRef, nil, false},
			{"forbidden group attribute", `<xs:group ref="g:Missing" bogus="1"/>`, `bogus="1"`, invalidSchemaCompositionCode, nil, schemaGroupParticleSpecRef, nil, false},
			{"forbidden child", `<xs:group ref="g:Missing"><xs:element/></xs:group>`, `<xs:element/>`, invalidSchemaCompositionCode, nil, schemaGroupParticleSpecRef, nil, false},
			{"later malformed element", `<xs:group ref="g:Missing"/><xs:element ref="g:item" minOccurs="bad"/>`, `minOccurs="bad"`, invalidSchemaCompositionCode, nil, schemaParticleOccurrenceDatatypeSpecRef, nil, true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="` + profile.version + `"><xs:group name="Outer"><xs:sequence>` + test.terms + `</xs:sequence></xs:group><xs:element name="item" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := namedGroupChoiceLoc(t, root, test.marker)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec(profile.factVersion) {
					t.Fatalf("diagnostic = %s, want %s at %s with %s", diagnostic, test.code, wantLoc, test.spec(profile.factVersion))
				}
				var related []Loc
				for _, marker := range test.related {
					related = append(related, namedGroupChoiceLoc(t, root, marker))
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic related/cause = %v/%v", diagnostic.Related(), err)
				}
				if test.lexical {
					var cause Diagnostic
					if !errors.As(errors.Unwrap(diagnostic), &cause) || cause.Code() != InvalidIntegerLexicalCode || cause.Loc() != wantLoc {
						t.Fatalf("lexical cause = %v, want located integer diagnostic", errors.Unwrap(diagnostic))
					}
					return
				}
				if test.cause == nil && errors.Unwrap(diagnostic) != nil {
					t.Fatalf("unexpected composition cause: %v", errors.Unwrap(diagnostic))
				}
			})
		}
	}
}

func TestNamedGroupSequenceReferenceUnsupportedOrder(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:sequence-ref" targetNamespace="urn:sequence-ref" version="` + profile.version + `">
  <xs:group name="Outer"><xs:sequence>
    <xs:group ref="g:Target" xml:base="first/"/>
    <xs:group ref="g:Target" xml:base="second/"/>
  </xs:sequence></xs:group>
  <xs:group name="Target"><xs:sequence/></xs:group>
</xs:schema>`
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != featureSchemaXMLBase || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `xml:base="first/"`) || diagnostic.SpecRef() != "xmlbase#matching" || len(diagnostic.Related()) != 0 || errors.Unwrap(diagnostic) != nil || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("ordered unsupported diagnostic = %s, related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}
