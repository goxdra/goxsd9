package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const inlineNestedSequence = `<xs:sequence minOccurs="0" maxOccurs="unbounded"><xs:group ref="t:Target"/><xs:element ref="t:annotation" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>`
const inlineNestedTargets = `<xs:group name="Target"><xs:sequence/></xs:group><xs:element name="annotation" type="xs:integer"/>`

func TestInlineExtensionNestedSequencePublicFactsAndConsumers(t *testing.T) { //nolint:gocognit // One public view checks ownership, order, bounds, targets, locations, and consumer exits.
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineExtensionChoiceSchema(profile.version, inlineNestedSequence, inlineNestedTargets)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			inline := inlineExtensionChoiceOwner(t, schema)
			if !inline.ID().IsZero() || inline.Name() != (QName{}) || inline.Derivation() != ComplexTypeDerivationExtension ||
				inline.Loc() != complexContentTestLoc(t, root, `<xs:complexType><xs:complexContent>`) ||
				inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:openAttrs"`) {
				t.Fatalf("anonymous extension = %v", inline)
			}
			wildcard, ok := inline.AnyAttribute()
			if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" ||
				wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
				t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
			}
			outer, ok := inline.Particle().(SequenceParticle)
			if !ok || outer.Occurrences().String() != "1/1" || len(outer.Particles()) != 3 ||
				outer.Loc() != complexContentTestLoc(t, root, `<xs:sequence><xs:element ref="t:first"`) {
				t.Fatalf("outer sequence = %T", inline.Particle())
			}
			nested, ok := outer.Particles()[1].(SequenceParticle)
			if !ok || nested.Occurrences().String() != "0/unbounded" || len(nested.Particles()) != 2 ||
				nested.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs="0" maxOccurs="unbounded"`) {
				t.Fatalf("nested sequence = %T", outer.Particles()[1])
			}
			group, ok := nested.Particles()[0].(ModelGroupReferenceParticle)
			groupTarget := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:inline-choice", "Target"))
			if !ok || len(groupTarget) != 1 || group.Ref() != groupTarget[0].Name() || group.TargetID() != groupTarget[0].ID() ||
				group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Target"`) ||
				group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Target"`) || group.Occurrences().String() != "1/1" {
				t.Fatalf("nested group = %T", nested.Particles()[0])
			}
			annotation, ok := nested.Particles()[1].(ElementReferenceParticle)
			annotationTarget := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:inline-choice", "annotation"))
			if !ok || len(annotationTarget) != 1 || annotation.Ref() != annotationTarget[0].Name() || annotation.TargetID() != annotationTarget[0].ID() ||
				annotation.Loc() != complexContentTestLoc(t, root, `<xs:element ref="t:annotation"`) ||
				annotation.RefLoc() != complexContentTestLoc(t, root, `ref="t:annotation"`) || annotation.Occurrences().String() != "0/unbounded" {
				t.Fatalf("nested annotation = %T", nested.Particles()[1])
			}
			terms := nested.Particles()
			terms[0] = nil
			if nested.Particles()[0] == nil {
				t.Fatal("nested particles exposed backing slice")
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:inline-choice"/>`)))
			validation := requireDiagnostic(t, validationErr)
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode ||
				validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || validation.SpecRef() != instanceValidationSpecRef(profile.factVersion) ||
				!errors.Is(validationErr, errInstanceComplexContentExtension) {
				t.Fatalf("validation = %v", validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
				generation.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="openAttrs"`) ||
				generation.SpecRef() != codegenDirectSequenceSpecReference(profile.factVersion, codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %q/%v", output, generationErr)
			}
		})
	}
}

func TestInlineExtensionNestedSequenceZeroAndDefaultBounds(t *testing.T) { //nolint:gocognit // Exact bound variants share one public omission check.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, term, bounds string }{
			{"zero", strings.Replace(inlineNestedSequence, `maxOccurs="unbounded"`, `maxOccurs="0"`, 1), ""},
			{"default", strings.Replace(inlineNestedSequence, ` minOccurs="0" maxOccurs="unbounded"`, "", 1), "1/1"},
			{"large", strings.Replace(inlineNestedSequence, `maxOccurs="unbounded"`, `maxOccurs="18446744073709551616"`, 1), "0/18446744073709551616"},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineExtensionChoiceSchema(profile.version, test.term, inlineNestedTargets)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := inlineExtensionChoiceOwner(t, schema).Particle().(SequenceParticle)
				if !ok {
					t.Fatal("anonymous extension has no outer sequence")
				}
				terms := outer.Particles()
				if test.name == "zero" {
					if len(terms) != 2 {
						t.Fatalf("zero nested sequence survived: %v", terms)
					}
					return
				}
				if len(terms) != 3 {
					t.Fatalf("outer terms = %d, want 3", len(terms))
				}
				nested, ok := terms[1].(SequenceParticle)
				if !ok || nested.Occurrences().String() != test.bounds {
					t.Fatalf("nested = %T/%s, want %s", terms[1], nested.Occurrences(), test.bounds)
				}
			})
		}
	}
}

func TestInlineExtensionNestedSequenceExcludedShapes(t *testing.T) { //nolint:gocognit // Each valid excluded form must stay located and unpublished, including 0/0.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, term, extra, marker string }{
			{"local", `<xs:sequence><xs:group ref="t:Target"/><xs:element name="local" type="xs:integer"/></xs:sequence>`, inlineNestedTargets, `<xs:element name="local"`},
			{"wildcard", `<xs:sequence><xs:group ref="t:Target"/><xs:any/></xs:sequence>`, inlineNestedTargets, `<xs:any`},
			{"deeper choice", `<xs:sequence><xs:group ref="t:Target"/><xs:choice><xs:element ref="t:annotation"/></xs:choice></xs:sequence>`, inlineNestedTargets, `<xs:choice>`},
			{"deeper sequence", `<xs:sequence><xs:group ref="t:Target"/><xs:sequence><xs:element ref="t:annotation"/></xs:sequence></xs:sequence>`, inlineNestedTargets, `<xs:sequence><xs:element ref="t:annotation"`},
			{"reversed", `<xs:sequence><xs:element ref="t:annotation"/><xs:group ref="t:Target"/></xs:sequence>`, inlineNestedTargets, `<xs:element ref="t:annotation"`},
			{"group bounds", `<xs:sequence><xs:group ref="t:Target" minOccurs="0"/><xs:element ref="t:annotation"/></xs:sequence>`, inlineNestedTargets, `<xs:group ref="t:Target"`},
			{"third sequence", inlineNestedSequence + inlineNestedSequence + inlineNestedSequence, inlineNestedTargets, `<xs:sequence minOccurs="0"`},
			{"later choice", inlineNestedSequence + `<xs:choice><xs:element ref="t:first"/></xs:choice>`, inlineNestedTargets, `<xs:choice>`},
		} {
			for _, zero := range []bool{false, true} {
				name := profile.name + "/" + test.name
				term := test.term
				if zero {
					name += "/zero"
					term = strings.Replace(term, `<xs:sequence>`, `<xs:sequence minOccurs="0" maxOccurs="0">`, 1)
					term = strings.Replace(term, `<xs:sequence minOccurs="0" maxOccurs="unbounded">`, `<xs:sequence minOccurs="0" maxOccurs="0">`, 1)
				}
				t.Run(name, func(t *testing.T) {
					root := inlineExtensionChoiceSchema(profile.version, term, test.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					loc := complexContentTestLoc(t, root, test.marker)
					if test.name == "third sequence" {
						loc = namedGroupLastLoc(t, root, test.marker)
					}
					if test.name == "wildcard" {
						loc = namedGroupLastLoc(t, root, test.marker)
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != loc ||
						diagnostic.SpecRef() != schemaComplexParticleExtensionSpecRef(profile.factVersion) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded %s = %v; want at %s", test.name, err, loc)
					}
				})
			}
		}
	}
}

func TestInlineExtensionNestedSequenceForbiddenAndUnknownChildren(t *testing.T) { //nolint:gocognit // The scoped child walker must distinguish invalid grammar from unsupported vocabulary before zero omission.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, child, marker string
			class               FailureClass
			code, spec          string
		}{
			{"attribute", `<xs:attribute name="bad"/>`, `<xs:attribute name="bad"`, FailureInvalid, invalidSchemaCompositionCode, ""},
			{"complex type", `<xs:complexType/>`, `<xs:complexType/>`, FailureInvalid, invalidSchemaCompositionCode, ""},
			{"all", `<xs:all/>`, `<xs:all/>`, FailureInvalid, invalidSchemaCompositionCode, ""},
			{"foreign", `<bad xmlns="urn:foreign"/>`, `<bad xmlns="urn:foreign"`, FailureInvalid, invalidSchemaCompositionCode, ""},
			{"text", `bad text`, `bad text`, FailureInvalid, invalidSchemaCompositionCode, ""},
			{"late annotation", `<xs:group ref="t:Target"/><xs:annotation/>`, `<xs:annotation/>`, FailureInvalid, invalidSchemaCompositionCode, ""},
			{"unknown", `<xs:bogus/>`, `<xs:bogus/>`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion)},
		} {
			for _, zero := range []bool{false, true} {
				name := profile.name + "/" + test.name
				sequence := `<xs:sequence>`
				if zero {
					name += "/zero"
					sequence = `<xs:sequence minOccurs="0" maxOccurs="0">`
				}
				t.Run(name, func(t *testing.T) {
					root := inlineExtensionChoiceSchema(profile.version, sequence+test.child+`</xs:sequence>`, inlineNestedTargets)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					loc := complexContentTestLoc(t, root, test.marker)
					if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != loc ||
						diagnostic.SpecRef() != test.spec || len(diagnostic.Related()) != 0 || errors.Unwrap(diagnostic) != nil {
						t.Fatalf("nested child = %v; want %s at %s, spec %q, no related locations or cause", err, test.code, loc, test.spec)
					}
					if (test.class == FailureUnsupported) != errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported classification = %v", err)
					}
				})
			}
		}
	}
}

func TestInlineExtensionNestedSequenceOtherOwnersAndCombinedAttributes(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, owner, marker string }{
			{"named extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + inlineNestedSequence + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`, `minOccurs="0"`},
			{"named direct", `<xs:complexType name="Derived"><xs:sequence>` + inlineNestedSequence + `</xs:sequence></xs:complexType>`, `minOccurs="0"`},
			{"inline direct", `<xs:element name="other"><xs:complexType><xs:sequence>` + inlineNestedSequence + `</xs:sequence></xs:complexType></xs:element>`, `minOccurs="0"`},
			{"named group", `<xs:group name="Other"><xs:sequence>` + inlineNestedSequence + `</xs:sequence></xs:group>`, `minOccurs="0"`},
			{"attribute", `<xs:element name="other"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + inlineNestedSequence + `</xs:sequence><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:attribute name="flag"`},
			{"prohibited attribute", `<xs:element name="other"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + inlineNestedSequence + `</xs:sequence><xs:attribute name="flag" type="xs:boolean" use="prohibited"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:attribute name="flag"`},
			{"attribute group", `<xs:element name="other"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + inlineNestedSequence + `</xs:sequence><xs:attributeGroup ref="t:Attrs"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:attributeGroup ref="t:Attrs"`},
			{"direct wildcard attribute", `<xs:element name="other"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + inlineNestedSequence + `</xs:sequence><xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:anyAttribute`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:complexType name="openAttrs"/>` + test.owner + inlineNestedTargets + `<xs:attributeGroup name="Attrs"><xs:attribute name="flag" type="xs:boolean"/></xs:attributeGroup></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
					diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("owner/attribute boundary = %v", err)
				}
			})
		}
	}
}

