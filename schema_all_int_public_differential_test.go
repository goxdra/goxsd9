package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // One exported-API fixture must distinguish the exact-base behavior.
func TestDirectAllIntPublicDifferential(t *testing.T) {
	const document = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:r="urn:all" targetNamespace="urn:all">
<xs:simpleType name="Narrow"><xs:restriction base="xs:int"><xs:minInclusive value="-7"/><xs:maxInclusive value="9"/></xs:restriction></xs:simpleType>
<xs:complexType name="Record"><xs:all><xs:element name="builtin" type="xs:int"/><xs:element name="named" type="r:Narrow"/></xs:all></xs:complexType>
</xs:schema>`
	source, err := goxsd9.NewResolvedSource(context.Background(), "differential.xsd", io.NopCloser(strings.NewReader(document)))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := goxsd9.ParseSchema(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	name, err := goxsd9.NewQName("urn:all", "Record")
	if err != nil {
		t.Fatal(err)
	}
	matches := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, name)
	if len(matches) != 1 {
		t.Fatalf("Record matches = %d", len(matches))
	}
	definition, ok := matches[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record has no complex type view")
	}
	all, ok := definition.Particle().(goxsd9.AllParticle)
	if !ok {
		t.Fatalf("Record particle = %T", definition.Particle())
	}
	members := all.Members()
	if len(members) != 2 {
		t.Fatalf("member count = %d", len(members))
	}
	for index, want := range []struct {
		local, typeLocal, minimum, maximum, typeToken string
		named                                         bool
	}{
		{"builtin", "int", "-2147483648", "2147483647", `type="xs:int"`, false},
		{"named", "Narrow", "-7", "9", `type="r:Narrow"`, true},
	} {
		member, ok := members[index].(goxsd9.ElementParticle)
		if !ok || member.Name().Local() != want.local || member.DeclaredType().Local() != want.typeLocal {
			t.Fatalf("member %d = %#v", index, members[index])
		}
		reference, ok := member.TypeReference()
		if !ok || reference.Name().Local() != want.typeLocal || reference.IsNamed() != want.named || reference.IsBuiltin() == want.named {
			t.Fatalf("member %d type = %#v", index, reference)
		}
		line := strings.Split(document, "\n")[2]
		if reference.Loc().Source() != "differential.xsd" || reference.Loc().Line() != 3 || reference.Loc().Column() != strings.Index(line, want.typeToken)+1 {
			t.Fatalf("member %d type location = %s", index, reference.Loc())
		}
		bounds, ok := reference.IntegerBounds()
		if !ok {
			t.Fatalf("member %d has no integer bounds", index)
		}
		minimum, hasMinimum := bounds.MinInclusiveFacet()
		maximum, hasMaximum := bounds.MaxInclusiveFacet()
		if !hasMinimum || !hasMaximum || minimum.Value().Canonical() != want.minimum || maximum.Value().Canonical() != want.maximum {
			t.Fatalf("member %d bounds = %v/%v", index, minimum, maximum)
		}
		id, hasID := reference.ComponentID()
		if hasID != want.named || hasID && id.IsZero() || !want.named && (!minimum.Loc().IsZero() || !maximum.Loc().IsZero()) {
			t.Fatalf("member %d identity/provenance = %v at %s/%s", index, id, minimum.Loc(), maximum.Loc())
		}
	}
}
