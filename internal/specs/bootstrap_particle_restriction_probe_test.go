package specs

import (
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

func pinnedAnnotatedSequence(t *testing.T, edition restrictionProbeEdition) string {
	t.Helper()
	id := "xsd11-schema-for-schemas"
	if edition.policy == goxsd9.Strict10 {
		id = "xsd10-schema-for-schemas"
	}
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapRawDigestStage(t, entry, raw)
	converted, err := convert(entry, raw)
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapManifestConversionStage(t, entry, raw)
	if err := validateBootstrapXML(entry, converted); err != nil {
		t.Fatal(err)
	}
	source := string(converted)
	annotated := strings.Index(source, `<xs:complexType name="annotated">`)
	if annotated < 0 {
		t.Fatal("pinned annotated declaration is missing")
	}
	end := strings.Index(source[annotated:], `</xs:complexType>`)
	if end < 0 {
		t.Fatal("pinned annotated declaration is incomplete")
	}
	declaration := source[annotated : annotated+end]
	start := strings.Index(declaration, `<xs:sequence>`)
	end = strings.Index(declaration, `</xs:sequence>`)
	if start < 0 || end < 0 {
		t.Fatal("pinned annotated sequence is missing")
	}
	sequence := declaration[start : end+len(`</xs:sequence>`)]
	if strings.Count(sequence, `<xs:element ref="xs:annotation" minOccurs="0"/>`) != 1 || strings.Count(sequence, `<xs:element`) != 1 {
		t.Fatalf("pinned annotated sequence changed: %q", sequence)
	}
	return sequence
}

func pinnedParticleRestrictionSource(t *testing.T, edition restrictionProbeEdition) string {
	t.Helper()
	fragments := pinnedAuthoredSimpleTypeFragments(t, edition)
	group := simpleDerivationPinnedGroup(t, edition)
	if fragments.group != group {
		t.Fatal("independent copied group projections differ")
	}
	base := restrictionProbeRemove(t, fragments.base, `<xs:attribute name="final"`, `/>`)
	base = restrictionProbeRemove(t, base, `<xs:attribute name="name"`, `</xs:attribute>`)
	derived := restrictionProbeRemove(t, fragments.derived, `<xs:attribute name="name"`, `</xs:attribute>`)
	derived = restrictionProbeRemove(t, derived, `<xs:anyAttribute`, `/>`)
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema" version="` + string(edition.policyVersion()) + `">` + "\n" +
		`<xs:element name="annotation" type="xs:string"/>` + "\n" +
		`<xs:element name="restriction" type="xs:string"/>` + "\n" +
		`<xs:element name="list" type="xs:string"/>` + "\n" +
		`<xs:element name="union" type="xs:string"/>` + "\n" +
		`<xs:complexType name="annotated">` + pinnedAnnotatedSequence(t, edition) + `</xs:complexType>` + "\n" +
		group + "\n" + base + "\n" + derived + "\n</xs:schema>"
}

func (edition restrictionProbeEdition) policyVersion() goxsd9.XSDVersion {
	if edition.policy == goxsd9.Strict10 {
		return goxsd9.XSDVersion10
	}
	return goxsd9.XSDVersion11
}

// The copied particles are authored bytes; declarations needed to resolve them
// are explicitly synthetic global targets with no inherited uses or wildcards.
func TestPinnedParticleRestrictionSyntheticQuery(t *testing.T) { //nolint:gocognit // Keep provenance and complete public copied facts together.
	for _, edition := range restrictionProbeEditions {
		for _, policy := range []goxsd9.LanguagePolicy{edition.policy, goxsd9.Compatibility} {
			t.Run(edition.id+"/"+string(policy), func(t *testing.T) {
				source := pinnedParticleRestrictionSource(t, edition)
				id := edition.id + "-synthetic-particle-" + string(policy)
				var first []goxsd9.Component
				for attempt := range 2 {
					schema, err := inlineProbeParse(t, id, source, policy)
					if err != nil {
						t.Fatalf("attempt %d: %v", attempt, err)
					}
					components := schema.Components()
					if len(components) != 8 || len(schema.Documents()) != 1 {
						t.Fatalf("components/documents = %d/%d", len(components), len(schema.Documents()))
					}
					annotated, annotatedOK := components[4].ComplexType()
					groupTarget, groupTargetOK := components[5].ModelGroup()
					baseType, baseTypeOK := components[6].ComplexType()
					annotationSequence, annotationSequenceOK := annotated.Particle().(goxsd9.SequenceParticle)
					baseGroup, baseGroupOK := baseType.Particle().(goxsd9.ModelGroupReferenceParticle)
					choice, choiceOK := groupTarget.Particle().(goxsd9.ChoiceParticle)
					if !annotatedOK || !groupTargetOK || !baseTypeOK || !annotationSequenceOK || !baseGroupOK || !choiceOK ||
						annotated.Name().Local() != "annotated" || baseType.Name().Local() != "simpleType" || !baseType.IsAbstract() ||
						len(annotated.AttributeUses()) != 0 || len(baseType.AttributeUses()) != 0 ||
						annotationSequence.Loc() != inlineProbeLoc(t, id, source, `<xs:sequence>`) ||
						len(annotationSequence.Particles()) != 1 || baseGroup.TargetID() != components[5].ID() ||
						baseGroup.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:simpleDerivation"`) ||
						!baseGroup.Occurrences().IsDefault() || choice.Loc() != inlineProbeLoc(t, id, source, `<xs:choice>`) ||
						len(choice.Alternatives()) != 3 {
						t.Fatalf("copied completed base/group facts = %v/%v/%v", annotated, baseType, groupTarget)
					}
					annotationBaseRef, annotationBaseOK := annotationSequence.Particles()[0].(goxsd9.ElementReferenceParticle)
					if !annotationBaseOK || annotationBaseRef.TargetID() != components[0].ID() || annotationBaseRef.Occurrences().String() != "0/1" ||
						annotationBaseRef.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:annotation" minOccurs="0"`) {
						t.Fatalf("copied optional base ref = %v", annotationSequence.Particles())
					}
					for index, name := range simpleDerivationStubNames {
						alternative, ok := choice.Alternatives()[index].(goxsd9.ElementReferenceParticle)
						if !ok || alternative.TargetID() != components[index+1].ID() || alternative.Ref().Local() != name ||
							alternative.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:`+name+`"`) || !alternative.Occurrences().IsDefault() {
							t.Fatalf("copied group alternative %d = %v", index, choice.Alternatives()[index])
						}
					}
					derived, ok := components[7].ComplexType()
					if !ok || derived.Name().Local() != "topLevelSimpleType" || derived.Derivation() != goxsd9.ComplexTypeDerivationRestriction || len(derived.AttributeUses()) != 0 || derived.BaseLoc() != inlineProbeLoc(t, id, source, `base="xs:simpleType"`) {
						t.Fatalf("copied derived facts = %v", derived)
					}
					baseRef, hasBase := derived.BaseReference()
					baseID, hasID := baseRef.ComponentID()
					if !hasBase || !hasID || baseID != components[6].ID() || baseRef.Name() != components[6].Name() {
						t.Fatalf("copied base reference = %v", baseRef)
					}
					sequence, ok := derived.Particle().(goxsd9.SequenceParticle)
					if !ok || sequence.Loc() != authoredSimpleTypeLoc(t, id, source, `<xs:sequence>`) || len(sequence.Particles()) != 2 {
						t.Fatalf("copied derived particle = %T", derived.Particle())
					}
					annotation, elementOK := sequence.Particles()[0].(goxsd9.ElementReferenceParticle)
					group, groupOK := sequence.Particles()[1].(goxsd9.ModelGroupReferenceParticle)
					if !elementOK || !groupOK || annotation.TargetID() != components[0].ID() || group.TargetID() != components[5].ID() || annotation.Occurrences().String() != "0/1" || !group.Occurrences().IsDefault() || annotation.RefLoc() != authoredSimpleTypeLoc(t, id, source, `ref="xs:annotation" minOccurs="0"`) || group.RefLoc() != authoredSimpleTypeLoc(t, id, source, `ref="xs:simpleDerivation"`) {
						t.Fatalf("copied refs = %v", sequence.Particles())
					}
					copiedMembers := sequence.Particles()
					copiedMembers[0] = nil
					copiedAlternatives := choice.Alternatives()
					copiedAlternatives[0] = nil
					if sequence.Particles()[0] == nil || choice.Alternatives()[0] == nil {
						t.Fatal("copied particle views expose mutable slices")
					}
					if attempt == 0 {
						first = components
						continue
					}
					if !reflect.DeepEqual(first, components) {
						t.Fatal("repeated copied components changed")
					}
				}
			})
		}
	}
}
