package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func namedGroupLocalRoot(version, attributes, sequence, declarations string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-local" targetNamespace="urn:group-local" version="` + version + `"` + attributes + `>
  <xs:group name="Outer"><xs:sequence>` + sequence + `</xs:sequence></xs:group>
  ` + declarations + `
</xs:schema>`
}

func groupLocalElement(t *testing.T, term Particle, name QName, occurrences string, loc Loc) ElementParticle {
	t.Helper()
	element, ok := term.(ElementParticle)
	if !ok || element.Name() != name || element.Occurrences().String() != occurrences || element.Loc() != loc {
		t.Fatalf("element = %T %v, want %s %s at %s", term, term, name, occurrences, loc)
	}
	return element
}

func groupLocalSequence(t *testing.T, group ModelGroupDefinition) SequenceParticle {
	t.Helper()
	sequence, ok := group.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("named group particle = %T, want sequence", group.Particle())
	}
	return sequence
}

func TestNamedGroupLocalSequenceAndNestedChoiceFacts(t *testing.T) { //nolint:gocognit // Public facts, exact copies, and policy provenance form one contract.
	sequence := `<xs:element name="alpha" type="xs:string" minOccurs="0" maxOccurs="unbounded"/>
<xs:element name="omitted" type="xs:int" minOccurs="0" maxOccurs="0"/>
<xs:choice minOccurs="2" maxOccurs="18446744073709551616"><xs:element name="beta" type="g:Narrow" minOccurs="3" maxOccurs="5"/><xs:element name="gamma" type="g:Words" form="unqualified"/></xs:choice>
<xs:element ref="g:global"/>`
	declarations := `<xs:element name="global" type="xs:integer"/>
<xs:simpleType name="Narrow"><xs:restriction base="xs:int"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType>
<xs:simpleType name="Words"><xs:restriction base="xs:token"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := namedGroupLocalRoot(profile.version, ` elementFormDefault="qualified"`, sequence, declarations)
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
					t.Fatal("component discovery order changed")
				}
				walked := make([]ComponentID, 0, len(first))
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				if len(walked) != len(first) {
					t.Fatalf("walk length = %d, want %d", len(walked), len(first))
				}
				for index, component := range first {
					if walked[index] != component.ID() {
						t.Fatalf("walk order %d = %v, want %v", index, walked[index], component.ID())
					}
				}
				group := namedSequenceGroup(t, schema, "urn:group-local", "Outer")
				outer, ok := group.Particle().(SequenceParticle)
				if !ok || len(outer.Particles()) != 3 || outer.Loc() != namedGroupChoiceLoc(t, root, `<xs:sequence>`) {
					t.Fatalf("outer = %T %v", group.Particle(), group.Particle())
				}
				alpha := groupLocalElement(t, outer.Particles()[0], mustTestQName(t, "urn:group-local", "alpha"), "0/unbounded", namedGroupChoiceLoc(t, root, `<xs:element name="alpha"`))
				alphaRef, hasAlphaRef := alpha.TypeReference()
				if !hasAlphaRef || !alphaRef.IsBuiltin() || alphaRef.Name() != mustTestQName(t, testXSDNamespace, "string") || alphaRef.Loc() != namedGroupChoiceLoc(t, root, `type="xs:string"`) {
					t.Fatalf("alpha type = %#v", alphaRef)
				}
				choice, ok := outer.Particles()[1].(ChoiceParticle)
				if !ok || choice.Occurrences().String() != "2/18446744073709551616" || choice.Loc() != namedGroupChoiceLoc(t, root, `<xs:choice minOccurs="2"`) || len(choice.Alternatives()) != 2 {
					t.Fatalf("choice = %T %v", outer.Particles()[1], outer.Particles()[1])
				}
				beta := groupLocalElement(t, choice.Alternatives()[0], mustTestQName(t, "urn:group-local", "beta"), "3/5", namedGroupChoiceLoc(t, root, `<xs:element name="beta"`))
				betaRef, hasBetaRef := beta.TypeReference()
				betaID, hasBetaID := beta.TypeID()
				betaRefID, hasBetaRefID := betaRef.ComponentID()
				wantBetaID := componentIDForName(t, schema, mustTestQName(t, "urn:group-local", "Narrow"))
				bounds, hasBounds := betaRef.IntegerBounds()
				minimum, hasMinimum := bounds.MinInclusiveFacet()
				if !hasBetaRef || !betaRef.IsNamed() || beta.DeclaredType() != betaRef.Name() || betaRef.Name() != mustTestQName(t, "urn:group-local", "Narrow") || !hasBetaID || !hasBetaRefID || betaID != wantBetaID || betaRefID != wantBetaID || betaRef.Loc() != namedGroupChoiceLoc(t, root, `type="g:Narrow"`) || betaRef.VarietyLoc() != namedGroupChoiceLoc(t, root, `<xs:restriction base="xs:int"`) || !hasBounds || bounds.Version() != profile.factVersion || !hasMinimum || minimum.Value().Canonical() != "2" || minimum.Loc() != namedGroupChoiceLoc(t, root, `value="2"`) {
					t.Fatalf("beta type/facets = kind %s declared %s name %s IDs %v/%v want %v loc %s variety %s bounds %v/%v min %v %s at %s", betaRef.Kind(), beta.DeclaredType(), betaRef.Name(), betaID, betaRefID, wantBetaID, betaRef.Loc(), betaRef.VarietyLoc(), hasBounds, bounds.Version(), hasMinimum, minimum.Value().Canonical(), minimum.Loc())
				}
				gamma := groupLocalElement(t, choice.Alternatives()[1], mustTestQName(t, "", "gamma"), "1/1", namedGroupChoiceLoc(t, root, `<xs:element name="gamma"`))
				gammaRef, hasGammaRef := gamma.TypeReference()
				if !hasGammaRef || !gammaRef.IsNamed() || gammaRef.Name() != mustTestQName(t, "urn:group-local", "Words") || !reflect.DeepEqual(gammaRef.StringEnumerationFacets().Values(), []string{"one", "two"}) || !reflect.DeepEqual(gammaRef.StringEnumerationFacets().Locations(), []Loc{namedGroupChoiceLoc(t, root, `value="one"`), namedGroupChoiceLoc(t, root, `value="two"`)}) {
					t.Fatalf("gamma type/facets = %#v", gammaRef)
				}
				if reference, ok := outer.Particles()[2].(ElementReferenceParticle); !ok || reference.Ref() != mustTestQName(t, "urn:group-local", "global") || reference.RefLoc() != namedGroupChoiceLoc(t, root, `ref="g:global"`) {
					t.Fatalf("global reference = %T %v", outer.Particles()[2], outer.Particles()[2])
				}
				outerParts := outer.Particles()
				outerParts[0] = nil
				alternatives := choice.Alternatives()
				alternatives[0] = nil
				maximum, finite := choice.Occurrences().Maximum().Finite()
				if !finite {
					t.Fatal("huge maximum became unbounded")
				}
				maximum.value.SetInt64(0)
				boundCopy, _ := betaRef.IntegerBounds()
				boundCopy.lower.value.value.SetInt64(0)
				if outer.Particles()[0] == nil || choice.Alternatives()[0] == nil || choice.Occurrences().String() != "2/18446744073709551616" {
					t.Fatal("public copies changed completed group")
				}
				minimumAgain, _ := func() (IntegerMinInclusiveFacet, bool) {
					copied, _ := betaRef.IntegerBounds()
					return copied.MinInclusiveFacet()
				}()
				if minimumAgain.Value().Canonical() != "2" {
					t.Fatal("returned facet changed completed group")
				}
			}
		})
	}
}

