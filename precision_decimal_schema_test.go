package goxsd9

import (
	"errors"
	"testing"
)

func TestStrictPrecisionDecimalPublicBoundary(t *testing.T) {
	loc := Loc{}
	negativeZero, err := ParseStrictPrecisionDecimal("-0.00", loc)
	if err != nil {
		t.Fatalf("ParseStrictPrecisionDecimal: %v", err)
	}
	positiveZero, err := ParsePrecisionDecimal("+0", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimal: %v", err)
	}
	if !negativeZero.IsNegativeZero() || !positiveZero.IsPositiveZero() || negativeZero.Compare(positiveZero) != PrecisionDecimalEqual {
		t.Fatalf("signed zero state or comparison is incorrect")
	}
	nan, err := ParsePrecisionDecimal("NaN", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimal(NaN): %v", err)
	}
	nanCopy := nan
	if nan.Compare(nanCopy) != PrecisionDecimalUnordered || nan.Equal(nanCopy) {
		t.Fatalf("NaN did not retain unordered comparison semantics")
	}
	canonical, canonicalErr := negativeZero.Canonical(6, loc)
	if canonicalErr != nil || canonical != "-0.0E0" {
		t.Fatalf("negative zero canonical = %q, %v", canonical, canonicalErr)
	}
	_, err = negativeZero.Canonical(5, loc)
	if err == nil || !errors.Is(err, ErrPrecisionDecimalCanonicalOutputLimit) {
		t.Fatalf("short canonical budget error = %v", err)
	}
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Loc() != loc {
		t.Fatalf("canonical budget diagnostic = %s, want located invalid diagnostic", diagnostic)
	}
}

func TestPrecisionDecimalPublicFacetsValidateExactValues(t *testing.T) {
	loc := Loc{}
	total, err := ParsePrecisionDecimalTotalDigitsFacet("3", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalTotalDigitsFacet: %v", err)
	}
	minScale, err := ParsePrecisionDecimalMinScaleFacet("-2", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinScaleFacet: %v", err)
	}
	pattern, err := ParsePrecisionDecimalPatternFacet("[0-9]+\\.[0-9]+", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalPatternFacet: %v", err)
	}
	facets, err := NewPrecisionDecimalFacetsFromDeclarations(
		NewPrecisionDecimalValueFacetDeclarations([]PrecisionDecimalPatternFacet{pattern}, nil, nil, nil, nil, nil, nil),
	)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacetsFromDeclarations(value): %v", err)
	}
	facets, err = RestrictPrecisionDecimalFacets(facets, NewPrecisionDecimalFacetDeclarations(&total, &minScale, nil))
	if err != nil {
		t.Fatalf("RestrictPrecisionDecimalFacets: %v", err)
	}
	if err := facets.Validate("12.3", loc); err != nil {
		t.Fatalf("facets.Validate(valid): %v", err)
	}
	if err := facets.Validate("1234", loc); err == nil {
		t.Fatal("facets.Validate accepted a value over totalDigits")
	}
	if got, ok := facets.MinScale(); !ok || got.Canonical() != "-2" {
		t.Fatalf("MinScale = %s/%t, want -2/true", got.Canonical(), ok)
	}
	if got := len(facets.PatternDeclarations()); got != 1 {
		t.Fatalf("PatternDeclarations length = %d, want 1", got)
	}
}

//nolint:gocognit // Keep the end-to-end schema boundary assertions together.
func TestSchemaBridgeIntegratesOptionalPrecisionDecimal(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.0">
  <xs:simpleType name="Base"><xs:restriction base="xs:precisionDecimal"><xs:totalDigits value="6"/><xs:minScale value="-2"/><xs:maxScale value="4"/><xs:pattern value="[0-9]+(\\.[0-9]+)?"/><xs:enumeration value="1.0"/><xs:minInclusive value="-INF"/><xs:maxExclusive value="INF"/><xs:whiteSpace value="collapse" fixed="true"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Derived"><xs:restriction base="t:Base"><xs:minScale value="-1"/></xs:restriction></xs:simpleType>
  <xs:element name="item" type="xs:precisionDecimal"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	components := schema.Components()
	if got, want := len(components), 3; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
	base, ok := components[0].SimpleType()
	if !ok || !base.HasPrecisionDecimalFacets() {
		t.Fatal("base precisionDecimal component is missing")
	}
	baseFacets := base.PrecisionDecimalFacets()
	if total, present := baseFacets.TotalDigits(); !present || total.Canonical() != "6" {
		t.Fatalf("base totalDigits = %s/%t, want 6/true", total.Canonical(), present)
	}
	if minScale, present := baseFacets.MinScale(); !present || minScale.Canonical() != "-2" {
		t.Fatalf("base minScale = %s/%t, want -2/true", minScale.Canonical(), present)
	}
	derived, ok := components[1].SimpleType()
	if !ok || !derived.HasPrecisionDecimalFacets() {
		t.Fatal("derived precisionDecimal component is missing")
	}
	if baseID, present := derived.BaseID(); !present || baseID != base.ID() {
		t.Fatalf("derived base ID = %v/%t, want %v/true", baseID, present, base.ID())
	}
	if minScale, present := derived.PrecisionDecimalFacets().MinScale(); !present || minScale.Canonical() != "-1" {
		t.Fatalf("derived minScale = %s/%t, want -1/true", minScale.Canonical(), present)
	}
	element, ok := components[2].Element()
	if !ok || element.DeclaredType().Local() != "precisionDecimal" {
		t.Fatal("direct precisionDecimal element declaration is missing")
	}
	if _, present := element.TypeID(); present {
		t.Fatal("built-in precisionDecimal received a synthetic component identity")
	}

	strict10Schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || strict10Schema.storage != nil {
		t.Fatal("Strict10 accepted precisionDecimal or returned a partial schema")
	}
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode {
		t.Fatalf("Strict10 diagnostic = %s, want version-policy invalid diagnostic", diagnostic)
	}
}
