package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineExtensionChoiceSchema(version, choice, extra string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice" version="` + version + `">
<xs:complexType name="openAttrs"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence><xs:element ref="t:first"/>` + choice + `<xs:element ref="t:last"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>
<xs:element name="first" type="xs:integer"/><xs:element name="last" type="xs:integer"/>` + extra + `
</xs:schema>`
}

func inlineExtensionChoiceOwner(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	for _, component := range schema.Components() {
		if component.Name().Local() != "root" {
			continue
		}
		element, ok := component.Element()
		if !ok {
			t.Fatal("root component is not an element")
		}
		inline, ok := element.InlineComplexType()
		if !ok {
			t.Fatal("root has no inline complex type")
		}
		return inline
	}
	t.Fatal("root component absent")
	return ComplexTypeDefinition{}
}

func TestInlineExtensionNestedChoicePublicFacts(t *testing.T) { //nolint:gocognit // Exact nested facts and owned copies form one public contract.
	profiles := []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"compatibility", "1.0", Compatibility}, {"strict10", "1.0", Strict10}, {"strict11", "1.1", Strict11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			choiceXML := `<xs:choice minOccurs="0" maxOccurs="18446744073709551616"><xs:element ref="t:last" minOccurs="2" maxOccurs="3"/><xs:element ref="t:first" minOccurs="0" maxOccurs="0"/></xs:choice>`
			root := inlineExtensionChoiceSchema(profile.version, choiceXML, "")
			var first []Component
			for repeat := 0; repeat < 3; repeat++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				if repeat == 0 {
					first = components
				}
				if !reflect.DeepEqual(first, components) || len(components) != 4 {
					t.Fatalf("component order = %v", components)
				}
				inline := inlineExtensionChoiceOwner(t, schema)
				if !inline.ID().IsZero() || inline.Name() != (QName{}) || inline.Loc() != complexContentTestLoc(t, root, `<xs:complexType><xs:complexContent><xs:extension`) || inline.Derivation() != ComplexTypeDerivationExtension || inline.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:openAttrs"`) {
					t.Fatalf("anonymous owner facts = %v", inline)
				}
				base, ok := inline.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID != components[0].ID() || base.Loc() != inline.BaseLoc() {
					t.Fatalf("base reference = %v", base)
				}
				wildcard, ok := inline.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				sequence, ok := inline.Particle().(SequenceParticle)
				if !ok || sequence.Occurrences().String() != "1/1" || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence>`) || len(sequence.Particles()) != 3 {
					t.Fatalf("sequence = %v", inline.Particle())
				}
				choice, ok := sequence.Particles()[1].(ChoiceParticle)
				if !ok || choice.Occurrences().String() != "0/18446744073709551616" || choice.Loc() != complexContentTestLoc(t, root, `<xs:choice`) || len(choice.Alternatives()) != 1 {
					t.Fatalf("choice = %v", sequence.Particles()[1])
				}
				last, ok := choice.Alternatives()[0].(ElementReferenceParticle)
				if !ok || last.Ref() != mustTestQName(t, "urn:inline-choice", "last") || last.TargetID() != components[3].ID() || last.RefLoc() != complexContentTestLoc(t, root, `ref="t:last" minOccurs="2"`) || last.Occurrences().String() != "2/3" {
					t.Fatalf("choice alternative = %v", choice.Alternatives()[0])
				}
				maximum, finite := choice.Occurrences().Maximum().Finite()
				if !finite {
					t.Fatal("large bound became unbounded")
				}
				maximum.value.SetInt64(0)
				alternatives := choice.Alternatives()
				alternatives[0] = nil
				terms := sequence.Particles()
				terms[1] = nil
				if choice.Occurrences().String() != "0/18446744073709551616" || choice.Alternatives()[0] == nil || sequence.Particles()[1] == nil {
					t.Fatal("public facts were mutable")
				}
			}
		})
	}
}

func TestInlineExtensionNestedChoiceZeroAndConsumers(t *testing.T) { //nolint:gocognit // Omission and separate consumer gates share one source fixture.
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			choiceXML := `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:last"/></xs:choice>`
			root := inlineExtensionChoiceSchema(profile.version, choiceXML, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			sequence, ok := inlineExtensionChoiceOwner(t, schema).Particle().(SequenceParticle)
			if !ok {
				t.Fatal("zero nested choice has no sequence")
			}
			if len(sequence.Particles()) != 2 {
				t.Fatalf("zero nested choice survived: %v", sequence.Particles())
			}
			for _, choice := range []string{`<xs:choice><xs:element ref="t:last"/></xs:choice>`, choiceXML} {
				root = inlineExtensionChoiceSchema(profile.version, choice, "")
				base := `<xs:complexType name="openAttrs"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
				root = strings.Replace(root, base+"\n", "", 1)
				root = strings.Replace(root, `<xs:element name="first"`, `<xs:complexType name="openAttrs"/><xs:element name="first"`, 1)
				schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:inline-choice"/>`)))
				validation := requireDiagnostic(t, validationErr)
				wantInstanceLoc := mustTestLoc(t, "instance.xml", 1, 1)
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantInstanceLoc || validation.SpecRef() != instanceValidationSpecRef(profile.factVersion) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v", validationErr)
				}
				output, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != complexContentTestLoc(t, root, `<xs:element name="root"`) || generation.SpecRef() != schemaElementTypeSpecRef(profile.factVersion) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v", output, generationErr)
				}
			}
		})
	}
}

