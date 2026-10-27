package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const namedExtensionChoice = `<xs:choice minOccurs="0" maxOccurs="18446744073709551616"><xs:element ref="t:first" minOccurs="2" maxOccurs="3"/><xs:element ref="t:second" minOccurs="0" maxOccurs="0"/></xs:choice>`
const namedExtensionSequence = `<xs:sequence minOccurs="0" maxOccurs="18446744073709551616"><xs:group ref="t:Target"/><xs:element ref="t:first" minOccurs="2" maxOccurs="3"/></xs:sequence>`

func namedExtensionNestedSchema(version, nested, base, extra string) string {
	if base == "" {
		base = `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:named-nested" targetNamespace="urn:named-nested" version="` + version + `">
<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence minOccurs="0" maxOccurs="3"><xs:element name="local" type="xs:integer"/>` + nested + `<xs:element ref="t:last"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
<xs:element name="root" type="t:Derived"/><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:integer"/><xs:element name="last" type="xs:integer"/>
<xs:group name="Target"><xs:sequence><xs:element ref="t:second"/></xs:sequence></xs:group>` + base + extra + `</xs:schema>`
}

func namedExtensionNestedOwner(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:named-nested", "Derived"))
	if len(components) != 1 {
		t.Fatalf("Derived components = %d", len(components))
	}
	owner, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("Derived is not a complex type")
	}
	return owner
}

func TestNamedExtensionNestedPublicFacts(t *testing.T) { //nolint:gocognit,funlen // One public contract checks both alternatives, exact bounds, identities, locations and copies.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []struct{ name, xml string }{{"choice", namedExtensionChoice}, {"sequence", namedExtensionSequence}} {
			t.Run(profile.name+"/"+alternative.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, alternative.xml, "", "")
				var firstWalk []ComponentID
				for repeat := 0; repeat < 3; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					var walk []ComponentID
					if err := schema.Walk(func(component Component) error { walk = append(walk, component.ID()); return nil }); err != nil {
						t.Fatal(err)
					}
					if repeat > 0 && !reflect.DeepEqual(firstWalk, walk) {
						t.Fatalf("nondeterministic walk = %v", walk)
					}
					firstWalk = walk
					owner := namedExtensionNestedOwner(t, schema)
					if owner.ID().IsZero() || owner.Name() != mustTestQName(t, "urn:named-nested", "Derived") || owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
						t.Fatalf("named owner provenance = %v", owner)
					}
					base := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:named-nested", "Base"))
					baseRef, ok := owner.BaseReference()
					baseID, hasID := baseRef.ComponentID()
					if !ok || !hasID || len(base) != 1 || baseID != base[0].ID() {
						t.Fatalf("base target = %v", baseRef)
					}
					wildcard, ok := owner.AnyAttribute()
					if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
						t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
					}
					outer, ok := owner.Particle().(SequenceParticle)
					if !ok || outer.Occurrences().String() != "0/3" || outer.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs="0" maxOccurs="3"`) || len(outer.Particles()) != 3 {
						t.Fatalf("outer = %T/%v", owner.Particle(), owner.Particle())
					}
					local, ok := outer.Particles()[0].(ElementParticle)
					if !ok || local.Name().Local() != "local" || local.Loc() != complexContentTestLoc(t, root, `<xs:element name="local"`) {
						t.Fatalf("surrounding direct local = %v", outer.Particles()[0])
					}
					last, ok := outer.Particles()[2].(ElementReferenceParticle)
					lastTarget := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:named-nested", "last"))
					if !ok || len(lastTarget) != 1 || last.TargetID() != lastTarget[0].ID() || last.Ref() != lastTarget[0].Name() || last.RefLoc() != complexContentTestLoc(t, root, `ref="t:last"`) {
						t.Fatalf("surrounding ref = %v", outer.Particles()[2])
					}
					var innerRef ElementReferenceParticle
					var innerRefOK bool
					switch alternative.name {
					case "choice":
						choice, choiceOK := outer.Particles()[1].(ChoiceParticle)
						if !choiceOK || choice.Loc() != complexContentTestLoc(t, root, `<xs:choice minOccurs`) || choice.Occurrences().String() != "0/18446744073709551616" || len(choice.Alternatives()) != 1 {
							t.Fatalf("nested choice = %v", outer.Particles()[1])
						}
						innerRef, innerRefOK = choice.Alternatives()[0].(ElementReferenceParticle)
						alternativeCopy := choice.Alternatives()
						alternativeCopy[0] = nil
						maximum, finite := choice.Occurrences().Maximum().Finite()
						if !finite || maximum.Canonical() != "18446744073709551616" || choice.Alternatives()[0] == nil {
							t.Fatal("choice bounds or alternative copy changed")
						}
						maximum.value.SetInt64(0)
						if choice.Occurrences().Maximum().String() != "18446744073709551616" {
							t.Fatal("choice maximum was mutable")
						}
					case "sequence":
						sequence, sequenceOK := outer.Particles()[1].(SequenceParticle)
						if !sequenceOK || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs="0" maxOccurs="18446744073709551616"`) || sequence.Occurrences().String() != "0/18446744073709551616" || len(sequence.Particles()) != 2 {
							t.Fatalf("nested sequence = %v", outer.Particles()[1])
						}
						group, groupOK := sequence.Particles()[0].(ModelGroupReferenceParticle)
						groupTarget := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:named-nested", "Target"))
						if !groupOK || len(groupTarget) != 1 || group.TargetID() != groupTarget[0].ID() || group.Ref() != groupTarget[0].Name() || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Target"`) || group.Occurrences().String() != "1/1" {
							t.Fatalf("nested group = %v", sequence.Particles()[0])
						}
						innerRef, innerRefOK = sequence.Particles()[1].(ElementReferenceParticle)
						members := sequence.Particles()
						members[0] = nil
						if sequence.Particles()[0] == nil {
							t.Fatal("nested sequence slice was mutable")
						}
					}
					firstTarget := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:named-nested", "first"))
					if !innerRefOK || len(firstTarget) != 1 || innerRef.TargetID() != firstTarget[0].ID() || innerRef.Ref() != firstTarget[0].Name() || innerRef.RefLoc() != complexContentTestLoc(t, root, `ref="t:first" minOccurs="2"`) || innerRef.Occurrences().String() != "2/3" {
						t.Fatalf("nested element ref = %v", innerRef)
					}
					terms := outer.Particles()
					terms[1] = nil
					if outer.Particles()[1] == nil {
						t.Fatal("outer slice was mutable")
					}
				}
			})
		}
	}
}

