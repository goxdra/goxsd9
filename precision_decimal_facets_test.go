package goxsd9

import (
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

func TestPrecisionDecimalFacetIntegerDomainsAreExactAndLocated(t *testing.T) {
	totalLoc := mustPrecisionDecimalFacetLoc(t, "total.xsd", 4, 2)
	minLoc := mustPrecisionDecimalFacetLoc(t, "min.xsd", 5, 3)
	maxLoc := mustPrecisionDecimalFacetLoc(t, "max.xsd", 6, 4)
	huge := strings.Repeat("9", 1024)

	assertPrecisionDecimalExactTotalDigits(t, totalLoc, huge)
	assertPrecisionDecimalExactScales(t, minLoc, maxLoc, huge)
	assertPrecisionDecimalInvalidIntegerDomains(t, totalLoc, minLoc, maxLoc)
}

func assertPrecisionDecimalExactTotalDigits(t *testing.T, loc Loc, huge string) {
	t.Helper()
	total, err := ParsePrecisionDecimalTotalDigits(" +000"+huge, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalTotalDigits: %v", err)
	}
	if got := total.Canonical(); got != huge {
		t.Fatalf("totalDigits = %q, want %q", got, huge)
	}
}

func assertPrecisionDecimalExactScales(t *testing.T, minLoc, maxLoc Loc, huge string) {
	t.Helper()
	for _, test := range []struct {
		name      string
		lexical   string
		canonical string
	}{
		{name: "negative", lexical: "-98765432101234567890", canonical: "-98765432101234567890"},
		{name: "positive", lexical: "+42", canonical: "42"},
		{name: "zero", lexical: "0", canonical: "0"},
		{name: "negative zero", lexical: "-0", canonical: "0"},
		{name: "huge negative", lexical: "-" + huge, canonical: "-" + huge},
	} {
		t.Run("minScale "+test.name, func(t *testing.T) {
			value, parseErr := ParsePrecisionDecimalMinScale(test.lexical, minLoc)
			if parseErr != nil {
				t.Fatalf("ParsePrecisionDecimalMinScale(%q): %v", test.lexical, parseErr)
			}
			if got := value.Canonical(); got != test.canonical {
				t.Fatalf("minScale = %q, want %q", got, test.canonical)
			}
		})
		t.Run("maxScale "+test.name, func(t *testing.T) {
			value, parseErr := ParsePrecisionDecimalMaxScale(test.lexical, maxLoc)
			if parseErr != nil {
				t.Fatalf("ParsePrecisionDecimalMaxScale(%q): %v", test.lexical, parseErr)
			}
			if got := value.Canonical(); got != test.canonical {
				t.Fatalf("maxScale = %q, want %q", got, test.canonical)
			}
		})
	}
}

func assertPrecisionDecimalInvalidIntegerDomains(t *testing.T, totalLoc, minLoc, maxLoc Loc) {
	t.Helper()
	for _, test := range []struct {
		name    string
		lexical string
		code    string
		ref     string
		cause   error
		loc     Loc
		parse   func(string, Loc) (StrictInteger, error)
	}{
		{name: "total empty", lexical: "", code: InvalidPrecisionDecimalTotalDigitsCode, ref: "xsd-precisionDecimal#rf-totalDigits", cause: errInvalidPrecisionDecimalTotalDigitsValue, loc: totalLoc, parse: ParsePrecisionDecimalTotalDigits},
		{name: "total zero", lexical: "0", code: InvalidPrecisionDecimalTotalDigitsCode, ref: "xsd-precisionDecimal#rf-totalDigits", cause: errInvalidPrecisionDecimalTotalDigitsValue, loc: totalLoc, parse: ParsePrecisionDecimalTotalDigits},
		{name: "total negative", lexical: "-1", code: InvalidPrecisionDecimalTotalDigitsCode, ref: "xsd-precisionDecimal#rf-totalDigits", cause: errInvalidPrecisionDecimalTotalDigitsValue, loc: totalLoc, parse: ParsePrecisionDecimalTotalDigits},
		{name: "total signed zero", lexical: "-0", code: InvalidPrecisionDecimalTotalDigitsCode, ref: "xsd-precisionDecimal#rf-totalDigits", cause: errInvalidPrecisionDecimalTotalDigitsValue, loc: totalLoc, parse: ParsePrecisionDecimalTotalDigits},
		{name: "total non-integer", lexical: "1.0", code: InvalidPrecisionDecimalTotalDigitsCode, ref: "xsd-precisionDecimal#rf-totalDigits", cause: errInvalidPrecisionDecimalTotalDigitsValue, loc: totalLoc, parse: ParsePrecisionDecimalTotalDigits},
		{name: "min malformed", lexical: "1.0", code: InvalidPrecisionDecimalMinScaleCode, ref: "xsd-precisionDecimal#f-mns-value", cause: errInvalidPrecisionDecimalMinScaleValue, loc: minLoc, parse: ParsePrecisionDecimalMinScale},
		{name: "max malformed", lexical: "1.0", code: InvalidPrecisionDecimalMaxScaleCode, ref: "xsd-precisionDecimal#f-ms-value", cause: errInvalidPrecisionDecimalMaxScaleValue, loc: maxLoc, parse: ParsePrecisionDecimalMaxScale},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, parseErr := test.parse(test.lexical, test.loc)
			assertPrecisionDecimalFacetDiagnostic(t, parseErr, test.code, test.loc, test.ref)
			if !errors.Is(parseErr, test.cause) {
				t.Fatalf("error does not preserve %v: %v", test.cause, parseErr)
			}
		})
	}
}

