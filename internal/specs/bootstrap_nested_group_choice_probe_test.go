package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

// The attrDecls group is copied from each digest-checked Structures artifact.
// Its local children resolve types outside the bounded atomic group surface.
func TestBootstrapNestedGroupChoiceBoundedProjection(t *testing.T) { //nolint:gocognit,funlen // Digest, copied bounds, supported projection, and next blocker stay together.
	manifest := readBootstrapProbeManifest(t)
	for _, artifact := range []struct {
		id, version, syntaxSpec string
		policy                  goxsd9.LanguagePolicy
	}{
		{"xsd10-schema-for-schemas", "1.0", "xsd10-structures#schema-document", goxsd9.Strict10},
		{"xsd11-schema-for-schemas", "1.1", "xsd11-structures#cSchemaDocument", goxsd9.Strict11},
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
			start := strings.Index(text, `<xs:group name="attrDecls">`)
			if start < 0 {
				t.Fatal("pinned attrDecls group is missing")
			}
			end := strings.Index(text[start:], `</xs:group>`)
			if end < 0 {
				t.Fatal("pinned attrDecls group has no close")
			}
			fragment := text[start : start+end+len(`</xs:group>`)]
			if !strings.Contains(fragment, `<xs:choice minOccurs="0" maxOccurs="unbounded">`) || !strings.Contains(fragment, `<xs:element name="attribute" type="xs:attribute"/>`) || !strings.Contains(fragment, `<xs:element name="attributeGroup" type="xs:attributeGroupRef"/>`) {
				t.Fatal("pinned group shape or exact choice bounds changed")
			}
			id := entry.ID + ":nested-choice"
			root := func(group string) string {
				return `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" xmlns:p="urn:bootstrap-choice" targetNamespace="urn:bootstrap-choice" version="` + artifact.version + `">` + group + `<xs:element name="attribute" type="xs:integer"/><xs:element name="attributeGroup" type="xs:integer"/><xs:element name="anyAttribute" type="xs:integer"/></xs:schema>`
			}
			original := root(fragment)
			schema, err := inlineProbeParse(t, id, original, artifact.policy)
			if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatalf("original fragment returned schema: %v", err)
			}
			diagnostic, ok := firstBootstrapProbeDiagnostic(err)
			if !ok || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode || diagnostic.Loc() != inlineProbeLoc(t, id, original, `type="xs:attribute"`) || diagnostic.SpecRef() != artifact.syntaxSpec || len(diagnostic.Related()) != 0 || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("next blocker = %v, want schema-for-schema complex type", err)
			}
			localProjection := strings.Replace(fragment, `type="xs:attribute"`, `type="xs:integer"`, 1)
			localProjection = strings.Replace(localProjection, `type="xs:attributeGroupRef"`, `type="xs:token"`, 1)
			localProjection = strings.Replace(localProjection, `ref="xs:anyAttribute"`, `ref="p:anyAttribute"`, 1)
			localSource := root(localProjection)
			localSchema, err := inlineProbeParse(t, id, localSource, artifact.policy)
			if err != nil {
				t.Fatalf("local bounded projection: %v", err)
			}
			localName, err := goxsd9.NewQName("urn:bootstrap-choice", "attrDecls")
			if err != nil {
				t.Fatal(err)
			}
			localGroups := localSchema.FindKind(goxsd9.ComponentKindModelGroupDefinition, localName)
			if len(localGroups) != 1 {
				t.Fatalf("local projection groups = %d", len(localGroups))
			}
			localGroup, ok := localGroups[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("local projected group has no view")
			}
			localSequence, ok := localGroup.Particle().(goxsd9.SequenceParticle)
			if !ok || len(localSequence.Particles()) != 2 {
				t.Fatalf("local projection particle = %T", localGroup.Particle())
			}
			localChoice, ok := localSequence.Particles()[0].(goxsd9.ChoiceParticle)
			if !ok || localChoice.Occurrences().String() != "0/unbounded" || len(localChoice.Alternatives()) != 2 || localChoice.Loc() != inlineProbeLoc(t, id, localSource, `<xs:choice minOccurs="0"`) {
				t.Fatalf("local projection choice = %T", localSequence.Particles()[0])
			}
			for index, want := range []struct{ name, typ string }{{"attribute", "integer"}, {"attributeGroup", "token"}} {
				leaf, leafOK := localChoice.Alternatives()[index].(goxsd9.ElementParticle)
				if !leafOK || leaf.Name().Local() != want.name || leaf.Name().Namespace() != "" || leaf.DeclaredType().Local() != want.typ || leaf.Loc() != inlineProbeLoc(t, id, localSource, `<xs:element name="`+want.name+`"`) {
					t.Fatalf("local projection leaf %d = %T", index, localChoice.Alternatives()[index])
				}
			}
			projected := strings.Replace(fragment, `<xs:element name="attribute" type="xs:attribute"/>`, `<xs:element ref="p:attribute"/>`, 1)
			projected = strings.Replace(projected, `<xs:element name="attributeGroup" type="xs:attributeGroupRef"/>`, `<xs:element ref="p:attributeGroup"/>`, 1)
			projected = strings.Replace(projected, `ref="xs:anyAttribute"`, `ref="p:anyAttribute"`, 1)
			source := root(projected)
			first, err := inlineProbeParse(t, id, source, artifact.policy)
			if err != nil {
				t.Fatalf("bounded projection: %v", err)
			}
			second, err := inlineProbeParse(t, id, source, artifact.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeat projection changed components: %v", err)
			}
			name, err := goxsd9.NewQName("urn:bootstrap-choice", "attrDecls")
			if err != nil {
				t.Fatal(err)
			}
			groups := first.FindKind(goxsd9.ComponentKindModelGroupDefinition, name)
			if len(groups) != 1 {
				t.Fatalf("attrDecls groups = %d, want one", len(groups))
			}
			definition, ok := groups[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("projected group has no model view")
			}
			sequence, ok := definition.Particle().(goxsd9.SequenceParticle)
			if !ok || len(sequence.Particles()) != 2 {
				t.Fatalf("projected particle = %T", definition.Particle())
			}
			choice, ok := sequence.Particles()[0].(goxsd9.ChoiceParticle)
			if !ok || choice.Occurrences().String() != "0/unbounded" || len(choice.Alternatives()) != 2 || choice.Loc() != inlineProbeLoc(t, id, source, `<xs:choice minOccurs="0"`) {
				t.Fatalf("projected choice = %T, bounds %s", sequence.Particles()[0], choice.Occurrences())
			}
		})
	}
}