func TestNamedGroupLocalZeroOmissionAndReferenceResolution(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := namedGroupLocalRoot(profile.version, "", `<xs:element name="gone" type="xs:integer" minOccurs="0" maxOccurs="0"/><xs:element name="untyped" minOccurs="0" maxOccurs="0"/><xs:element name="inline" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:integer"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType></xs:element><xs:element name="union" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:union memberTypes="xs:integer xs:token"/></xs:simpleType></xs:element><xs:choice minOccurs="0" maxOccurs="0"><xs:element name="alsoGone" type="xs:token"/><xs:element name="nestedUntyped" minOccurs="0" maxOccurs="0"/><xs:element name="nestedInline" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:list itemType="xs:integer"/></xs:simpleType></xs:element></xs:choice><xs:element name="kept" type="xs:string"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			group := namedSequenceGroup(t, schema, "urn:group-local", "Outer")
			sequence := groupLocalSequence(t, group)
			if len(sequence.Particles()) != 1 {
				t.Fatalf("omitted terms retained: %v", sequence.Particles())
			}
			groupLocalElement(t, sequence.Particles()[0], mustTestQName(t, "", "kept"), "1/1", namedGroupChoiceLoc(t, root, `<xs:element name="kept"`))
		})
	}
}

func TestNamedGroupLocalConsumersRejectOpaqueGroup(t *testing.T) { //nolint:gocognit // Both consumers and admitted group shapes share one boundary.
	for _, shape := range []struct{ name, terms string }{
		{"direct", `<xs:element name="local" type="xs:string"/>`},
		{"nested choice", `<xs:choice><xs:element name="local" type="xs:string"/></xs:choice>`},
	} {
		for _, owner := range []string{"named", "inline"} {
			t.Run(shape.name+"/"+owner, func(t *testing.T) {
				declarations := `<xs:complexType name="Owner"><xs:group ref="g:Outer"/></xs:complexType><xs:element name="root" type="g:Owner"/>`
				if owner == "inline" {
					declarations = `<xs:element name="root"><xs:complexType><xs:group ref="g:Outer"/></xs:complexType></xs:element>`
				}
				root := namedGroupLocalRoot("1.1", "", shape.terms, declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				if owner == "inline" {
					assertNamedGroupLocalFailure(t, schema, err, UnsupportedSchemaSyntaxCode, FailureUnsupported, namedGroupChoiceLoc(t, root, `ref="g:Outer"`), nil, schemaSyntaxSpecRefForVersion(XSDVersion10), nil)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:group-local"><local xmlns="">x</local></root>`)))
				validation := requireDiagnostic(t, validationErr)
				output, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				if output != nil || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(generationErr, ErrUnsupported) {
					t.Fatalf("consumers = %s / %q %s", validation, output, generation)
				}
				want := namedGroupChoiceLoc(t, root, `ref="g:Outer"`)
				if validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != want || !errors.Is(validationErr, errInstanceModelGroupReference) || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != want || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
					t.Fatalf("named consumers = %s / %s", validation, generation)
				}
			})
		}
	}
}

