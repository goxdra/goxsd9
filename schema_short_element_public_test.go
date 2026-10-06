package goxsd9_test

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit,funlen // Exercise the copied public element and type views together.
func TestPublicGlobalShortElementReferences(t *testing.T) {
	for _, profile := range []struct {
		name, version string
		policy        goxsd9.LanguagePolicy
		factVersion   goxsd9.XSDVersion
	}{
		{"Compatibility 1.0", "1.0", goxsd9.Compatibility, goxsd9.XSDVersion11},
		{"Compatibility 1.1", "1.1", goxsd9.Compatibility, goxsd9.XSDVersion11},
		{"Strict10", "1.0", goxsd9.Strict10, goxsd9.XSDVersion10},
		{"Strict11", "1.1", goxsd9.Strict11, goxsd9.XSDVersion11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:s="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + profile.version + `">
  <xs:element name="direct" type="s:short"/>
  <xs:element name="forward" type="t:Narrow"/>
  <xs:simpleType name="Narrow"><xs:restriction base="s:short"><xs:minInclusive value="-12"/><xs:maxInclusive value="34"/></xs:restriction></xs:simpleType>
</xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			components := schema.Components()
			if len(components) != 3 || components[0].Name().Local() != "direct" || components[1].Name().Local() != "forward" || components[2].Name().Local() != "Narrow" {
				t.Fatalf("component order = %v", components)
			}
			for _, want := range []struct {
				local, typeName, lexical, min, max, minMarker, maxMarker string
				named                                                    bool
			}{
				{"direct", "short", `type="s:short"`, "-32768", "32767", "", "", false},
				{"forward", "Narrow", `type="t:Narrow"`, "-12", "34", `value="-12"`, `value="34"`, true},
			} {
				matches := schema.FindKind(goxsd9.ComponentKindElementDeclaration, parseTestQName(t, "urn:test", want.local))
				if len(matches) != 1 {
					t.Fatalf("%s element count = %d", want.local, len(matches))
				}
				declaration, ok := matches[0].ElementDeclaration()
				if !ok || declaration.ID() != matches[0].ID() || declaration.ID().Source() != "root.xsd" {
					t.Fatalf("%s declaration identity = %v/%t", want.local, declaration.ID(), ok)
				}
				if declaration.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="`+want.local+`"`) {
					t.Fatalf("%s declaration Loc = %s", want.local, declaration.Loc())
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.IsNamed() != want.named || reference.IsBuiltin() == want.named || reference.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("%s reference kind = %q/%t", want.local, reference.Kind(), ok)
				}
				namespace := parseTestXSDNamespace
				if want.named {
					namespace = "urn:test"
				}
				wantQName := parseTestQName(t, namespace, want.typeName)
				if declaration.DeclaredType() != wantQName || reference.QName() != wantQName || reference.Loc() != publicPositiveIntegerAttributeLoc(t, root, want.lexical) {
					t.Fatalf("%s type QName/Loc = %q/%q/%s", want.local, declaration.DeclaredType(), reference.QName(), reference.Loc())
				}
				id, hasID := declaration.TypeID()
				refID, refHasID := reference.ComponentID()
				if want.named {
					target := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, wantQName)
					if len(target) != 1 || !hasID || !refHasID || id != target[0].ID() || refID != id {
						t.Fatalf("%s target identity = %v/%t, ref %v/%t", want.local, id, hasID, refID, refHasID)
					}
				}
				if !want.named && (hasID || refHasID || !id.IsZero() || !refID.IsZero()) {
					t.Fatalf("built-in short gained component identity: %v/%v", id, refID)
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || bounds.Version() != profile.factVersion {
					t.Fatalf("%s bounds version = %s/%t", want.local, bounds.Version(), ok)
				}
				ordered := bounds.Bounds()
				if len(ordered) != 2 || ordered[0].Kind() != goxsd9.BoundMinInclusive || ordered[1].Kind() != goxsd9.BoundMaxInclusive || ordered[0].Value().Canonical() != want.min || ordered[1].Value().Canonical() != want.max {
					t.Fatalf("%s effective ordered bounds = %v", want.local, ordered)
				}
				if want.named && (ordered[0].Loc() != publicPositiveIntegerAttributeLoc(t, root, want.minMarker) || ordered[1].Loc() != publicPositiveIntegerAttributeLoc(t, root, want.maxMarker)) {
					t.Fatalf("%s source bound Locs = %s/%s", want.local, ordered[0].Loc(), ordered[1].Loc())
				}
				if !want.named && (!ordered[0].Loc().IsZero() || !ordered[1].Loc().IsZero()) {
					t.Fatal("intrinsic short bounds gained source locations")
				}
				ordered[0] = goxsd9.IntegerBoundFacet{}
				secondReference, _ := declaration.TypeReference()
				secondBounds, _ := secondReference.IntegerBounds()
				if secondBounds.Bounds()[0].Value().Canonical() != want.min {
					t.Fatal("mutating copied bounds changed the schema")
				}
			}
			components[0] = goxsd9.Component{}
			if direct := schema.FindKind(goxsd9.ComponentKindElementDeclaration, parseTestQName(t, "urn:test", "direct")); len(direct) != 1 {
				t.Fatal("mutating copied components changed the schema")
			}
			secondSource, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			second, err := goxsd9.ParseSchemaWithPolicy(secondSource, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), second.Components()) {
				t.Fatalf("repeated public build changed facts/order: %v", err)
			}
		})
	}
}
