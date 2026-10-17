package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const inlinePairFirst = `<xs:sequence minOccurs="0" maxOccurs="unbounded"><xs:group ref="t:FirstGroup"/><xs:element ref="t:one" minOccurs="2" maxOccurs="3"/></xs:sequence>`
const inlinePairSecond = `<xs:sequence minOccurs="2" maxOccurs="18446744073709551616"><xs:group ref="t:SecondGroup"/><xs:element ref="t:two" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>`
const inlinePairTargets = `<xs:group name="FirstGroup"><xs:sequence/></xs:group><xs:group name="SecondGroup"><xs:sequence/></xs:group><xs:element name="one" type="xs:integer"/><xs:element name="two" type="xs:integer"/>`

func inlinePairSchema(version, middle, targets string) string {
	return inlineExtensionChoiceSchema(version, middle, targets)
}

func TestInlineExtensionPairPublicFactsAndConsumers(t *testing.T) { //nolint:gocognit,funlen // One projection verifies every ordered public fact and both consumer exits.
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlinePairSchema(profile.version, inlinePairFirst+inlinePairSecond, inlinePairTargets)
			root = strings.Replace(root, `<xs:sequence><xs:element ref="t:first"`, `<xs:sequence minOccurs="2" maxOccurs="3"><xs:element ref="t:first"`, 1)
			var firstComponents []Component
			for repeat := 0; repeat < 3; repeat++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				if repeat == 0 {
					firstComponents = components
				}
				if !reflect.DeepEqual(components, firstComponents) {
					t.Fatal("component order changed between parses")
				}
				walked := make([]ComponentID, 0, len(components))
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				for index, component := range components {
					if walked[index] != component.ID() {
						t.Fatalf("walk order = %v", walked)
					}
				}
				owner := inlineExtensionChoiceOwner(t, schema)
				if !owner.ID().IsZero() || owner.Name() != (QName{}) || owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType><xs:complexContent>`) || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:openAttrs"`) {
					t.Fatalf("owner provenance = %v", owner)
				}
				base, ok := owner.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID != components[0].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base reference = %v", base)
				}
				wildcard, ok := owner.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				outer, ok := owner.Particle().(SequenceParticle)
				if !ok || outer.Occurrences().String() != "2/3" || outer.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs="2"`) || len(outer.Particles()) != 4 {
					t.Fatalf("outer = %T", owner.Particle())
				}
				for index, want := range []struct{ group, element, bounds, elementBounds string }{
					{"FirstGroup", "one", "0/unbounded", "2/3"},
					{"SecondGroup", "two", "2/18446744073709551616", "0/unbounded"},
				} {
					nested, ok := outer.Particles()[index+1].(SequenceParticle)
					wantNestedLoc := complexContentTestLoc(t, root, `<xs:sequence minOccurs="0" maxOccurs="unbounded"`)
					if index == 1 {
						wantNestedLoc = complexContentTestLoc(t, root, `<xs:sequence minOccurs="2" maxOccurs="18446744073709551616"`)
					}
					if !ok || nested.Occurrences().String() != want.bounds || nested.Loc() != wantNestedLoc || len(nested.Particles()) != 2 {
						t.Fatalf("nested %d = %T", index, outer.Particles()[index+1])
					}
					group, groupOK := nested.Particles()[0].(ModelGroupReferenceParticle)
					element, elementOK := nested.Particles()[1].(ElementReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:inline-choice", want.group))
					elements := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:inline-choice", want.element))
					if !groupOK || !elementOK || len(groups) != 1 || len(elements) != 1 || group.TargetID() != groups[0].ID() || element.TargetID() != elements[0].ID() || group.Ref() != groups[0].Name() || element.Ref() != elements[0].Name() || group.Occurrences().String() != "1/1" || element.Occurrences().String() != want.elementBounds || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:`+want.group+`"`) || element.RefLoc() != complexContentTestLoc(t, root, `ref="t:`+want.element+`"`) || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:`+want.group+`"`) || element.Loc() != complexContentTestLoc(t, root, `<xs:element ref="t:`+want.element+`"`) {
						t.Fatalf("nested %d refs = %v/%v", index, nested.Particles()[0], nested.Particles()[1])
					}
					parts := nested.Particles()
					parts[0] = nil
					if nested.Particles()[0] == nil {
						t.Fatal("nested slice aliases public facts")
					}
				}
				first, firstOK := outer.Particles()[1].(SequenceParticle)
				second, secondOK := outer.Particles()[2].(SequenceParticle)
				if !firstOK || !secondOK {
					t.Fatal("ordered nested terms disappeared")
				}
				firstGroup, firstGroupOK := first.Particles()[0].(ModelGroupReferenceParticle)
				secondGroup, secondGroupOK := second.Particles()[0].(ModelGroupReferenceParticle)
				if !firstGroupOK || !secondGroupOK {
					t.Fatal("nested group terms disappeared")
				}
				if firstGroup.TargetID() == secondGroup.TargetID() {
					t.Fatal("distinct sibling targets alias")
				}
				maximum, finite := second.Occurrences().Maximum().Finite()
				if !finite {
					t.Fatal("large bound became unbounded")
				}
				maximum.value.SetInt64(0)
				terms := outer.Particles()
				terms[1] = nil
				if outer.Particles()[1] == nil || second.Occurrences().String() != "2/18446744073709551616" {
					t.Fatal("public bounds or terms are mutable")
				}
				if repeat == 0 {
					t.Run("ValidateInstance", func(t *testing.T) {
						validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:inline-choice"/>`)))
						diagnostic := requireDiagnostic(t, validationErr)
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || diagnostic.SpecRef() != instanceValidationSpecRef(profile.factVersion) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
							t.Fatalf("validation = %v", validationErr)
						}
					})
					t.Run("GenerateGo", func(t *testing.T) {
						output, generationErr := GenerateGo(schema, "generated")
						diagnostic := requireDiagnostic(t, generationErr)
						if output != nil || diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="openAttrs"`) || diagnostic.SpecRef() != codegenDirectSequenceSpecReference(profile.factVersion, codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, errCodegenUnsupported) {
							t.Fatalf("generation = %q/%v", output, generationErr)
						}
					})
				}
			}
		})
	}
}

func TestInlineExtensionPairLaterFailureAfterZero(t *testing.T) { //nolint:gocognit // Each later phase exit retains its own cause and location after early omission.
	firstZero := strings.Replace(inlinePairFirst, `maxOccurs="unbounded"`, `maxOccurs="0"`, 1)
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, second, marker, code, spec, related string
			class                                     FailureClass
			cause                                     error
			lexical                                   bool
		}{
			{"missing group", strings.Replace(inlinePairSecond, `ref="t:SecondGroup"`, `ref="t:MissingGroup"`, 1), `ref="t:MissingGroup"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), "", FailureInvalid, errSchemaModelGroupReferenceUnresolved, false},
			{"wrong group kind", strings.Replace(inlinePairSecond, `ref="t:SecondGroup"`, `ref="t:two"`, 1), `ref="t:two"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), `<xs:element name="two"`, FailureInvalid, errSchemaModelGroupReferenceWrongKind, false},
			{"missing element", strings.Replace(inlinePairSecond, `ref="t:two"`, `ref="t:MissingElement"`, 1), `ref="t:MissingElement"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), "", FailureInvalid, errSchemaElementReferenceUnresolved, false},
			{"wrong element kind", strings.Replace(inlinePairSecond, `ref="t:two"`, `ref="t:SecondGroup"`, 1), `ref="t:SecondGroup" minOccurs`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.factVersion), `<xs:group name="SecondGroup"`, FailureInvalid, errSchemaElementReferenceWrongKind, false},
			{"malformed nested bound", strings.Replace(inlinePairSecond, `minOccurs="2"`, `minOccurs="bad"`, 1), `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), "", FailureInvalid, nil, true},
			{"reversed nested range", strings.Replace(inlinePairSecond, `maxOccurs="18446744073709551616"`, `maxOccurs="1"`, 1), `<xs:sequence minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), `minOccurs="2"`, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, false},
			{"local element", strings.Replace(inlinePairSecond, `<xs:element ref="t:two" minOccurs="0" maxOccurs="unbounded"/>`, `<xs:element name="local" type="xs:integer"/>`, 1), `<xs:element name="local"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), "", FailureUnsupported, ErrUnsupported, false},
			{"wildcard", strings.Replace(inlinePairSecond, `<xs:element ref="t:two" minOccurs="0" maxOccurs="unbounded"/>`, `<xs:any/>`, 1), `<xs:any/>`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), "", FailureUnsupported, ErrUnsupported, false},
		} {
			for _, outerZero := range []bool{false, true} {
				name := profile.name + "/" + test.name
				if outerZero {
					name += "/outer-zero"
				}
				t.Run(name, func(t *testing.T) {
					root := inlinePairSchema(profile.version, firstZero+test.second, inlinePairTargets)
					if outerZero {
						root = strings.Replace(root, `<xs:sequence><xs:element ref="t:first"`, `<xs:sequence minOccurs="0" maxOccurs="0"><xs:element ref="t:first"`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := complexContentTestLoc(t, root, test.marker)
					var related []Loc
					if test.related != "" {
						related = []Loc{complexContentTestLoc(t, root, test.related)}
					}
					if test.name == "reversed nested range" {
						related = []Loc{complexContentTestLoc(t, root, `minOccurs="2" maxOccurs="1"`), complexContentTestLoc(t, root, `maxOccurs="1"`)}
					}
					if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), related) {
						t.Fatalf("later failure = %v; related %v, want %v", err, diagnostic.Related(), related)
					}
					if test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("cause = %v, want %v", err, test.cause)
					}
					if test.lexical && errors.Unwrap(diagnostic) == nil {
						t.Fatal("lexical cause was lost")
					}
				})
			}
		}
	}
}

func TestInlineExtensionPairOverlapBoundary(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, first, second, targets, marker, related, code, spec string
			class                                                     FailureClass
			cause                                                     error
		}{
			{"ambiguous optional first", strings.Replace(inlinePairFirst, `ref="t:one"`, `ref="t:two"`, 1), strings.Replace(inlinePairSecond, `minOccurs="2"`, `minOccurs="0"`, 1), inlinePairTargets, `ref="t:two" minOccurs="0"`, `ref="t:two" minOccurs="2"`, diagnosticSchemaInlineExtensionPairAmbiguousCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureInvalid, errSchemaInlineExtensionPairAmbiguous},
			{"ambiguous group first leaves", inlinePairFirst, strings.Replace(inlinePairSecond, `minOccurs="2"`, `minOccurs="0"`, 1), `<xs:group name="FirstGroup"><xs:sequence><xs:element ref="t:shared"/><xs:element ref="t:alpha"/></xs:sequence></xs:group><xs:group name="SecondGroup"><xs:sequence><xs:element ref="t:shared"/><xs:element ref="t:beta"/></xs:sequence></xs:group><xs:element name="one" type="xs:integer"/><xs:element name="two" type="xs:integer"/><xs:element name="shared" type="xs:integer"/><xs:element name="alpha" type="xs:integer"/><xs:element name="beta" type="xs:integer"/>`, `ref="t:SecondGroup"`, `ref="t:FirstGroup"`, diagnosticSchemaInlineExtensionPairAmbiguousCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureInvalid, errSchemaInlineExtensionPairAmbiguous},
			{"unproved repeated name", strings.Replace(inlinePairFirst, `ref="t:one"`, `ref="t:two"`, 1), inlinePairSecond, inlinePairTargets, `ref="t:two" minOccurs="0"`, `ref="t:two" minOccurs="2"`, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported},
			{"substitution overlap", inlinePairFirst, inlinePairSecond, `<xs:group name="FirstGroup"><xs:sequence><xs:element ref="t:head"/></xs:sequence></xs:group><xs:group name="SecondGroup"><xs:sequence><xs:element ref="t:member"/></xs:sequence></xs:group><xs:element name="one" type="xs:integer"/><xs:element name="two" type="xs:integer"/><xs:element name="head" type="xs:integer"/><xs:element name="member" type="xs:integer" substitutionGroup="t:head"/>`, `ref="t:SecondGroup"`, `ref="t:FirstGroup"`, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), FailureUnsupported, ErrUnsupported},
			{"inconsistent local types", inlinePairFirst, inlinePairSecond, strings.Replace(strings.Replace(inlinePairTargets, `<xs:sequence/></xs:group>`, `<xs:sequence><xs:element name="shared" type="xs:integer"/></xs:sequence></xs:group>`, 1), `<xs:sequence/></xs:group>`, `<xs:sequence><xs:element name="shared" type="xs:token"/></xs:sequence></xs:group>`, 1), `ref="t:SecondGroup"`, `ref="t:FirstGroup"`, diagnosticSchemaGroupElementConsistencyCode, schemaGroupElementConsistencySpecRef(profile.factVersion), FailureInvalid, errSchemaGroupElementConsistency},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlinePairSchema(profile.version, test.first+test.second, test.targets)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), []Loc{complexContentTestLoc(t, root, test.related)}) || !errors.Is(err, test.cause) {
					t.Fatalf("overlap = %v; related %v", err, diagnostic.Related())
				}
			})
		}
	}
}

func TestInlineExtensionPairGraphVisibility(t *testing.T) { //nolint:gocognit // Each discovery shape checks both sibling identities and authored source locations.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, shape := range []string{"forward", "include", "import", "chameleon"} {
			t.Run(profile.name+"/"+shape, func(t *testing.T) {
				prefix := "t"
				targetNS := "urn:inline-choice"
				targetSource := SourceID("root.xsd")
				ownerSource := SourceID("root.xsd")
				directive := ""
				targets := inlinePairTargets
				var fixtures map[string]discoveryFixture
				if shape == "include" {
					directive = `<xs:include schemaLocation="child.xsd"/>`
					targets = ""
					targetSource = "child.xsd"
					fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice">` + inlinePairTargets + `</xs:schema>`}}
				}
				if shape == "import" {
					prefix = "o"
					targetNS = "urn:other"
					directive = `<xs:import namespace="urn:other" schemaLocation="other.xsd"/>`
					targets = ""
					targetSource = "other.xsd"
					fixtures = map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + inlinePairTargets + `</xs:schema>`}}
				}
				pair := strings.ReplaceAll(inlinePairFirst+inlinePairSecond, `t:`, prefix+`:`)
				owner := `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + pair + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`
				base := `<xs:complexType name="openAttrs"/>`
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" xmlns:o="urn:other" targetNamespace="urn:inline-choice" version="` + profile.version + `">` + directive + base + owner + targets + `</xs:schema>`
				ownerXML := root
				if shape == "chameleon" {
					ownerSource = "child.xsd"
					ownerXML = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice">` + owner + `</xs:schema>`
					root = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/>` + base + targets + `</xs:schema>`
					fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: ownerXML}}
				}
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
					outer, ok := inlineExtensionChoiceOwner(t, schema).Particle().(SequenceParticle)
					if !ok || len(outer.Particles()) != 2 {
						t.Fatalf("graph outer = %T", inlineExtensionChoiceOwner(t, schema).Particle())
					}
					for index, want := range []struct{ group, element string }{{"FirstGroup", "one"}, {"SecondGroup", "two"}} {
						nested, ok := outer.Particles()[index].(SequenceParticle)
						if !ok || len(nested.Particles()) != 2 {
							t.Fatalf("graph nested %d = %T", index, outer.Particles()[index])
						}
						group, groupOK := nested.Particles()[0].(ModelGroupReferenceParticle)
						element, elementOK := nested.Particles()[1].(ElementReferenceParticle)
						groupTargets := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, targetNS, want.group))
						elementTargets := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, targetNS, want.element))
						if !groupOK || !elementOK || len(groupTargets) != 1 || len(elementTargets) != 1 || group.TargetID() != groupTargets[0].ID() || element.TargetID() != elementTargets[0].ID() || group.TargetID().Source() != targetSource || element.TargetID().Source() != targetSource || group.Ref() != groupTargets[0].Name() || element.Ref() != elementTargets[0].Name() || group.RefLoc() != complexContentTestSourceLoc(t, ownerSource, ownerXML, `ref="`+prefix+`:`+want.group+`"`) || element.RefLoc() != complexContentTestSourceLoc(t, ownerSource, ownerXML, `ref="`+prefix+`:`+want.element+`"`) || nested.Loc().Source() != ownerSource {
							t.Fatalf("graph nested %d = %v/%v", index, nested.Particles()[0], nested.Particles()[1])
						}
					}
				}
			})
		}
	}
}

func TestInlineExtensionPairExcludedSiblingShapes(t *testing.T) { //nolint:gocognit // Each excluded direct, nested, and attribute shape has a located nil-schema result.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, middle, suffix, marker string }{
			{"direct group sibling", inlinePairFirst + `<xs:group ref="t:ThirdGroup"/>` + inlinePairSecond, "", `ref="t:ThirdGroup"`},
			{"third nested sequence", inlinePairFirst + inlinePairSecond + strings.Replace(inlinePairSecond, `minOccurs="2"`, `minOccurs="4"`, 1), "", `<xs:sequence minOccurs="4"`},
			{"choice plus two sequences", `<xs:choice><xs:element ref="t:first"/></xs:choice>` + inlinePairFirst + inlinePairSecond, "", `<xs:sequence minOccurs="2"`},
			{"later nested choice", inlinePairFirst + `<xs:choice><xs:element ref="t:first"/></xs:choice>` + inlinePairSecond, "", `<xs:choice><xs:element`},
			{"deeper sequence", inlinePairFirst + strings.Replace(inlinePairSecond, `<xs:element ref="t:two" minOccurs="0" maxOccurs="unbounded"/>`, `<xs:sequence><xs:element ref="t:two"/></xs:sequence>`, 1), "", `<xs:sequence><xs:element ref="t:two"`},
			{"deeper choice", inlinePairFirst + strings.Replace(inlinePairSecond, `<xs:element ref="t:two" minOccurs="0" maxOccurs="unbounded"/>`, `<xs:choice><xs:element ref="t:two"/></xs:choice>`, 1), "", `<xs:choice><xs:element ref="t:two"`},
			{"attribute", inlinePairFirst + inlinePairSecond, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:attribute name="flag"`},
			{"prohibited attribute", inlinePairFirst + inlinePairSecond, `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`, `<xs:attribute name="flag"`},
			{"attribute group ref", inlinePairFirst + inlinePairSecond, `<xs:attributeGroup ref="t:Attrs"/>`, `<xs:attributeGroup ref="t:Attrs"`},
			{"attribute wildcard", inlinePairFirst + inlinePairSecond, `<xs:anyAttribute namespace="##other" processContents="lax"/>`, `<xs:anyAttribute namespace="##other"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlinePairSchema(profile.version, test.middle, inlinePairTargets+`<xs:group name="ThirdGroup"><xs:sequence/></xs:group><xs:attributeGroup name="Attrs"><xs:attribute name="flag" type="xs:boolean"/></xs:attributeGroup>`)
				if test.suffix != "" {
					root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence>`+test.suffix+`</xs:extension>`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := complexContentTestLoc(t, root, test.marker)
				wantSpec := schemaComplexParticleExtensionSpecRef(profile.factVersion)
				if test.suffix != "" {
					wantSpec = schemaComplexTypeExtensionSpecRef(profile.factVersion)
				}
				if test.name == "attribute wildcard" {
					wantLoc = namedGroupLastLoc(t, root, test.marker)
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded %s = %v; loc %s want %s, spec %s want %s", test.name, err, diagnostic.Loc(), wantLoc, diagnostic.SpecRef(), wantSpec)
				}
			})
		}
	}
}

