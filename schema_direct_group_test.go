package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupRoot(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="` + version + `">
	  <xs:element name="rootChoice" type="g:ChoiceOwner"/><xs:element name="rootSequence" type="g:SequenceOwner"/>
  <xs:complexType name="ChoiceOwner"><xs:choice minOccurs="2" maxOccurs="3">
    <xs:element ref="g:first"/>
    <xs:group ref="g:ChoiceTarget" minOccurs="0" maxOccurs="18446744073709551616"/>
    <xs:group ref="g:SequenceTarget" minOccurs="0" maxOccurs="0"/>
    <xs:element ref="g:last"/>
  </xs:choice><xs:attribute name="label" type="xs:normalizedString"/></xs:complexType>
  <xs:complexType name="SequenceOwner"><xs:sequence>
    <xs:element ref="g:first"/>
    <xs:group ref="g:SequenceTarget" minOccurs="2" maxOccurs="unbounded"/>
    <xs:group ref="g:ChoiceTarget"/>
    <xs:element ref="g:last"/>
  </xs:sequence></xs:complexType>
  <xs:group name="ChoiceTarget"><xs:choice><xs:element ref="g:middle"/></xs:choice></xs:group>
  <xs:group name="SequenceTarget"><xs:sequence><xs:element ref="g:other"/></xs:sequence></xs:group>
  <xs:element name="first" type="xs:integer"/><xs:element name="middle" type="xs:integer"/>
  <xs:element name="other" type="xs:integer"/><xs:element name="last" type="xs:integer"/>
</xs:schema>`
}

func TestDirectNamedGroupReferencesRetainPublicFacts(t *testing.T) { //nolint:gocognit,funlen // One matrix proves the public ordered component contract.
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", "1.0", Compatibility},
		{"Strict10", "1.0", Strict10},
		{"Strict11", "1.1", Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := directGroupRoot(profile.version)
			var first []Component
			for repeat := 0; repeat < 2; repeat++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if repeat == 0 {
					first = schema.Components()
				}
				if !reflect.DeepEqual(first, schema.Components()) {
					t.Fatal("component discovery changed across repeated parses")
				}
				for _, owner := range []struct {
					name, model string
					positions   []int
					groups      []string
					bounds      []string
					lines       []int
				}{
					{"ChoiceOwner", "choice", []int{1}, []string{"ChoiceTarget"}, []string{"0/18446744073709551616"}, []int{5}},
					{"SequenceOwner", "sequence", []int{1, 2}, []string{"SequenceTarget", "ChoiceTarget"}, []string{"2/unbounded", "1/1"}, []int{11, 12}},
				} {
					definition := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", owner.name)
					var terms []Particle
					switch owner.model {
					case "choice":
						choice, ok := definition.Particle().(ChoiceParticle)
						if !ok || choice.Occurrences().String() != "2/3" || choice.Loc() != namedGroupChoiceLoc(t, root, `<xs:choice minOccurs="2"`) {
							t.Fatalf("choice owner = %T %s", definition.Particle(), definition.Particle().Loc())
						}
						terms = choice.Alternatives()
						if len(definition.AttributeUses()) != 1 {
							t.Fatal("direct owner attribute use disappeared")
						}
					case "sequence":
						sequence, ok := definition.Particle().(SequenceParticle)
						if !ok || sequence.Loc() != namedGroupChoiceLoc(t, root, `<xs:sequence>`) {
							t.Fatalf("sequence owner = %T", definition.Particle())
						}
						terms = sequence.Particles()
					default:
						t.Fatalf("unknown test model %q", owner.model)
					}
					wantTerms := 4
					if owner.model == "choice" {
						wantTerms = 3
					}
					if len(terms) != wantTerms {
						t.Fatalf("%s terms = %d, want lexical %d", owner.name, len(terms), wantTerms)
					}
					if _, ok := terms[0].(ElementReferenceParticle); !ok {
						t.Fatalf("first term = %T", terms[0])
					}
					if _, ok := terms[len(terms)-1].(ElementReferenceParticle); !ok {
						t.Fatalf("last term = %T", terms[len(terms)-1])
					}
					for i, position := range owner.positions {
						group, ok := terms[position].(ModelGroupReferenceParticle)
						if !ok {
							t.Fatalf("term %d = %T", position, terms[position])
						}
						name := mustTestQName(t, "urn:direct-group", owner.groups[i])
						target := namedSequenceGroup(t, schema, "urn:direct-group", owner.groups[i])
						marker := `ref="g:` + owner.groups[i] + `"`
						if group.Name() != name || group.Ref() != name || group.TargetID() != target.ID() || group.Occurrences().String() != owner.bounds[i] || group.RefLoc() != mustSchemaTokenLoc(t, "root.xsd", root, owner.lines[i], marker) || group.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, owner.lines[i], `<xs:group `+marker) {
							t.Fatalf("group facts = %q %v %s %s %s", group.Name(), group.TargetID(), group.Occurrences(), group.Loc(), group.RefLoc())
						}
						minimum := group.Occurrences().Minimum()
						minimum.value.SetInt64(99)
						maximum, finite := group.Occurrences().Maximum().Finite()
						if finite {
							maximum.value.SetInt64(99)
						}
						if group.Occurrences().String() != owner.bounds[i] {
							t.Fatal("mutating returned occurrence values changed the group particle")
						}
					}
					terms[0] = nil
					if definition.Particle() == nil {
						t.Fatal("mutating term copy changed definition")
					}
				}
				walked := make([]ComponentID, 0)
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				for index, component := range schema.Components() {
					if walked[index] != component.ID() {
						t.Fatalf("walk order %d changed", index)
					}
				}
			}
		})
	}
}

func TestDirectNamedGroupChoiceUPAIsInvalid(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">
  <xs:complexType name="Owner"><xs:choice><xs:group ref="g:Left"/><xs:group ref="g:Right"/></xs:choice></xs:complexType>
  <xs:group name="Left"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>
  <xs:group name="Right"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("ambiguous choice returned a schema")
	}
	assertZeroSchema(t, schema)
	var diagnostic Diagnostic
	wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 3, `ref="g:item"`), mustSchemaTokenLoc(t, "root.xsd", root, 4, `ref="g:item"`)}
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaDirectGroupUPACode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:Right"`) || diagnostic.SpecRef() != schemaGroupNonAmbiguousSpecRef(XSDVersion11) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaDirectGroupUPA) {
		t.Fatalf("UPA diagnostic = %v", err)
	}
}

