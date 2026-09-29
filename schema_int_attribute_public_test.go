package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep the public reference, exact bounds, and identity checks together across policies.
func TestPublicGlobalIntAttributeBoundsAcrossPolicies(t *testing.T) {
	for _, profile := range []struct {
		name   string
		policy goxsd9.LanguagePolicy
	}{
		{name: "Compatibility", policy: goxsd9.Compatibility},
		{name: "Strict10", policy: goxsd9.Strict10},
		{name: "Strict11", policy: goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:test"><xs:attribute name="direct" type="xs:int"/></xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			name := parseTestQName(t, "urn:test", "direct")
			components := schema.FindKind(goxsd9.ComponentKindAttributeDeclaration, name)
			if len(components) != 1 {
				t.Fatalf("attribute count = %d, want 1", len(components))
			}
			declaration, ok := components[0].AttributeDeclaration()
			if !ok {
				t.Fatal("global attribute declaration view is missing")
			}
			reference, ok := declaration.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Name() != parseTestQName(t, parseTestXSDNamespace, "int") || reference.Loc().IsZero() {
				t.Fatalf("type reference = %q/%t at %s, want located built-in xs:int", reference.Name(), ok, reference.Loc())
			}
			if id, present := reference.ComponentID(); present || !id.IsZero() {
				t.Fatalf("built-in type ID = %v/%t, want none", id, present)
			}
			bounds, present := reference.IntegerBounds()
			if !present {
				t.Fatal("built-in xs:int reference has no public integer bounds")
			}
			minimum, minPresent := bounds.MinInclusive()
			maximum, maxPresent := bounds.MaxInclusive()
			if !minPresent || minimum.Canonical() != "-2147483648" || !maxPresent || maximum.Canonical() != "2147483647" {
				t.Fatalf("bounds = %q/%t..%q/%t, want exact signed 32-bit range", minimum.Canonical(), minPresent, maximum.Canonical(), maxPresent)
			}
			ordered := bounds.Bounds()
			if len(ordered) != 2 || ordered[0].Kind() != goxsd9.BoundMinInclusive || ordered[1].Kind() != goxsd9.BoundMaxInclusive || !ordered[0].Loc().IsZero() || !ordered[1].Loc().IsZero() {
				t.Fatalf("ordered intrinsic bounds = %#v, want two inclusive location-free facets", ordered)
			}
			if _, present := (goxsd9.SimpleTypeReference{}).IntegerBounds(); present {
				t.Fatal("zero reference unexpectedly has integer bounds")
			}
		})
	}
}