func TestNamedExtensionNestedConsumers(t *testing.T) { //nolint:gocognit // Assert separate consumer locations and causes for both alternatives.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []struct{ name, xml string }{{"choice", namedExtensionChoice}, {"sequence", namedExtensionSequence}} {
			t.Run(profile.name+"/"+alternative.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, alternative.xml, "", "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:named-nested"/>`)))
				validation := requireDiagnostic(t, validationErr)
				wantValidationRelated := []Loc{
					complexContentTestLoc(t, root, `<xs:element name="root"`),
					complexContentTestLoc(t, root, `<xs:complexType name="Derived"`),
					complexContentTestLoc(t, root, `<xs:sequence minOccurs="0" maxOccurs="3"`),
					complexContentTestLoc(t, root, `<xs:complexContent>`),
					complexContentTestLoc(t, root, `<xs:extension`),
					complexContentTestLoc(t, root, `base="t:Base"`),
					complexContentTestLoc(t, root, `<xs:anyAttribute`),
				}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || validation.SpecRef() != instanceValidationSpecRef(profile.factVersion) || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v; related=%v want=%v; spec=%q", validationErr, validation.Related(), wantValidationRelated, validation.SpecRef())
				}
				output, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				wantGenerationRelated := []Loc{wantValidationRelated[3], wantValidationRelated[4], wantValidationRelated[5], wantValidationRelated[2], wantValidationRelated[6]}
				if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantValidationRelated[4] || generation.SpecRef() != codegenDirectSequenceSpecReference(profile.factVersion, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v; related=%v want=%v; spec=%q", output, generationErr, generation.Related(), wantGenerationRelated, generation.SpecRef())
				}
			})
		}
	}
}

func assertNamedExtensionNestedFailure(t *testing.T, root string, policy LanguagePolicy, marker string, class FailureClass, code, spec string, cause error, related ...string) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.SpecRef() != spec || diagnostic.Loc() != complexContentTestLoc(t, root, marker) {
		t.Fatalf("diagnostic = %s/%s/%q at %s, want %s/%s/%q at %s", diagnostic.Class(), diagnostic.Code(), diagnostic.SpecRef(), diagnostic.Loc(), class, code, spec, complexContentTestLoc(t, root, marker))
	}
	if class == FailureUnsupported && diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("unsupported feature = %q, want %q", diagnostic.Feature(), FeatureSchemaSyntax)
	}
	var wantRelated []Loc
	if len(related) > 0 {
		wantRelated = make([]Loc, 0, len(related))
	}
	for _, location := range related {
		wantRelated = append(wantRelated, complexContentTestLoc(t, root, location))
	}
	if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("related = %v, want %v", diagnostic.Related(), wantRelated)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func TestNamedExtensionNestedOmissionAndPrecedence(t *testing.T) { //nolint:gocognit // Assert source and occurrence precedence at the public diagnostic boundary.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []struct{ name, xml string }{{"choice", namedExtensionChoice}, {"sequence", namedExtensionSequence}} {
			t.Run(profile.name+"/"+alternative.name, func(t *testing.T) {
				zero := strings.Replace(alternative.xml, `maxOccurs="18446744073709551616"`, `maxOccurs="0"`, 1)
				root := namedExtensionNestedSchema(profile.version, zero, "", "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := namedExtensionNestedOwner(t, schema).Particle().(SequenceParticle)
				if !ok || len(outer.Particles()) != 2 {
					t.Fatalf("zero nested particle survived: %v", namedExtensionNestedOwner(t, schema).Particle())
				}
				malformed := strings.Replace(zero, `minOccurs="0"`, `minOccurs="bad"`, 1)
				root = namedExtensionNestedSchema(profile.version, malformed, "", "")
				assertNamedExtensionNestedFailure(t, root, profile.policy, `minOccurs="bad"`, FailureInvalid, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.factVersion), nil)
				_, malformedErr := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var lexical Diagnostic
				if !errors.As(requireDiagnostic(t, malformedErr).Unwrap(), &lexical) || lexical.Code() != InvalidIntegerLexicalCode || lexical.Loc() != complexContentTestLoc(t, root, `minOccurs="bad"`) {
					t.Fatalf("malformed occurrence lost lexical cause: %v", malformedErr)
				}
				reversed := strings.Replace(zero, `minOccurs="0" maxOccurs="0"`, `minOccurs="2" maxOccurs="1"`, 1)
				root = namedExtensionNestedSchema(profile.version, reversed, "", "")
				assertNamedExtensionNestedFailure(t, root, profile.policy, `<xs:`+alternative.name+` minOccurs="2" maxOccurs="1"`, FailureInvalid, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.factVersion), errParticleOccurrenceMinimumExceedsMaximum, `minOccurs="2"`, `maxOccurs="1"`)
				unresolved := strings.Replace(zero, `ref="t:first"`, `ref="t:Missing"`, 1)
				root = namedExtensionNestedSchema(profile.version, unresolved, "", "")
				assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:Missing"`, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceUnresolved)
			})
		}
	}
}