func directGroupNthLoc(t *testing.T, root, marker string, occurrence int) Loc {
	t.Helper()
	position := 0
	for index := 0; index <= occurrence; index++ {
		next := strings.Index(root[position:], marker)
		if next < 0 {
			t.Fatalf("fixture has fewer than %d occurrences of %q", occurrence+1, marker)
		}
		position += next
		if index == occurrence {
			return namedGroupLocAt(t, root, position)
		}
		position += len(marker)
	}
	return Loc{}
}

func TestDirectNamedGroupConsumersRefuseLocatedParticles(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">
  <xs:element name="root" type="g:Owner"/>
  <xs:complexType name="Owner"><xs:` + model + `><xs:group ref="g:Target"/></xs:` + model + `></xs:complexType>
  <xs:group name="Target"><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatal(err)
			}
			wantLoc := namedGroupChoiceLoc(t, root, `ref="g:Target"`)
			rootLoc := namedGroupChoiceLoc(t, root, `<xs:element name="root"`)
			ownerLoc := namedGroupChoiceLoc(t, root, `<xs:complexType name="Owner"`)
			modelLoc := namedGroupChoiceLoc(t, root, `<xs:`+model+`>`)
			groupLoc := namedGroupChoiceLoc(t, root, `<xs:group ref="g:Target"`)
			targetLoc := namedGroupChoiceLoc(t, root, `<xs:group name="Target"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:direct-group"><item>1</item></root>`)))
			validation := requireDiagnostic(t, validationErr)
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantLoc || validation.SpecRef() != "xsd11-structures#cvc-elt" || !reflect.DeepEqual(validation.Related(), []Loc{rootLoc, ownerLoc, modelLoc, groupLoc}) || !errors.Is(validationErr, errInstanceModelGroupReference) {
				t.Fatalf("validation %s = %v", model, validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantLoc || generation.SpecRef() != codegenDirectSequenceXSD11ParticlesSpecRef || !reflect.DeepEqual(generation.Related(), []Loc{ownerLoc, groupLoc, wantLoc, targetLoc}) || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
				t.Fatalf("generation %s = (%q, %v)", model, output, generationErr)
			}
		})
	}
}