func TestInlineExtensionNestedChoiceReferenceAndOccurrenceExits(t *testing.T) { //nolint:gocognit // Each alternate exit is checked at the same nested boundary.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, choice, extra, marker, code, spec string
			class                                   FailureClass
			cause                                   error
			related                                 []string
		}{
			{"missing under zero", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:missing"/></xs:choice>`, "", `ref="t:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceUnresolved, nil},
			{"wrong kind under zero", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:Other"/></xs:choice>`, `<xs:complexType name="Other"/>`, `ref="t:Other"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceWrongKind, []string{`<xs:complexType name="Other"`}},
			{"duplicate under zero", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/><xs:element ref="t:first"/></xs:choice>`, "", `ref="t:first"`, diagnosticSchemaElementReferenceDuplicateCode, schemaElementReferenceDuplicateSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceDuplicate, []string{`ref="t:first"`}},
			{"malformed choice minimum", `<xs:choice minOccurs="bad" maxOccurs="0"><xs:element ref="t:missing"/></xs:choice>`, "", `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
			{"negative choice minimum", `<xs:choice minOccurs="-1" maxOccurs="0"><xs:element ref="t:missing"/></xs:choice>`, "", `minOccurs="-1"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, errParticleOccurrenceNegative, nil},
			{"reversed choice range", `<xs:choice minOccurs="2" maxOccurs="1"><xs:element ref="t:missing"/></xs:choice>`, "", `<xs:choice minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"malformed child maximum", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first" maxOccurs="bad"/></xs:choice>`, "", `maxOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineExtensionChoiceSchema(profile.version, test.choice, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := complexContentTestLoc(t, root, test.marker)
				if test.name == "duplicate under zero" {
					wantLoc = namedGroupLastLoc(t, root, test.marker)
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec {
					t.Fatalf("diagnostic = %v, want %s at %s with %s", err, test.code, test.marker, test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("cause = %v, want %v", err, test.cause)
				}
				if test.name == "malformed choice minimum" || test.name == "malformed child maximum" {
					if errors.Unwrap(diagnostic) == nil {
						t.Fatal("malformed occurrence lost lexical cause")
					}
				}
				var related []Loc
				for _, marker := range test.related {
					related = append(related, complexContentTestLoc(t, root, marker))
				}
				if test.name == "duplicate under zero" {
					firstNested := strings.Index(root, `<xs:choice`)
					related = []Loc{namedGroupLocAt(t, root, firstNested+strings.Index(root[firstNested:], test.marker))}
				}
				if !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), related)
				}
			})
		}
	}
}

func TestInlineExtensionNestedChoiceExcludedShapes(t *testing.T) { //nolint:gocognit // Policy and omission variants retain one located exclusion contract.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, child, marker string }{
			{"local declaration", `<xs:element name="local" type="xs:integer"/>`, `<xs:element name="local"`},
			{"nested group", `<xs:group ref="t:Missing"/>`, `ref="t:Missing"`},
			{"nested sequence", `<xs:sequence><xs:element ref="t:first"/></xs:sequence>`, `<xs:sequence><xs:element`},
			{"deeper choice", `<xs:choice><xs:element ref="t:first"/></xs:choice>`, `<xs:choice><xs:element`},
			{"wildcard", `<xs:any/>`, `<xs:any`},
		} {
			for _, zero := range []bool{false, true} {
				name := profile.name + "/" + test.name
				attrs := ""
				if zero {
					name += "/zero"
					attrs = ` minOccurs="0" maxOccurs="0"`
				}
				t.Run(name, func(t *testing.T) {
					root := inlineExtensionChoiceSchema(profile.version, `<xs:choice`+attrs+`>`+test.child+`</xs:choice>`, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := complexContentTestLoc(t, root, test.marker)
					if test.name == "nested sequence" || test.name == "wildcard" {
						wantLoc = namedGroupLastLoc(t, root, test.marker)
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaComplexParticleExtensionSpecRef(profile.factVersion) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded %s = %v; got %s/%s at %s, spec=%q, want %s; unsupported=%t", test.name, err, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef(), wantLoc, errors.Is(err, ErrUnsupported))
					}
				})
			}
		}
	}
}