func TestPrecisionDecimalFacetDeclarationsKeepLocalAndEffectiveStateSeparate(t *testing.T) {
	totalLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 10, 2)
	minLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 11, 2)
	maxLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 12, 2)
	childMinLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 20, 2)

	baseTotal := mustPrecisionDecimalTotalFacet(t, "11", totalLoc, false)
	baseMin := mustPrecisionDecimalMinFacet(t, "-5", minLoc, false)
	baseMax := mustPrecisionDecimalMaxFacet(t, "8", maxLoc, false)
	base, err := NewPrecisionDecimalFacets(baseTotal, baseMin, baseMax)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets: %v", err)
	}

	empty, err := NewPrecisionDecimalFacets(nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets(empty): %v", err)
	}
	assertEmptyPrecisionDecimalFacets(t, empty)

	childMin := mustPrecisionDecimalMinFacet(t, "-2", childMinLoc, false)
	child, err := RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(nil, childMin, nil))
	if err != nil {
		t.Fatalf("RestrictPrecisionDecimalFacets: %v", err)
	}
	if value, ok := child.TotalDigits(); !ok || value.Canonical() != "11" {
		t.Fatalf("inherited totalDigits = (%s, %t), want 11,true", value.Canonical(), ok)
	}
	if value, ok := child.MinScale(); !ok || value.Canonical() != "-2" {
		t.Fatalf("local minScale = (%s, %t), want -2,true", value.Canonical(), ok)
	}
	if value, ok := child.MaxScale(); !ok || value.Canonical() != "8" {
		t.Fatalf("inherited maxScale = (%s, %t), want 8,true", value.Canonical(), ok)
	}
	if loc, ok := child.TotalDigitsLoc(); !ok || loc != totalLoc {
		t.Fatalf("inherited totalDigits Loc() = (%v, %t), want %v,true", loc, ok, totalLoc)
	}
	if loc, ok := child.MaxScaleLoc(); !ok || loc != maxLoc {
		t.Fatalf("inherited maxScale Loc() = (%v, %t), want %v,true", loc, ok, maxLoc)
	}

	minAboveTotal := mustPrecisionDecimalMinFacet(t, "100", childMinLoc, false)
	maxAboveTotal := mustPrecisionDecimalMaxFacet(t, "100", maxLoc, false)
	if _, err := NewPrecisionDecimalFacets(baseTotal, minAboveTotal, maxAboveTotal); err != nil {
		t.Fatalf("minScale greater than totalDigits rejected: %v", err)
	}
}

func assertEmptyPrecisionDecimalFacets(t *testing.T, facets PrecisionDecimalFacets) {
	t.Helper()
	if facets.HasTotalDigits() || facets.HasMinScale() || facets.HasMaxScale() {
		t.Fatal("omitted local facets unexpectedly became present")
	}
}