func TestInlineExtensionNestedSequenceReferenceAndOccurrenceExits(t *testing.T) { //nolint:gocognit // Same boundary, alternate invalid/resolution/unsupported exits and no schema.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, term, extra, marker, code, spec string
			class                                 FailureClass
			cause                                 error
			related                               []string
		}{
			{"missing group under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:Missing"/><xs:element ref="t:annotation"/></xs:sequence>`, inlineNestedTargets, `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
			{"wrong group kind under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:annotation"/><xs:element ref="t:annotation"/></xs:sequence>`, inlineNestedTargets, `ref="t:annotation"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="annotation"`}},
			{"missing element under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:Target"/><xs:element ref="t:Missing"/></xs:sequence>`, inlineNestedTargets, `ref="t:Missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceUnresolved, nil},
			{"wrong element kind under zero", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:Target"/><xs:element ref="t:Target"/></xs:sequence>`, inlineNestedTargets, `ref="t:Target"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.factVersion), FailureInvalid, errSchemaElementReferenceWrongKind, []string{`<xs:group name="Target"`}},
			{"malformed nested minimum", strings.Replace(inlineNestedSequence, `minOccurs="0"`, `minOccurs="bad"`, 1), inlineNestedTargets, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
			{"malformed minimum before unknown child", `<xs:sequence minOccurs="bad"><xs:bogus/></xs:sequence>`, inlineNestedTargets, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
			{"negative nested minimum", strings.Replace(inlineNestedSequence, `minOccurs="0"`, `minOccurs="-1"`, 1), inlineNestedTargets, `minOccurs="-1"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, errParticleOccurrenceNegative, nil},
			{"reversed nested range", strings.Replace(inlineNestedSequence, `minOccurs="0" maxOccurs="unbounded"`, `minOccurs="2" maxOccurs="1"`, 1), inlineNestedTargets, `<xs:sequence minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"malformed element maximum", strings.Replace(inlineNestedSequence, `maxOccurs="unbounded"/>`, `maxOccurs="bad"/>`, 1), inlineNestedTargets, `maxOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), FailureInvalid, nil, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := inlineExtensionChoiceSchema(profile.version, test.term, test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				var related []Loc
				for _, marker := range test.related {
					related = append(related, complexContentTestLoc(t, root, marker))
				}
				loc := complexContentTestLoc(t, root, test.marker)
				if test.name == "wrong element kind under zero" {
					loc = namedGroupLastLoc(t, root, test.marker)
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != loc ||
					diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), related) {
					t.Fatalf("exit = %v, related %v; want %s at %s related %v spec %s", err, diagnostic.Related(), test.code, test.marker, related, test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("cause = %v, want %v", err, test.cause)
				}
				if strings.HasPrefix(test.name, "malformed") && errors.Unwrap(diagnostic) == nil {
					t.Fatal("malformed occurrence lost lexical cause")
				}
			})
		}
	}
}