func TestInlineExtensionNestedChoiceGraphVisibility(t *testing.T) { //nolint:gocognit // Each graph shape verifies the public target, source, and authored location.
	owner := `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:choice minOccurs="0" maxOccurs="unbounded"><xs:element ref="t:remote"/></xs:choice></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`
	base := `<xs:complexType name="Base"/>`
	for _, test := range []struct {
		name, root, ownerXML, ownerSource, targetSource, targetNS string
		fixtures                                                  map[string]discoveryFixture
	}{
		{name: "forward", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice">` + owner + base + `<xs:element name="remote" type="xs:integer"/></xs:schema>`, ownerSource: "root.xsd", targetSource: "root.xsd", targetNS: "urn:inline-choice"},
		{name: "include", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice"><xs:include schemaLocation="child.xsd"/>` + owner + base + `</xs:schema>`, ownerSource: "root.xsd", targetSource: "child.xsd", targetNS: "urn:inline-choice", fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice"><xs:element name="remote" type="xs:integer"/></xs:schema>`}}},
		{name: "import", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" xmlns:o="urn:other" targetNamespace="urn:inline-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/>` + strings.Replace(owner, `ref="t:remote"`, `ref="o:remote"`, 1) + base + `</xs:schema>`, ownerSource: "root.xsd", targetSource: "other.xsd", targetNS: "urn:other", fixtures: map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="remote" type="xs:integer"/></xs:schema>`}}},
		{name: "chameleon", root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice"><xs:include schemaLocation="child.xsd"/>` + base + `<xs:element name="remote" type="xs:integer"/></xs:schema>`, ownerXML: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice">` + owner + `</xs:schema>`, ownerSource: "child.xsd", targetSource: "root.xsd", targetNS: "urn:inline-choice"},
	} {
		for _, profile := range nestedGroupChoiceProfiles {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				fixtures := test.fixtures
				if test.name == "chameleon" {
					fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: test.ownerXML}}
				}
				root := strings.Replace(test.root, `<xs:schema `, `<xs:schema version="`+profile.version+`" `, 1)
				var first []Component
				for repeat := 0; repeat < 3; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if repeat == 0 {
						first = schema.Components()
					}
					if !reflect.DeepEqual(first, schema.Components()) {
						t.Fatal("graph component order changed")
					}
					inline := inlineExtensionChoiceOwner(t, schema)
					sequence, ok := inline.Particle().(SequenceParticle)
					if !ok || len(sequence.Particles()) != 1 {
						t.Fatalf("graph sequence = %v", inline.Particle())
					}
					choice, ok := sequence.Particles()[0].(ChoiceParticle)
					if !ok || len(choice.Alternatives()) != 1 {
						t.Fatalf("graph choice = %v", sequence.Particles()[0])
					}
					ref, ok := choice.Alternatives()[0].(ElementReferenceParticle)
					if !ok {
						t.Fatalf("graph reference = %v", choice.Alternatives()[0])
					}
					var target Component
					for _, component := range schema.Components() {
						if component.Name() == mustTestQName(t, test.targetNS, "remote") {
							target = component
						}
					}
					if target.ID().IsZero() || ref.TargetID() != target.ID() || target.ID().Source() != SourceID(test.targetSource) || ref.Ref() != target.Name() {
						t.Fatalf("graph target = %v/%v", ref, target)
					}
					ownerXML := root
					if test.ownerSource != "root.xsd" {
						ownerXML = test.ownerXML
					}
					written := `ref="t:remote"`
					if test.name == "import" {
						written = `ref="o:remote"`
					}
					wantLoc := complexContentTestSourceLoc(t, SourceID(test.ownerSource), ownerXML, written)
					if ref.RefLoc() != wantLoc || choice.Loc().Source() != SourceID(test.ownerSource) || inline.Loc().Source() != SourceID(test.ownerSource) {
						t.Fatalf("graph locations = %s/%s/%s, want %s", ref.RefLoc(), choice.Loc(), inline.Loc(), wantLoc)
					}
				}
			})
		}
	}
}

