package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Check public parsing and copied intrinsic facts under each policy.
func TestPublicParseAllBuiltinNonNegativeIntegerIsQueryable(t *testing.T) {
	for _, profile := range []struct {
		name, schemaVersion string
		policy              goxsd9.LanguagePolicy
	}{
		{"compatibility", "1.1", goxsd9.Compatibility},
		{"strict10", "1.0", goxsd9.Strict10},
		{"strict11", "1.1", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + profile.schemaVersion + `"><xs:complexType name="Record"><xs:all><xs:element name="debt" type="xs:nonNegativeInteger"/></xs:all></xs:complexType></xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			definitions := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, parseTestQName(t, "urn:all", "Record"))
			if len(definitions) != 1 {
				t.Fatalf("definitions = %d, want one", len(definitions))
			}
			definition, ok := definitions[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Record has no complex type view")
			}
			all, ok := definition.Particle().(goxsd9.AllParticle)
			if !ok || len(all.Members()) != 1 {
				t.Fatalf("particle = %#v, want one all member", definition.Particle())
			}
			member, ok := all.Members()[0].(goxsd9.ElementParticle)
			if !ok || member.DeclaredType() != parseTestQName(t, parseTestXSDNamespace, "nonNegativeInteger") || member.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="debt"`) {
				t.Fatalf("member = %#v, want located nonNegativeInteger declaration", all.Members()[0])
			}
			reference, ok := member.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Loc() != publicPositiveIntegerAttributeLoc(t, root, `type="xs:nonNegativeInteger"`) {
				t.Fatalf("reference = %#v/%t", reference, ok)
			}
			bounds, ok := reference.IntegerBounds()
			minimum, hasMinimum := bounds.MinInclusiveFacet()
			if !ok || !hasMinimum || minimum.Value().Canonical() != "0" || !minimum.Loc().IsZero() {
				t.Fatalf("minInclusive = %v/%t", minimum, hasMinimum)
			}
		})
	}
}