func directGroupDiagnosticRoot(version, model, reference, declaration string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="` + version + `">
  <xs:complexType name="Owner"><xs:` + model + `><xs:group ` + reference + `/></xs:` + model + `></xs:complexType>
  ` + declaration + `
</xs:schema>`
}

func TestDirectNamedGroupReferenceDiagnosticsBeforeOmission(t *testing.T) { //nolint:gocognit // Each alternate reference exit is checked in both compositor shapes and every policy.
	for _, profile := range []struct {
		name, version, spec string
		policy              LanguagePolicy
	}{
		{"Compatibility", "1.0", schemaModelGroupReferenceXSD11SpecRef, Compatibility},
		{"Strict10", "1.0", schemaModelGroupReferenceXSD10SpecRef, Strict10},
		{"Strict11", "1.1", schemaModelGroupReferenceXSD11SpecRef, Strict11},
	} {
		for _, model := range []string{"choice", "sequence"} {
			for _, failure := range []struct {
				name, ref, declaration, code string
				cause                        error
				related                      string
			}{
				{"unresolved", `ref="g:Missing" minOccurs="0" maxOccurs="0"`, ``, diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved, ""},
				{"wrong kind", `ref="g:Wrong" minOccurs="0" maxOccurs="0"`, `<xs:element name="Wrong" type="xs:integer"/>`, diagnosticSchemaModelGroupReferenceWrongKindCode, errSchemaModelGroupReferenceWrongKind, `<xs:element name="Wrong"`},
			} {
				t.Run(profile.name+"/"+model+"/"+failure.name, func(t *testing.T) {
					root := directGroupDiagnosticRoot(profile.version, model, failure.ref, failure.declaration)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					refMarker := `ref="g:Missing"`
					if failure.name == "wrong kind" {
						refMarker = `ref="g:Wrong"`
					}
					var related []Loc
					if failure.related != "" {
						related = []Loc{namedGroupChoiceLoc(t, root, failure.related)}
					}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != failure.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, refMarker) || diagnostic.SpecRef() != profile.spec || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, failure.cause) {
						t.Fatalf("reference diagnostic = %v, spec %q want %q, loc %s want %s, related %#v want %#v, cause %v", err, diagnostic.SpecRef(), profile.spec, diagnostic.Loc(), namedGroupChoiceLoc(t, root, refMarker), diagnostic.Related(), related, errors.Is(err, failure.cause))
					}
				})
			}
		}
	}
}

func TestDirectNamedGroupReferenceGraphVisibility(t *testing.T) { //nolint:gocognit // Public target IDs and source locations distinguish each discovery edge.
	for _, graph := range []struct {
		name, root, written, namespace, source string
		fixtures                               map[string]discoveryFixture
	}{
		{
			name: "forward include",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">
  <xs:include schemaLocation="child.xsd"/><xs:complexType name="Owner"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType>
</xs:schema>`,
			written: "g:Target", namespace: "urn:direct-group", source: "child.xsd",
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group"><xs:group name="Target"><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:group><xs:element name="item" type="xs:integer"/></xs:schema>`}},
		},
		{
			name: "import",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:direct-other" targetNamespace="urn:direct-group" version="1.1">
  <xs:import namespace="urn:direct-other" schemaLocation="other.xsd"/><xs:complexType name="Owner"><xs:sequence><xs:group ref="o:Target"/></xs:sequence></xs:complexType>
</xs:schema>`,
			written: "o:Target", namespace: "urn:direct-other", source: "other.xsd",
			fixtures: map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:direct-other" targetNamespace="urn:direct-other"><xs:group name="Target"><xs:choice><xs:element ref="o:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/></xs:schema>`}},
		},
		{
			name: "chameleon include",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">
  <xs:include schemaLocation="child.xsd"/><xs:complexType name="Owner"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:complexType>
</xs:schema>`,
			written: "g:Target", namespace: "urn:direct-group", source: "child.xsd",
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`}},
		},
	} {
		t.Run(graph.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, graph.root, graph.fixtures, Strict11)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
			var terms []Particle
			switch model := owner.Particle().(type) {
			case ChoiceParticle:
				terms = model.Alternatives()
			case SequenceParticle:
				terms = model.Particles()
			default:
				t.Fatalf("owner model = %T", owner.Particle())
			}
			if len(terms) != 1 {
				t.Fatalf("terms = %d", len(terms))
			}
			reference, ok := terms[0].(ModelGroupReferenceParticle)
			if !ok {
				t.Fatalf("term = %T", terms[0])
			}
			target := namedSequenceGroup(t, schema, graph.namespace, "Target")
			if reference.TargetID() != target.ID() || string(reference.TargetID().Source()) != graph.source || reference.Name() != mustTestQName(t, graph.namespace, "Target") || reference.RefLoc() != namedGroupChoiceLoc(t, graph.root, `ref="`+graph.written+`"`) {
				t.Fatalf("graph target = %v/%s, ref = %q %s", reference.TargetID(), graph.source, reference.Name(), reference.RefLoc())
			}
		})
	}
}

