package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

//nolint:gocognit // Digest, projected reference, and next boundary share pinned provenance.
func TestBootstrapQNameBoundedReferences(t *testing.T) {
	manifest := readBootstrapProbeManifest(t)
	for _, artifact := range []struct {
		id     string
		line   int
		policy goxsd9.LanguagePolicy
	}{
		{"xsd10-schema-for-schemas", 2329, goxsd9.Strict10},
		{"xsd10-datatypes-schema", 923, goxsd9.Strict10},
		{"xsd11-schema-for-schemas", 1647, goxsd9.Strict11},
		{"xsd11-datatypes-schema", 187, goxsd9.Strict11},
	} {
		t.Run(artifact.id, func(t *testing.T) {
			entry, err := manifest.Find(artifact.id)
			if err != nil {
				t.Fatalf("manifest entry: %v", err)
			}
			raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + artifact.id + ".raw")
			if err != nil {
				t.Fatalf("read pinned artifact: %v", err)
			}
			if got := testDigest(raw); !strings.EqualFold(got, entry.SHA256) {
				t.Fatalf("raw digest = %s, want %s", got, entry.SHA256)
			}
			lines := strings.Split(string(raw), "\n")
			const attribute = `<xs:attribute name="base" type="xs:QName" use="optional"/>`
			if artifact.line > len(lines) || strings.TrimSpace(lines[artifact.line-1]) != attribute {
				t.Fatalf("pinned QName declaration changed at line %d", artifact.line)
			}
			source := goxsd9.SourceID(artifact.id + ":QName")
			element := strings.Replace(attribute, "<xs:attribute", "<xs:element", 1)
			element = strings.Replace(element, ` use="optional"`, "", 1)
			root := boundedPositiveIntegerRoot(element)
			first, err := parseBoundedPositiveInteger(t, source, root, artifact.policy)
			if err != nil {
				t.Fatalf("projected QName element: %v", err)
			}
			second, err := parseBoundedPositiveInteger(t, source, root, artifact.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeat projection changed components: %v", err)
			}
			name, err := goxsd9.NewQName("urn:bootstrap-bounded", "base")
			if err != nil {
				t.Fatalf("NewQName: %v", err)
			}
			found := first.FindKind(goxsd9.ComponentKindElementDeclaration, name)
			if len(found) != 1 || len(first.Components()) != 1 {
				t.Fatalf("projected components = %d/%d", len(found), len(first.Components()))
			}
			declaration, ok := found[0].ElementDeclaration()
			if !ok {
				t.Fatal("projected element view missing")
			}
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Name().Namespace() != boundedPositiveIntegerNamespace || reference.Name().Local() != "QName" || reference.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction {
				t.Fatalf("QName reference = %#v/%t", reference, ok)
			}
			loc := boundedPositiveIntegerLoc(t, source, strings.Index(root, `type="xs:QName"`))
			if reference.Loc() != loc || reference.VarietyLoc() != loc {
				t.Fatalf("reference location = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), loc)
			}
			if id, ok := reference.ComponentID(); ok || !id.IsZero() {
				t.Fatalf("built-in ID = %v/%t", id, ok)
			}
			withOriginalAttribute := boundedPositiveIntegerRoot(attribute)
			schema, err := parseBoundedPositiveInteger(t, source, withOriginalAttribute, artifact.policy)
			if err == nil || len(schema.Components()) != 0 {
				t.Fatal("original attribute shape returned schema or no error")
			}
			var d goxsd9.Diagnostic
			if !errors.As(err, &d) || d.Class() != goxsd9.FailureInvalid || d.Code() != "XSD3010" || d.Loc() != boundedPositiveIntegerLoc(t, source, strings.Index(withOriginalAttribute, `use="optional"`)) {
				t.Fatalf("original attribute outcome = %v", err)
			}
			withoutUse := boundedPositiveIntegerRoot(strings.Replace(attribute, ` use="optional"`, "", 1))
			schema, err = parseBoundedPositiveInteger(t, source, withoutUse, artifact.policy)
			if err == nil || len(schema.Components()) != 0 {
				t.Fatal("global QName attribute returned schema or no error")
			}
			if !errors.As(err, &d) || d.Class() != goxsd9.FailureUnsupported || d.Code() != goxsd9.UnsupportedSchemaSyntaxCode || d.Loc() != boundedPositiveIntegerLoc(t, source, strings.Index(withoutUse, `type="xs:QName"`)) || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("QName attribute outcome = %v", err)
			}
		})
	}
}