func namedGroupNestedLocalDocument(t *testing.T, document string) string {
	t.Helper()
	start := strings.Index(document, `<xs:element name="local"`)
	if start < 0 {
		t.Fatal("graph fixture has no local element")
	}
	end := strings.Index(document[start:], `/>`)
	if end < 0 {
		t.Fatal("graph fixture local element is not closed")
	}
	end += start + len(`/>`)
	return document[:start] + `<xs:choice>` + document[start:end] + `</xs:choice>` + document[end:]
}

func TestNamedGroupLocalTypeGraphProvenance(t *testing.T) { //nolint:gocognit,funlen // Each graph has the same public identity and source-order contract.
	for _, test := range []struct {
		name, root, owner, typeSource, typeNamespace string
		fixtures                                     map[string]discoveryFixture
	}{
		{
			name: "forward", typeSource: "root.xsd", typeNamespace: "urn:group-local",
			root: namedGroupLocalRoot("1.1", ` elementFormDefault="qualified"`, `<xs:element name="local" type="g:Later"/>`, `<xs:simpleType name="Later"><xs:restriction base="xs:integer"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType>`),
		},
		{
			name: "include cycle", typeSource: "child.xsd", typeNamespace: "urn:group-local",
			root:     `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-local" targetNamespace="urn:group-local" elementFormDefault="qualified"><xs:include schemaLocation="child.xsd"/><xs:group name="Outer"><xs:sequence><xs:element name="local" type="g:Later"/></xs:sequence></xs:group></xs:schema>`,
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:group-local"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Later"><xs:restriction base="xs:integer"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType></xs:schema>`}},
		},
		{
			name: "import", typeSource: "other.xsd", typeNamespace: "urn:other",
			root:     `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:group-local" elementFormDefault="qualified"><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Outer"><xs:sequence><xs:element name="local" type="o:Later"/></xs:sequence></xs:group></xs:schema>`,
			fixtures: map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Later"><xs:restriction base="xs:integer"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType></xs:schema>`}},
		},
		{
			name: "chameleon", typeSource: "root.xsd", typeNamespace: "urn:group-local",
			root:     `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:group-local"><xs:include schemaLocation="child.xsd"/><xs:simpleType name="Later"><xs:restriction base="xs:integer"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType></xs:schema>`,
			owner:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-local" elementFormDefault="qualified"><xs:group name="Outer"><xs:sequence><xs:element name="local" type="g:Later"/></xs:sequence></xs:group></xs:schema>`,
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-local" elementFormDefault="qualified"><xs:group name="Outer"><xs:sequence><xs:element name="local" type="g:Later"/></xs:sequence></xs:group></xs:schema>`}},
		},
	} {
		for _, profile := range nestedGroupChoiceProfiles {
			for _, shape := range []string{"direct", "nested choice"} {
				t.Run(profile.name+"/"+test.name+"/"+shape, func(t *testing.T) {
					root := test.root
					ownerDocument := test.owner
					fixtures := make(map[string]discoveryFixture)
					for _, source := range []string{"child.xsd", "other.xsd", "root.xsd"} {
						if fixture, ok := test.fixtures[source]; ok {
							fixtures[source] = fixture
						}
					}
					if shape == "nested choice" && ownerDocument == "" {
						root = namedGroupNestedLocalDocument(t, root)
					}
					if shape == "nested choice" && ownerDocument != "" {
						ownerDocument = namedGroupNestedLocalDocument(t, ownerDocument)
						fixtures["child.xsd"] = discoveryFixture{id: "child.xsd", contents: ownerDocument}
					}
					if test.name == "include cycle" {
						fixtures["root.xsd"] = discoveryFixture{id: "root.xsd", contents: root}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("graph component order changed: %v", err)
					}
					ownerSource := SourceID("root.xsd")
					if ownerDocument == "" {
						ownerDocument = root
					}
					if test.owner != "" {
						ownerSource = "child.xsd"
					}
					group := namedSequenceGroup(t, schema, "urn:group-local", "Outer")
					if group.ID().Source() != ownerSource {
						t.Fatalf("owner source = %s, want %s", group.ID().Source(), ownerSource)
					}
					sequence := groupLocalSequence(t, group)
					if len(sequence.Particles()) != 1 {
						t.Fatalf("particles = %v", sequence.Particles())
					}
					memberTerm := sequence.Particles()[0]
					if shape == "nested choice" {
						choice, ok := memberTerm.(ChoiceParticle)
						if !ok || len(choice.Alternatives()) != 1 || choice.Loc() != allParticleTestTokenLoc(t, ownerSource, ownerDocument, `<xs:choice>`, 1) {
							t.Fatalf("nested graph choice = %T", memberTerm)
						}
						memberTerm = choice.Alternatives()[0]
					}
					member := groupLocalElement(t, memberTerm, mustTestQName(t, "urn:group-local", "local"), "1/1", allParticleTestTokenLoc(t, ownerSource, ownerDocument, `<xs:element name="local"`, 1))
					reference, ok := member.TypeReference()
					id, hasID := member.TypeID()
					want := componentIDForName(t, schema, mustTestQName(t, test.typeNamespace, "Later"))
					if !ok || !reference.IsNamed() || !hasID || id != want || want.Source() != SourceID(test.typeSource) || reference.Loc() != allParticleTestTokenLoc(t, ownerSource, ownerDocument, `type="`, 1) {
						t.Fatalf("type reference = %#v/%v, want %v", reference, id, want)
					}
					declaration := root
					if test.typeSource != "root.xsd" {
						declaration = fixtures[test.typeSource].contents
					}
					if reference.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(test.typeSource), declaration, `<xs:restriction base="xs:integer"`, 1) {
						t.Fatalf("variety source = %s", reference.VarietyLoc())
					}
					if test.name == "include cycle" && len(schema.Documents()) != 2 {
						t.Fatalf("cycle document count = %d, want 2", len(schema.Documents()))
					}
				})
			}
		}
	}
}

