package specs

import (
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

// This bounded projection copies the two occurrence-bearing sequence starts
// from the digest-checked XSD 1.1 source. The base, groups, and elements below
// are labeled dependency stubs; they do not represent the complete artifact.
func TestBootstrapXSD11InlinePairBoundedProjection(t *testing.T) { //nolint:gocognit // Source anchors and the ordered public result form one projection.
	entry, err := readBootstrapProbeManifest(t).Find("xsd11-schema-for-schemas")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := testDigest(raw); !strings.EqualFold(got, entry.SHA256) {
		t.Fatalf("pinned artifact digest = %s, want %s", got, entry.SHA256)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) < 249 {
		t.Fatalf("pinned artifact has %d lines", len(lines))
	}
	firstStart := strings.TrimSpace(lines[238])
	secondStart := strings.TrimSpace(lines[243])
	if firstStart != `<xs:sequence minOccurs="0">` || secondStart != `<xs:sequence minOccurs="0" maxOccurs="unbounded">` ||
		!strings.Contains(lines[239], `ref="xs:defaultOpenContent"`) || !strings.Contains(lines[244], `ref="xs:schemaTop"`) ||
		!strings.Contains(lines[240], `ref="xs:annotation"`) || !strings.Contains(lines[245], `ref="xs:annotation"`) {
		t.Fatal("pinned two-sibling source shape changed")
	}
	const dependencyStubs = `<xs:complexType name="openAttrs"/><xs:group name="defaultOpenContentStub"><xs:sequence/></xs:group><xs:group name="schemaTopStub"><xs:sequence/></xs:group><xs:element name="annotationA" type="xs:integer"/><xs:element name="annotationB" type="xs:integer"/>`
	projected := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" xmlns:p="urn:inline-pair-probe" targetNamespace="urn:inline-pair-probe" version="1.1">` +
		`<xs:element name="schema"><xs:complexType><xs:complexContent><xs:extension base="p:openAttrs"><xs:sequence>` +
		firstStart + `<xs:group ref="p:defaultOpenContentStub"/><xs:element ref="p:annotationA"/></xs:sequence>` +
		secondStart + `<xs:group ref="p:schemaTopStub"/><xs:element ref="p:annotationB" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>` +
		`</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>` + dependencyStubs + `</xs:schema>`
	id := entry.ID + ":two-nested-sequence-projection"
	var first []goxsd9.Component
	for repeat := 0; repeat < 3; repeat++ {
		schema, err := inlineProbeParse(t, id, projected, goxsd9.Strict11)
		if err != nil {
			t.Fatalf("bounded projection: %v", err)
		}
		if repeat == 0 {
			first = schema.Components()
		}
		if !reflect.DeepEqual(first, schema.Components()) {
			t.Fatal("bounded projection component order changed")
		}
		qname, err := goxsd9.NewQName("urn:inline-pair-probe", "schema")
		if err != nil {
			t.Fatal(err)
		}
		found := schema.FindKind(goxsd9.ComponentKindElementDeclaration, qname)
		if len(found) != 1 {
			t.Fatalf("projected owner components = %d", len(found))
		}
		element, ok := found[0].Element()
		if !ok {
			t.Fatal("projected owner has no element view")
		}
		owner, ok := element.InlineComplexType()
		if !ok {
			t.Fatal("projected owner has no inline type")
		}
		outer, ok := owner.Particle().(goxsd9.SequenceParticle)
		if !ok || len(outer.Particles()) != 2 {
			t.Fatalf("projected outer = %T", owner.Particle())
		}
		for index, want := range []struct{ group, element, bounds string }{
			{"defaultOpenContentStub", "annotationA", "0/1"},
			{"schemaTopStub", "annotationB", "0/unbounded"},
		} {
			nested, ok := outer.Particles()[index].(goxsd9.SequenceParticle)
			if !ok || nested.Occurrences().String() != want.bounds || len(nested.Particles()) != 2 {
				t.Fatalf("projected nested %d = %T", index, outer.Particles()[index])
			}
			group, groupOK := nested.Particles()[0].(goxsd9.ModelGroupReferenceParticle)
			element, elementOK := nested.Particles()[1].(goxsd9.ElementReferenceParticle)
			if !groupOK || !elementOK || group.Ref().Local() != want.group || element.Ref().Local() != want.element || group.RefLoc() != inlineProbeLoc(t, id, projected, `ref="p:`+want.group+`"`) || element.RefLoc() != inlineProbeLoc(t, id, projected, `ref="p:`+want.element+`"`) {
				t.Fatalf("projected nested %d refs = %v/%v", index, nested.Particles()[0], nested.Particles()[1])
			}
		}
	}
}