func TestInlineExtensionPairExcludedOwners(t *testing.T) {
	outer := `<xs:sequence>` + inlinePairFirst + inlinePairSecond + `</xs:sequence>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, owner string }{
			{"named extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:openAttrs">` + outer + `</xs:extension></xs:complexContent></xs:complexType>`},
			{"named direct", `<xs:complexType name="Derived">` + outer + `</xs:complexType>`},
			{"inline direct", `<xs:element name="other"><xs:complexType>` + outer + `</xs:complexType></xs:element>`},
			{"named group", `<xs:group name="Other">` + outer + `</xs:group>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:complexType name="openAttrs"/>` + test.owner + inlinePairTargets + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != complexContentTestLoc(t, root, `minOccurs="0"`) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded owner %s = %v; loc %s want %s", test.name, err, diagnostic.Loc(), complexContentTestLoc(t, root, `minOccurs="0"`))
				}
			})
		}
	}
}

func TestInlineExtensionPairLaterGroupCycleBeforeOmission(t *testing.T) {
	firstZero := strings.Replace(inlinePairFirst, `maxOccurs="unbounded"`, `maxOccurs="0"`, 1)
	targets := strings.Replace(inlinePairTargets, `<xs:group name="SecondGroup"><xs:sequence/></xs:group>`, `<xs:group name="SecondGroup"><xs:sequence><xs:group ref="t:SecondGroup"/></xs:sequence></xs:group>`, 1)
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlinePairSchema(profile.version, firstZero+inlinePairSecond, targets)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantSpec := "xsd11-structures#mg-props-correct"
			if profile.factVersion == XSDVersion10 {
				wantSpec = "xsd10-structures#mg-props-correct"
			}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode || diagnostic.Loc() != namedGroupLastLoc(t, root, `ref="t:SecondGroup"`) || diagnostic.SpecRef() != wantSpec || !reflect.DeepEqual(diagnostic.Related(), []Loc{complexContentTestLoc(t, root, `<xs:group name="SecondGroup"`)}) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
				t.Fatalf("later group cycle = %v; related %v", err, diagnostic.Related())
			}
		})
	}
}