func assertNamedGroupLocalFailure(t *testing.T, schema Schema, err error, code string, class FailureClass, loc Loc, related []Loc, spec string, cause error) {
	t.Helper()
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != code || diagnostic.Class() != class || diagnostic.Loc() != loc || diagnostic.SpecRef() != spec || !reflect.DeepEqual(diagnostic.Related(), related) {
		t.Fatalf("diagnostic = %s related %v spec %q; want %s/%s at %s related %v spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), class, code, loc, related, spec)
	}
	if class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported cause is missing: %v", err)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("cause %v is missing: %v", cause, err)
	}
}

func TestNamedGroupLocalTypeAndShapeExits(t *testing.T) { //nolint:gocognit // Direct and nested shapes share exact diagnostic assertions.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, shape := range []string{"direct", "nested choice"} {
			for _, test := range []struct {
				name, leaf, declarations, marker, related string
				code, spec                                string
				class                                     FailureClass
				cause                                     error
			}{
				{"unresolved under zero", `<xs:element name="v" type="g:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="g:Missing"`, "", diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaElementTypeUnresolved},
				{"wrong kind under zero", `<xs:element name="v" type="g:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `type="g:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaElementTypeWrongKind},
				{"decimal family under zero", `<xs:element name="v" type="xs:decimal" minOccurs="0" maxOccurs="0"/>`, "", `type="xs:decimal"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"named decimal family under zero", `<xs:element name="v" type="g:Decimal" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Decimal"><xs:restriction base="xs:decimal"/></xs:simpleType>`, `type="g:Decimal"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"named list variety", `<xs:element name="v" type="g:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:integer"/></xs:simpleType>`, `type="g:List"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"named complex type under zero", `<xs:element name="v" type="g:Complex" minOccurs="0" maxOccurs="0"/>`, `<xs:complexType name="Complex"/>`, `type="g:Complex"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"inline simple", `<xs:element name="v"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, "", `<xs:simpleType>`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"inline complex", `<xs:element name="v"><xs:complexType/></xs:element>`, "", `<xs:complexType/>`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"inline complex under zero", `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:complexType/></xs:element>`, "", `<xs:complexType/>`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"untyped", `<xs:element name="v"/>`, "", `<xs:element name="v"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"default constraint", `<xs:element name="v" type="xs:integer" default="2" minOccurs="0" maxOccurs="0"/>`, "", `default="2"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
			} {
				t.Run(profile.name+"/"+shape+"/"+test.name, func(t *testing.T) {
					terms := test.leaf
					if shape == "nested choice" {
						terms = `<xs:choice>` + terms + `</xs:choice>`
					}
					root := namedGroupLocalRoot(profile.version, "", terms, test.declarations)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{namedGroupChoiceLoc(t, root, test.related)}
					}
					assertNamedGroupLocalFailure(t, schema, err, test.code, test.class, namedGroupChoiceLoc(t, root, test.marker), related, test.spec, test.cause)
				})
			}
			t.Run(profile.name+"/"+shape+"/default before inline resolution", func(t *testing.T) {
				leaf := `<xs:element name="v" default="2" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:union memberTypes="xs:date g:Missing"/></xs:simpleType></xs:element>`
				if shape == "nested choice" {
					leaf = `<xs:choice>` + leaf + `</xs:choice>`
				}
				root := namedGroupLocalRoot(profile.version, "", leaf, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNamedGroupLocalFailure(t, schema, err, UnsupportedSchemaSyntaxCode, FailureUnsupported, namedGroupChoiceLoc(t, root, `default="2"`), nil, schemaSyntaxSpecRefForVersion(profile.factVersion), nil)
			})
		}
	}
}