func TestInlineExtensionNestedChoiceOtherOwnersAndUses(t *testing.T) {
	choice := `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/></xs:choice>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, owner, marker string }{
			{"named extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + choice + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`, `minOccurs="0"`},
			{"named direct", `<xs:complexType name="Derived"><xs:sequence>` + choice + `</xs:sequence></xs:complexType>`, `minOccurs="0"`},
			{"inline direct", `<xs:element name="other"><xs:complexType><xs:sequence>` + choice + `</xs:sequence></xs:complexType></xs:element>`, `minOccurs="0"`},
			{"inline extension attribute", `<xs:element name="other"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + choice + `</xs:sequence><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:attribute name="flag"`},
			{"inline extension prohibited attribute", `<xs:element name="other"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + choice + `</xs:sequence><xs:attribute name="flag" type="xs:boolean" use="prohibited"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:attribute name="flag"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:complexType name="openAttrs"/>` + test.owner + `<xs:element name="first" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("owner boundary = %v", err)
				}
			})
		}
	}
}

func TestInlineExtensionNestedChoiceInvisibleAndBaseFailures(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name+"/invisible", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" xmlns:o="urn:other" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="o:remote"/></xs:choice></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:complexType name="Base"/></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="remote" type="xs:integer"/></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
				"other.xsd": {id: "other.xsd", contents: other},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := complexContentTestSourceLoc(t, "other.xsd", other, `<xs:element name="remote"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceNamespaceCode || diagnostic.Loc() != complexContentTestLoc(t, root, `ref="o:remote"`) || diagnostic.SpecRef() != schemaElementReferenceImportSpecRef(profile.factVersion) || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, errSchemaElementReferenceNamespace) {
				t.Fatalf("invisible ref = %v; related=%v", err, diagnostic.Related())
			}
		})
		t.Run(profile.name+"/base cycle", func(t *testing.T) {
			root := inlineExtensionChoiceSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/></xs:choice>`, `<xs:complexType name="Loop"><xs:complexContent><xs:extension base="t:openAttrs"/></xs:complexContent></xs:complexType>`)
			root = strings.Replace(root, `<xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`, `<xs:extension base="t:Loop"/>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantPrimary := namedGroupLastLoc(t, root, `base="t:openAttrs"`)
			wantRelated := []Loc{complexContentTestLoc(t, root, `base="t:Loop"`)}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.factVersion) || !errors.Is(err, errSchemaComplexTypeBaseCycle) || diagnostic.Loc() != wantPrimary || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
				t.Fatalf("base cycle = %v; related=%v, want %v", err, diagnostic.Related(), wantRelated)
			}
		})
	}
}

func TestInlineExtensionNestedChoiceAmbiguousReferenceBoundary(t *testing.T) {
	name := mustTestQName(t, "urn:inline-choice", "remote")
	owner := schemaComponentRecord{id: ComponentID{source: "root.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, loc: mustTestLoc(t, "root.xsd", 2, 1), element: &schemaElementInput{}}
	first := schemaComponentRecord{id: ComponentID{source: "one.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, name: name, loc: mustTestLoc(t, "one.xsd", 2, 1), element: &schemaElementInput{}}
	second := schemaComponentRecord{id: ComponentID{source: "two.xsd", ordinal: 1}, kind: ComponentKindElementDeclaration, name: name, loc: mustTestLoc(t, "two.xsd", 2, 1), element: &schemaElementInput{}}
	refLoc := mustTestLoc(t, "root.xsd", 3, 20)
	zero := namedGroupTestOccurrenceRange(t, "0", "0")
	input := &schemaSequenceParticleInput{loc: owner.loc, occurrences: namedGroupTestOccurrenceRange(t, "1", "1"), particles: []schemaParticleTermInput{
		&schemaChoiceParticleInput{loc: refLoc, occurrences: zero, alternatives: []schemaParticleTermInput{
			schemaElementParticleInput{loc: refLoc, occurrences: zero, reference: &schemaElementReferenceInput{name: name, loc: refLoc}},
		}},
	}}
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			particle, err := resolveSchemaSequenceParticle(input, owner, []schemaComponentRecord{owner, first, second}, map[QName][]int{name: {1, 2}}, map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}}, schemaSimpleTypeResolution{}, profile.factVersion)
			if particle != nil {
				t.Fatalf("ambiguous zero choice returned %v", particle)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementReferenceAmbiguousCode || diagnostic.Loc() != refLoc || diagnostic.SpecRef() != schemaElementReferenceSpecRef(profile.factVersion) || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) || !errors.Is(err, errSchemaElementReferenceAmbiguous) {
				t.Fatalf("ambiguous ref = %v; related=%v", err, diagnostic.Related())
			}
		})
	}
}

