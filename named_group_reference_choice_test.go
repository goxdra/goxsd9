package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestNamedGroupChoiceRetainsOpaqueReferencesAcrossPolicies(t *testing.T) { //nolint:gocognit,funlen // One public fact matrix covers ordering, copies, omission, and editions.
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := namedGroupChoiceReferenceRoot(profile.version)
			var schema Schema
			for repeat := 0; repeat < 3; repeat++ {
				current, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if repeat == 0 {
					schema = current
					continue
				}
				if !reflect.DeepEqual(schema.Components(), current.Components()) {
					t.Fatal("repeated discovery changed components")
				}
			}
			outer := namedGroupChoiceReferenceDefinition(t, schema, "Outer")
			choice, ok := outer.Particle().(ChoiceParticle)
			if !ok {
				t.Fatalf("outer particle = %T, want choice", outer.Particle())
			}
			alternatives := choice.Alternatives()
			if len(alternatives) != 4 {
				t.Fatalf("alternatives = %d, want four after 0/0 omission", len(alternatives))
			}
			if _, ok := alternatives[0].(ElementReferenceParticle); !ok {
				t.Fatalf("first term = %T, want element ref", alternatives[0])
			}
			if _, ok := alternatives[3].(ElementReferenceParticle); !ok {
				t.Fatalf("last term = %T, want element ref", alternatives[3])
			}
			for _, check := range []struct {
				index                         int
				name, lexical, bounds, target string
			}{
				{1, "ChoiceTarget", `ref="g:ChoiceTarget"`, "0/1", "ChoiceTarget"},
				{2, "SequenceTarget", `ref="g:SequenceTarget"`, "2/18446744073709551616", "SequenceTarget"},
			} {
				ref, ok := alternatives[check.index].(ModelGroupReferenceParticle)
				if !ok {
					t.Fatalf("term %d = %T, want group ref", check.index, alternatives[check.index])
				}
				name := mustTestQName(t, "urn:group-choice-ref", check.name)
				target := namedGroupChoiceReferenceDefinition(t, schema, check.target)
				if ref.Name() != name || ref.Ref() != name || ref.TargetID() != target.ID() {
					t.Fatalf("term %d QName/target = %q/%q/%v, want %q/%v", check.index, ref.Name(), ref.Ref(), ref.TargetID(), name, target.ID())
				}
				if ref.Occurrences().String() != check.bounds || ref.MinOccurs() != 0 || ref.MaxOccurs() != 0 {
					t.Fatalf("term %d bounds = %s (%d/%d), want exact %s", check.index, ref.Occurrences(), ref.MinOccurs(), ref.MaxOccurs(), check.bounds)
				}
				if ref.Loc() != namedGroupChoiceLoc(t, root, `<xs:group `+check.lexical) || ref.RefLoc() != namedGroupChoiceLoc(t, root, check.lexical) {
					t.Fatalf("term %d locations = %s/%s", check.index, ref.Loc(), ref.RefLoc())
				}
				if check.index == 2 {
					minimum := ref.Occurrences().Minimum()
					maximum, finite := ref.Occurrences().Maximum().Finite()
					if !finite {
						t.Fatal("huge finite maximum became unbounded")
					}
					minimum.value.SetInt64(99)
					maximum.value.SetInt64(99)
					if ref.Occurrences().String() != check.bounds {
						t.Fatal("mutating returned occurrence values changed group reference facts")
					}
				}
			}
			if _, ok := namedGroupChoiceReferenceDefinition(t, schema, "ChoiceTarget").Particle().(ChoiceParticle); !ok {
				t.Fatal("choice target did not complete")
			}
			if _, ok := namedGroupChoiceReferenceDefinition(t, schema, "SequenceTarget").Particle().(SequenceParticle); !ok {
				t.Fatal("sequence target did not complete")
			}
			before := schema.Components()
			alternatives[1] = nil
			if choice.Alternatives()[1] == nil || !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("public choice slice changed completed facts")
			}
			walked := make([]ComponentID, 0)
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			if len(walked) != len(schema.Components()) {
				t.Fatalf("walked %d components, want %d without expanded members", len(walked), len(schema.Components()))
			}
			for index, component := range schema.Components() {
				if walked[index] != component.ID() {
					t.Fatalf("walk index %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

func namedGroupChoiceReferenceRoot(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref" version="` + version + `">
  <xs:group name="Outer"><xs:choice>
    <xs:element ref="g:first"/>
    <xs:group ref="g:ChoiceTarget" minOccurs="0"/>
    <xs:group ref="g:SequenceTarget" minOccurs="2" maxOccurs="18446744073709551616"/>
    <xs:group ref="g:ChoiceTarget" minOccurs="0" maxOccurs="0"/>
    <xs:element ref="g:last"/>
  </xs:choice></xs:group>
  <xs:group name="ChoiceTarget"><xs:choice><xs:element ref="g:first"/></xs:choice></xs:group>
  <xs:group name="SequenceTarget"><xs:sequence><xs:element ref="g:last"/></xs:sequence></xs:group>
  <xs:element name="first" type="xs:integer"/><xs:element name="last" type="xs:integer"/>
</xs:schema>`
}

func namedGroupChoiceReferenceDefinition(t *testing.T, schema Schema, local string) ModelGroupDefinition {
	t.Helper()
	groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:group-choice-ref", local))
	if len(groups) != 1 {
		t.Fatalf("group %s count = %d, want 1", local, len(groups))
	}
	definition, ok := groups[0].ModelGroupDefinition()
	if !ok {
		t.Fatalf("group %s has no model-group view", local)
	}
	return definition
}

func TestNamedGroupChoiceReferenceDiagnostics(t *testing.T) { //nolint:gocognit // All exits from the named-choice ref boundary share exact assertions.
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
			related            string
		}{
			{"unresolved", "", diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved, ""},
			{"wrong kind", `<xs:element name="Target" type="xs:integer"/>`, diagnosticSchemaModelGroupReferenceWrongKindCode, errSchemaModelGroupReferenceWrongKind, `<xs:element name="Target"`},
			{"cycle", `<xs:group name="Target"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:group>`, diagnosticSchemaModelGroupReferenceCycleCode, errSchemaModelGroupReferenceCycle, `<xs:group name="Target"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref" version="` + profile.version + `">
  <xs:group name="Outer"><xs:choice><xs:group ref="g:Target" minOccurs="0" maxOccurs="0"/></xs:choice></xs:group>
  ` + test.target + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Target"`)
				wantSpec := profile.spec
				if test.name == "cycle" {
					wantLoc = namedGroupLastLoc(t, root, `ref="g:Target"`)
					wantSpec = profile.cycleSpec
				}
				if diagnostic.Code() != test.code || diagnostic.Class() != FailureInvalid || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %s, want %s at %s spec %s with cause %v", diagnostic, test.code, wantLoc, wantSpec, test.cause)
				}
				if test.related != "" && (len(diagnostic.Related()) == 0 || diagnostic.Related()[0] != namedGroupChoiceLoc(t, root, test.related)) {
					t.Fatalf("related = %v, want target declaration", diagnostic.Related())
				}
			})
		}
	}
}

