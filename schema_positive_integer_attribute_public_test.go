package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit,funlen // These checks describe one public attribute-reference contract across policies.
func TestPublicGlobalPositiveIntegerAttributeReferences(t *testing.T) {
	for _, profile := range []struct {
		policy  goxsd9.LanguagePolicy
		version goxsd9.XSDVersion
	}{
		{goxsd9.Compatibility, goxsd9.XSDVersion11},
		{goxsd9.Strict10, goxsd9.XSDVersion10},
		{goxsd9.Strict11, goxsd9.XSDVersion11},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.0">
  <xs:attribute name="direct" type="xs:positiveInteger"/>
  <xs:attribute name="forward" type="t:Derived"/>
  <xs:attribute name="narrowed" type="t:Tight"/>
  <xs:simpleType name="Derived"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>
  <xs:simpleType name="Tight"><xs:restriction base="t:Derived"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct {
				local, typeLocal, lexical, minimum, boundMarker string
				named                                           bool
			}{
				{local: "direct", typeLocal: "positiveInteger", lexical: "xs:positiveInteger", minimum: "1"},
				{local: "forward", typeLocal: "Derived", lexical: "t:Derived", minimum: "1", named: true},
				{local: "narrowed", typeLocal: "Tight", lexical: "t:Tight", minimum: "2", boundMarker: `value="2"`, named: true},
			} {
				components := schema.FindKind(goxsd9.ComponentKindAttributeDeclaration, parseTestQName(t, "urn:test", want.local))
				if len(components) != 1 {
					t.Fatalf("%s: attribute count = %d", want.local, len(components))
				}
				declaration, ok := components[0].AttributeDeclaration()
				if !ok || declaration.ID() != components[0].ID() {
					t.Fatalf("%s: missing declaration or identity", want.local)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.IsNamed() != want.named || reference.IsBuiltin() == want.named {
					t.Fatalf("%s: reference kind = %q/%t", want.local, reference.Kind(), ok)
				}
				namespace := parseTestXSDNamespace
				if want.named {
					namespace = "urn:test"
				}
				wantName := parseTestQName(t, namespace, want.typeLocal)
				wantLoc := publicPositiveIntegerAttributeLoc(t, root, `type="`+want.lexical+`"`)
				if declaration.DeclaredType() != wantName || reference.Name() != wantName || reference.QName() != wantName || reference.Loc() != wantLoc {
					t.Fatalf("%s: type facts = %q/%q/%q at %s, want %q at %s", want.local, declaration.DeclaredType(), reference.Name(), reference.QName(), reference.Loc(), wantName, wantLoc)
				}
				id, present := declaration.TypeID()
				refID, refPresent := reference.ComponentID()
				if want.named {
					target := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, wantName)
					if len(target) != 1 || !present || id != target[0].ID() || !refPresent || refID != id {
						t.Fatalf("%s: named target = %v/%t, reference = %v/%t", want.local, id, present, refID, refPresent)
					}
				}
				if !want.named && (present || refPresent || !id.IsZero() || !refID.IsZero()) {
					t.Fatalf("built-in gained identity = %v/%t, reference = %v/%t", id, present, refID, refPresent)
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || bounds.Version() != profile.version {
					t.Fatalf("%s: bounds version = %q/%t, want %q", want.local, bounds.Version(), ok, profile.version)
				}
				minimum, ok := bounds.MinInclusiveFacet()
				if !ok || minimum.Value().Canonical() != want.minimum || minimum.Kind() != goxsd9.BoundMinInclusive || len(bounds.Bounds()) != 1 {
					t.Fatalf("%s: minimum = %#v/%t", want.local, minimum, ok)
				}
				if want.boundMarker == "" && !minimum.Loc().IsZero() {
					t.Fatalf("%s: inherited intrinsic bound has source location %s", want.local, minimum.Loc())
				}
				if want.boundMarker != "" && minimum.Loc() != publicPositiveIntegerAttributeLoc(t, root, want.boundMarker) {
					t.Fatalf("%s: narrowed bound location = %s", want.local, minimum.Loc())
				}
				boundsCopy := bounds.Bounds()
				boundsCopy[0] = goxsd9.IntegerBoundFacet{}
				second, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("%s: second reference missing", want.local)
				}
				secondBounds, _ := second.IntegerBounds()
				secondMin, _ := secondBounds.MinInclusive()
				if secondMin.Canonical() != want.minimum {
					t.Fatalf("%s: copied bound mutated schema to %q", want.local, secondMin.Canonical())
				}
			}
			copied := schema.Components()
			copied[0] = goxsd9.Component{}
			if len(schema.FindKind(goxsd9.ComponentKindAttributeDeclaration, parseTestQName(t, "urn:test", "direct"))) != 1 {
				t.Fatal("mutating copied components changed schema")
			}
		})
	}
}

func publicPositiveIntegerAttributeLoc(t *testing.T, source, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(source, marker)
	if index < 0 {
		t.Fatalf("missing %q in schema", marker)
	}
	line := strings.Count(source[:index], "\n") + 1
	column := index - strings.LastIndex(source[:index], "\n")
	return parseTestLoc(t, "root.xsd", line, column)
}