func TestInlineExtensionPairLaterInvisibleTargets(t *testing.T) {
	firstZero := strings.Replace(inlinePairFirst, `maxOccurs="unbounded"`, `maxOccurs="0"`, 1)
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="SecondGroup"><xs:sequence/></xs:group><xs:element name="two" type="xs:integer"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}, "other.xsd": {id: "other.xsd", contents: other}}
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, second, local, marker, related, code, spec string
			cause                                            error
		}{
			{"group", strings.Replace(inlinePairSecond, `ref="t:SecondGroup"`, `ref="o:SecondGroup"`, 1), `<xs:element name="two" type="xs:integer"/>`, `ref="o:SecondGroup"`, `<xs:group name="SecondGroup"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.factVersion), errSchemaModelGroupReferenceNamespace},
			{"element", strings.Replace(inlinePairSecond, `ref="t:two"`, `ref="o:two"`, 1), `<xs:group name="SecondGroup"><xs:sequence/></xs:group>`, `ref="o:two"`, `<xs:element name="two"`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" xmlns:o="urn:other" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/><xs:complexType name="openAttrs"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + firstZero + test.second + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:group name="FirstGroup"><xs:sequence/></xs:group><xs:element name="one" type="xs:integer"/>` + test.local + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc{complexContentTestSourceLoc(t, "other.xsd", other, test.related)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, test.cause) {
					t.Fatalf("later invisible %s = %v; related %v", test.name, err, diagnostic.Related())
				}
			})
		}
	}
}

func TestInlineExtensionPairDuplicateTargetPrecedesAmbiguousResolution(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlinePairSchema(profile.version, inlinePairFirst+inlinePairSecond, inlinePairTargets+`<xs:group name="SecondGroup"><xs:sequence/></xs:group>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaGlobalDuplicateCode || diagnostic.Loc() != namedGroupLastLoc(t, root, `<xs:group name="SecondGroup"`) || diagnostic.SpecRef() != schemaGlobalDuplicateSpecRef(profile.factVersion) || !reflect.DeepEqual(diagnostic.Related(), []Loc{complexContentTestLoc(t, root, `<xs:group name="SecondGroup"`)}) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
				t.Fatalf("duplicate target = %v; related %v", err, diagnostic.Related())
			}
		})
	}
}