func TestPrecisionDecimalFacetRestrictionsAreMonotonicAndLocated(t *testing.T) {
	baseTotalLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 10, 2)
	baseMinLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 11, 2)
	baseMaxLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 12, 2)
	childTotalLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 20, 2)
	childMinLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 21, 2)
	childMaxLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 22, 2)

	baseTotal := mustPrecisionDecimalTotalFacet(t, "10", baseTotalLoc, false)
	baseMin := mustPrecisionDecimalMinFacet(t, "-5", baseMinLoc, false)
	baseMax := mustPrecisionDecimalMaxFacet(t, "8", baseMaxLoc, false)
	base, err := NewPrecisionDecimalFacets(baseTotal, baseMin, baseMax)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets: %v", err)
	}

	childTotal := mustPrecisionDecimalTotalFacet(t, "9", childTotalLoc, false)
	childMin := mustPrecisionDecimalMinFacet(t, "-4", childMinLoc, false)
	childMax := mustPrecisionDecimalMaxFacet(t, "7", childMaxLoc, false)
	child, err := RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(childTotal, childMin, childMax))
	if err != nil {
		t.Fatalf("narrowing restriction: %v", err)
	}
	if total, ok := child.TotalDigits(); !ok || total.Canonical() != "9" {
		t.Fatalf("narrowed totalDigits = (%s, %t), want 9,true", total.Canonical(), ok)
	}
	if minValue, ok := child.MinScale(); !ok || minValue.Canonical() != "-4" {
		t.Fatalf("narrowed minScale = (%s, %t), want -4,true", minValue.Canonical(), ok)
	}
	if maxValue, ok := child.MaxScale(); !ok || maxValue.Canonical() != "7" {
		t.Fatalf("narrowed maxScale = (%s, %t), want 7,true", maxValue.Canonical(), ok)
	}

	tooBroadTotal := mustPrecisionDecimalTotalFacet(t, "11", childTotalLoc, false)
	_, err = RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(tooBroadTotal, nil, nil))
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalFacetRestrictionCode, childTotalLoc, "xsd11-datatypes#totalDigits-valid-restriction")
	assertPrecisionDecimalRelated(t, err, baseTotalLoc)
	if !errors.Is(err, errInvalidPrecisionDecimalFacetRestriction) {
		t.Fatalf("totalDigits restriction does not preserve cause: %v", err)
	}

	tooBroadMin := mustPrecisionDecimalMinFacet(t, "-6", childMinLoc, false)
	_, err = RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(nil, tooBroadMin, nil))
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalFacetRestrictionCode, childMinLoc, "xsd-precisionDecimal#minScale-valid-restriction")
	assertPrecisionDecimalRelated(t, err, baseMinLoc)

	tooBroadMax := mustPrecisionDecimalMaxFacet(t, "9", childMaxLoc, false)
	_, err = RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(nil, nil, tooBroadMax))
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalFacetRestrictionCode, childMaxLoc, "xsd-precisionDecimal#maxScale-valid-restriction")
	assertPrecisionDecimalRelated(t, err, baseMaxLoc)
}

func TestPrecisionDecimalFacetFixedValuesSurviveInheritance(t *testing.T) {
	baseTotalLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 30, 2)
	baseMinLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 31, 2)
	baseMaxLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 32, 2)
	childTotalLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 40, 2)
	childMinLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 41, 2)
	childMaxLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 42, 2)

	baseTotal := mustPrecisionDecimalTotalFacet(t, "10", baseTotalLoc, true)
	baseMin := mustPrecisionDecimalMinFacet(t, "-5", baseMinLoc, true)
	baseMax := mustPrecisionDecimalMaxFacet(t, "8", baseMaxLoc, true)
	base, err := NewPrecisionDecimalFacets(baseTotal, baseMin, baseMax)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets: %v", err)
	}

	equalTotal := mustPrecisionDecimalTotalFacet(t, "10", childTotalLoc, false)
	equalMin := mustPrecisionDecimalMinFacet(t, "-5", childMinLoc, false)
	equalMax := mustPrecisionDecimalMaxFacet(t, "8", childMaxLoc, false)
	equal, err := RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(equalTotal, equalMin, equalMax))
	if err != nil {
		t.Fatalf("equal fixed redeclaration: %v", err)
	}
	assertPrecisionDecimalFacetsFixed(t, equal)

	inherited, err := RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{})
	if err != nil {
		t.Fatalf("inherited fixed facets: %v", err)
	}
	assertPrecisionDecimalFacetsFixed(t, inherited)

	for _, test := range []struct {
		name     string
		local    PrecisionDecimalFacetDeclarations
		loc      Loc
		baseLoc  Loc
		fixedRef string
	}{
		{
			name:  "totalDigits",
			local: NewPrecisionDecimalFacetDeclarations(mustPrecisionDecimalTotalFacet(t, "9", childTotalLoc, false), nil, nil),
			loc:   childTotalLoc, baseLoc: baseTotalLoc, fixedRef: "xsd11-datatypes#f-td-fixed",
		},
		{
			name:  "minScale",
			local: NewPrecisionDecimalFacetDeclarations(nil, mustPrecisionDecimalMinFacet(t, "-4", childMinLoc, false), nil),
			loc:   childMinLoc, baseLoc: baseMinLoc, fixedRef: "xsd-precisionDecimal#f-mns-fixed",
		},
		{
			name:  "maxScale",
			local: NewPrecisionDecimalFacetDeclarations(nil, nil, mustPrecisionDecimalMaxFacet(t, "7", childMaxLoc, false)),
			loc:   childMaxLoc, baseLoc: baseMaxLoc, fixedRef: "xsd-precisionDecimal#f-ms-fixed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, restrictionErr := RestrictPrecisionDecimalFacets(base, test.local)
			assertPrecisionDecimalFacetDiagnostic(t, restrictionErr, InvalidPrecisionDecimalFacetRestrictionCode, test.loc, test.fixedRef)
			assertPrecisionDecimalRelated(t, restrictionErr, test.baseLoc)
			if !errors.Is(restrictionErr, errInvalidPrecisionDecimalFacetRestriction) {
				t.Fatalf("fixed restriction does not preserve cause: %v", restrictionErr)
			}
		})
	}
}

