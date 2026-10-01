package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep cross-policy identity, provenance, bound, and consumer checks together.
func TestSchemaGlobalNegativeIntegerElementAcrossPolicies(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := globalNegativeIntegerElementSchemaRoot(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated global negativeInteger builds changed component facts or order")
			}

			matches := first.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "value"))
			if len(matches) != 1 {
				t.Fatalf("global value matches = %d, want 1", len(matches))
			}
			component := matches[0]
			if component.ID().IsZero() || component.ID().Source() != "root.xsd" || component.ID().Ordinal() != 1 {
				t.Fatalf("global value identity = %v, want root.xsd ordinal 1", component.ID())
			}
			declaration, ok := component.ElementDeclaration()
			if !ok {
				t.Fatal("global value has no element declaration view")
			}
			if declaration.ID() != component.ID() || declaration.Name() != mustTestQName(t, "urn:test", "value") {
				t.Fatalf("global value declaration identity/name = %v/%q, want component identity/{urn:test}value", declaration.ID(), declaration.Name())
			}
			wantDeclarationLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element name="value"`)
			if declaration.Loc() != wantDeclarationLoc {
				t.Fatalf("global value declaration location = %s, want %s", declaration.Loc(), wantDeclarationLoc)
			}
			if declaration.DeclaredType() != mustTestQName(t, testXSDNamespace, "negativeInteger") {
				t.Fatalf("global value declared type = %q, want xs:negativeInteger", declaration.DeclaredType())
			}
			if typeID, hasTypeID := declaration.TypeID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("global value type ownership = %v/%t, want zero/false", typeID, hasTypeID)
			}

			reference, ok := declaration.TypeReference()
			if !ok {
				t.Fatal("global value type reference is missing")
			}
			wantTypeLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:negativeInteger"`)
			if !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "negativeInteger") || reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantTypeLoc {
				t.Fatalf("global value type reference = kind %q/name %q/loc %s/variety %s, want built-in negativeInteger at %s", reference.Kind(), reference.Name(), reference.Loc(), reference.VarietyLoc(), wantTypeLoc)
			}
			if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
				t.Fatalf("global value built-in ownership = %v/%t, want zero/false", typeID, hasTypeID)
			}
			if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicNegativeInteger {
				t.Fatalf("global value atomic kind = %#v, want negativeInteger", reference.facts)
			}
			facets, ok := reference.facts.facets.(schemaDigitFacetVariant)
			if !ok {
				t.Fatalf("global value facets = %T, want digit facets", reference.facts.facets)
			}
			if facets.value.Kind() != DigitDatatypeInteger || facets.value.Version() != profile.version {
				t.Fatalf("global value digit facts = %q/%q, want integer/%q", facets.value.Kind(), facets.value.Version(), profile.version)
			}
			maximum, present := facets.integerBounds.MaxInclusive()
			if !present || maximum.Canonical() != "-1" {
				t.Fatalf("global value maxInclusive = %q/%t, want -1/true", maximum.Canonical(), present)
			}
			maximumFacet, present := facets.integerBounds.MaxInclusiveFacet()
			if !present || maximumFacet.Kind() != BoundMaxInclusive || maximumFacet.Value().Canonical() != "-1" || maximumFacet.Loc() != wantTypeLoc || maximumFacet.Version() != profile.version {
				t.Fatalf("global value maxInclusive facts = %q/%s/%q/%t, want -1/%s/%q/true", maximumFacet.Value().Canonical(), maximumFacet.Loc(), maximumFacet.Kind(), present, wantTypeLoc, profile.version)
			}

			output, err := GenerateGo(first, "generated")
			if output != nil || err == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", output, err)
			}
			codegenDiagnostic := requireDiagnostic(t, err)
			if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Code() != diagnosticCodegenUnsupported || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want explicit unsupported", codegenDiagnostic)
			}

			validationErr := ValidateInstance(first, "instance.xml", io.NopCloser(strings.NewReader(`<value xmlns="urn:test">-1</value>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted a negativeInteger global element")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("ValidateInstance diagnostic = %s, want explicit unsupported", validationDiagnostic)
			}
		})
	}
}

func globalNegativeIntegerElementSchemaRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="value" type="xs:negativeInteger"/>
</xs:schema>`
}
