package specs

import (
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

// The adjacent attribute rows are copied from the manifest-converted pinned
// schema-for-schemas artifacts. Base and child declarations are synthetic;
// unsupported sibling attributes and the surrounding bootstrap grammar are removed.
func TestBootstrapInlineTokenURIAttributeProjection(t *testing.T) { //nolint:gocognit // One shared projection checks both editions and independent rows.
	manifest := readBootstrapProbeManifest(t)
	for _, edition := range []struct {
		id     string
		line   int
		policy goxsd9.LanguagePolicy
	}{
		{"xsd10-schema-for-schemas", 254, goxsd9.Strict10},
		{"xsd11-schema-for-schemas", 250, goxsd9.Strict11},
	} {
		t.Run(edition.id, func(t *testing.T) {
			entry, err := manifest.Find(edition.id)
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
			lines := strings.Split(string(converted), "\n")
			if edition.line+1 > len(lines) {
				t.Fatalf("converted lines = %d", len(lines))
			}
			attributes := []string{strings.TrimSpace(lines[edition.line-1]), strings.TrimSpace(lines[edition.line])}
			if !reflect.DeepEqual(attributes, []string{`<xs:attribute name="targetNamespace" type="xs:anyURI"/>`, `<xs:attribute name="version" type="xs:token"/>`}) {
				t.Fatalf("pinned rows = %v", attributes)
			}
			for _, count := range []int{1, 2} {
				for first := 0; first < len(attributes); first++ {
					if count == 2 && first == 1 {
						continue
					}
					selected := attributes[first]
					if count == 2 {
						selected += attributes[1]
					}
					root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:t="urn:projected" targetNamespace="urn:projected">` +
						`<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element ref="t:child"/></xs:sequence>` + selected +
						`</xs:extension></xs:complexContent></xs:complexType></xs:element><xs:complexType name="Base"/><xs:element name="child" type="xs:integer"/></xs:schema>`
					id := goxsd9.SourceID(edition.id + ":token-anyURI-projection")
					firstSchema, err := parseBoundedPositiveInteger(t, id, root, edition.policy)
					if err != nil {
						t.Fatalf("projected parse: %v", err)
					}
					secondSchema, err := parseBoundedPositiveInteger(t, id, root, edition.policy)
					if err != nil || !reflect.DeepEqual(firstSchema.Components(), secondSchema.Components()) {
						t.Fatalf("repeat projection: %v", err)
					}
					name, err := goxsd9.NewQName("urn:projected", "root")
					if err != nil {
						t.Fatal(err)
					}
					found := firstSchema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
					if len(found) != 1 {
						t.Fatalf("root = %v", found)
					}
					element, ok := found[0].Element()
					if !ok {
						t.Fatal("projected root has no element view")
					}
					owner, ok := element.InlineComplexType()
					if !ok || !owner.ID().IsZero() || len(owner.AttributeUses()) != count {
						t.Fatalf("projected owner = %v/%t", owner, ok)
					}
					for offset, use := range owner.AttributeUses() {
						want := attributes[first+offset]
						local, ok := use.(goxsd9.LocalAttributeUse)
						if !ok || local.Loc() != inlineSequenceLocAfter(t, string(id), root, `<xs:sequence>`, want) || local.Name().Local() != []string{"targetNamespace", "version"}[first+offset] {
							t.Fatalf("projected use %d = %v", offset, use)
						}
						reference, ok := local.TypeReference()
						if !ok || !reference.IsBuiltin() || reference.Name().Local() != []string{"anyURI", "token"}[first+offset] {
							t.Fatalf("projected type %d = %v/%t", offset, reference, ok)
						}
					}
				}
			}
		})
	}
}
