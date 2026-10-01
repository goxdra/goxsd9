package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

// TestPublicGlobalIntAttributeValueConstraints is the base/head fixture for issue #570.
//
//nolint:gocognit // Keep the public type, constraint, and source-location contract together.
func TestPublicGlobalIntAttributeValueConstraints(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test"><xs:attribute name="direct" type="xs:int" default=" +0001 "/><xs:attribute name="named" type="r:Bounded" fixed="2147483647"/><xs:simpleType name="Bounded"><xs:restriction base="xs:int"><xs:maxInclusive value="2147483647"/></xs:restriction></xs:simpleType></xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct {
				name, lexical, canonical, marker string
				kind                             goxsd9.AttributeValueConstraintKind
				named                            bool
			}{
				{"direct", "+0001", "1", `default=" +0001 "`, goxsd9.AttributeValueConstraintDefault, false},
				{"named", "2147483647", "2147483647", `fixed="2147483647"`, goxsd9.AttributeValueConstraintFixed, true},
			} {
				matches := schema.FindKind(goxsd9.ComponentKindAttributeDeclaration, parseTestQName(t, "urn:test", want.name))
				if len(matches) != 1 {
					t.Fatalf("%s attribute count = %d", want.name, len(matches))
				}
				declaration, ok := matches[0].AttributeDeclaration()
				if !ok {
					t.Fatalf("%s declaration missing", want.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.IsNamed() != want.named {
					t.Fatalf("%s type reference = %v/%t", want.name, reference, ok)
				}
				constraint, ok := declaration.ValueConstraint()
				loc := parseTestLoc(t, "root.xsd", 1, strings.Index(root, want.marker)+1)
				if !ok || constraint.Kind() != want.kind || constraint.Lexical() != want.lexical || constraint.Loc() != loc {
					t.Fatalf("%s value constraint = %v/%t, want %q at %s", want.name, constraint, ok, want.lexical, loc)
				}
				value, ok := constraint.IntegerValue()
				if !ok || value.Canonical() != want.canonical {
					t.Fatalf("%s integer = %q/%t", want.name, value.Canonical(), ok)
				}
			}
		})
	}
}