func TestNamedExtensionNestedUPAAndConsistency(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		t.Run(profile.name+"/required repeated name", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, `<xs:choice><xs:element ref="t:first"/></xs:choice>`, "", "")
			root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, `<xs:element ref="t:first"/>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || namedExtensionNestedOwner(t, schema).Particle() == nil {
				t.Fatalf("required adjacent same-name particles are deterministic: %v", err)
			}
		})
		t.Run(profile.name+"/optional repeated name", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, `<xs:choice><xs:element ref="t:first"/></xs:choice>`, "", "")
			root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, `<xs:element ref="t:first" minOccurs="0"/>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:first"/></xs:choice>`, FailureInvalid, diagnosticSchemaNamedExtensionUPACode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), errSchemaNamedExtensionUPA, `ref="t:first" minOccurs="0"`)
		})
		t.Run(profile.name+"/substitution attribution", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, `<xs:choice><xs:element ref="t:member"/></xs:choice>`, "", `<xs:element name="member" type="xs:integer" substitutionGroup="t:first"/>`)
			root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, `<xs:element ref="t:first" minOccurs="0"/>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:member"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), ErrUnsupported, `ref="t:first" minOccurs="0"`)
		})
		t.Run(profile.name+"/UPA", func(t *testing.T) {
			nested := `<xs:sequence><xs:group ref="t:Target"/><xs:element ref="t:second"/></xs:sequence>`
			root := namedExtensionNestedSchema(profile.version, nested, "", "")
			root = strings.Replace(root, `<xs:element ref="t:second"/></xs:sequence></xs:group>`, `<xs:element ref="t:second" minOccurs="0"/></xs:sequence></xs:group>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:second"/></xs:sequence>`, FailureInvalid, diagnosticSchemaNamedExtensionUPACode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), errSchemaNamedExtensionUPA, `ref="t:Target"`)
		})
		t.Run(profile.name+"/element consistency", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, namedExtensionChoice, "", "")
			root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, `<xs:element name="first" form="qualified" type="xs:string"/>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:first" minOccurs="2"`, FailureInvalid, diagnosticSchemaGroupElementConsistencyCode, schemaGroupElementConsistencySpecRef(profile.factVersion), errSchemaGroupElementConsistency, `<xs:element name="first" form="qualified"`)
		})
		t.Run(profile.name+"/unproved anonymous consistency", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, namedExtensionChoice, "", "")
			root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, `<xs:element name="first" form="qualified"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:first" minOccurs="2"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaGroupElementConsistencySpecRef(profile.factVersion), ErrUnsupported, `<xs:element name="first" form="qualified"`)
		})
		t.Run(profile.name+"/group local consistency", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, namedExtensionSequence, "", "")
			root = strings.Replace(root, `<xs:element ref="t:second"/></xs:sequence></xs:group>`, `<xs:element name="first" form="qualified" type="xs:string"/></xs:sequence></xs:group>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:first" minOccurs="2"`, FailureInvalid, diagnosticSchemaGroupElementConsistencyCode, schemaGroupElementConsistencySpecRef(profile.factVersion), errSchemaGroupElementConsistency, `ref="t:Target"`)
		})
	}
}

func TestNamedExtensionNestedFixedCountAttribution(t *testing.T) { //nolint:gocognit // Both admitted compositors retain exact counts across all policies.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []struct{ name, body string }{
			{"choice", `<xs:choice minOccurs="2" maxOccurs="2"><xs:element ref="t:first"/></xs:choice>`},
			{"sequence", `<xs:sequence minOccurs="2" maxOccurs="2"><xs:group ref="t:Target"/><xs:element ref="t:first"/></xs:sequence>`},
		} {
			t.Run(profile.name+"/"+alternative.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, alternative.body, "", "")
				root = strings.Replace(root, `<xs:element ref="t:last"/></xs:sequence>`, `<xs:element ref="t:first"/></xs:sequence>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := namedExtensionNestedOwner(t, schema).Particle().(SequenceParticle)
				if !ok || len(outer.Particles()) != 3 || outer.Particles()[1].Occurrences().String() != "2/2" || outer.Particles()[1].Loc() != complexContentTestLoc(t, root, `<xs:`+alternative.name+` minOccurs="2"`) {
					t.Fatalf("fixed repetition facts = %v", namedExtensionNestedOwner(t, schema).Particle())
				}
				last, ok := outer.Particles()[2].(ElementReferenceParticle)
				first := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:named-nested", "first"))
				if !ok || len(first) != 1 || last.TargetID() != first[0].ID() || last.RefLoc() != complexContentTestLoc(t, root, `ref="t:first"/></xs:sequence></xs:extension>`) {
					t.Fatalf("fixed repetition following ref = %v", outer.Particles()[2])
				}
			})
		}
		root := namedExtensionNestedSchema(profile.version, `<xs:choice minOccurs="1" maxOccurs="2"><xs:element ref="t:first"/></xs:choice>`, "", "")
		root = strings.Replace(root, `<xs:element ref="t:last"/></xs:sequence>`, `<xs:element ref="t:first"/></xs:sequence>`, 1)
		assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:first"/></xs:sequence></xs:extension>`, FailureInvalid, diagnosticSchemaNamedExtensionUPACode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), errSchemaNamedExtensionUPA, `ref="t:first"/></xs:choice>`)
		huge := namedExtensionNestedSchema(profile.version, `<xs:choice minOccurs="18446744073709551616" maxOccurs="18446744073709551616"><xs:element ref="t:first"/></xs:choice>`, "", "")
		huge = strings.Replace(huge, `<xs:element ref="t:last"/></xs:sequence>`, `<xs:element ref="t:first"/></xs:sequence>`, 1)
		assertNamedExtensionNestedFailure(t, huge, profile.policy, `ref="t:first"/></xs:sequence></xs:extension>`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), ErrUnsupported, `ref="t:first"/></xs:choice>`)
	}
}

