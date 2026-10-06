package specs

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

const boundedPositiveIntegerNamespace = "http://www.w3.org/2001/XMLSchema"

// These rows identify the positiveInteger attribute use in digest-pinned W3C
// bootstrap artifacts. The projection below changes only its declaration kind
// and unsupported use control to exercise the issue's global-element boundary.
var boundedPositiveIntegerArtifacts = []struct {
	id      string
	line    int
	policy  goxsd9.LanguagePolicy
	version goxsd9.XSDVersion
}{
	{id: "xsd10-schema-for-schemas", line: 2452, policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
	{id: "xsd10-datatypes-schema", line: 1038, policy: goxsd9.Strict10, version: goxsd9.XSDVersion10},
	{id: "xsd11-schema-for-schemas", line: 1785, policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
	{id: "xsd11-datatypes-schema", line: 325, policy: goxsd9.Strict11, version: goxsd9.XSDVersion11},
}

//nolint:gocognit // Keep pinned provenance and both parse outcomes in one matrix.
func TestBootstrapPositiveIntegerBoundedProjection(t *testing.T) {
	manifest := readBootstrapProbeManifest(t)
	for _, artifact := range boundedPositiveIntegerArtifacts {
		t.Run(artifact.id, func(t *testing.T) {
			attribute := pinnedPositiveIntegerAttribute(t, manifest, artifact.id, artifact.line)
			element := strings.Replace(attribute, "<xs:attribute", "<xs:element", 1)
			element = strings.Replace(element, ` use="required"`, "", 1)
			if element == attribute || strings.Contains(element, `use="required"`) {
				t.Fatalf("projection did not isolate a global element from %q line %d", artifact.id, artifact.line)
			}
			source := goxsd9.SourceID(artifact.id + ":bounded-positiveInteger")
			oneElement := boundedPositiveIntegerRoot(element)
			first, err := parseBoundedPositiveInteger(t, source, oneElement, artifact.policy)
			if err != nil {
				t.Fatalf("parse projected element: %v", err)
			}
			second, err := parseBoundedPositiveInteger(t, source, oneElement, artifact.policy)
			if err != nil {
				t.Fatalf("repeat projected element: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) || !reflect.DeepEqual(first.Documents(), second.Documents()) {
				t.Fatal("bounded artifact projection changed public facts or order between parses")
			}
			assertBoundedPositiveIntegerElement(t, first, source, oneElement, artifact.version)

			withOriginalAttribute := boundedPositiveIntegerRoot(element + boundedPositiveIntegerAttributeOwner(attribute))
			for repeat := 0; repeat < 2; repeat++ {
				schema, nestedErr := parseBoundedPositiveInteger(t, source, withOriginalAttribute, artifact.policy)
				assertBoundedPositiveIntegerAttributeFailure(t, schema, nestedErr, source, withOriginalAttribute, artifact.version)
			}
			originalOnly := boundedPositiveIntegerRoot(boundedPositiveIntegerAttributeOwner(attribute))
			schema, err := parseBoundedPositiveInteger(t, source, originalOnly, artifact.policy)
			assertBoundedPositiveIntegerAttributeFailure(t, schema, err, source, originalOnly, artifact.version)
		})
	}
}

func pinnedPositiveIntegerAttribute(t *testing.T, manifest Manifest, id string, line int) string {
	t.Helper()
	entry, err := manifest.Find(id)
	if err != nil {
		t.Fatalf("manifest.Find(%q): %v", id, err)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + id + ".raw")
	if err != nil {
		t.Fatalf("read pinned artifact %q: %v", id, err)
	}
	if got := testDigest(raw); !strings.EqualFold(got, entry.SHA256) {
		t.Fatalf("artifact %q raw SHA-256 = %s, want %s", id, got, entry.SHA256)
	}
	lines := strings.Split(string(raw), "\n")
	if line < 1 || line > len(lines) {
		t.Fatalf("artifact %q has no line %d", id, line)
	}
	attribute := strings.TrimSpace(lines[line-1])
	const want = `<xs:attribute name="value" type="xs:positiveInteger" use="required"/>`
	if attribute != want {
		t.Fatalf("artifact %q line %d = %q, want %q", id, line, attribute, want)
	}
	return attribute
}

func boundedPositiveIntegerRoot(body string) string {
	return `<xs:schema xmlns:xs="` + boundedPositiveIntegerNamespace + `" targetNamespace="urn:bootstrap-bounded">` + body + `</xs:schema>`
}

func boundedPositiveIntegerAttributeOwner(attribute string) string {
	return `<xs:complexType name="Facet">` + attribute + `</xs:complexType>`
}

func parseBoundedPositiveInteger(t *testing.T, source goxsd9.SourceID, schemaText string, policy goxsd9.LanguagePolicy) (goxsd9.Schema, error) {
	t.Helper()
	resolved, err := goxsd9.NewResolvedSource(context.Background(), source, io.NopCloser(strings.NewReader(schemaText)))
	if err != nil {
		t.Fatalf("NewResolvedSource(%q): %v", source, err)
	}
	return goxsd9.ParseSchemaWithPolicy(resolved, nil, policy)
}

func assertBoundedPositiveIntegerElement(t *testing.T, schema goxsd9.Schema, source goxsd9.SourceID, schemaText string, version goxsd9.XSDVersion) {
	t.Helper()
	name, err := goxsd9.NewQName("urn:bootstrap-bounded", "value")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	matches := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(matches) != 1 || len(schema.Components()) != 1 {
		t.Fatalf("projected element/components = %d/%d, want 1/1", len(matches), len(schema.Components()))
	}
	declaration, ok := matches[0].ElementDeclaration()
	if !ok {
		t.Fatal("projected element has no declaration view")
	}
	reference, ok := declaration.TypeReference()
	if !ok || !reference.IsBuiltin() || reference.Name().Namespace() != boundedPositiveIntegerNamespace || reference.Name().Local() != "positiveInteger" {
		t.Fatalf("projected type reference = %#v/%t, want built-in positiveInteger", reference, ok)
	}
	if id, hasID := reference.ComponentID(); hasID || !id.IsZero() {
		t.Fatalf("built-in component identity = %v/%t, want zero/false", id, hasID)
	}
	wantTypeLoc := boundedPositiveIntegerLoc(t, source, strings.Index(schemaText, `type="xs:positiveInteger"`))
	if reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantTypeLoc {
		t.Fatalf("built-in locations = %s/%s, want %s", reference.Loc(), reference.VarietyLoc(), wantTypeLoc)
	}
	bounds, ok := reference.IntegerBounds()
	if !ok || bounds.Version() != version {
		t.Fatalf("integer bounds = %v/%t, want %s", bounds, ok, version)
	}
	minimum, ok := bounds.MinInclusiveFacet()
	if !ok || minimum.Value().Canonical() != "1" || minimum.Kind() != goxsd9.BoundMinInclusive || !minimum.Loc().IsZero() || minimum.Version() != version {
		t.Fatalf("intrinsic lower bound = %#v/%t, want minInclusive=1 at Loc{} for %s", minimum, ok, version)
	}
}

func assertBoundedPositiveIntegerAttributeFailure(t *testing.T, schema goxsd9.Schema, err error, source goxsd9.SourceID, schemaText string, version goxsd9.XSDVersion) {
	t.Helper()
	if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
		t.Fatalf("excluded original attribute returned schema/error %v/%v", schema, err)
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("attribute failure %v has no diagnostic", err)
	}
	wantLoc := boundedPositiveIntegerLoc(t, source, strings.LastIndex(schemaText, `type="xs:positiveInteger"`))
	wantSpec := "xsd11-structures#Attribute_Declaration_details"
	if version == goxsd9.XSDVersion10 {
		wantSpec = "xsd10-structures#Attribute_Declaration_details"
	}
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode || diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 || !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("attribute diagnostic = %s related=%v, want located schema-syntax exclusion at %s with %s", diagnostic, diagnostic.Related(), wantLoc, wantSpec)
	}
}

func boundedPositiveIntegerLoc(t *testing.T, source goxsd9.SourceID, offset int) goxsd9.Loc {
	t.Helper()
	if offset < 0 {
		t.Fatal("bounded fixture has no positiveInteger type attribute")
	}
	loc, err := goxsd9.NewLoc(source, 1, offset+1)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}