func TestNamedGroupLocalNamespacePolicy(t *testing.T) { //nolint:gocognit // Edition and local namespace forms are asserted at the same boundary.
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name+"/form and matching target", func(t *testing.T) {
			terms := `<xs:element name="unqualified" form="unqualified" type="xs:string"/><xs:element name="qualified" form="qualified" type="xs:token"/>`
			if profile.policy != Strict10 {
				terms += `<xs:element name="targeted" targetNamespace="urn:group-local" type="xs:integer"/>`
			}
			root := namedGroupLocalRoot(profile.version, "", terms, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			sequence := groupLocalSequence(t, namedSequenceGroup(t, schema, "urn:group-local", "Outer"))
			if len(sequence.Particles()) != 2 && profile.policy == Strict10 || len(sequence.Particles()) != 3 && profile.policy != Strict10 {
				t.Fatalf("namespace policy particles = %v", sequence.Particles())
			}
			groupLocalElement(t, sequence.Particles()[0], mustTestQName(t, "", "unqualified"), "1/1", namedGroupChoiceLoc(t, root, `<xs:element name="unqualified"`))
			groupLocalElement(t, sequence.Particles()[1], mustTestQName(t, "urn:group-local", "qualified"), "1/1", namedGroupChoiceLoc(t, root, `<xs:element name="qualified"`))
			if profile.policy != Strict10 {
				groupLocalElement(t, sequence.Particles()[2], mustTestQName(t, "urn:group-local", "targeted"), "1/1", namedGroupChoiceLoc(t, root, `<xs:element name="targeted"`))
			}
		})
		for _, shape := range []string{"direct", "nested choice"} {
			t.Run(profile.name+"/"+shape+"/target namespace under zero", func(t *testing.T) {
				leaf := `<xs:element name="v" targetNamespace="urn:other" type="xs:integer" minOccurs="0" maxOccurs="0"/>`
				if shape == "nested choice" {
					leaf = `<xs:choice>` + leaf + `</xs:choice>`
				}
				root := namedGroupLocalRoot(profile.version, "", leaf, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				loc := namedGroupChoiceLoc(t, root, `targetNamespace="urn:other"`)
				if profile.policy == Strict10 {
					assertNamedGroupLocalFailure(t, schema, err, UnsupportedSchemaSyntaxCode, FailureUnsupported, loc, nil, schemaSyntaxSpecRefForVersion(XSDVersion11), errLanguagePolicyMismatch)
					return
				}
				assertNamedGroupLocalFailure(t, schema, err, invalidSchemaCompositionCode, FailureInvalid, loc, []Loc{namedGroupChoiceLoc(t, root, `targetNamespace="urn:group-local"`)}, schemaElementTargetNamespaceXSD11SpecRef, errSchemaElementTargetNamespace)
			})
		}
	}
}

