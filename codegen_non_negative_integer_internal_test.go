package goxsd9

import (
	"errors"
	"testing"
)

//nolint:gocognit // Keep stale plan and schema fact rejection cases together.
func TestCodegenScalarNonNegativeIntegerRejectsStalePlanAndSchemaFacts(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		mutate func(*testing.T, Schema, *codegenSourcePlan)
	}{
		{
			name: "plan scalar kind",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:element name="value" type="xs:nonNegativeInteger"/></xs:schema>`,
			mutate: func(_ *testing.T, _ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].target.scalarKind = codegenSourceScalarInteger
			},
		},
		{
			name: "built-in lower bound",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:element name="value" type="xs:nonNegativeInteger"/></xs:schema>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan) {
				mutateCodegenNonNegativeIntegerBounds(t, &schema.Components()[0].element.typeReference.facets, "-1")
			},
		},
		{
			name: "named lower bound",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:element name="value" type="t:Value"/><xs:simpleType name="Value"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`,
			mutate: func(t *testing.T, schema Schema, _ *codegenSourcePlan) {
				mutateCodegenNonNegativeIntegerBounds(t, &schema.Components()[1].simpleType.facets, "-1")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, test.root, nil)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			plan, err := planCodegenSource(schema, mustScalarCodegenNaming(t, schema))
			if err != nil {
				t.Fatalf("planCodegenSource: %v", err)
			}
			test.mutate(t, schema, &plan)
			output, err := renderCodegenSource(plan, schema)
			if output != nil || err == nil {
				t.Fatalf("stale nonNegativeInteger result = (%q, %v), want nil output and error", output, err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
				t.Fatalf("diagnostic = %s, want internal codegen invariant %s", diagnostic, diagnosticCodegenInvariant)
			}
			if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
				t.Fatalf("diagnostic location = %s, want a root.xsd location", diagnostic.Loc())
			}
			if !errors.Is(err, errCodegenSchemaInvariant) {
				t.Fatalf("stale nonNegativeInteger error lost its internal cause: %v", err)
			}
		})
	}
}

func mutateCodegenNonNegativeIntegerBounds(t *testing.T, facets *schemaSimpleTypeFacetVariant, lexical string) {
	t.Helper()
	minimum, err := ParseIntegerMinInclusiveFacet(lexical, mustTestLoc(t, "root.xsd", 1, 1), XSDVersion11)
	if err != nil {
		t.Fatalf("ParseIntegerMinInclusiveFacet: %v", err)
	}
	bounds, err := NewIntegerBoundFacets([]IntegerBoundFacet{minimum}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets: %v", err)
	}
	switch typed := (*facets).(type) {
	case schemaDigitFacetVariant:
		typed.integerBounds = bounds
		*facets = typed
	case schemaIntegerFacetVariant:
		typed.bounds = bounds
		*facets = typed
	default:
		t.Fatalf("nonNegativeInteger facets = %T, want integer facts", *facets)
	}
}
