package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Check the public ordered, copied all view under every language policy.
func TestPublicDirectAllBuiltinNMTOKENIsQueryable(t *testing.T) {
	for _, profile := range []struct {
		name, schemaVersion, bounds, wantBounds string
		policy                                  goxsd9.LanguagePolicy
	}{
		{"compatibility", "1.0", `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616", goxsd9.Compatibility},
		{"strict10", "1.0", `minOccurs="0" maxOccurs="1"`, "0/1", goxsd9.Strict10},
		{"strict11", "1.1", `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + profile.schemaVersion + `">
  <xs:element name="root" type="r:Record"/>
  <xs:complexType name="Record"><xs:all minOccurs="0">
    <xs:element name="before" type="xs:token"/>
    <xs:element name="word" type="xs:NMTOKEN" ` + profile.bounds + `/>
    <xs:element name="after" type="xs:boolean"/>
  </xs:all></xs:complexType>
</xs:schema>`
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
				t.Fatalf("Record definitions = %d, want 1", len(definitions))
			}
			definition, hasDefinition := definitions[0].ComplexTypeDefinition()
			if !hasDefinition {
				t.Fatal("Record complex type missing")
			}
			all, hasAll := definition.Particle().(goxsd9.AllParticle)
			if !hasAll || all.Occurrences().String() != "0/1" || all.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:all minOccurs="0"`) {
				t.Fatalf("all = %#v, want located 0/1 particle", definition.Particle())
			}
			members := all.Members()
			if len(members) != 3 {
				t.Fatalf("members = %d, want 3", len(members))
			}
			for index, want := range []struct {
				name, kind string
			}{{"before", "token"}, {"word", "NMTOKEN"}, {"after", "boolean"}} {
				member, hasMember := members[index].(goxsd9.ElementParticle)
				if !hasMember || member.Name().Local() != want.name || member.DeclaredType() != parseTestQName(t, parseTestXSDNamespace, want.kind) || member.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="`+want.name+`"`) {
					t.Fatalf("member %d = %#v, want located built-in %s", index, members[index], want.kind)
				}
			}
			word, hasWord := members[1].(goxsd9.ElementParticle)
			if !hasWord {
				t.Fatalf("word member = %T, want ElementParticle", members[1])
			}
			if word.Occurrences().String() != profile.wantBounds {
				t.Fatalf("word bounds = %s, want %s", word.Occurrences(), profile.wantBounds)
			}
			reference, hasReference := word.TypeReference()
			if !hasReference || !reference.IsBuiltin() || reference.QName() != word.DeclaredType() || reference.Loc() != publicPositiveIntegerAttributeLoc(t, root, `type="xs:NMTOKEN"`) {
				t.Fatalf("word reference = %#v/%t, want located built-in NMTOKEN", reference, hasReference)
			}
			if _, hasID := word.TypeID(); hasID {
				t.Fatal("built-in NMTOKEN gained a component ID")
			}
			if _, hasID := reference.ComponentID(); hasID {
				t.Fatal("built-in NMTOKEN reference gained a component ID")
			}
			space, hasSpace := reference.StringWhiteSpaceFacet()
			if !hasSpace || space.Value() != "collapse" {
				t.Fatalf("NMTOKEN whitespace = %q/%t, want collapse", space.Value(), hasSpace)
			}
			members[1] = nil
			copied, hasCopy := all.Members()[1].(goxsd9.ElementParticle)
			if !hasCopy || copied.DeclaredType() != word.DeclaredType() || copied.Occurrences().String() != profile.wantBounds {
				t.Fatalf("mutating member slice changed schema: %#v", all.Members()[1])
			}
			components := schema.Components()
			components[1] = goxsd9.Component{}
			if found := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, parseTestQName(t, "urn:all", "Record")); len(found) != 1 {
				t.Fatal("mutating component slice changed schema")
			}
		})
	}
}