func TestInlineExtensionNestedSequenceGraphVisibilityAndOrder(t *testing.T) { //nolint:gocognit // Both nested refs must retain source identity across four discovery shapes.
	base := `<xs:complexType name="openAttrs"/>`
	owner := `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + inlineNestedSequence + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`
	targets := inlineNestedTargets
	for _, test := range []struct {
		name, root, ownerXML, targetNS, targetSource, groupRef, elementRef string
		fixtures                                                           map[string]discoveryFixture
	}{
		{"forward", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice">` + owner + base + targets + `</xs:schema>`, "", "urn:inline-choice", "root.xsd", `ref="t:Target"`, `ref="t:annotation"`, nil},
		{"include", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" targetNamespace="urn:inline-choice"><xs:include schemaLocation="child.xsd"/>` + owner + base + `</xs:schema>`, "", "urn:inline-choice", "child.xsd", `ref="t:Target"`, `ref="t:annotation"`, map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice">` + targets + `</xs:schema>`}}},
		{"import", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" xmlns:o="urn:other" targetNamespace="urn:inline-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/>` + strings.ReplaceAll(owner, `ref="t:`, `ref="o:`) + base + `</xs:schema>`, "", "urn:other", "other.xsd", `ref="o:Target"`, `ref="o:annotation"`, map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + targets + `</xs:schema>`}}},
		{"chameleon", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice"><xs:include schemaLocation="child.xsd"/>` + base + targets + `</xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice">` + strings.ReplaceAll(owner, `ref="t:`, `ref="`) + `</xs:schema>`, "urn:inline-choice", "root.xsd", `ref="Target"`, `ref="annotation"`, nil},
	} {
		for _, profile := range nestedGroupChoiceProfiles {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := strings.Replace(test.root, `<xs:schema `, `<xs:schema version="`+profile.version+`" `, 1)
				fixtures := test.fixtures
				ownerXML := root
				ownerSource := SourceID("root.xsd")
				if test.name == "chameleon" {
					fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: test.ownerXML}}
					ownerXML = test.ownerXML
					ownerSource = "child.xsd"
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
						t.Fatal("component discovery order changed")
					}
					inline := inlineExtensionChoiceOwner(t, schema)
					outer, ok := inline.Particle().(SequenceParticle)
					if !ok || len(outer.Particles()) != 1 {
						t.Fatalf("outer = %T", inline.Particle())
					}
					nested, ok := outer.Particles()[0].(SequenceParticle)
					if !ok || len(nested.Particles()) != 2 || nested.Occurrences().String() != "0/unbounded" {
						t.Fatalf("nested = %T", outer.Particles()[0])
					}
					group, groupOK := nested.Particles()[0].(ModelGroupReferenceParticle)
					element, elementOK := nested.Particles()[1].(ElementReferenceParticle)
					groupTargets := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, test.targetNS, "Target"))
					elementTargets := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, test.targetNS, "annotation"))
					if !groupOK || !elementOK || len(groupTargets) != 1 || len(elementTargets) != 1 ||
						group.TargetID() != groupTargets[0].ID() || element.TargetID() != elementTargets[0].ID() ||
						group.TargetID().Source() != SourceID(test.targetSource) || element.TargetID().Source() != SourceID(test.targetSource) ||
						group.Ref() != groupTargets[0].Name() || element.Ref() != elementTargets[0].Name() {
						t.Fatalf("nested target identities = %v/%v", nested.Particles()[0], nested.Particles()[1])
					}
					if group.RefLoc() != complexContentTestSourceLoc(t, ownerSource, ownerXML, test.groupRef) ||
						element.RefLoc() != complexContentTestSourceLoc(t, ownerSource, ownerXML, test.elementRef) ||
						inline.Loc().Source() != ownerSource || nested.Loc().Source() != ownerSource {
						t.Fatalf("nested source locations = %s/%s/%s", group.RefLoc(), element.RefLoc(), nested.Loc())
					}
				}
			})
		}
	}
}

func TestInlineExtensionNestedSequenceCycleBeforeZeroOmission(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name, func(t *testing.T) {
			zero := strings.Replace(inlineNestedSequence, `maxOccurs="unbounded"`, `maxOccurs="0"`, 1)
			cycle := `<xs:group name="Target"><xs:sequence><xs:group ref="t:Target"/></xs:sequence></xs:group><xs:element name="annotation" type="xs:integer"/>`
			root := inlineExtensionChoiceSchema(profile.version, zero, cycle)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := namedGroupLastLoc(t, root, `ref="t:Target"`)
			wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:group name="Target"`)}
			wantSpec := "xsd11-structures#mg-props-correct"
			if profile.factVersion == XSDVersion10 {
				wantSpec = "xsd10-structures#mg-props-correct"
			}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceCycleCode ||
				diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec ||
				!reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
				t.Fatalf("cycle = %v; related %v want %v", err, diagnostic.Related(), wantRelated)
			}
		})
	}
}