func TestDirectNamedGroupMixedEditionGraph(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">
  <xs:include schemaLocation="child.xsd"/><xs:complexType name="Owner"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:direct-group" version="1.0"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Compatibility)
	if err != nil {
		t.Fatal(err)
	}
	owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner")
	choice, ok := owner.Particle().(ChoiceParticle)
	if !ok || len(choice.Alternatives()) != 1 {
		t.Fatalf("mixed graph owner particle = %T", owner.Particle())
	}
	reference, ok := choice.Alternatives()[0].(ModelGroupReferenceParticle)
	if !ok {
		t.Fatalf("mixed graph term = %T", choice.Alternatives()[0])
	}
	target := namedSequenceGroup(t, schema, "urn:direct-group", "Target")
	if reference.TargetID() != target.ID() || reference.TargetID().Source() != "child.xsd" || reference.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:Target"`) {
		t.Fatalf("mixed graph reference = %v %s", reference.TargetID(), reference.RefLoc())
	}
}

func TestDirectNamedGroupExclusionsStayLocated(t *testing.T) {
	for _, test := range []struct {
		name, body, target, marker string
	}{
		{"extension choice", `<xs:complexType name="Empty"/><xs:complexType name="Owner"><xs:complexContent><xs:extension base="g:Empty"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>`, ``, `ref="g:Target"`},
		{"extension sequence", `<xs:complexType name="Empty"/><xs:complexType name="Owner"><xs:complexContent><xs:extension base="g:Empty"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`, ``, `ref="g:Target"`},
		{"anonymous choice", `<xs:element name="root"><xs:complexType><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType></xs:element>`, ``, `ref="g:Target"`},
		{"anonymous sequence", `<xs:element name="root"><xs:complexType><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:complexType></xs:element>`, ``, `ref="g:Target"`},
		{"all owner", `<xs:complexType name="Owner"><xs:all><xs:group ref="g:Target"/></xs:all></xs:complexType>`, ``, `ref="g:Target"`},
		{"nested choice", `<xs:complexType name="Owner"><xs:choice><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:choice></xs:complexType>`, ``, `ref="g:Target"`},
		{"nested sequence", `<xs:complexType name="Owner"><xs:sequence><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:sequence></xs:complexType>`, ``, `ref="g:Target"`},
		{"choice all target", `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:Target"/></xs:choice></xs:complexType>`, `<xs:group name="Target"><xs:all/></xs:group>`, `<xs:all`},
		{"sequence all target", `<xs:complexType name="Owner"><xs:sequence><xs:group ref="g:Target"/></xs:sequence></xs:complexType>`, `<xs:group name="Target"><xs:all/></xs:group>`, `<xs:all`},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := test.target
			if target == "" {
				target = `<xs:group name="Target"><xs:choice/></xs:group>`
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">` + test.body + target + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("excluded %s diagnostic = %v at %s, want unsupported at %s", test.name, err, diagnostic.Loc(), namedGroupChoiceLoc(t, root, test.marker))
			}
		})
	}
}