func TestNamedGroupChoiceReferenceConsumersRemainUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref" version="1.1">
  <xs:element name="root" type="g:Container"/>
  <xs:complexType name="Container"><xs:group ref="g:Outer"/></xs:complexType>
  <xs:group name="Outer"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:group>
  <xs:group name="Target"><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatal(err)
	}
	wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Outer"`)
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:group-choice-ref"><item>1</item></root>`)))
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Loc() != wantLoc || !errors.Is(validationErr, errInstanceModelGroupReference) {
		t.Fatalf("validation = %s, want unsupported at %s", validationDiagnostic, wantLoc)
	}
	output, generationErr := GenerateGo(schema, "generated")
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if output != nil || generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Loc() != wantLoc || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
		t.Fatalf("generation = (%q, %s), want nil unsupported at %s", output, generationDiagnostic, wantLoc)
	}
}

func TestNamedGroupChoiceReferenceGraphVisibility(t *testing.T) { //nolint:gocognit // The cases share one public identity/order contract.
	for _, test := range []struct {
		name, root, ownerNS, targetNS, targetSource string
		fixtures                                    map[string]discoveryFixture
	}{
		{
			name: "repeated included chameleon",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:group-choice-ref">
  <xs:include schemaLocation="child.xsd"/><xs:include schemaLocation="child.xsd"/>
  <xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`,
			ownerNS: "urn:group-choice-ref", targetNS: "urn:group-choice-ref", targetSource: "root.xsd",
			fixtures: map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Outer"><xs:choice><xs:group ref="Target"/></xs:choice></xs:group></xs:schema>`},
			},
		},
		{
			name: "imported forward target",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other-choice" targetNamespace="urn:group-choice-ref">
  <xs:import namespace="urn:other-choice" schemaLocation="other.xsd"/>
  <xs:group name="Outer"><xs:choice><xs:group ref="o:Target"/></xs:choice></xs:group></xs:schema>`,
			ownerNS: "urn:group-choice-ref", targetNS: "urn:other-choice", targetSource: "other.xsd",
			fixtures: map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other-choice"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`},
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
					t.Fatal("repeated graph discovery changed components")
				}
				outer := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, test.ownerNS, "Outer"))
				target := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, test.targetNS, "Target"))
				if len(outer) != 1 || len(target) != 1 || target[0].ID().Source() != SourceID(test.targetSource) {
					t.Fatalf("groups = %v/%v", outer, target)
				}
				definition, ok := outer[0].ModelGroupDefinition()
				if !ok {
					t.Fatal("no outer definition")
				}
				choice, ok := definition.Particle().(ChoiceParticle)
				if !ok || len(choice.Alternatives()) != 1 {
					t.Fatalf("outer particle = %T", definition.Particle())
				}
				ref, ok := choice.Alternatives()[0].(ModelGroupReferenceParticle)
				if !ok || ref.TargetID() != target[0].ID() || ref.Name() != target[0].Name() {
					t.Fatalf("reference = %v, want %v", ref, target[0].ID())
				}
				if ref.RefLoc().Source() != outer[0].ID().Source() {
					t.Fatalf("ref Loc source = %s, want %s", ref.RefLoc().Source(), outer[0].ID().Source())
				}
			}
		})
	}
}