func TestNamedExtensionNestedFixedLeafCountAttribution(t *testing.T) { //nolint:gocognit // Child particle ranges need the same count-sensitive proof as compositors.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []struct{ name, body string }{
			{"choice", `<xs:choice><xs:element ref="t:first" minOccurs="2" maxOccurs="2"/></xs:choice>`},
			{"sequence", `<xs:sequence><xs:group ref="t:Target"/><xs:element ref="t:first" minOccurs="2" maxOccurs="2"/></xs:sequence>`},
		} {
			t.Run(profile.name+"/"+alternative.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, alternative.body, "", "")
				root = strings.Replace(root, `<xs:element ref="t:last"/></xs:sequence>`, `<xs:element ref="t:first"/></xs:sequence>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := namedExtensionNestedOwner(t, schema).Particle().(SequenceParticle)
				if !ok || len(outer.Particles()) != 3 {
					t.Fatalf("outer = %v", namedExtensionNestedOwner(t, schema).Particle())
				}
				var inner ElementReferenceParticle
				if alternative.name == "choice" {
					choice, choiceOK := outer.Particles()[1].(ChoiceParticle)
					if !choiceOK || len(choice.Alternatives()) != 1 {
						t.Fatalf("nested choice = %v", outer.Particles()[1])
					}
					inner, ok = choice.Alternatives()[0].(ElementReferenceParticle)
				}
				if alternative.name == "sequence" {
					sequence, sequenceOK := outer.Particles()[1].(SequenceParticle)
					if !sequenceOK || len(sequence.Particles()) != 2 {
						t.Fatalf("nested sequence = %v", outer.Particles()[1])
					}
					inner, ok = sequence.Particles()[1].(ElementReferenceParticle)
				}
				if !ok || inner.Occurrences().String() != "2/2" || inner.RefLoc() != complexContentTestLoc(t, root, `ref="t:first" minOccurs="2"`) {
					t.Fatalf("fixed child facts = %v", inner)
				}
				variable := strings.Replace(root, `minOccurs="2" maxOccurs="2"`, `minOccurs="1" maxOccurs="2"`, 1)
				assertNamedExtensionNestedFailure(t, variable, profile.policy, `ref="t:first"/></xs:sequence></xs:extension>`, FailureInvalid, diagnosticSchemaNamedExtensionUPACode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), errSchemaNamedExtensionUPA, `ref="t:first" minOccurs="1" maxOccurs="2"`)
			})
		}
	}
}

func TestNamedExtensionNestedSharedSubstitutionAttribution(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct{ name, extra string }{
			{"shared", `<xs:element name="shared" type="xs:integer" substitutionGroup="t:first t:second"/>`},
			{"transitive", `<xs:element name="middle" type="xs:integer" substitutionGroup="t:first"/><xs:element name="shared" type="xs:integer" substitutionGroup="t:middle t:second"/>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, `<xs:choice><xs:element ref="t:first"/><xs:element ref="t:second"/></xs:choice>`, "", test.extra)
				if profile.policy == Strict10 {
					marker := `substitutionGroup="t:first t:second"`
					if test.name == "transitive" {
						marker = `substitutionGroup="t:middle t:second"`
					}
					assertNamedExtensionNestedFailure(t, root, profile.policy, marker, FailureInvalid, invalidSchemaCompositionCode, schemaSubstitutionAffiliationXSD10SpecRef, errSchemaSubstitutionCardinality)
					return
				}
				assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:second"/></xs:choice>`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaGroupNonAmbiguousSpecRef(profile.factVersion), ErrUnsupported, `ref="t:first"/><xs:element`)
			})
		}
	}
}

