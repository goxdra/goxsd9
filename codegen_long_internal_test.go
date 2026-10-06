package goxsd9

import (
	"errors"
	"testing"
)

//nolint:gocognit,funlen // Keep the policy and malformed-fact matrix at one render boundary.
func TestCodegenLongRejectsStaleAndMalformedFactsAcrossPolicies(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		mutate func(*testing.T, Schema, *codegenSourcePlan, XSDVersion)
	}{
		{
			name: "stale scalar plan",
			root: `<xs:element name="value" type="xs:long"/>`,
			mutate: func(_ *testing.T, _ Schema, plan *codegenSourcePlan, _ XSDVersion) {
				plan.declarations[0].target.scalarKind = codegenSourceScalarInteger
			},
		},
		{
			name: "built-in missing upper bound",
			root: `<xs:element name="value" type="xs:long"/>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan, version XSDVersion) {
				minimum, err := ParseIntegerMinInclusiveFacet(codegenLongMinimum, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum}, version)
				if err != nil {
					t.Fatal(err)
				}
				facets, ok := schema.Components()[0].element.typeReference.facets.(schemaDigitFacetVariant)
				if !ok {
					t.Fatal("built-in long facet variant changed")
				}
				facets.integerBounds = bounds
				schema.Components()[0].element.typeReference.facets = facets
			},
		},
		{
			name: "named relaxed lower bound",
			root: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:long"/></xs:simpleType>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan, version XSDVersion) {
				minimum, err := ParseIntegerMinInclusiveFacet("-9223372036854775809", Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				maximum, err := ParseIntegerMaxInclusiveFacet(codegenLongMaximum, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum, maximum}, version)
				if err != nil {
					t.Fatal(err)
				}
				facets, ok := schema.Components()[1].simpleType.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatal("named long facet variant changed")
				}
				facets.bounds = bounds
				schema.Components()[1].simpleType.facets = facets
			},
		},
		{
			name: "named reference facets diverge",
			root: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:long"/></xs:simpleType>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan, version XSDVersion) {
				maximum, err := ParseIntegerMaxInclusiveFacet("9", Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				minimum, err := ParseIntegerMinInclusiveFacet(codegenLongMinimum, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum, maximum}, version)
				if err != nil {
					t.Fatal(err)
				}
				facets, ok := schema.Components()[0].element.typeReference.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatal("named long reference facet variant changed")
				}
				facets.bounds = bounds
				schema.Components()[0].element.typeReference.facets = facets
			},
		},
		{
			name: "named malformed digit facts",
			root: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:long"/></xs:simpleType>`,
			mutate: func(_ *testing.T, schema Schema, _ *codegenSourcePlan, _ XSDVersion) {
				facets, ok := schema.Components()[1].simpleType.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatal("named long facet variant changed")
				}
				facets.digits.kind = DigitDatatypeDecimal
				schema.Components()[1].simpleType.facets = facets
			},
		},
	}
	for _, profile := range longPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + test.root + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := planCodegenSource(schema, mustScalarCodegenNaming(t, schema))
				if err != nil {
					t.Fatal(err)
				}
				test.mutate(t, schema, &plan, profile.version)
				output, err := renderCodegenSource(plan, schema)
				if output != nil || err == nil {
					t.Fatalf("renderCodegenSource = (%q, %v), want nil output and error", output, err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != schema.Components()[0].Loc() || !errors.Is(err, errCodegenSchemaInvariant) {
					t.Fatalf("diagnostic = %s, want internal GOXSD9030 at first element with cause", diagnostic)
				}
				if test.name != "stale scalar plan" && diagnostic.SpecRef() != codegenLongSpecRef(profile.version) {
					t.Fatalf("SpecRef = %q, want %q", diagnostic.SpecRef(), codegenLongSpecRef(profile.version))
				}
			})
		}
	}
}