func assertPrecisionDecimalFacetsFixed(t *testing.T, facets PrecisionDecimalFacets) {
	t.Helper()
	if fixed, ok := facets.TotalDigitsFixed(); !ok || !fixed {
		t.Fatalf("totalDigits fixed = (%t, %t), want true,true", fixed, ok)
	}
	if fixed, ok := facets.MinScaleFixed(); !ok || !fixed {
		t.Fatalf("minScale fixed = (%t, %t), want true,true", fixed, ok)
	}
	if fixed, ok := facets.MaxScaleFixed(); !ok || !fixed {
		t.Fatalf("maxScale fixed = (%t, %t), want true,true", fixed, ok)
	}
}

func TestPrecisionDecimalFacetScaleBoundsUseEffectiveOverlayOnly(t *testing.T) {
	baseMinLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 50, 2)
	baseMaxLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 51, 2)
	childMinLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 60, 2)
	childMaxLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 61, 2)

	baseMin := mustPrecisionDecimalMinFacet(t, "-10", baseMinLoc, false)
	baseMax := mustPrecisionDecimalMaxFacet(t, "10", baseMaxLoc, false)
	base, err := NewPrecisionDecimalFacets(nil, baseMin, baseMax)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets: %v", err)
	}

	localMin := mustPrecisionDecimalMinFacet(t, "11", childMinLoc, false)
	_, err = RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(nil, localMin, nil))
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalScaleCombinationCode, childMinLoc, "xsd-precisionDecimal#minScale-totalDigits")
	assertPrecisionDecimalRelated(t, err, baseMaxLoc)
	if !errors.Is(err, errInvalidPrecisionDecimalScaleCombination) {
		t.Fatalf("inherited maxScale contradiction does not preserve cause: %v", err)
	}

	localMax := mustPrecisionDecimalMaxFacet(t, "-11", childMaxLoc, false)
	_, err = RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(nil, nil, localMax))
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalScaleCombinationCode, childMaxLoc, "xsd-precisionDecimal#minScale-totalDigits")
	assertPrecisionDecimalRelated(t, err, baseMinLoc)

	localMin = mustPrecisionDecimalMinFacet(t, "-2", childMinLoc, false)
	localMax = mustPrecisionDecimalMaxFacet(t, "-3", childMaxLoc, false)
	_, err = NewPrecisionDecimalFacets(nil, localMin, localMax)
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalScaleCombinationCode, childMinLoc, "xsd-precisionDecimal#minScale-totalDigits")
	assertPrecisionDecimalRelated(t, err, childMaxLoc)
}

