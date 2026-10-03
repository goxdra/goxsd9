package goxsd9_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

func TestPublicParseAllBuiltinNegativeIntegerIsQueryable(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="1.1"><xs:complexType name="Record"><xs:all><xs:element name="debt" type="xs:negativeInteger"/></xs:all></xs:complexType></xs:schema>`
	source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := goxsd9.ParseSchema(source, nil)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
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
	if !ok || member.DeclaredType() != parseTestQName(t, parseTestXSDNamespace, "negativeInteger") || member.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="debt"`) {
		t.Fatalf("member = %#v, want located negativeInteger declaration", all.Members()[0])
	}
	reference, ok := member.TypeReference()
	if !ok || !reference.IsBuiltin() || reference.Loc() != publicPositiveIntegerAttributeLoc(t, root, `type="xs:negativeInteger"`) {
		t.Fatalf("reference = %#v/%t", reference, ok)
	}
	bounds, ok := reference.IntegerBounds()
	maximum, hasMaximum := bounds.MaxInclusiveFacet()
	if !ok || !hasMaximum || maximum.Value().Canonical() != "-1" {
		t.Fatalf("maxInclusive = %v/%t", maximum, hasMaximum)
	}
}
