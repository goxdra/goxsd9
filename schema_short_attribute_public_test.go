package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep the public reference, intrinsic bounds, and identity checks together across policies.
func TestPublicGlobalShortAttributeBoundsAcrossPolicies(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        goxsd9.LanguagePolicy
	}{
		{name: "Compatibility 1.0", version: "1.0", policy: goxsd9.Compatibility},
		{name: "Compatibility 1.1", version: "1.1", policy: goxsd9.Compatibility},
		{name: "Strict10", version: "1.0", policy: goxsd9.Strict10},
		{name: "Strict11", version: "1.1", policy: goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:test" version="` + profile.version + `"><xs:attribute name="direct" type="xs:short"/></xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			components := schema.FindKind(goxsd9.ComponentKindAttributeDeclaration, parseTestQName(t, "urn:test", "direct"))
			if len(components) != 1 {
				t.Fatalf("attribute count = %d, want 1", len(components))
			}
			declaration, ok := components[0].AttributeDeclaration()
			if !ok {
				t.Fatal("global attribute declaration view is missing")
			}
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Name() != parseTestQName(t, parseTestXSDNamespace, "short") || reference.Loc().IsZero() {
				t.Fatalf("type reference = %q/%t at %s, want located built-in xs:short", reference.Name(), ok, reference.Loc())
			}
			if id, present := reference.ComponentID(); present || !id.IsZero() {
				t.Fatalf("built-in type ID = %v/%t, want none", id, present)
			}
			bounds, present := reference.IntegerBounds()
			if !present {
				t.Fatal("built-in xs:short reference has no public integer bounds")
			}
			minimum, minPresent := bounds.MinInclusive()
			maximum, maxPresent := bounds.MaxInclusive()
			if !minPresent || minimum.Canonical() != "-32768" || !maxPresent || maximum.Canonical() != "32767" {
				t.Fatalf("bounds = %q/%t..%q/%t, want exact signed 16-bit range", minimum.Canonical(), minPresent, maximum.Canonical(), maxPresent)
			}
			ordered := bounds.Bounds()
			if len(ordered) != 2 || ordered[0].Kind() != goxsd9.BoundMinInclusive || ordered[1].Kind() != goxsd9.BoundMaxInclusive || !ordered[0].Loc().IsZero() || !ordered[1].Loc().IsZero() {
				t.Fatalf("ordered intrinsic bounds = %#v, want two inclusive location-free facets", ordered)
			}
		})
	}
}