func TestDirectNamedGroupProofBoundaries(t *testing.T) { //nolint:gocognit // Each attribution outcome has a distinct public parse observable.
	for _, test := range []struct {
		name, model, target, other, code, marker, spec string
		class                                          FailureClass
		cause                                          error
		related                                        []int
	}{
		{
			name: "choice uses first leaves", model: `<xs:choice><xs:group ref="g:Target"/><xs:element ref="g:second"/></xs:choice>`,
			target: `<xs:group name="Target"><xs:sequence><xs:element ref="g:first"/><xs:element ref="g:second"/></xs:sequence></xs:group>`,
			other:  `<xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:integer"/>`,
		},
		{
			name: "fixed adjacent singleton", model: `<xs:sequence><xs:group ref="g:Target"/><xs:element ref="g:item"/></xs:sequence>`,
			target: `<xs:group name="Target"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>`,
			other:  `<xs:element name="item" type="xs:integer"/>`,
		},
		{
			name: "optional adjacent singleton", model: `<xs:sequence><xs:group ref="g:Target" minOccurs="0"/><xs:element ref="g:item"/></xs:sequence>`,
			target: `<xs:group name="Target"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>`,
			other:  `<xs:element name="item" type="xs:integer"/>`, code: diagnosticSchemaDirectGroupUPACode, marker: `ref="g:item"`, spec: schemaGroupNonAmbiguousSpecRef(XSDVersion11), class: FailureInvalid, cause: errSchemaDirectGroupUPA, related: []int{1, 0},
		},
		{
			name: "unproved sequence attribution", model: `<xs:sequence><xs:group ref="g:Target"/><xs:element ref="g:second"/></xs:sequence>`,
			target: `<xs:group name="Target"><xs:sequence><xs:element ref="g:first"/><xs:element ref="g:second"/></xs:sequence></xs:group>`,
			other:  `<xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:integer"/>`,
			code:   UnsupportedSchemaSyntaxCode, marker: `ref="g:second"`, spec: schemaGroupNonAmbiguousSpecRef(XSDVersion11), class: FailureUnsupported, cause: errSchemaDirectGroupProofUnsupported, related: []int{1, 0},
		},
		{
			name: "element consistency", model: `<xs:choice><xs:element name="item" type="xs:string"/><xs:group ref="g:Target"/></xs:choice>`,
			target: `<xs:group name="Target"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:group>`,
			code:   diagnosticSchemaGroupElementConsistencyCode, marker: `ref="g:Target"`, spec: schemaGroupElementConsistencySpecRef(XSDVersion11), class: FailureInvalid, cause: errSchemaGroupElementConsistency, related: []int{0, 1},
		},
		{
			name: "omitted owner checks target UPA", model: `<xs:choice minOccurs="0" maxOccurs="0"><xs:group ref="g:Target"/></xs:choice>`,
			target: `<xs:group name="Target"><xs:choice><xs:group ref="g:Left"/><xs:group ref="g:Right"/></xs:choice></xs:group>
<xs:group name="Left"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>
<xs:group name="Right"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>`,
			other: `<xs:element name="item" type="xs:integer"/>`, code: diagnosticSchemaDirectGroupUPACode, marker: `ref="g:Target"`, spec: schemaGroupNonAmbiguousSpecRef(XSDVersion11), class: FailureInvalid, cause: errSchemaDirectGroupUPA, related: []int{0, 1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" elementFormDefault="qualified" version="1.1">
  <xs:complexType name="Owner">` + test.model + `</xs:complexType>
  ` + test.target + test.other + `
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				if modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:direct-group", "Owner").Particle() == nil {
					t.Fatal("proved owner lost its particle")
				}
				return
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			relatedMarker := `ref="g:item"`
			if test.name == "unproved sequence attribution" {
				relatedMarker = `ref="g:second"`
			}
			if test.name == "element consistency" {
				relatedMarker = `<xs:element name="item"`
			}
			wantRelated := make([]Loc, 0, len(test.related))
			for _, occurrence := range test.related {
				wantRelated = append(wantRelated, directGroupNthLoc(t, root, relatedMarker, occurrence))
			}
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, test.marker) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, test.cause) {
				t.Fatalf("proof diagnostic = %v, related %v, want %s %s %s", err, diagnostic.Related(), test.code, namedGroupChoiceLoc(t, root, test.marker), test.spec)
			}
		})
	}
}

