package specs

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

type namedRestrictionEdition struct {
	id, digest, version string
	start, end          int
	policy              goxsd9.LanguagePolicy
}

var namedRestrictionEditions = []namedRestrictionEdition{
	{"xsd10-schema-for-schemas", "710bb5f4bdaf945f84a7041d670b3c0e47bcf6d392fe61aff6ecf33a9b51ac88", "1.0", 101, 114, goxsd9.Strict10},
	{"xsd11-schema-for-schemas", "047e652e9863065d297651c103e366daa24fb2c0933cebcf1982272cb80600f5", "1.1", 95, 108, goxsd9.Strict11},
}

func pinnedOpenAttrsFragment(t *testing.T, edition namedRestrictionEdition) string {
	t.Helper()
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find(edition.id)
	if err != nil {
		t.Fatal(err)
	}
	if entry.SHA256 != edition.digest || entry.Representation != manifestXMLRepresentation {
		t.Fatalf("manifest provenance = %s/%s", entry.SHA256, entry.Representation)
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
	if !bytes.Equal(raw, converted) {
		t.Fatal("XML identity conversion changed source bytes")
	}
	if err := validateBootstrapXML(entry, converted); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(converted), "\n")
	if edition.end > len(lines) {
		t.Fatalf("source has %d lines, want %d", len(lines), edition.end)
	}
	fragment := strings.Join(lines[edition.start-1:edition.end], "\n")
	if strings.TrimSpace(lines[edition.start-1]) != `<xs:complexType name="openAttrs">` ||
		strings.TrimSpace(lines[edition.end-1]) != `</xs:complexType>` ||
		strings.Count(fragment, `<xs:restriction base="xs:anyType">`) != 1 ||
		strings.Count(fragment, `<xs:anyAttribute namespace="##other" processContents="lax"/>`) != 1 {
		t.Fatalf("copied openAttrs bounds or facts changed: %q", fragment)
	}
	return fragment
}

//nolint:gocognit // Digest-derived bytes, synthetic owner, and all published facts form one probe.
func TestPinnedOpenAttrsNamedRestrictionUnlock(t *testing.T) {
	for _, edition := range namedRestrictionEditions {
		t.Run(edition.id, func(t *testing.T) {
			base := pinnedOpenAttrsFragment(t, edition)
			derived := `<xs:complexType name="SyntheticDerived"><xs:complexContent><xs:restriction base="xs:openAttrs"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					id := edition.id + "-" + policy.name + "-named-restriction"
					source := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="` + edition.version + `">` + "\n" + derived + "\n" + base + "\n</xs:schema>"
					var first []goxsd9.Component
					for attempt := range 2 {
						schema, err := inlineProbeParse(t, id, source, policy.value)
						if err != nil {
							t.Fatalf("attempt %d: %v", attempt, err)
						}
						components := schema.Components()
						if len(schema.Documents()) != 1 || len(components) != 2 || components[0].Name().Local() != "SyntheticDerived" || components[1].Name().Local() != "openAttrs" {
							t.Fatalf("declaration order = %v", components)
						}
						copiedBase, ok := components[1].ComplexType()
						if !ok || copiedBase.Loc() != inlineProbeLoc(t, id, source, `<xs:complexType name="openAttrs"`) ||
							copiedBase.Derivation() != goxsd9.ComplexTypeDerivationRestriction ||
							copiedBase.DerivationLoc() != inlineSequenceLocAfter(t, id, source, `<xs:complexType name="openAttrs"`, `<xs:restriction base="xs:anyType"`) ||
							copiedBase.BaseLoc() != inlineSequenceLocAfter(t, id, source, `<xs:complexType name="openAttrs"`, `base="xs:anyType"`) ||
							copiedBase.Particle() != nil || len(copiedBase.AttributeUses()) != 0 {
							t.Fatalf("copied base facts = %v/%v", copiedBase, ok)
						}
						baseWildcard, ok := copiedBase.AnyAttribute()
						if !ok || baseWildcard.Namespace() != "##other" || baseWildcard.ProcessContents() != "lax" ||
							baseWildcard.Loc() != inlineSequenceLocAfter(t, id, source, `<xs:complexType name="openAttrs"`, `<xs:anyAttribute`) ||
							baseWildcard.NamespaceLoc() != inlineSequenceLocAfter(t, id, source, `<xs:complexType name="openAttrs"`, `namespace="##other"`) ||
							baseWildcard.ProcessContentsLoc() != inlineSequenceLocAfter(t, id, source, `<xs:complexType name="openAttrs"`, `processContents="lax"`) {
							t.Fatalf("copied base wildcard = %v/%v", baseWildcard, ok)
						}
						definition, ok := components[0].ComplexType()
						if !ok || definition.Particle() != nil || len(definition.AttributeUses()) != 0 || definition.Derivation() != goxsd9.ComplexTypeDerivationRestriction ||
							definition.DerivationLoc() != inlineProbeLoc(t, id, source, `<xs:restriction base="xs:openAttrs"`) ||
							definition.BaseLoc() != inlineProbeLoc(t, id, source, `base="xs:openAttrs"`) ||
							definition.Base().Namespace() != inlineProbeXSDNamespace || definition.Base().Local() != "openAttrs" {
							t.Fatalf("derived facts = %v/%v", definition, ok)
						}
						ref, ok := definition.BaseReference()
						baseID, hasID := ref.ComponentID()
						if !ok || !ref.IsNamed() || !hasID || baseID != components[1].ID() || ref.Loc() != definition.BaseLoc() {
							t.Fatalf("base reference = %v/%v", ref, ok)
						}
						wildcard, ok := definition.AnyAttribute()
						if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" ||
							wildcard.Loc() != inlineProbeLoc(t, id, source, `<xs:anyAttribute`) ||
							wildcard.NamespaceLoc() != inlineProbeLoc(t, id, source, `namespace="##other"`) ||
							wildcard.ProcessContentsLoc() != inlineProbeLoc(t, id, source, `processContents="lax"`) {
							t.Fatalf("derived wildcard = %v/%v", wildcard, ok)
						}
						if attempt == 0 {
							first = components
							continue
						}
						if !reflect.DeepEqual(first, components) {
							t.Fatal("repeated component facts changed")
						}
					}
				})
			}
		})
	}
}