func TestInlineExtensionNestedSequenceInvisibleReferences(t *testing.T) {
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + inlineNestedTargets + `</xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:inline-choice"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, nested, local, marker, related, code, spec string
			cause                                            error
		}{
			{"group", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="o:Target"/><xs:element ref="o:annotation"/></xs:sequence>`, "", `ref="o:Target"`, `<xs:group name="Target"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.factVersion), errSchemaModelGroupReferenceNamespace},
			{"element", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:Target"/><xs:element ref="o:annotation"/></xs:sequence>`, `<xs:group name="Target"><xs:sequence/></xs:group>`, `ref="o:annotation"`, `<xs:element name="annotation"`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:inline-choice" xmlns:o="urn:other" targetNamespace="urn:inline-choice" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/><xs:complexType name="openAttrs"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:openAttrs"><xs:sequence>` + test.nested + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>` + test.local + `</xs:schema>`
				fixtures := map[string]discoveryFixture{
					"child.xsd": {id: "child.xsd", contents: child},
					"other.xsd": {id: "other.xsd", contents: other},
				}
				for repeat := 0; repeat < 3; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantRelated := []Loc{complexContentTestSourceLoc(t, "other.xsd", other, test.related)}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code ||
						diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || diagnostic.SpecRef() != test.spec ||
						!reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, test.cause) {
						t.Fatalf("invisible %s = %v; related %v want %v", test.name, err, diagnostic.Related(), wantRelated)
					}
				}
			})
		}
	}
}