func TestNamedExtensionNestedExcludedShapes(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, nested, marker string
		}{
			{"choice group ref", `<xs:choice><xs:group ref="t:Target"/></xs:choice>`, `ref="t:Target"`},
			{"choice typed local", `<xs:choice><xs:element name="inner" type="xs:integer"/></xs:choice>`, `<xs:element name="inner"`},
			{"choice untyped local", `<xs:choice><xs:element name="inner"/></xs:choice>`, `<xs:element name="inner"`},
			{"choice inline local", `<xs:choice><xs:element name="inner"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:choice>`, `<xs:element name="inner"`},
			{"sequence typed local", `<xs:sequence><xs:group ref="t:Target"/><xs:element name="inner" type="xs:integer"/></xs:sequence>`, `<xs:element name="inner"`},
			{"sequence untyped local", `<xs:sequence><xs:group ref="t:Target"/><xs:element name="inner"/></xs:sequence>`, `<xs:element name="inner"`},
			{"sequence inline local", `<xs:sequence><xs:group ref="t:Target"/><xs:element name="inner"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:sequence>`, `<xs:element name="inner"`},
			{"second choice", namedExtensionChoice + `<xs:choice minOccurs="0" maxOccurs="2"><xs:element ref="t:second"/></xs:choice>`, `<xs:choice minOccurs="0" maxOccurs="2"`},
			{"combined alternatives", namedExtensionChoice + namedExtensionSequence, `<xs:sequence minOccurs="0" maxOccurs="18446744073709551616"`},
			{"deeper choice", `<xs:choice><xs:choice><xs:element ref="t:first"/></xs:choice></xs:choice>`, `<xs:choice><xs:element ref="t:first"`},
			{"nested wildcard", `<xs:choice><xs:any/></xs:choice>`, `<xs:any/>`},
			{"direct group", namedExtensionChoice + `<xs:group ref="t:Target"/>`, `ref="t:Target"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, test.nested, "", "")
				marker := test.marker
				switch test.name {
				case "choice group ref", "choice typed local", "choice untyped local", "choice inline local", "deeper choice", "nested wildcard":
					marker = `<xs:choice>`
				case "sequence typed local", "sequence untyped local", "sequence inline local":
					marker = `<xs:sequence>`
				}
				assertNamedExtensionNestedFailure(t, root, profile.policy, marker, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), ErrUnsupported)
			})
		}
		t.Run(profile.name+"/all group", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, `<xs:all/>`, "", "")
			assertNamedExtensionNestedFailure(t, root, profile.policy, `<xs:all/>`, FailureInvalid, invalidSchemaCompositionCode, "", nil)
		})
	}
}

func TestNamedExtensionNestedBaseAndGroupDependencies(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		zero := strings.Replace(namedExtensionSequence, `maxOccurs="18446744073709551616"`, `maxOccurs="0"`, 1)
		t.Run(profile.name+"/nonempty base", func(t *testing.T) {
			base := `<xs:complexType name="Base"><xs:sequence><xs:element name="baseItem" type="xs:integer"/></xs:sequence></xs:complexType>`
			root := namedExtensionNestedSchema(profile.version, zero, base, "")
			assertNamedExtensionNestedFailure(t, root, profile.policy, `base="t:Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(profile.factVersion), errSchemaComplexTypeBaseUnsupported, `<xs:complexType name="Base"`, `<xs:sequence><xs:element name="baseItem"`)
		})
		t.Run(profile.name+"/final", func(t *testing.T) {
			base := `<xs:complexType name="Base" final="extension"/>`
			root := namedExtensionNestedSchema(profile.version, zero, base, "")
			assertNamedExtensionNestedFailure(t, root, profile.policy, `base="t:Base"`, FailureInvalid, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.factVersion), errSchemaComplexTypeBaseUnsupported, `final="extension"`)
		})
		t.Run(profile.name+"/group cycle", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, zero, "", "")
			root = strings.Replace(root, `<xs:group name="Target"><xs:sequence><xs:element ref="t:second"/></xs:sequence></xs:group>`, `<xs:group name="Target"><xs:sequence><xs:group ref="t:Target"/></xs:sequence></xs:group>`, 1)
			wantSpec := "xsd11-structures#mg-props-correct"
			if profile.factVersion == XSDVersion10 {
				wantSpec = "xsd10-structures#mg-props-correct"
			}
			assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:Target"/></xs:sequence></xs:group>`, FailureInvalid, diagnosticSchemaModelGroupReferenceCycleCode, wantSpec, errSchemaModelGroupReferenceCycle, `<xs:group name="Target"`)
		})
	}
}

func TestNamedExtensionNestedGraphVisibility(t *testing.T) { //nolint:gocognit // Both alternatives share four source graphs and exact public provenance.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []string{"choice", "sequence"} {
			for _, graph := range []string{"forward", "include", "import", "chameleon"} {
				t.Run(profile.name+"/"+alternative+"/"+graph, func(t *testing.T) {
					refPrefix, targetNamespace, targetSource := "t:", "urn:named-nested", SourceID("root.xsd")
					if graph == "import" {
						refPrefix, targetNamespace, targetSource = "o:", "urn:other", "other.xsd"
					}
					if graph == "include" {
						targetSource = "child.xsd"
					}
					ownerSource := SourceID("root.xsd")
					if graph == "chameleon" {
						ownerSource = "child.xsd"
					}
					nested := `<xs:choice><xs:element ref="` + refPrefix + `remote"/></xs:choice>`
					if alternative == "sequence" {
						nested = `<xs:sequence><xs:group ref="` + refPrefix + `Target"/><xs:element ref="` + refPrefix + `remote"/></xs:sequence>`
					}
					owner := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + nested + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
					base := `<xs:complexType name="Base"/>`
					targets := `<xs:group name="Target"><xs:sequence/></xs:group><xs:element name="remote" type="xs:integer"/>`
					rootHeader := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:named-nested" xmlns:o="urn:other" targetNamespace="urn:named-nested" version="` + profile.version + `">`
					root := rootHeader + owner + base + targets + `</xs:schema>`
					var fixtures map[string]discoveryFixture
					ownerXML := root
					if graph == "include" {
						root = rootHeader + `<xs:include schemaLocation="child.xsd"/>` + owner + base + `</xs:schema>`
						fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-nested">` + targets + `</xs:schema>`}}
						ownerXML = root
					}
					if graph == "import" {
						root = rootHeader + `<xs:import namespace="urn:other" schemaLocation="other.xsd"/>` + owner + base + `</xs:schema>`
						fixtures = map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + targets + `</xs:schema>`}}
						ownerXML = root
					}
					if graph == "chameleon" {
						root = rootHeader + `<xs:include schemaLocation="child.xsd"/>` + base + targets + `</xs:schema>`
						ownerXML = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:named-nested">` + owner + `</xs:schema>`
						fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: ownerXML}}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					ownerView := namedExtensionNestedOwner(t, schema)
					outer, ok := ownerView.Particle().(SequenceParticle)
					if !ok || len(outer.Particles()) != 1 || ownerView.Loc().Source() != ownerSource {
						t.Fatalf("owner/outer = %v/%v", ownerView, ownerView.Particle())
					}
					var ref ElementReferenceParticle
					var refOK bool
					switch alternative {
					case "choice":
						choice, choiceOK := outer.Particles()[0].(ChoiceParticle)
						if !choiceOK || len(choice.Alternatives()) != 1 {
							t.Fatalf("choice = %v", outer.Particles()[0])
						}
						ref, refOK = choice.Alternatives()[0].(ElementReferenceParticle)
					case "sequence":
						sequence, sequenceOK := outer.Particles()[0].(SequenceParticle)
						if !sequenceOK || len(sequence.Particles()) != 2 {
							t.Fatalf("sequence = %v", outer.Particles()[0])
						}
						group, groupOK := sequence.Particles()[0].(ModelGroupReferenceParticle)
						groupTarget := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, targetNamespace, "Target"))
						if !groupOK || len(groupTarget) != 1 || group.TargetID() != groupTarget[0].ID() || group.RefLoc() != complexContentTestSourceLoc(t, ownerSource, ownerXML, `ref="`+refPrefix+`Target"`) {
							t.Fatalf("group target/provenance = %v", sequence.Particles()[0])
						}
						ref, refOK = sequence.Particles()[1].(ElementReferenceParticle)
					}
					target := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, targetNamespace, "remote"))
					if !refOK || len(target) != 1 || ref.TargetID() != target[0].ID() || ref.TargetID().Source() != targetSource || ref.Ref() != target[0].Name() || ref.RefLoc() != complexContentTestSourceLoc(t, ownerSource, ownerXML, `ref="`+refPrefix+`remote"`) {
						t.Fatalf("element target/provenance = %v", ref)
					}
				})
			}
		}
	}
}

