package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the policy and malformed-fact matrix at one render boundary.
func testCodegenBoundedIntegerRejectsStaleMalformedFactsAcrossPolicies(t *testing.T, kind codegenBoundedIntegerKind, tooLow string) {
	tests := []struct {
		name   string
		root   string
		mutate func(*testing.T, Schema, *codegenSourcePlan, XSDVersion)
	}{
		{
			name: "stale scalar plan",
			root: `<xs:element name="value" type="xs:byte"/>`,
			mutate: func(_ *testing.T, _ Schema, plan *codegenSourcePlan, _ XSDVersion) {
				plan.declarations[0].target.scalarKind = codegenSourceScalarInteger
			},
		},
		{
			name: "built-in missing upper bound",
			root: `<xs:element name="value" type="xs:byte"/>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan, version XSDVersion) {
				minimum, err := ParseIntegerMinInclusiveFacet(kind.minimum, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum}, version)
				if err != nil {
					t.Fatal(err)
				}
				facets, ok := schema.Components()[0].element.typeReference.facets.(schemaDigitFacetVariant)
				if !ok {
					t.Fatal("built-in byte facet variant changed")
				}
				facets.integerBounds = bounds
				schema.Components()[0].element.typeReference.facets = facets
			},
		},
		{
			name: "named relaxed lower bound",
			root: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan, version XSDVersion) {
				minimum, err := ParseIntegerMinInclusiveFacet(tooLow, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				maximum, err := ParseIntegerMaxInclusiveFacet(kind.maximum, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum, maximum}, version)
				if err != nil {
					t.Fatal(err)
				}
				facets, ok := schema.Components()[1].simpleType.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatal("named byte facet variant changed")
				}
				facets.bounds = bounds
				schema.Components()[1].simpleType.facets = facets
			},
		},
		{
			name: "named reference facets diverge",
			root: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan, version XSDVersion) {
				maximum, err := ParseIntegerMaxInclusiveFacet("9", Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				minimum, err := ParseIntegerMinInclusiveFacet(kind.minimum, Loc{}, version)
				if err != nil {
					t.Fatal(err)
				}
				bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum, maximum}, version)
				if err != nil {
					t.Fatal(err)
				}
				facets, ok := schema.Components()[0].element.typeReference.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatal("named byte reference facet variant changed")
				}
				facets.bounds = bounds
				schema.Components()[0].element.typeReference.facets = facets
			},
		},
		{
			name: "named malformed digit facts",
			root: `<xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:byte"/></xs:simpleType>`,
			mutate: func(_ *testing.T, schema Schema, _ *codegenSourcePlan, _ XSDVersion) {
				facets, ok := schema.Components()[1].simpleType.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatal("named byte facet variant changed")
				}
				facets.digits.kind = DigitDatatypeDecimal
				schema.Components()[1].simpleType.facets = facets
			},
		},
	}
	for _, profile := range longPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + strings.ReplaceAll(test.root, "xs:byte", "xs:"+kind.name) + `</xs:schema>`
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
				if test.name != "stale scalar plan" && diagnostic.SpecRef() != codegenBoundedIntegerSpecRef(profile.version, kind) {
					t.Fatalf("SpecRef = %q, want %q", diagnostic.SpecRef(), codegenBoundedIntegerSpecRef(profile.version, kind))
				}
				var wantRelated []Loc
				switch test.name {
				case "named relaxed lower bound", "named malformed digit facts":
					wantRelated = []Loc{schema.Components()[1].Loc(), elementReferenceTestAttributeLoc(t, root, `base="xs:`+kind.name+`"`)}
				case "named reference facets diverge":
					wantRelated = []Loc{schema.Components()[1].Loc(), elementReferenceTestAttributeLoc(t, root, `type="t:Value"`)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("Related = %v, want %v", diagnostic.Related(), wantRelated)
				}
			})
		}
	}
}

func TestCodegenByteRejectsStaleAndMalformedFactsAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsStaleMalformedFactsAcrossPolicies(t, codegenByteKind(), "-129")
}

//nolint:gocognit // Verify both named consumers at the shared bounded-facet boundary.
func testCodegenBoundedIntegerRejectsNamedDigitVariantAcrossPolicies(t *testing.T, kind codegenBoundedIntegerKind) {
	for _, profile := range longPolicyProfiles() {
		for _, consumer := range []string{"standalone", "named element"} {
			t.Run(profile.name+"/"+consumer, func(t *testing.T) {
				element := ""
				if consumer == "named element" {
					element = `<xs:element name="value" type="t:Value"/>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `">` + element + `<xs:simpleType name="Value"><xs:restriction base="xs:` + kind.name + `"><xs:totalDigits value="2"/><xs:enumeration value="12"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				valueIndex := 0
				if consumer == "named element" {
					valueIndex = 1
				}
				value := components[valueIndex]
				facets, ok := value.simpleType.facets.(schemaIntegerFacetVariant)
				if !ok {
					t.Fatalf("named %s facets = %T, want integer facet variant", kind.name, value.simpleType.facets)
				}
				malformed := schemaDigitFacetVariant{value: facets.digits, integerBounds: facets.bounds}
				value.simpleType.facets = malformed
				if consumer == "named element" {
					components[0].element.typeReference.facets = malformed
				}
				output, generationErr := GenerateGo(schema, "generated")
				if output != nil || generationErr == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil output and malformed-fact error", output, generationErr)
				}
				diagnostic := requireDiagnostic(t, generationErr)
				primary := value.Loc()
				wantRelated := []Loc{
					elementReferenceTestAttributeLoc(t, root, `base="xs:`+kind.name+`"`),
					elementReferenceTestAttributeLoc(t, root, `value="2"`),
				}
				if consumer == "named element" {
					primary = components[0].Loc()
					wantRelated = append([]Loc{value.Loc()}, wantRelated...)
				}
				if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != primary || diagnostic.SpecRef() != codegenBoundedIntegerSpecRef(profile.version, kind) || !errors.Is(generationErr, errCodegenSchemaInvariant) {
					t.Fatalf("diagnostic = %s, want located GOXSD9030 with preserved cause and edition SpecRef", diagnostic)
				}
				if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("Related = %v, want %v", diagnostic.Related(), wantRelated)
				}
			})
		}
	}
}

func TestCodegenByteRejectsNamedDigitVariantAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsNamedDigitVariantAcrossPolicies(t, codegenByteKind())
}