func TestDirectNamedGroupProofDiagnosticsSelectEdition(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
		facts         XSDVersion
	}{
		{"Compatibility", "1.0", Compatibility, XSDVersion11},
		{"Strict10", "1.0", Strict10, XSDVersion10},
		{"Strict11", "1.1", Strict11, XSDVersion11},
	} {
		for _, failure := range []struct {
			name, body, marker, code string
			class                    FailureClass
			cause                    error
			spec                     func(XSDVersion) string
		}{
			{
				"UPA", `<xs:complexType name="Owner"><xs:choice><xs:group ref="g:Left"/><xs:group ref="g:Right"/></xs:choice></xs:complexType><xs:group name="Left"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group><xs:group name="Right"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/>`,
				`ref="g:Right"`, diagnosticSchemaDirectGroupUPACode, FailureInvalid, errSchemaDirectGroupUPA, schemaGroupNonAmbiguousSpecRef,
			},
			{
				"element consistency", `<xs:complexType name="Owner"><xs:choice><xs:element name="item" type="xs:string"/><xs:group ref="g:Target"/></xs:choice></xs:complexType><xs:group name="Target"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:group>`,
				`ref="g:Target"`, diagnosticSchemaGroupElementConsistencyCode, FailureInvalid, errSchemaGroupElementConsistency, schemaGroupElementConsistencySpecRef,
			},
			{
				"unproved", `<xs:complexType name="Owner"><xs:sequence><xs:group ref="g:Target"/><xs:element ref="g:second"/></xs:sequence></xs:complexType><xs:group name="Target"><xs:sequence><xs:element ref="g:first"/><xs:element ref="g:second"/></xs:sequence></xs:group><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:integer"/>`,
				`ref="g:second"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaDirectGroupProofUnsupported, schemaGroupNonAmbiguousSpecRef,
			},
		} {
			t.Run(profile.name+"/"+failure.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" elementFormDefault="qualified" version="` + profile.version + `">` + failure.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != failure.class || diagnostic.Code() != failure.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, failure.marker) || diagnostic.SpecRef() != failure.spec(profile.facts) || len(diagnostic.Related()) == 0 || !errors.Is(err, failure.cause) {
					t.Fatalf("edition diagnostic = %v, related %v, want %s", err, diagnostic.Related(), failure.spec(profile.facts))
				}
			})
		}
	}
}