func TestNamedGroupChoiceReferenceExclusionsStayLocated(t *testing.T) {
	profiles := []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	}
	for _, test := range []struct {
		name, body, marker string
	}{
		{"nested choice", `<xs:group name="Outer"><xs:choice><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:choice></xs:group>`, `ref="g:Target"`},
		{"deeper sequence", `<xs:group name="Outer"><xs:choice><xs:sequence><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:sequence></xs:choice></xs:group>`, `ref="g:Target"`},
		{"inline complex choice reference", `<xs:element name="root"><xs:complexType><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType></xs:element>`, `ref="g:Target"`},
		{"attribute group composition", `<xs:attributeGroup name="Attrs"><xs:attributeGroup ref="g:Other"/></xs:attributeGroup>`, `<xs:attributeGroup ref=`},
		{"all target", `<xs:group name="Outer"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:group><xs:group name="Target"><xs:all/></xs:group>`, `<xs:all`},
	} {
		for _, profile := range profiles {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				body := test.body
				if test.name != "all target" {
					body += `<xs:group name="Target"><xs:sequence/></xs:group>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref" version="` + profile.version + `">` + body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want unsupported at %s", diagnostic, namedGroupChoiceLoc(t, root, test.marker))
				}
			})
		}
	}
}

func TestNamedGroupChoiceReferenceInvisibleImport(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other-choice" targetNamespace="urn:group-choice-ref">
  <xs:include schemaLocation="child.xsd"/>
  <xs:group name="Outer"><xs:choice><xs:group ref="o:Target"/></xs:choice></xs:group>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other-choice"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:group-choice-ref"><xs:import namespace="urn:other-choice" schemaLocation="other.xsd"/></xs:schema>`},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantRelated := namedGroupChoiceLoc(t, other, `<xs:group name="Target"`)
	wantRelated = Loc{source: "other.xsd", line: wantRelated.line, column: wantRelated.column}
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:Target"`) || diagnostic.SpecRef() != schemaModelGroupReferenceImportXSD11SpecRef || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
		t.Fatalf("invisible import diagnostic = %s, related %v", diagnostic, diagnostic.Related())
	}
}

