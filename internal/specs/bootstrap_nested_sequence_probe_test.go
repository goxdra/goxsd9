package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

func TestBootstrapNamedChoiceNestedSequenceProjection(t *testing.T) { //nolint:gocognit,funlen // Pinned artifact shape and public projection are one boundary.
	manifest := readBootstrapProbeManifest(t)
	for _, artifact := range []struct {
		id, version string
		policy      goxsd9.LanguagePolicy
		names       []string
		groups      []string
	}{
		{"xsd10-schema-for-schemas", "1.0", goxsd9.Strict10, []string{"simpleContent", "complexContent"}, []string{"typeDefParticle", "attrDecls"}},
		{"xsd11-schema-for-schemas", "1.1", goxsd9.Strict11, []string{"simpleContent", "complexContent", "openContent"}, []string{"typeDefParticle", "attrDecls", "assertions"}},
	} {
		t.Run(artifact.id, func(t *testing.T) {
			entry, err := manifest.Find(artifact.id)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
			if err != nil {
				t.Fatal(err)
			}
			if got := testDigest(raw); !strings.EqualFold(got, entry.SHA256) {
				t.Fatalf("raw digest = %s, want %s", got, entry.SHA256)
			}
			text := string(raw)
			start := strings.Index(text, `<xs:group name="complexTypeModel">`)
			if start < 0 {
				t.Fatal("pinned complexTypeModel group is missing")
			}
			end := strings.Index(text[start:], `</xs:group>`)
			if end < 0 {
				t.Fatal("pinned complexTypeModel group has no close")
			}
			fragment := text[start : start+end+len(`</xs:group>`)]
			if !strings.Contains(fragment, `<xs:choice>`) || !strings.Contains(fragment, `<xs:sequence>`) || !strings.Contains(fragment, `<xs:group ref="xs:typeDefParticle" minOccurs="0"/>`) || !strings.Contains(fragment, `<xs:group ref="xs:attrDecls"/>`) {
				t.Fatal("pinned nested sequence or authored group refs changed")
			}
			var stubs strings.Builder
			for _, name := range artifact.names {
				stubs.WriteString(`<xs:element name="` + name + `" type="xs:integer"/>`)
			}
			for _, name := range artifact.groups {
				stubs.WriteString(`<xs:group name="` + name + `"><xs:sequence/></xs:group>`)
			}
			id := entry.ID + ":nested-sequence"
			projected := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="` + artifact.version + `">` + fragment + stubs.String() + `</xs:schema>`
			first, err := inlineProbeParse(t, id, projected, artifact.policy)
			if err != nil {
				t.Fatalf("bounded projection: %v", err)
			}
			second, err := inlineProbeParse(t, id, projected, artifact.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeat projection changed components: %v", err)
			}
			name, err := goxsd9.NewQName(inlineProbeXSDNamespace, "complexTypeModel")
			if err != nil {
				t.Fatal(err)
			}
			found := first.FindKind(goxsd9.ComponentKindModelGroupDefinition, name)
			if len(found) != 1 {
				t.Fatalf("complexTypeModel groups = %d, want one", len(found))
			}
			definition, ok := found[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("projected group has no model view")
			}
			choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
			if !ok || len(choice.Alternatives()) != 3 {
				t.Fatalf("projected choice = %T, alternatives %d", definition.Particle(), len(choice.Alternatives()))
			}
			sequence, ok := choice.Alternatives()[2].(goxsd9.SequenceParticle)
			if !ok || sequence.Occurrences().String() != "1/1" || sequence.Loc() != inlineProbeLoc(t, id, projected, `<xs:sequence>`) {
				t.Fatalf("projected sequence = %T", choice.Alternatives()[2])
			}
			wantTerms := len(artifact.groups)
			if len(artifact.names) == 3 {
				wantTerms++
			}
			if len(sequence.Particles()) != wantTerms {
				t.Fatalf("projected sequence terms = %d, want %d", len(sequence.Particles()), wantTerms)
			}
			for index, groupName := range artifact.groups {
				termIndex := index
				if len(artifact.names) == 3 {
					termIndex++
				}
				group, groupOK := sequence.Particles()[termIndex].(goxsd9.ModelGroupReferenceParticle)
				if !groupOK || group.Ref().Local() != groupName || group.RefLoc() != inlineProbeLoc(t, id, projected, `ref="xs:`+groupName+`"`) {
					t.Fatalf("projected term %d = %T, want %s ref", termIndex, sequence.Particles()[termIndex], groupName)
				}
			}
			afterGroup := text[start+end+len(`</xs:group>`):]
			ownerStart := strings.Index(afterGroup, `<xs:complexType name="complexType" abstract="true">`)
			if ownerStart < 0 {
				t.Fatal("pinned following complexType is missing")
			}
			ownerEnd := strings.Index(afterGroup[ownerStart:], `</xs:complexType>`)
			if ownerEnd < 0 {
				t.Fatal("pinned following complexType has no close")
			}
			owner := afterGroup[ownerStart : ownerStart+ownerEnd+len(`</xs:complexType>`)]
			withOwner := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="` + artifact.version + `">` + fragment + owner + stubs.String() + `</xs:schema>`
			ownerSchema, ownerErr := inlineProbeParse(t, id, withOwner, artifact.policy)
			if ownerErr == nil || len(ownerSchema.Documents()) != 0 || len(ownerSchema.Components()) != 0 {
				t.Fatalf("following owner returned partial schema: %v", ownerErr)
			}
			ownerDiagnostic, ok := firstBootstrapProbeDiagnostic(ownerErr)
			if !ok {
				t.Fatalf("following owner has no diagnostic: %v", ownerErr)
			}
			spec := "xsd10-structures#cos-ct-extends"
			if artifact.version == "1.1" {
				spec = "xsd11-structures#cos-ct-extends"
			}
			if ownerDiagnostic.Class() != goxsd9.FailureUnsupported || ownerDiagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode || ownerDiagnostic.Loc() != inlineProbeLoc(t, id, withOwner, `default="false"`) || ownerDiagnostic.SpecRef() != spec || len(ownerDiagnostic.Related()) != 0 || !errors.Is(ownerErr, goxsd9.ErrUnsupported) {
				t.Fatalf("following owner diagnostic = %s, want local attribute default blocker", ownerDiagnostic)
			}
		})
	}
}
