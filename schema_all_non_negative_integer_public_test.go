package goxsd9_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Check public parsing and copied intrinsic facts under each policy.
func TestPublicParseAllBuiltinNonNegativeIntegerIsQueryable(t *testing.T) {
	for _, profile := range []struct {
		name, schemaVersion string
		policy              goxsd9.LanguagePolicy
	}{
		{"compatibility", "1.1", goxsd9.Compatibility},
		{"strict10", "1.0", goxsd9.Strict10},
		{"strict11", "1.1", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + profile.schemaVersion + `"><xs:complexType name="Record"><xs:all><xs:element name="debt" type="xs:nonNegativeInteger"/></xs:all></xs:complexType></xs:schema>`
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
			if !ok || member.DeclaredType() != parseTestQName(t, parseTestXSDNamespace, "nonNegativeInteger") || member.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="debt"`) {
				t.Fatalf("member = %#v, want located nonNegativeInteger declaration", all.Members()[0])
			}
			reference, ok := member.TypeReference()
			if !ok || !reference.IsBuiltin() || reference.Loc() != publicPositiveIntegerAttributeLoc(t, root, `type="xs:nonNegativeInteger"`) {
				t.Fatalf("reference = %#v/%t", reference, ok)
			}
			bounds, ok := reference.IntegerBounds()
			minimum, hasMinimum := bounds.MinInclusiveFacet()
			if !ok || !hasMinimum || minimum.Value().Canonical() != "0" || !minimum.Loc().IsZero() {
				t.Fatalf("minInclusive = %v/%t", minimum, hasMinimum)
			}
		})
	}
}

//nolint:gocognit // Exercise the exact public parse, query, and consumer contract.
func TestPublicParseAllNamedNonNegativeIntegerIsQueryable(t *testing.T) {
	for _, profile := range []struct {
		name, schemaVersion string
		policy              goxsd9.LanguagePolicy
	}{
		{"compatibility", "1.1", goxsd9.Compatibility},
		{"strict10", "1.0", goxsd9.Strict10},
		{"strict11", "1.1", goxsd9.Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:r="urn:all" targetNamespace="urn:all" version="` + profile.schemaVersion + `"><xs:complexType name="Record"><xs:all><xs:element name="count" type="r:Count"/></xs:all></xs:complexType><xs:simpleType name="Count"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType><xs:element name="root" type="r:Record"/></xs:schema>`
			source, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if err != nil {
				t.Fatal(err)
			}
			schema, err := goxsd9.ParseSchemaWithPolicy(source, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			definitions := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, parseTestQName(t, "urn:all", "Record"))
			if len(definitions) != 1 {
				t.Fatalf("complex definitions = %d", len(definitions))
			}
			definition, ok := definitions[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("missing complex type view")
			}
			all, ok := definition.Particle().(goxsd9.AllParticle)
			if !ok || len(all.Members()) != 1 {
				t.Fatalf("all = %#v", definition.Particle())
			}
			member, ok := all.Members()[0].(goxsd9.ElementParticle)
			if !ok || member.DeclaredType() != parseTestQName(t, "urn:all", "Count") || member.Loc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:element name="count"`) {
				t.Fatalf("member = %#v", all.Members()[0])
			}
			ref, ok := member.TypeReference()
			id, hasID := member.TypeID()
			refID, hasRefID := ref.ComponentID()
			if !ok || !ref.IsNamed() || ref.Name() != member.DeclaredType() || ref.Loc() != publicPositiveIntegerAttributeLoc(t, root, `type="r:Count"`) || ref.VarietyLoc() != publicPositiveIntegerAttributeLoc(t, root, `<xs:restriction base="xs:nonNegativeInteger"`) || !hasID || !hasRefID || id != refID || id != schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, member.DeclaredType())[0].ID() {
				t.Fatalf("named reference = %#v/%v/%v", ref, id, refID)
			}
			bounds, ok := ref.IntegerBounds()
			minimum, hasMinimum := bounds.MinInclusiveFacet()
			if !ok || !hasMinimum || minimum.Value().Canonical() != "3" || minimum.Loc() != publicPositiveIntegerAttributeLoc(t, root, `value="3"`) {
				t.Fatalf("minimum = %v", minimum)
			}
			members := all.Members()
			members[0] = nil
			if _, ok := definition.Particle().(goxsd9.AllParticle).Members()[0].(goxsd9.ElementParticle); !ok {
				t.Fatal("caller changed public all members")
			}
			instanceErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:all"/>`)))
			var diagnostic goxsd9.Diagnostic
			if !errors.As(instanceErr, &diagnostic) || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Loc() != all.Loc() {
				t.Fatalf("validation = %v", instanceErr)
			}
			output, generationErr := goxsd9.GenerateGo(schema, "generated")
			if output != nil || !errors.As(generationErr, &diagnostic) || diagnostic.Loc() != all.Loc() {
				t.Fatalf("generation = %v/%v", output, generationErr)
			}
		})
	}
}