func TestPrecisionDecimalFacetValuesAreCopiedAtEveryBoundary(t *testing.T) {
	input := StrictInteger{value: new(big.Int).Exp(big.NewInt(10), big.NewInt(300), nil)}
	totalLoc := mustPrecisionDecimalFacetLoc(t, "copy.xsd", 70, 2)
	minLoc := mustPrecisionDecimalFacetLoc(t, "copy.xsd", 71, 2)
	maxLoc := mustPrecisionDecimalFacetLoc(t, "copy.xsd", 72, 2)

	total, err := NewPrecisionDecimalTotalDigitsFacet(input, totalLoc, false)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalTotalDigitsFacet: %v", err)
	}
	minValue, err := ParsePrecisionDecimalMinScale("-12345678901234567890", minLoc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinScale: %v", err)
	}
	minFacet, err := NewPrecisionDecimalMinScaleFacet(minValue, minLoc, false)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalMinScaleFacet: %v", err)
	}
	maxValue, err := ParsePrecisionDecimalMaxScale("12345678901234567890", maxLoc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMaxScale: %v", err)
	}
	maxFacet, err := NewPrecisionDecimalMaxScaleFacet(maxValue, maxLoc, false)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalMaxScaleFacet: %v", err)
	}

	base, err := NewPrecisionDecimalFacets(&total, &minFacet, &maxFacet)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets: %v", err)
	}
	wantTotal := total.Value().Canonical()
	wantMin := minFacet.Value().Canonical()
	wantMax := maxFacet.Value().Canonical()

	input.value.SetInt64(1)
	minValue.value.SetInt64(1)
	maxValue.value.SetInt64(1)
	total.value.value.SetInt64(1)
	minFacet.value.value.SetInt64(1)
	maxFacet.value.value.SetInt64(1)
	if got := base.totalDigits.Value().Canonical(); got != wantTotal {
		t.Fatalf("base totalDigits changed through caller mutation: %q, want %q", got, wantTotal)
	}
	if got := base.minScale.Value().Canonical(); got != wantMin {
		t.Fatalf("base minScale changed through caller mutation: %q, want %q", got, wantMin)
	}
	if got := base.maxScale.Value().Canonical(); got != wantMax {
		t.Fatalf("base maxScale changed through caller mutation: %q, want %q", got, wantMax)
	}

	returnedTotal, ok := base.TotalDigits()
	if !ok {
		t.Fatal("base totalDigits unexpectedly absent")
	}
	returnedTotal.value.SetInt64(1)
	if got := base.totalDigits.Value().Canonical(); got != wantTotal {
		t.Fatalf("base totalDigits changed through returned value: %q, want %q", got, wantTotal)
	}

	child, err := RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{})
	if err != nil {
		t.Fatalf("inheritance: %v", err)
	}
	base.totalDigits.value.value.SetInt64(2)
	if got := child.totalDigits.Value().Canonical(); got != wantTotal {
		t.Fatalf("child inherited totalDigits aliases base: %q, want %q", got, wantTotal)
	}
}

func TestPrecisionDecimalFacetLayerRejectsInapplicableFacets(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "facet-name.xsd", 80, 2)
	for _, name := range []string{"fractionDigits", "length", "minLength", "maxLength"} {
		t.Run("inapplicable "+name, func(t *testing.T) {
			err := ValidatePrecisionDecimalFacetName(name, loc)
			assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalDisallowedFacetCode, loc, "xsd-precisionDecimal#facets")
			if errors.Is(err, ErrUnsupported) {
				t.Fatalf("inapplicable facet was classified as unsupported: %v", err)
			}
			if !errors.Is(err, errInvalidPrecisionDecimalDisallowedFacet) {
				t.Fatalf("inapplicable facet does not preserve cause: %v", err)
			}
		})
	}
}

func TestPrecisionDecimalFacetLayerAcceptsImplementedFacetsAndGatesAssertions(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "facet-name.xsd", 80, 2)
	for _, name := range []string{"pattern", "enumeration", "minInclusive", "minExclusive", "maxInclusive", "maxExclusive", "whiteSpace"} {
		t.Run("implemented "+name, func(t *testing.T) {
			err := ValidatePrecisionDecimalFacetName(name, loc)
			if err != nil {
				t.Fatalf("implemented facet was rejected: %v", err)
			}
		})
	}
	err := ValidatePrecisionDecimalFacetName("assertions", loc)
	assertPrecisionDecimalUnsupportedFacetWithFeature(t, err, loc, FeatureID("xsd.assertion"), "xsd11-structures#cAssertions")
	err = ValidatePrecisionDecimalFacetName("assertion", loc)
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalUnknownFacetCode, loc, precisionDecimalFacetSetSpecRef)
}

func TestPrecisionDecimalFacetLayerRejectsUnknownFacets(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "facet-name.xsd", 80, 2)
	for _, name := range []string{"unknown", ""} {
		t.Run("unknown "+name, func(t *testing.T) {
			err := ValidatePrecisionDecimalFacetName(name, loc)
			assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalUnknownFacetCode, loc, "xsd-precisionDecimal#facets")
			if errors.Is(err, ErrUnsupported) {
				t.Fatalf("unknown facet was classified as unsupported: %v", err)
			}
			if !errors.Is(err, errInvalidPrecisionDecimalUnknownFacet) {
				t.Fatalf("unknown facet does not preserve cause: %v", err)
			}
		})
	}
}

