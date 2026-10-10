package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func negativeIntegerInternalSchema(t *testing.T, policy LanguagePolicy) Schema {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:negative" targetNamespace="urn:negative"><xs:element name="direct" type="xs:negativeInteger"/><xs:element name="named" type="r:Named"/><xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
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

type negativeIntegerInternalCorruption struct {
	name, local string
	change      func(*testing.T, *Component)
	relatedType bool
}

func negativeIntegerInternalCorruptions(version XSDVersion) []negativeIntegerInternalCorruption {
	return []negativeIntegerInternalCorruption{
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
			facets, err := NewIntegerDigitFacets(nil, version)
			if err != nil {
				t.Fatal(err)
			}
			bounds, err := NewIntegerBoundFacets(nil, version)
			if err != nil {
				t.Fatal(err)
			}
			facts.facets = schemaDigitFacetVariant{value: facets, integerBounds: bounds}
			component.simpleType = &facts
		}, true},
		{"named decimal digit kind", "named", func(t *testing.T, component *Component) {
			facts := *component.simpleType
			facets, err := NewDecimalDigitFacets(nil, nil, version)
			if err != nil {
				t.Fatal(err)
			}
			bounds, err := NewDecimalBoundFacets(nil, version)
			if err != nil {
				t.Fatal(err)
			}
			facts.facets = schemaDigitFacetVariant{value: facets, decimalBounds: bounds}
			component.simpleType = &facts
		}, true},
	}
}

//nolint:gocognit // Keep malformed direct and named fact exits together across policies.
func TestNegativeIntegerRootRejectsIncompleteCompletedFacts(t *testing.T) {
	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{"Compatibility", Compatibility, XSDVersion11},
		{"Strict10", Strict10, XSDVersion10},
		{"Strict11", Strict11, XSDVersion11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			for _, test := range negativeIntegerInternalCorruptions(profile.version) {
				t.Run(test.name, func(t *testing.T) {
					schema := negativeIntegerInternalSchema(t, profile.policy)
					declaration := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:negative", test.local))[0]
					id := declaration.ID()
					if test.relatedType {
						id = schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:negative", "Named"))[0].ID()
					}
					schema = negativeIntegerCorruptComponent(t, schema, id, func(component *Component) { test.change(t, component) })
					declaration = schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:negative", test.local))[0]
					rootLoc := mustTestLoc(t, "instance.xml", 1, 1)
					wantRelated := []Loc{declaration.Loc()}
					if test.relatedType {
						wantRelated = append(wantRelated, schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:negative", "Named"))[0].Loc())
					}
					assertInternal := func(label string, err error) {
						t.Helper()
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticInstanceValidationCode || diagnostic.Loc() != rootLoc || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errInstanceValidationInvariant) {
							t.Fatalf("%s diagnostic = %s related %v, want internal at root with %v", label, diagnostic, diagnostic.Related(), wantRelated)
						}
					}
					element, ok := declaration.ElementDeclaration()
					if !ok {
						t.Fatal("root declaration has no element view")
					}
					plan, planErr := instanceScalarTypeFor(schema, element, rootLoc)
					if !reflect.DeepEqual(plan, instanceScalarType{}) {
						t.Fatalf("corrupt root produced partial scalar plan: %#v", plan)
					}
					assertInternal("root plan", planErr)
					input := `<` + test.local + ` xmlns="urn:negative">-1</` + test.local + `>`
					err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
					assertInternal("validation", err)
				})
			}
		})
	}
}
