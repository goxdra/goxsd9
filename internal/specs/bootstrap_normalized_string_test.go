package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

// These lines are the xs:token derivations from normalizedString in the pinned
// XSD 1.0 bootstrap artifacts. The full artifacts still have earlier blockers.
//
//nolint:gocognit // Keep digest, projection, copied facts, and next outcome together.
func TestBootstrapNormalizedStringBoundedReferences(t *testing.T) {
	manifest := readBootstrapProbeManifest(t)
	for _, artifact := range []struct {
		id   string
		line int
	}{
		{"xsd10-schema-for-schemas", 1849},
		{"xsd10-datatypes-schema", 505},
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
			if artifact.line > len(lines) || strings.TrimSpace(lines[artifact.line-1]) != `<xs:restriction base="xs:normalizedString">` {
				t.Fatalf("pinned normalizedString reference changed at line %d", artifact.line)
			}
			source := goxsd9.SourceID(artifact.id + ":normalizedString")
			root := `<xs:schema xmlns:xs="` + boundedPositiveIntegerNamespace + `" targetNamespace="urn:bootstrap-bounded"><xs:simpleType name="Token">` + strings.TrimSpace(lines[artifact.line-1]) + `<xs:whiteSpace value="collapse"/></xs:restriction></xs:simpleType></xs:schema>`
			first, err := parseBoundedPositiveInteger(t, source, root, goxsd9.Strict10)
			if err != nil {
				t.Fatalf("projected normalizedString reference: %v", err)
			}
			second, err := parseBoundedPositiveInteger(t, source, root, goxsd9.Strict10)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeat projection changed components: %v", err)
			}
			name, err := goxsd9.NewQName("urn:bootstrap-bounded", "Token")
			if err != nil {
				t.Fatalf("NewQName: %v", err)
			}
			found := first.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, name)
			if len(found) != 1 {
				t.Fatalf("Token count = %d, want 1", len(found))
			}
			definition, ok := found[0].SimpleTypeDefinition()
			if !ok || !definition.IsString() {
				t.Fatal("projected Token has no string-derived facts")
			}
			base, ok := definition.BaseReference()
			if !ok || !base.IsBuiltin() || base.Name().Local() != "normalizedString" || base.Name().Namespace() != boundedPositiveIntegerNamespace {
				t.Fatalf("projected base = %#v/%t", base, ok)
			}
			whiteSpace, ok := base.StringWhiteSpaceFacet()
			if !ok || whiteSpace.Value() != "replace" || whiteSpace.Fixed() || !whiteSpace.Loc().IsZero() {
				t.Fatalf("base whitespace = %#v/%t", whiteSpace, ok)
			}
			if id, hasID := base.ComponentID(); hasID || !id.IsZero() {
				t.Fatalf("built-in ID = %v/%t", id, hasID)
			}
			withNext := strings.Replace(root, `</xs:restriction>`, `<xs:pattern value=".*"/></xs:restriction>`, 1)
			schema, err := parseBoundedPositiveInteger(t, source, withNext, goxsd9.Strict10)
			if err == nil || len(schema.Components()) != 0 {
				t.Fatal("next unsupported facet returned a schema or no error")
			}
			var diagnostic goxsd9.Diagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedDatatypeFacetCode || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("next outcome = %v", err)
			}
		})
	}
}