func TestNamedGroupChoiceReferenceIndirectCycleKeepsPath(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref">
  <xs:group name="A"><xs:choice><xs:group ref="g:B"/></xs:choice></xs:group>
  <xs:group name="B"><xs:choice><xs:group ref="g:C" minOccurs="0" maxOccurs="0"/></xs:choice></xs:group>
  <xs:group name="C"><xs:choice><xs:group ref="g:A"/></xs:choice></xs:group>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantRelated := []Loc{
		namedGroupChoiceLoc(t, root, `<xs:group name="A"`),
		namedGroupChoiceLoc(t, root, `<xs:group name="B"`),
		namedGroupChoiceLoc(t, root, `ref="g:B"`),
		namedGroupChoiceLoc(t, root, `<xs:group name="C"`),
		namedGroupChoiceLoc(t, root, `ref="g:C"`),
	}
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:A"`) || diagnostic.SpecRef() != "xsd11-structures#mg-props-correct" || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
		t.Fatalf("indirect cycle diagnostic = %s, related %v, want %v", diagnostic, diagnostic.Related(), wantRelated)
	}
}

func TestNamedGroupChoiceReferenceInvalidAttributesPrecedeTargetResolution(t *testing.T) {
	for _, test := range []struct{ name, attrs, marker, code string }{
		{"invalid occurrence", ` minOccurs="bad"`, `minOccurs="bad"`, invalidSchemaCompositionCode},
		{"missing ref", ``, `<xs:group/>`, invalidSchemaCompositionCode},
		{"duplicate ref", ` ref="g:Other"`, `ref="g:Other"`, "XSD3001"},
	} {
		t.Run(test.name, func(t *testing.T) {
			group := `<xs:group ref="g:Missing"` + test.attrs + `/>`
			if test.name == "missing ref" {
				group = `<xs:group/>`
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref"><xs:group name="Outer"><xs:choice>` + group + `</xs:choice></xs:group></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) {
				t.Fatalf("invalid attribute = %s, want composition at %s", diagnostic, namedGroupChoiceLoc(t, root, test.marker))
			}
		})
	}
}