func TestDirectNamedGroupInvisibleTargetAndCycle(t *testing.T) {
	t.Run("invisible import", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:direct-other" targetNamespace="urn:direct-group" version="1.1">
  <xs:include schemaLocation="child.xsd"/>
  <xs:complexType name="Owner"><xs:choice><xs:group ref="o:Target"/></xs:choice></xs:complexType>
</xs:schema>`
		other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:direct-other"><xs:group name="Target"><xs:sequence/></xs:group></xs:schema>`
		fixtures := map[string]discoveryFixture{
			"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:direct-group"><xs:import namespace="urn:direct-other" schemaLocation="other.xsd"/></xs:schema>`},
			"other.xsd": {id: "other.xsd", contents: other},
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantRelated := namedGroupChoiceLoc(t, other, `<xs:group name="Target"`)
		wantRelated.source = "other.xsd"
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:Target"`) || diagnostic.SpecRef() != schemaModelGroupReferenceImportXSD11SpecRef || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
			t.Fatalf("invisible target = %v related %v", err, diagnostic.Related())
		}
	})
	t.Run("target cycle", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1">
  <xs:complexType name="Owner"><xs:sequence><xs:group ref="g:First"/></xs:sequence></xs:complexType>
  <xs:group name="First"><xs:choice><xs:group ref="g:Second"/></xs:choice></xs:group>
  <xs:group name="Second"><xs:sequence><xs:group ref="g:First" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:group>
</xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantRelated := []Loc{namedGroupChoiceLoc(t, root, `<xs:group name="First"`), namedGroupChoiceLoc(t, root, `<xs:group name="Second"`), namedGroupChoiceLoc(t, root, `ref="g:Second"`)}
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:First" minOccurs`) || diagnostic.SpecRef() != "xsd11-structures#mg-props-correct" || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
			t.Fatalf("target cycle = %v related %v", err, diagnostic.Related())
		}
	})
}

func TestDirectNamedGroupMalformedBeforeZeroOmission(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		for _, failure := range []struct {
			name, attributes, child, marker, code, spec string
			cause                                       error
			related                                     []string
			hasCause                                    bool
		}{
			{"reversed bounds", `ref="g:Missing" minOccurs="2" maxOccurs="1"`, ``, `<xs:group ref=`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(XSDVersion11), errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}, true},
			{"malformed minimum", `ref="g:Missing" minOccurs="bad" maxOccurs="0"`, ``, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(XSDVersion11), nil, nil, true},
			{"missing ref", `minOccurs="0" maxOccurs="0"`, ``, `<xs:group minOccurs`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef(XSDVersion11), nil, nil, false},
			{"forbidden child", `ref="g:Missing" minOccurs="0" maxOccurs="0"`, `<xs:element/>`, `<xs:element/>`, invalidSchemaCompositionCode, schemaGroupParticleSpecRef(XSDVersion11), nil, nil, false},
		} {
			t.Run(model+"/"+failure.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:direct-group" targetNamespace="urn:direct-group" version="1.1"><xs:complexType name="Owner"><xs:` + model + `><xs:group ` + failure.attributes + `>` + failure.child + `</xs:group></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				var wantRelated []Loc
				for _, marker := range failure.related {
					wantRelated = append(wantRelated, namedGroupChoiceLoc(t, root, marker))
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != failure.code || diagnostic.Loc() != namedGroupChoiceLoc(t, root, failure.marker) || diagnostic.SpecRef() != failure.spec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || (diagnostic.Unwrap() != nil) != failure.hasCause || failure.cause != nil && !errors.Is(err, failure.cause) {
					t.Fatalf("malformed %s = %v, spec %q related %v cause %v", failure.name, err, diagnostic.SpecRef(), diagnostic.Related(), diagnostic.Unwrap())
				}
			})
		}
	}
}

func FuzzDirectNamedGroupParse(f *testing.F) {
	f.Add(directGroupRoot("1.1"))
	f.Add(directGroupDiagnosticRoot("1.1", "choice", `ref="g:Missing" minOccurs="0" maxOccurs="0"`, ""))
	f.Fuzz(func(t *testing.T, input string) {
		schema, err := discoverTestSchemaWithPolicy(t, input, nil, Strict11)
		if err != nil {
			assertZeroSchema(t, schema)
			return
		}
		if err := schema.Walk(func(Component) error { return nil }); err != nil {
			t.Fatal(err)
		}
	})
}