func TestNamedExtensionNestedWrongKindReferences(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, nested, marker, related, code, spec string
			cause                                     error
		}{
			{"choice element", `<xs:choice><xs:element ref="t:Base"/></xs:choice>`, `ref="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceWrongKind},
			{"sequence group", `<xs:sequence><xs:group ref="t:first"/><xs:element ref="t:second"/></xs:sequence>`, `ref="t:first"`, `<xs:element name="first"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.factVersion), errSchemaModelGroupReferenceWrongKind},
			{"sequence element", `<xs:sequence><xs:group ref="t:Target"/><xs:element ref="t:Base"/></xs:sequence>`, `ref="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceWrongKind},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, test.nested, "", "")
				assertNamedExtensionNestedFailure(t, root, profile.policy, test.marker, FailureInvalid, test.code, test.spec, test.cause, test.related)
			})
		}
	}
}

func TestNamedExtensionNestedBaseCycleBeforeZero(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		root := namedExtensionNestedSchema(profile.version, `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="t:first"/></xs:choice>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"/></xs:complexContent></xs:complexType>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.factVersion) || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
			t.Fatalf("%s base cycle = %v", profile.name, err)
		}
		if diagnostic.Loc() != complexContentTestLoc(t, root, `base="t:Derived"`) || !reflect.DeepEqual(diagnostic.Related(), []Loc{complexContentTestLoc(t, root, `base="t:Base"`)}) {
			t.Fatalf("%s base cycle locations = %s/%v", profile.name, diagnostic.Loc(), diagnostic.Related())
		}
	}
}

func TestNamedExtensionNestedInvisibleReferences(t *testing.T) {
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="Target"><xs:sequence/></xs:group><xs:element name="remote" type="xs:integer"/></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:named-nested"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	for _, profile := range nestedGroupChoiceProfiles {
		for _, test := range []struct {
			name, nested, local, marker, related, code, spec string
			cause                                            error
		}{
			{"choice element", `<xs:choice minOccurs="0" maxOccurs="0"><xs:element ref="o:remote"/></xs:choice>`, "", `ref="o:remote"`, `<xs:element name="remote"`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
			{"sequence group", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="o:Target"/><xs:element ref="t:first"/></xs:sequence>`, "", `ref="o:Target"`, `<xs:group name="Target"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.factVersion), errSchemaModelGroupReferenceNamespace},
			{"sequence element", `<xs:sequence minOccurs="0" maxOccurs="0"><xs:group ref="t:Local"/><xs:element ref="o:remote"/></xs:sequence>`, `<xs:group name="Local"><xs:sequence/></xs:group>`, `ref="o:remote"`, `<xs:element name="remote"`, diagnosticSchemaElementReferenceNamespaceCode, schemaElementReferenceImportSpecRef(profile.factVersion), errSchemaElementReferenceNamespace},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:named-nested" xmlns:o="urn:other" targetNamespace="urn:named-nested" version="` + profile.version + `"><xs:include schemaLocation="child.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence>` + test.nested + `</xs:sequence></xs:extension></xs:complexContent></xs:complexType><xs:complexType name="Base"/><xs:element name="first" type="xs:integer"/>` + test.local + `</xs:schema>`
				fixtures := map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}, "other.xsd": {id: "other.xsd", contents: other}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := complexContentTestSourceLoc(t, "other.xsd", other, test.related)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.spec || diagnostic.Loc() != complexContentTestLoc(t, root, test.marker) || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) || !errors.Is(err, test.cause) {
					t.Fatalf("invisible reference = %v; related=%v want=%s", err, diagnostic.Related(), wantRelated)
				}
			})
		}
	}
}

func TestNamedExtensionNestedSurroundingDirectShapes(t *testing.T) { //nolint:gocognit // Test direct and omitted shapes beside the new nested term.
	for _, profile := range nestedGroupChoiceProfiles {
		for _, direct := range []struct {
			name, xml string
		}{
			{"inline simple local", `<xs:element name="local"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`},
			{"global ref", `<xs:element ref="t:second"/>`},
			{"omitted inline", `<xs:element name="local" minOccurs="0" maxOccurs="0"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`},
		} {
			t.Run(profile.name+"/"+direct.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, namedExtensionChoice, "", "")
				root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, direct.xml, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				outer, ok := namedExtensionNestedOwner(t, schema).Particle().(SequenceParticle)
				wantCount := 3
				if direct.name == "omitted inline" {
					wantCount = 2
				}
				if !ok || len(outer.Particles()) != wantCount {
					t.Fatalf("surrounding direct shape = %v", namedExtensionNestedOwner(t, schema).Particle())
				}
			})
		}
		t.Run(profile.name+"/untyped local", func(t *testing.T) {
			root := namedExtensionNestedSchema(profile.version, namedExtensionChoice, "", "")
			root = strings.Replace(root, `<xs:element name="local" type="xs:integer"/>`, `<xs:element name="local"/>`, 1)
			assertNamedExtensionNestedFailure(t, root, profile.policy, `<xs:element name="local"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), ErrUnsupported)
		})
	}
}