func TestPrecisionDecimalFacetLayerAllowsDeclaredFacets(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "facet-name.xsd", 80, 2)
	if err := ValidatePrecisionDecimalFacetName("totalDigits", loc); err != nil {
		t.Fatalf("allowed totalDigits rejected: %v", err)
	}
	if err := ValidatePrecisionDecimalFacetName("minScale", loc); err != nil {
		t.Fatalf("allowed minScale rejected: %v", err)
	}
	if err := ValidatePrecisionDecimalFacetName("maxScale", loc); err != nil {
		t.Fatalf("allowed maxScale rejected: %v", err)
	}
	if _, ok := reflect.TypeOf(PrecisionDecimalFacetDeclarations{}).FieldByName("FractionDigits"); ok {
		t.Fatal("precisionDecimal declarations represent fractionDigits")
	}
}

func assertPrecisionDecimalUnsupportedFacetWithFeature(t *testing.T, err error, loc Loc, feature FeatureID, specRef string) {
	t.Helper()
	if err == nil {
		t.Fatal("unsupported facet was accepted")
	}
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureUnsupported)
	}
	if diagnostic.Code() != UnsupportedPrecisionDecimalFacetCode {
		t.Fatalf("Code() = %q, want %q", diagnostic.Code(), UnsupportedPrecisionDecimalFacetCode)
	}
	if diagnostic.Loc() != loc {
		t.Fatalf("Loc() = %v, want %v", diagnostic.Loc(), loc)
	}
	if diagnostic.Feature() != feature {
		t.Fatalf("Feature() = %q, want %q", diagnostic.Feature(), feature)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported facet does not match ErrUnsupported: %v", err)
	}
}

func TestPrecisionDecimalFacetLayerHasNoValueConstructionPath(t *testing.T) {
	value, err := ParsePrecisionDecimalTotalDigits(strings.Repeat("7", 4096), Loc{})
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalTotalDigits: %v", err)
	}
	facet, err := NewPrecisionDecimalTotalDigitsFacet(value, Loc{}, false)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalTotalDigitsFacet: %v", err)
	}
	facets, err := NewPrecisionDecimalFacets(&facet, nil, nil)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacets: %v", err)
	}
	if got, ok := facets.TotalDigits(); !ok || got.Canonical() != strings.Repeat("7", 4096) {
		t.Fatalf("declaration-only construction changed exact value: (%s, %t)", got.Canonical(), ok)
	}
}

func mustPrecisionDecimalFacetLoc(t *testing.T, source SourceID, line, column int) Loc {
	t.Helper()
	loc, err := NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func mustPrecisionDecimalTotalFacet(t *testing.T, lexical string, loc Loc, fixed bool) *PrecisionDecimalTotalDigitsFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalTotalDigitsFacetWithFixed(lexical, loc, fixed)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalTotalDigitsFacetWithFixed(%q): %v", lexical, err)
	}
	return &facet
}

func mustPrecisionDecimalMinFacet(t *testing.T, lexical string, loc Loc, fixed bool) *PrecisionDecimalMinScaleFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalMinScaleFacetWithFixed(lexical, loc, fixed)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinScaleFacetWithFixed(%q): %v", lexical, err)
	}
	return &facet
}

func mustPrecisionDecimalMaxFacet(t *testing.T, lexical string, loc Loc, fixed bool) *PrecisionDecimalMaxScaleFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalMaxScaleFacetWithFixed(lexical, loc, fixed)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMaxScaleFacetWithFixed(%q): %v", lexical, err)
	}
	return &facet
}

func assertPrecisionDecimalFacetDiagnostic(t *testing.T, err error, code string, loc Loc, specRef string) {
	t.Helper()
	if err == nil {
		t.Fatal("operation accepted invalid precisionDecimal facet input")
	}
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureInvalid)
	}
	if diagnostic.Code() != code {
		t.Fatalf("Code() = %q, want %q", diagnostic.Code(), code)
	}
	if diagnostic.Loc() != loc {
		t.Fatalf("Loc() = %v, want %v", diagnostic.Loc(), loc)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), specRef)
	}
}

func assertPrecisionDecimalRelated(t *testing.T, err error, want Loc) {
	t.Helper()
	if related := mustDiagnostic(t, err).Related(); len(related) != 1 || related[0] != want {
		t.Fatalf("Related() = %#v, want [%v]", related, want)
	}
}
