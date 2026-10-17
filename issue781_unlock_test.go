package goxsd9_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

// TestIssue781TwoNestedSequencesPublicUnlock uses only the public library API.
// It can be copied unchanged to the exact PR base to show the admission delta.
func TestIssue781TwoNestedSequencesPublicUnlock(t *testing.T) { //nolint:gocognit // A standalone public base/head probe checks each ordered nested term.
	for _, profile := range []struct {
		name, version string
		policy        goxsd9.LanguagePolicy
	}{
		{"Compatibility", "1.0", goxsd9.Compatibility},
		{"Strict10", "1.0", goxsd9.Strict10},
		{"Strict11", "1.1", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			xml := fmt.Sprintf(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:p="urn:issue781" targetNamespace="urn:issue781" version="%s">
<xs:complexType name="Empty"/>
<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="p:Empty"><xs:sequence>
<xs:sequence minOccurs="0" maxOccurs="2"><xs:group ref="p:First"/><xs:element ref="p:one"/></xs:sequence>
<xs:sequence minOccurs="3" maxOccurs="4"><xs:group ref="p:Second"/><xs:element ref="p:two"/></xs:sequence>
</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>
<xs:group name="First"><xs:sequence/></xs:group><xs:group name="Second"><xs:sequence/></xs:group>
<xs:element name="one" type="xs:integer"/><xs:element name="two" type="xs:integer"/>
</xs:schema>`, profile.version)
			source, err := goxsd9.NewResolvedSource(context.Background(), "issue781.xsd", io.NopCloser(strings.NewReader(xml)))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			rootName, err := goxsd9.NewQName("urn:issue781", "root")
			if err != nil {
				t.Fatal(err)
			}
			found := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
			if len(found) != 1 {
				t.Fatalf("root components = %d", len(found))
			}
			root, ok := found[0].Element()
			if !ok {
				t.Fatal("root has no element view")
			}
			owner, ok := root.InlineComplexType()
			if !ok {
				t.Fatal("root has no inline complex type")
			}
			outer, ok := owner.Particle().(goxsd9.SequenceParticle)
			if !ok || len(outer.Particles()) != 2 {
				t.Fatalf("outer sequence = %T", owner.Particle())
			}
			for index, want := range []struct{ group, element, bounds string }{{"First", "one", "0/2"}, {"Second", "two", "3/4"}} {
				nested, ok := outer.Particles()[index].(goxsd9.SequenceParticle)
				if !ok || nested.Occurrences().String() != want.bounds || len(nested.Particles()) != 2 {
					t.Fatalf("nested %d = %T", index, outer.Particles()[index])
				}
				group, groupOK := nested.Particles()[0].(goxsd9.ModelGroupReferenceParticle)
				element, elementOK := nested.Particles()[1].(goxsd9.ElementReferenceParticle)
				if !groupOK || !elementOK || group.Ref().Local() != want.group || element.Ref().Local() != want.element || group.Occurrences().String() != "1/1" || element.Occurrences().String() != "1/1" || group.TargetID().IsZero() || element.TargetID().IsZero() || group.RefLoc().IsZero() || element.RefLoc().IsZero() {
					t.Fatalf("nested %d targets = %v/%v", index, nested.Particles()[0], nested.Particles()[1])
				}
			}
		})
	}
}