func TestNamedExtensionNestedExcludedUses(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, nested := range []struct{ name, xml, marker string }{
			{"choice", namedExtensionChoice, `<xs:choice minOccurs`},
			{"sequence", namedExtensionSequence, `<xs:sequence minOccurs="0" maxOccurs="18446744073709551616"`},
		} {
			for _, use := range []struct{ name, xml, extra string }{
				{"local attribute", `<xs:attribute name="flag" type="xs:boolean"/>`, ""},
				{"prohibited attribute", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`, ""},
				{"attribute group", `<xs:attributeGroup ref="t:Attrs"/>`, ""},
				{"attribute wildcard", `<xs:anyAttribute namespace="##other" processContents="lax"/>`, ""},
			} {
				t.Run(profile.name+"/"+nested.name+"/"+use.name, func(t *testing.T) {
					root := namedExtensionNestedSchema(profile.version, nested.xml, "", use.extra)
					root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence>`+use.xml+`</xs:extension>`, 1)
					if use.name == "attribute group" {
						assertNamedExtensionNestedFailure(t, root, profile.policy, `<xs:complexContent>`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", ErrUnsupported)
						return
					}
					assertNamedExtensionNestedFailure(t, root, profile.policy, nested.marker, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(profile.factVersion), ErrUnsupported)
				})
			}
		}
	}
}