func TestNamedGroupChoiceGroupReferenceUnsupportedDoesNotHideInvalidSyntax(t *testing.T) { //nolint:gocognit // Public diagnostic matrix covers the sibling and child exits of one boundary.
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
			name, children, marker string
			related                []string
			spec                   func(XSDVersion) string
			lexical, reversed      bool
		}{
			{
				name: "group attribute before later malformed element occurrence",
				children: `<xs:group ref="g:Target" xml:base="child/"/>` +
					`<xs:element ref="g:item" minOccurs="bad"/>`,
				marker: `minOccurs="bad"`, spec: schemaParticleOccurrenceDatatypeSpecRef, lexical: true,
			},
			{
				name: "group annotation before later reversed element occurrences",
				children: `<xs:group ref="g:Target"><xs:annotation xml:base="child/"/></xs:group>` +
					`<xs:element ref="g:item" minOccurs="2" maxOccurs="1"/>`,
				marker: `<xs:element ref="g:item"`, related: []string{`minOccurs="2"`, `maxOccurs="1"`},
				spec: schemaParticleCorrectSpecRef, reversed: true,
			},
			{
				name:     "group attribute before forbidden group attribute",
				children: `<xs:group ref="g:Target" xml:base="child/" bogus="1"/>`,
				marker:   `bogus="1"`, spec: schemaGroupParticleSpecRef,
			},
			{
				name:     "group attribute before forbidden group child",
				children: `<xs:group ref="g:Target" xml:base="child/"><xs:element/></xs:group>`,
				marker:   `<xs:element/>`, spec: schemaGroupParticleSpecRef,
			},
			{
				name:     "group attribute before invalid annotation",
				children: `<xs:group ref="g:Target" xml:base="child/"><xs:annotation id="bad:id"/></xs:group>`,
				marker:   `id="bad:id"`, spec: schemaGroupParticleSpecRef,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedGroupChoiceGroupReferenceStagingRoot(profile.version, "", test.children)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := namedGroupChoiceLoc(t, root, test.marker)
				wantSpec := test.spec(profile.factVersion)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s (spec %s), want invalid XSD3010 at %s with %s", diagnostic, diagnostic.SpecRef(), wantLoc, wantSpec)
				}
				var wantRelated []Loc
				for _, marker := range test.related {
					wantRelated = append(wantRelated, namedGroupChoiceLoc(t, root, marker))
				}
				if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), wantRelated)
				}
				if test.reversed {
					if !errors.Is(err, errParticleOccurrenceMinimumExceedsMaximum) {
						t.Fatalf("reversed occurrence lost cause: %v", err)
					}
					return
				}
				if test.lexical {
					var cause Diagnostic
					if !errors.As(errors.Unwrap(diagnostic), &cause) || cause.Code() != InvalidIntegerLexicalCode || cause.Loc() != wantLoc {
						t.Fatalf("malformed occurrence lost located lexical cause: %v", errors.Unwrap(diagnostic))
					}
					return
				}
				if errors.Unwrap(diagnostic) != nil {
					t.Fatalf("composition error has unexpected cause: %v", errors.Unwrap(diagnostic))
				}
			})
		}
	}
}

func TestNamedGroupChoiceGroupReferenceUnsupportedOrdering(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	} {
		for _, test := range []struct {
			name, choiceAttrs, children, marker string
		}{
			{
				name: "first group attribute before later annotation",
				children: `<xs:group ref="g:Target" xml:base="first/"/>` +
					`<xs:group ref="g:Target"><xs:annotation xml:base="second/"/></xs:group>`,
				marker: `xml:base="first/"`,
			},
			{
				name: "first group annotation before later attribute",
				children: `<xs:group ref="g:Target"><xs:annotation xml:base="first/"/></xs:group>` +
					`<xs:group ref="g:Target" xml:base="second/"/>`,
				marker: `xml:base="first/"`,
			},
			{
				name:        "outer choice attribute before group attribute",
				choiceAttrs: ` xml:base="first/"`,
				children:    `<xs:group ref="g:Target" xml:base="second/"/>`,
				marker:      `xml:base="first/"`,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedGroupChoiceGroupReferenceStagingRoot(profile.version, test.choiceAttrs, test.children)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := namedGroupChoiceLoc(t, root, test.marker)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != featureSchemaXMLBase || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != "xmlbase#matching" || len(diagnostic.Related()) != 0 || errors.Unwrap(diagnostic) != nil || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s (feature %s, spec %s, related %v, cause %v), want first XML Base candidate at %s", diagnostic, diagnostic.Feature(), diagnostic.SpecRef(), diagnostic.Related(), errors.Unwrap(diagnostic), wantLoc)
				}
			})
		}
	}
}

func namedGroupChoiceGroupReferenceStagingRoot(version, choiceAttrs, children string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-choice-ref" targetNamespace="urn:group-choice-ref" version="` + version + `">
  <xs:group name="Outer"><xs:choice` + choiceAttrs + `>` + children + `</xs:choice></xs:group>
  <xs:group name="Target"><xs:sequence/></xs:group>
  <xs:element name="item" type="xs:string"/>
</xs:schema>`
}