func TestNamedGroupLocalOccurrenceAndCompositionPrecedeOmission(t *testing.T) { //nolint:gocognit // Error priority and source positions are shared by direct/nested forms.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, shape := range []string{"direct", "nested choice"} {
			for _, test := range []struct {
				name, leaf, marker string
				related            []string
				spec               string
				cause              error
			}{
				{"reversed bounds", `<xs:element name="v" type="xs:integer" minOccurs="2" maxOccurs="1"/>`, `<xs:element name="v"`, []string{`minOccurs="2"`, `maxOccurs="1"`}, schemaParticleCorrectSpecRef(profile.factVersion), errParticleOccurrenceMinimumExceedsMaximum},
				{"bad lexical minimum", `<xs:element name="v" type="g:Missing" minOccurs="many" maxOccurs="0"/>`, `minOccurs="many"`, nil, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil},
				{"bad lexical maximum", `<xs:element name="v" type="g:Missing" minOccurs="0" maxOccurs="many"/>`, `maxOccurs="many"`, nil, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil},
				{"unbounded minimum", `<xs:element name="v" type="g:Missing" minOccurs="unbounded" maxOccurs="0"/>`, `minOccurs="unbounded"`, nil, schemaParticleMinimumSpecRef(profile.factVersion), nil},
			} {
				t.Run(profile.name+"/"+shape+"/"+test.name, func(t *testing.T) {
					terms := test.leaf
					if shape == "nested choice" {
						terms = `<xs:choice minOccurs="0" maxOccurs="0">` + terms + `</xs:choice>`
					}
					root := namedGroupLocalRoot(profile.version, "", terms, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					for _, marker := range test.related {
						related = append(related, namedGroupChoiceLoc(t, root, marker))
					}
					assertNamedGroupLocalFailure(t, schema, err, invalidSchemaCompositionCode, FailureInvalid, namedGroupChoiceLoc(t, root, test.marker), related, test.spec, test.cause)
					if test.cause == nil && requireDiagnostic(t, err).Unwrap() == nil {
						t.Fatal("invalid occurrence lost lexical cause")
					}
				})
			}
		}
	}
}

func TestNamedGroupLocalInlineZeroRetainsFacetFailure(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, shape := range []string{"direct", "nested choice"} {
			t.Run(profile.name+"/"+shape, func(t *testing.T) {
				leaf := `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:integer"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType></xs:element>`
				if shape == "nested choice" {
					leaf = `<xs:choice>` + leaf + `</xs:choice>`
				}
				root := namedGroupLocalRoot(profile.version, "", leaf, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNamedGroupLocalFailure(t, schema, err, InvalidBoundCode, FailureInvalid, namedGroupChoiceLoc(t, root, `value="bad"`), nil, boundSpecRef(profile.factVersion, BoundMinInclusive, boundDefinitionRule), errInvalidBoundValue)
			})
		}
	}
}