func TestNamedExtensionNestedDeferredDependencyPrecedence(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		groupSpec := schemaModelGroupReferenceSpecRef(profile.factVersion)
		groupCycleSpec := "xsd11-structures#mg-props-correct"
		if profile.factVersion == XSDVersion10 {
			groupCycleSpec = "xsd10-structures#mg-props-correct"
		}
		for _, test := range []struct {
			name, nested, base, extra, marker, spec, code string
			class                                         FailureClass
			cause                                         error
			related                                       []string
		}{
			{"excluded choice group unresolved", `<xs:choice><xs:group ref="t:Missing"/></xs:choice>`, "", "", `ref="t:Missing"`, groupSpec, diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
			{"excluded choice group wrong kind", `<xs:choice><xs:group ref="t:first"/></xs:choice>`, "", "", `ref="t:first"`, groupSpec, diagnosticSchemaModelGroupReferenceWrongKindCode, FailureInvalid, errSchemaModelGroupReferenceWrongKind, []string{`<xs:element name="first"`}},
			{"excluded choice group omitted unresolved", `<xs:choice minOccurs="0" maxOccurs="0"><xs:group ref="t:Missing"/></xs:choice>`, "", "", `ref="t:Missing"`, groupSpec, diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
			{"excluded choice group cycle", `<xs:choice><xs:group ref="t:Target"/></xs:choice>`, "", "", `ref="t:Target"/></xs:sequence></xs:group>`, groupCycleSpec, diagnosticSchemaModelGroupReferenceCycleCode, FailureInvalid, errSchemaModelGroupReferenceCycle, []string{`<xs:group name="Target"`}},
			{"excluded choice local type unresolved", `<xs:choice><xs:element name="inner" type="t:Missing"/></xs:choice>`, "", "", `type="t:Missing"`, schemaElementTypeSpecRef(profile.factVersion), diagnosticSchemaElementTypeUnresolvedCode, FailureInvalid, errSchemaElementTypeUnresolved, nil},
			{"excluded choice inline base unresolved", `<xs:choice><xs:element name="inner"><xs:simpleType><xs:restriction base="t:Missing"/></xs:simpleType></xs:element></xs:choice>`, "", "", `base="t:Missing"`, schemaSimpleTypeSpecRef(profile.factVersion), diagnosticSchemaSimpleTypeUnresolvedCode, FailureInvalid, errSchemaSimpleTypeBaseUnresolved, nil},
			{"excluded choice attribute unresolved", namedExtensionChoice, "", "", `ref="t:Missing"`, schemaAttributeUseResolveSpecRef(profile.factVersion), diagnosticSchemaAttributeReferenceUnresolvedCode, FailureInvalid, errSchemaAttributeReferenceUnresolved, nil},
			{"excluded choice invalid base", `<xs:choice><xs:group ref="t:Target"/></xs:choice>`, `<xs:complexType name="Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:complexType>`, "", `base="t:Base"`, schemaComplexTypeExtensionSpecRef(profile.factVersion), UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, []string{`<xs:complexType name="Base"`, `<xs:sequence><xs:element name="item"`}},
			{"excluded choice base cycle", `<xs:choice><xs:group ref="t:Target"/></xs:choice>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"/></xs:complexContent></xs:complexType>`, "", `base="t:Derived"`, schemaComplexTypeExtensionSpecRef(profile.factVersion), invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseCycle, []string{`base="t:Base"`}},
			{"excluded extra nested final", `<xs:choice><xs:element ref="t:first"/></xs:choice><xs:sequence><xs:group ref="t:Target"/><xs:element ref="t:second"/></xs:sequence>`, `<xs:complexType name="Base" final="extension"/>`, "", `base="t:Base"`, schemaComplexTypeExtensionSpecRef(profile.factVersion), invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, []string{`final="extension"`}},
			{"excluded choice lexical first", `<xs:element ref="t:FirstMissing"/><xs:choice><xs:group ref="t:SecondMissing"/></xs:choice>`, "", "", `ref="t:FirstMissing"`, schemaElementReferenceSpecRef(profile.factVersion), diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, errSchemaElementReferenceUnresolved, nil},
			{"excluded choice lexical second", `<xs:choice><xs:group ref="t:FirstMissing"/></xs:choice><xs:element ref="t:SecondMissing"/>`, "", "", `ref="t:FirstMissing"`, groupSpec, diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedExtensionNestedSchema(profile.version, test.nested, test.base, test.extra)
				if test.name == "excluded choice group cycle" {
					root = strings.Replace(root, `<xs:group name="Target"><xs:sequence><xs:element ref="t:second"/></xs:sequence></xs:group>`, `<xs:group name="Target"><xs:sequence><xs:group ref="t:Target"/></xs:sequence></xs:group>`, 1)
				}
				if test.name == "excluded choice attribute unresolved" {
					root = strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence><xs:attribute ref="t:Missing"/></xs:extension>`, 1)
				}
				assertNamedExtensionNestedFailure(t, root, profile.policy, test.marker, test.class, test.code, test.spec, test.cause, test.related...)
			})
		}
	}
}

func TestNamedExtensionNestedOwnWildcardDependencyPrecedence(t *testing.T) {
	for _, profile := range nestedGroupChoiceProfiles {
		for _, alternative := range []struct{ name, nested string }{
			{"choice", namedExtensionChoice},
			{"sequence", namedExtensionSequence},
		} {
			wildcardRoot := func(base string) string {
				root := namedExtensionNestedSchema(profile.version, alternative.nested, base, "")
				return strings.Replace(root, `</xs:sequence></xs:extension>`, `</xs:sequence><xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension>`, 1)
			}
			t.Run(profile.name+"/"+alternative.name+"/missing base", func(t *testing.T) {
				root := strings.Replace(wildcardRoot(""), `base="t:Base"`, `base="t:Missing"`, 1)
				assertNamedExtensionNestedFailure(t, root, profile.policy, `base="t:Missing"`, FailureInvalid, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.factVersion), errSchemaComplexTypeBaseUnresolved)
			})
			t.Run(profile.name+"/"+alternative.name+"/final base", func(t *testing.T) {
				root := wildcardRoot(`<xs:complexType name="Base" final="extension"/>`)
				assertNamedExtensionNestedFailure(t, root, profile.policy, `base="t:Base"`, FailureInvalid, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.factVersion), errSchemaComplexTypeBaseUnsupported, `final="extension"`)
			})
			t.Run(profile.name+"/"+alternative.name+"/base cycle", func(t *testing.T) {
				root := wildcardRoot(`<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"/></xs:complexContent></xs:complexType>`)
				assertNamedExtensionNestedFailure(t, root, profile.policy, `base="t:Derived"`, FailureInvalid, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.factVersion), errSchemaComplexTypeBaseCycle, `base="t:Base"`)
			})
			t.Run(profile.name+"/"+alternative.name+"/invalid nested ref", func(t *testing.T) {
				root := strings.Replace(wildcardRoot(""), `ref="t:first"`, `ref="t:Missing"`, 1)
				assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:Missing"`, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceUnresolved)
			})
			t.Run(profile.name+"/"+alternative.name+"/omitted invalid nested ref", func(t *testing.T) {
				root := strings.Replace(wildcardRoot(""), `maxOccurs="18446744073709551616"`, `maxOccurs="0"`, 1)
				root = strings.Replace(root, `ref="t:first"`, `ref="t:Missing"`, 1)
				assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:Missing"`, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.factVersion), errSchemaElementReferenceUnresolved)
			})
			if alternative.name == "sequence" {
				t.Run(profile.name+"/sequence/group cycle", func(t *testing.T) {
					root := strings.Replace(wildcardRoot(""), `<xs:group name="Target"><xs:sequence><xs:element ref="t:second"/></xs:sequence></xs:group>`, `<xs:group name="Target"><xs:sequence><xs:group ref="t:Target"/></xs:sequence></xs:group>`, 1)
					groupSpec := "xsd11-structures#mg-props-correct"
					if profile.factVersion == XSDVersion10 {
						groupSpec = "xsd10-structures#mg-props-correct"
					}
					assertNamedExtensionNestedFailure(t, root, profile.policy, `ref="t:Target"/></xs:sequence></xs:group>`, FailureInvalid, diagnosticSchemaModelGroupReferenceCycleCode, groupSpec, errSchemaModelGroupReferenceCycle, `<xs:group name="Target"`)
				})
			}
		}
	}
}
