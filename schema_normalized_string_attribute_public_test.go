package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Public type identity, facet copies, and source locations are one contract.
func TestPublicNormalizedStringGlobalAttributeTypeFacts(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        goxsd9.LanguagePolicy
	}{
		{"Compatibility 1.0", "1.0", goxsd9.Compatibility},
		{"Compatibility 1.1", "1.1", goxsd9.Compatibility},
		{"Strict10", "1.0", goxsd9.Strict10},
		{"Strict11", "1.1", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + profile.version + `">
  <xs:attribute name="direct" type="xs:normalizedString"/>
  <xs:attribute name="named" type="t:Restricted"/>
  <xs:simpleType name="Restricted"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>
</xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatalf("NewResolvedSource: %v", err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchemaWithPolicy: %v", err)
			}
			if schema.LanguagePolicy() != profile.policy || len(schema.Components()) != 3 {
				t.Fatalf("schema policy/components = %s/%d", schema.LanguagePolicy(), len(schema.Components()))
			}
			for _, test := range []struct {
				name, qname string
				line        int
				builtin     bool
			}{
				{"direct", "normalizedString", 2, true},
				{"named", "Restricted", 3, false},
			} {
				components := schema.FindKind(goxsd9.ComponentKindAttributeDeclaration, parseTestQName(t, "urn:test", test.name))
				if len(components) != 1 {
					t.Fatalf("attribute %s count = %d", test.name, len(components))
				}
				declaration, ok := components[0].AttributeDeclaration()
				if !ok {
					t.Fatalf("attribute %s has no declaration view", test.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Loc().Source() != "root.xsd" || reference.Loc().Line() != test.line || reference.Loc().Column() == 0 || reference.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("attribute %s reference = %#v/%t", test.name, reference, ok)
				}
				namespace := "urn:test"
				if test.builtin {
					namespace = parseTestXSDNamespace
				}
				wantName := parseTestQName(t, namespace, test.qname)
				if declaration.DeclaredType() != wantName || reference.QName() != wantName {
					t.Fatalf("attribute %s QName = %q/%q", test.name, declaration.DeclaredType(), reference.QName())
				}
				id, hasID := declaration.TypeID()
				if test.builtin && (!reference.IsBuiltin() || hasID || !id.IsZero()) {
					t.Fatalf("built-in identity = %v/%t", id, hasID)
				}
				if !test.builtin && (!reference.IsNamed() || !hasID || id.IsZero() || id.Source() != "root.xsd") {
					t.Fatalf("named identity = %v/%t", id, hasID)
				}
				white, ok := reference.StringWhiteSpaceFacet()
				if !ok || white.Value() != "replace" || white.Fixed() || !white.Loc().IsZero() {
					t.Fatalf("attribute %s whitespace = %#v/%t", test.name, white, ok)
				}
				if bounds, present := reference.IntegerBounds(); present {
					t.Fatalf("string attribute %s has numeric bounds: %#v", test.name, bounds)
				}
				facets := reference.StringEnumerationFacets()
				if test.builtin && facets.HasEnumeration() {
					t.Fatalf("built-in enumeration = %v", facets.Values())
				}
				if !test.builtin {
					values := facets.Values()
					locs := facets.Locations()
					if len(values) != 2 || values[0] != "a\nb" || values[1] != " a  b " || len(locs) != 2 || locs[0].Line() != 4 || locs[1].Line() != 4 {
						t.Fatalf("named enumeration = %v at %v", values, locs)
					}
					values[0] = "changed"
					locs[0] = goxsd9.Loc{}
					if reference.StringEnumerationFacets().Values()[0] != "a\nb" || reference.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("public facet copies mutated schema")
					}
				}
			}
		})
	}
}