func TestInlineExtensionNestedChoiceUnsupportedBase(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineExtensionChoiceSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/></xs:choice>`, "")
			root = strings.Replace(root, `<xs:complexType name="openAttrs"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, `<xs:complexType name="openAttrs"><xs:sequence><xs:element name="baseItem" type="xs:integer"/></xs:sequence></xs:complexType>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:complexType name="openAttrs"`), complexContentTestLoc(t, root, `<xs:sequence><xs:element name="baseItem"`)}
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != complexContentTestLoc(t, root, `base="t:openAttrs"`) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.factVersion) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
				t.Fatalf("unsupported base = %v; related=%v, want %v", err, diagnostic.Related(), wantRelated)
			}
		})
	}
}

func TestInlineExtensionSecondNestedChoiceBoundary(t *testing.T) { //nolint:gocognit // The second choice and its alternate syntax exits share one boundary.
	first := `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/></xs:choice>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, second, marker, code, spec string
			class                            FailureClass
			cause                            error
		}{
			{"second omitted", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:last"/></xs:choice>`, `<xs:choice minOccurs="0"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported},
			{"second malformed", `<xs:choice minOccurs="bad"><xs:element ref="t:last"/></xs:choice>`, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil},
			{"second wrong kind", `<xs:choice><xs:element ref="t:Other"/></xs:choice>`, `<xs:choice><xs:element`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineExtensionChoiceSchema(profile.version, first+test.second, `<xs:complexType name="Other"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := complexContentTestLoc(t, root, test.marker)
				if test.name == "second omitted" {
					wantLoc = namedGroupLastLoc(t, root, test.marker)
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 {
					t.Fatalf("second choice = %v; related=%v, spec=%q want %q, want %s at %s", err, diagnostic.Related(), diagnostic.SpecRef(), test.spec, test.code, wantLoc)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("second choice cause = %v, want %v", err, test.cause)
				}
				if test.name == "second malformed" && errors.Unwrap(diagnostic) == nil {
					t.Fatal("malformed second choice lost lexical cause")
				}
			})
		}
	}
}

func TestInlineExtensionChoiceLaterSequenceAndGroupStayExcluded(t *testing.T) {
	choice := `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/></xs:choice>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, later, marker string }{
			{"later sequence", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="t:last"/></xs:sequence>`, `<xs:element ref="t:last"`},
			{"later group", `<xs:group ref="t:Missing" minOccurs="0" maxOccurs="0"/>`, `ref="t:Missing"`},
			{"sibling local", `<xs:element name="local" type="xs:integer" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="local"`},
			{"sibling wildcard", `<xs:any namespace="##other" minOccurs="0" maxOccurs="0"/>`, `<xs:any namespace="##other"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineExtensionChoiceSchema(profile.version, choice, "")
				root = strings.Replace(root, `<xs:element ref="t:last"/></xs:sequence></xs:extension>`, test.later+`</xs:sequence></xs:extension>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := complexContentTestLoc(t, root, test.marker)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaComplexParticleExtensionSpecRef(profile.factVersion) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("later %s = %v; related=%v, spec=%q", test.name, err, diagnostic.Related(), diagnostic.SpecRef())
				}
			})
		}
	}
}

func TestInlineExtensionNestedChoiceMalformedComposition(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, choice, marker string }{
			{"late annotation", `<xs:choice><xs:element ref="t:first"/><xs:annotation/></xs:choice>`, `<xs:annotation/>`},
			{"forbidden all", `<xs:choice minOccurs="0" maxOccurs="0"><xs:all/></xs:choice>`, `<xs:all/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineExtensionChoiceSchema(profile.version, test.choice, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || diagnostic.SpecRef() != schemaComplexParticleExtensionSpecRef(profile.factVersion) || len(diagnostic.Related()) != 0 || errors.Is(err, ErrUnsupported) {
					t.Fatalf("malformed choice = %v; related=%v", err, diagnostic.Related())
				}
			})
		}
	}
}
