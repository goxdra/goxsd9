package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func negativeIntegerInternalSchema(t *testing.T) Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:negative" targetNamespace="urn:negative"><xs:element name="direct" type="xs:negativeInteger"/><xs:element name="named" type="r:Named"/><xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func negativeIntegerCorruptComponent(t *testing.T, schema Schema, id ComponentID, corrupt func(*Component)) Schema {
	t.Helper()
	storage := *schema.storage
	storage.components = schema.Components()
	index, ok := storage.byID[id]
	if !ok {
		t.Fatalf("component %v absent", id)
	}
	component := storage.components[index]
	corrupt(&component)
	storage.components[index] = component
	schema.storage = &storage
	return schema
}

//nolint:gocognit // Keep malformed direct and named fact exits together.
func TestNegativeIntegerRootRejectsIncompleteCompletedFacts(t *testing.T) {
	for _, test := range []struct {
		name, local string
		change      func(*testing.T, *Component)
		relatedType bool
	}{
		{"direct missing reference", "direct", func(_ *testing.T, component *Component) {
			facts := *component.element
			facts.hasTypeReference = false
			component.element = &facts
		}, false},
		{"direct wrong kind", "direct", func(_ *testing.T, component *Component) {
			facts := *component.element
			facts.typeReference.atomicKind = schemaSimpleTypeAtomicInteger
			component.element = &facts
		}, false},
		{"direct missing maximum", "direct", func(t *testing.T, component *Component) {
			facts := *component.element
			facets, ok := facts.typeReference.facets.(schemaDigitFacetVariant)
			if !ok {
				t.Fatalf("built-in digit facts = %T", facts.typeReference.facets)
			}
			facets.integerBounds = IntegerBoundFacets{}
			facts.typeReference.facets = facets
			component.element = &facts
		}, false},
		{"named missing maximum", "named", func(t *testing.T, component *Component) {
			facts := *component.simpleType
			facets, err := NewIntegerDigitFacets(nil, XSDVersion11)
			if err != nil {
				t.Fatal(err)
			}
			bounds, err := NewIntegerBoundFacets(nil, XSDVersion11)
			if err != nil {
				t.Fatal(err)
			}
			facts.facets = schemaDigitFacetVariant{value: facets, integerBounds: bounds}
			component.simpleType = &facts
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := negativeIntegerInternalSchema(t)
			declaration := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:negative", test.local))[0]
			id := declaration.ID()
			if test.relatedType {
				id = schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:negative", "Named"))[0].ID()
			}
			schema = negativeIntegerCorruptComponent(t, schema, id, func(component *Component) { test.change(t, component) })
			input := `<` + test.local + ` xmlns="urn:negative">-1</` + test.local + `>`
			err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{declaration.Loc()}
			if test.relatedType {
				wantRelated = append(wantRelated, schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:negative", "Named"))[0].Loc())
			}
			if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errInstanceValidationInvariant) {
				t.Fatalf("corrupt root diagnostic = %s related %v, want internal at root with %v", diagnostic, diagnostic.Related(), wantRelated)
			}
		})
	}
}
