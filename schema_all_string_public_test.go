package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Exercise the bounded all member through only exported parse and query contracts.
func TestPublicDirectAllBuiltinStringIsQueryable(t *testing.T) {
	for _, profile := range []struct {
		policy     goxsd9.LanguagePolicy
		version    string
		bounds     string
		wantBounds string
	}{
		{goxsd9.Compatibility, "1.0", `minOccurs="2" maxOccurs="18446744073709551616"`, "2/18446744073709551616"},
		{goxsd9.Strict10, "1.0", `minOccurs="0" maxOccurs="1"`, "0/1"},
		{goxsd9.Strict11, "1.1", `minOccurs="2" maxOccurs="unbounded"`, "2/unbounded"},
	} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + profile.version + `">
  <xs:complexType name="Record"><xs:all>
    <xs:element name="before" type="xs:integer"/>
    <xs:element name="text" type="xs:string" ` + profile.bounds + `/>
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
				t.Fatalf("Record definitions = %d, want one", len(definitions))
			}
			definition, ok := definitions[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("Record complex type facts missing")
			}
			all, ok := definition.Particle().(goxsd9.AllParticle)
			if !ok {
				t.Fatalf("Record particle = %T, want AllParticle", definition.Particle())
			}
			members := all.Members()
			if len(members) != 3 {
				t.Fatalf("member count = %d, want three", len(members))
			}
			for index, want := range []string{"before", "text", "after"} {
				member, hasMember := members[index].(goxsd9.ElementParticle)
				if !hasMember || member.Name().Local() != want {
					t.Fatalf("member %d = %#v, want %s", index, members[index], want)
				}
			}
			text, ok := members[1].(goxsd9.ElementParticle)
			if !ok {
				t.Fatalf("text member = %T, want ElementParticle", members[1])
			}
			reference, ok := text.TypeReference()
			if !ok || text.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="text"`) || text.Occurrences().String() != profile.wantBounds || text.DeclaredType() != parseTestQName(t, parseTestXSDNamespace, "string") || !reference.IsBuiltin() || reference.QName() != text.DeclaredType() || reference.Loc() != publicPositiveIntegerAttributeLoc(t, root, `type="xs:string"`) {
				t.Fatalf("text facts = %#v, type reference = %#v/%t", text, reference, ok)
			}
			if _, hasID := text.TypeID(); hasID {
				t.Fatal("built-in string gained a component ID")
			}
			if _, hasID := reference.ComponentID(); hasID {
				t.Fatal("built-in string reference gained a component ID")
			}
			members[1] = nil
			copied, ok := all.Members()[1].(goxsd9.ElementParticle)
			if !ok || copied.DeclaredType() != text.DeclaredType() || copied.Occurrences().String() != profile.wantBounds {
				t.Fatalf("caller mutated string view: %#v", all.Members()[1])
			}
		})
	}
}