func TestNamedGroupLocalInlineZeroRequiresCompleteResolution(t *testing.T) { //nolint:gocognit // Order, shape, and edition share one omission boundary.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, shape := range []string{"direct", "nested choice"} {
			for _, test := range []struct {
				name, body, declarations, marker, related string
				code, spec                                string
				class                                     FailureClass
				cause                                     error
			}{
				{"unsupported then unresolved", `<xs:union memberTypes="xs:date g:Missing"/>`, "", `memberTypes="xs:date g:Missing"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"unresolved then unsupported", `<xs:union memberTypes="g:Missing xs:date"/>`, "", `memberTypes="g:Missing xs:date"`, "", diagnosticSchemaSimpleTypeUnresolvedCode, schemaSimpleTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaSimpleTypeBaseUnresolved},
				{"unsupported then wrong kind", `<xs:union memberTypes="xs:date g:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `memberTypes="xs:date g:Wrong"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"wrong kind then unsupported", `<xs:union memberTypes="g:Wrong xs:date"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `memberTypes="g:Wrong xs:date"`, `<xs:element name="Wrong"`, diagnosticSchemaSimpleTypeWrongKindCode, schemaSimpleTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaSimpleTypeBaseWrongKind},
				{"unsupported then unknown builtin", `<xs:union memberTypes="xs:date xs:NoSuchType"/>`, "", `memberTypes="xs:date xs:NoSuchType"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"nested union", `<xs:union><xs:simpleType><xs:union memberTypes="xs:date g:Missing"/></xs:simpleType></xs:union>`, "", `memberTypes="xs:date g:Missing"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"list item", `<xs:list itemType="xs:date"/>`, "", `itemType="xs:date"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"list item unresolved", `<xs:list itemType="g:Missing"/>`, "", `itemType="g:Missing"`, "", diagnosticSchemaSimpleTypeUnresolvedCode, schemaSimpleTypeSpecRef(profile.factVersion), FailureInvalid, errSchemaSimpleTypeBaseUnresolved},
				{"restriction base", `<xs:restriction base="xs:date"/>`, "", `base="xs:date"`, "", UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.factVersion), FailureUnsupported, nil},
				{"restriction list base", `<xs:restriction base="g:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:integer"/></xs:simpleType>`, `base="g:List"`, `<xs:list itemType="xs:integer"`, UnsupportedSchemaSyntaxCode, schemaSimpleTypeSpecRef(profile.factVersion), FailureUnsupported, errSchemaSimpleTypeRestrictionUnsupported},
			} {
				t.Run(profile.name+"/"+shape+"/"+test.name, func(t *testing.T) {
					leaf := `<xs:element name="v" minOccurs="0" maxOccurs="0"><xs:simpleType>` + test.body + `</xs:simpleType></xs:element>`
					if shape == "nested choice" {
						leaf = `<xs:choice minOccurs="0" maxOccurs="0">` + leaf + `</xs:choice>`
					}
					root := namedGroupLocalRoot(profile.version, "", leaf, test.declarations)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{namedGroupChoiceLoc(t, root, test.related)}
					}
					assertNamedGroupLocalFailure(t, schema, err, test.code, test.class, namedGroupChoiceLoc(t, root, test.marker), related, test.spec, test.cause)
				})
			}
		}
	}
}

func TestNamedGroupLocalConsistencyAndAttributionGate(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, terms, declarations, marker, related string
			code, spec                                 string
			class                                      FailureClass
			cause                                      error
		}{
			{
				name: "different local types", terms: `<xs:element name="v" type="xs:integer"/><xs:element name="v" type="xs:string"/>`,
				marker: `<xs:element name="v" type="xs:string"`, related: `<xs:element name="v" type="xs:integer"`,
				code: diagnosticSchemaGroupElementConsistencyCode, spec: schemaGroupElementConsistencySpecRef(profile.factVersion), class: FailureInvalid, cause: errSchemaGroupElementConsistency,
			},
			{
				name: "same local type needs attribution", terms: `<xs:element name="v" type="xs:integer"/><xs:element name="v" type="xs:integer" minOccurs="0"/>`,
				marker: `<xs:element name="v" type="xs:integer" minOccurs="0"`, related: `<xs:element name="v" type="xs:integer"/>`,
				code: UnsupportedSchemaSyntaxCode, spec: schemaGroupNonAmbiguousSpecRef(profile.factVersion), class: FailureUnsupported,
			},
			{
				name: "nested alternatives", terms: `<xs:choice><xs:element name="v" type="xs:integer"/><xs:element name="v" type="xs:integer" minOccurs="0"/></xs:choice>`,
				marker: `<xs:element name="v" type="xs:integer" minOccurs="0"`, related: `<xs:element name="v" type="xs:integer"/>`,
				code: UnsupportedSchemaSyntaxCode, spec: schemaGroupNonAmbiguousSpecRef(profile.factVersion), class: FailureUnsupported,
			},
			{
				name: "local and global ref types differ", terms: `<xs:element name="v" type="xs:string"/><xs:element ref="g:v"/>`,
				declarations: `<xs:element name="v" type="xs:integer"/>`, marker: `ref="g:v"`, related: `<xs:element name="v" type="xs:string"`,
				code: diagnosticSchemaGroupElementConsistencyCode, spec: schemaGroupElementConsistencySpecRef(profile.factVersion), class: FailureInvalid, cause: errSchemaGroupElementConsistency,
			},
			{
				name: "local and group ref overlap", terms: `<xs:element name="v" type="xs:integer"/><xs:group ref="g:Target"/>`,
				declarations: `<xs:group name="Target"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence></xs:group>`, marker: `ref="g:Target"`, related: `<xs:element name="v" type="xs:integer"`,
				code: UnsupportedSchemaSyntaxCode, spec: schemaGroupNonAmbiguousSpecRef(profile.factVersion), class: FailureUnsupported,
			},
			{
				name: "repeated local-bearing group", terms: `<xs:group ref="g:Target"/><xs:group ref="g:Target" minOccurs="0"/>`,
				declarations: `<xs:group name="Target"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence></xs:group>`, marker: `ref="g:Target" minOccurs="0"`, related: `ref="g:Target"/>`,
				code: UnsupportedSchemaSyntaxCode, spec: schemaGroupNonAmbiguousSpecRef(profile.factVersion), class: FailureUnsupported,
			},
			{
				name: "substitution overlap", terms: `<xs:choice><xs:element name="member" type="xs:integer"/><xs:element ref="g:head"/></xs:choice>`,
				declarations: `<xs:element name="head" type="xs:integer"/><xs:element name="member" substitutionGroup="g:head" type="xs:integer"/>`, marker: `ref="g:head"`, related: `<xs:element name="member" type="xs:integer"`,
				code: UnsupportedSchemaSyntaxCode, spec: schemaGroupNonAmbiguousSpecRef(profile.factVersion), class: FailureUnsupported,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedGroupLocalRoot(profile.version, ` elementFormDefault="qualified"`, test.terms, test.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNamedGroupLocalFailure(t, schema, err, test.code, test.class, namedGroupChoiceLoc(t, root, test.marker), []Loc{namedGroupChoiceLoc(t, root, test.related)}, test.spec, test.cause)
			})
		}
		t.Run(profile.name+"/omitted duplicate", func(t *testing.T) {
			root := namedGroupLocalRoot(profile.version, "", `<xs:element name="v" type="xs:string" minOccurs="0" maxOccurs="0"/><xs:element name="v" type="xs:integer"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			sequence := groupLocalSequence(t, namedSequenceGroup(t, schema, "urn:group-local", "Outer"))
			if len(sequence.Particles()) != 1 {
				t.Fatalf("omitted duplicate retained: %v", sequence.Particles())
			}
			groupLocalElement(t, sequence.Particles()[0], mustTestQName(t, "", "v"), "1/1", namedGroupLastLoc(t, root, `<xs:element name="v"`))
		})
		t.Run(profile.name+"/repeated reference-only group", func(t *testing.T) {
			root := namedGroupLocalRoot(profile.version, ` elementFormDefault="qualified"`, `<xs:group ref="g:Target"/><xs:group ref="g:Target"/><xs:element name="other" type="xs:integer"/>`, `<xs:group name="Target"><xs:sequence><xs:element ref="g:item"/></xs:sequence></xs:group><xs:element name="item" type="xs:integer"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			sequence := groupLocalSequence(t, namedSequenceGroup(t, schema, "urn:group-local", "Outer"))
			if len(sequence.Particles()) != 3 {
				t.Fatalf("reference-only group terms changed: %v", sequence.Particles())
			}
			groupLocalElement(t, sequence.Particles()[2], mustTestQName(t, "urn:group-local", "other"), "1/1", namedGroupChoiceLoc(t, root, `<xs:element name="other"`))
		})
	}
}

func TestNamedGroupLocalInvisibleAndDuplicateTypeGraphs(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:group-local"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:group name="Outer"><xs:sequence><xs:element name="v" type="h:Hidden" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:group></xs:schema>`
		hidden := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
		fixtures := map[string]discoveryFixture{
			"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`},
			"hidden.xsd": {id: "hidden.xsd", contents: hidden},
		}
		t.Run(profile.name+"/invisible under zero", func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertNamedGroupLocalFailure(t, schema, err, diagnosticSchemaElementTypeUnresolvedCode, FailureInvalid, namedGroupChoiceLoc(t, root, `type="h:Hidden"`), []Loc{allParticleTestTokenLoc(t, "hidden.xsd", hidden, `<xs:simpleType name="Hidden"`, 1)}, schemaElementTypeSpecRef(profile.factVersion), errSchemaElementTypeUnresolved)
		})
		t.Run(profile.name+"/duplicate declaration before ambiguous use", func(t *testing.T) {
			duplicate := namedGroupLocalRoot(profile.version, "", `<xs:element name="v" type="g:Same"/>`, `<xs:simpleType name="Same"><xs:restriction base="xs:string"/></xs:simpleType><xs:simpleType name="Same"><xs:restriction base="xs:token"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaGlobalDuplicateCode || diagnostic.Loc() != namedGroupLastLoc(t, duplicate, `<xs:simpleType name="Same"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{namedGroupChoiceLoc(t, duplicate, `<xs:simpleType name="Same"`)}) {
				t.Fatalf("duplicate type = %s related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

func TestNamedGroupLocalReferenceCycleAndZeroResolution(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		root := namedGroupLocalRoot(profile.version, "", `<xs:element name="local" type="xs:integer"/><xs:choice><xs:group ref="g:Target"/></xs:choice>`, `<xs:group name="Target"><xs:sequence><xs:group ref="g:Outer"/></xs:sequence></xs:group>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantSpec := "xsd11-structures#mg-props-correct"
		if profile.factVersion == XSDVersion10 {
			wantSpec = "xsd10-structures#mg-props-correct"
		}
		if diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Class() != FailureInvalid || diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="g:Outer"`) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
			t.Fatalf("cycle = %s related %v", diagnostic, diagnostic.Related())
		}
		if len(diagnostic.Related()) == 0 || diagnostic.Related()[0] != namedGroupChoiceLoc(t, root, `<xs:group name="Outer"`) {
			t.Fatalf("cycle path = %v", diagnostic.Related())
		}

		root = namedGroupLocalRoot(profile.version, "", `<xs:element name="local" type="xs:integer"/><xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="g:Missing"/></xs:choice>`, "")
		schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		assertNamedGroupLocalFailure(t, schema, err, diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, namedGroupChoiceLoc(t, root, `ref="g:Missing"`), nil, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceUnresolved)
	}
}
