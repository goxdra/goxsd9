package goxsd9

import (
	"errors"
	"strings"
	"testing"
)

func TestPrecisionDecimalValueFacetsUseExactEqualityAndSpecialValues(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "value.xsd", 10, 2)
	enumeration := []PrecisionDecimalEnumerationFacet{
		mustPrecisionDecimalEnumerationFacet(t, "3", loc),
		mustPrecisionDecimalEnumerationFacet(t, "0", loc),
		mustPrecisionDecimalEnumerationFacet(t, "+INF", loc),
		mustPrecisionDecimalEnumerationFacet(t, "-INF", loc),
		mustPrecisionDecimalEnumerationFacet(t, "NaN", loc),
	}
	facets, err := NewPrecisionDecimalFacetsFromDeclarations(NewPrecisionDecimalValueFacetDeclarations(
		nil,
		enumeration,
		nil,
		nil,
		nil,
		nil,
		nil,
	))
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacetsFromDeclarations: %v", err)
	}
	for _, value := range []string{"3.0", "-0.000", "+INF", "-INF"} {
		err = validatePrecisionDecimalFacets(value, facets, loc)
		if err != nil {
			t.Fatalf("enumeration accepted value %q with error: %v", value, err)
		}
	}
	err = validatePrecisionDecimalFacets("NaN", facets, loc)
	assertPrecisionDecimalValueViolation(t, err, loc, precisionDecimalEnumerationValidSpecRef)
	err = validatePrecisionDecimalFacets("4", facets, loc)
	assertPrecisionDecimalValueViolation(t, err, loc, precisionDecimalEnumerationValidSpecRef)
}

func TestPrecisionDecimalValueFacetsEvaluateAllBoundsAndUnorderedNaN(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "bounds.xsd", 20, 3)
	tests := []struct {
		name  string
		local PrecisionDecimalFacetDeclarations
		pass  []string
		fail  []string
		ref   string
	}{
		{
			name:  "minInclusive",
			local: PrecisionDecimalFacetDeclarations{MinInclusive: precisionDecimalMinInclusive(t, "3", loc)},
			pass:  []string{"3", "4", "+INF"},
			fail:  []string{"2", "NaN"},
			ref:   precisionDecimalMinInclusiveValidSpecRef,
		},
		{
			name:  "minExclusive",
			local: PrecisionDecimalFacetDeclarations{MinExclusive: precisionDecimalMinExclusive(t, "3", loc)},
			pass:  []string{"3.1", "+INF"},
			fail:  []string{"3", "-INF", "NaN"},
			ref:   precisionDecimalMinExclusiveValidSpecRef,
		},
		{
			name:  "maxInclusive",
			local: PrecisionDecimalFacetDeclarations{MaxInclusive: precisionDecimalMaxInclusive(t, "3", loc)},
			pass:  []string{"3", "2", "-INF"},
			fail:  []string{"4", "NaN"},
			ref:   precisionDecimalMaxInclusiveValidSpecRef,
		},
		{
			name:  "maxExclusive",
			local: PrecisionDecimalFacetDeclarations{MaxExclusive: precisionDecimalMaxExclusive(t, "3", loc)},
			pass:  []string{"2.9", "-INF"},
			fail:  []string{"3", "4", "NaN"},
			ref:   precisionDecimalMaxExclusiveValidSpecRef,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facets := mustPrecisionDecimalValueFacets(t, test.local)
			for _, value := range test.pass {
				if err := validatePrecisionDecimalFacets(value, facets, loc); err != nil {
					t.Fatalf("bound rejected %q: %v", value, err)
				}
			}
			for _, value := range test.fail {
				err := validatePrecisionDecimalFacets(value, facets, loc)
				assertPrecisionDecimalValueViolation(t, err, loc, test.ref)
			}
		})
	}
}

func TestPrecisionDecimalValueFacetsCountRetainedDigitsAndExactScales(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "digits.xsd", 30, 4)
	total := mustPrecisionDecimalTotalFacet(t, "3", loc, false)
	facets := mustPrecisionDecimalFacets(t, NewPrecisionDecimalFacetDeclarations(total, nil, nil))
	for _, value := range []string{"3.00", "3.0e2", "0", "+INF", "-INF", "NaN"} {
		if err := validatePrecisionDecimalFacets(value, facets, loc); err != nil {
			t.Fatalf("totalDigits rejected %q: %v", value, err)
		}
	}
	err := validatePrecisionDecimalFacets("3.0001", facets, loc)
	assertPrecisionDecimalValueViolation(t, err, loc, precisionDecimalTotalDigitsSpecRef)

	minScaleFacet := mustPrecisionDecimalMinFacet(t, "2", loc, false)
	maxScaleFacet := mustPrecisionDecimalMaxFacet(t, "2", loc, false)
	scaleFacets := mustPrecisionDecimalFacets(t, NewPrecisionDecimalFacetDeclarations(nil, minScaleFacet, maxScaleFacet))
	for _, value := range []string{"1.23", "0.00", "+INF", "-INF", "NaN"} {
		err = validatePrecisionDecimalFacets(value, scaleFacets, loc)
		if err != nil {
			t.Fatalf("scale facets rejected %q: %v", value, err)
		}
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("1.2", scaleFacets, loc), loc, precisionDecimalMinScaleValueSpecRef)
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("1.234", scaleFacets, loc), loc, precisionDecimalMaxScaleValueSpecRef)
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("0.000", scaleFacets, loc), loc, precisionDecimalMaxScaleValueSpecRef)

	huge := strings.Repeat("9", 2048)
	hugeScale := strings.Repeat("7", 512)
	hugeMin, err := ParsePrecisionDecimalMinScale("-"+hugeScale, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinScale huge: %v", err)
	}
	hugeTotal, err := ParsePrecisionDecimalTotalDigits(huge, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalTotalDigits huge: %v", err)
	}
	hugeFacet, err := NewPrecisionDecimalMinScaleFacet(hugeMin, loc, false)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalMinScaleFacet huge: %v", err)
	}
	hugeTotalFacet, err := NewPrecisionDecimalTotalDigitsFacet(hugeTotal, loc, false)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalTotalDigitsFacet huge: %v", err)
	}
	hugeFacets := mustPrecisionDecimalFacets(t, NewPrecisionDecimalFacetDeclarations(&hugeTotalFacet, &hugeFacet, nil))
	if err := validatePrecisionDecimalFacets("1e"+hugeScale, hugeFacets, loc); err != nil {
		t.Fatalf("huge exact coefficient/scale rejected: %v", err)
	}
}

func TestPrecisionDecimalWhiteSpaceFacetIsFixedCollapseAndInherited(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "white-space.xsd", 35, 2)
	whiteSpace, err := ParsePrecisionDecimalWhiteSpaceFacet(" collapse ", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalWhiteSpaceFacet: %v", err)
	}
	base := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
		nil, nil, nil, nil, nil, nil, &whiteSpace,
	))
	declared, ok := base.WhiteSpaceFacet()
	if !ok || declared.Value() != "collapse" || !declared.Fixed() {
		t.Fatalf("effective whiteSpace = (%q, fixed=%t, present=%t), want collapse/fixed/present", declared.Value(), declared.Fixed(), ok)
	}
	derived, err := RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{})
	if err != nil {
		t.Fatalf("whiteSpace inheritance: %v", err)
	}
	inherited, ok := derived.WhiteSpaceFacet()
	if !ok || inherited.Value() != "collapse" || !inherited.Fixed() {
		t.Fatalf("inherited whiteSpace = (%q, fixed=%t, present=%t), want collapse/fixed/present", inherited.Value(), inherited.Fixed(), ok)
	}
}

func TestPrecisionDecimalValueFacetsUseNormalizedLexicalXMLSchemaPatterns(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "pattern.xsd", 40, 5)
	pattern := mustPrecisionDecimalPatternFacet(t, `3\.0`, loc)
	facets := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
		[]PrecisionDecimalPatternFacet{pattern}, nil, nil, nil, nil, nil, nil,
	))
	if err := validatePrecisionDecimalFacets(" \t3.0\n ", facets, loc); err != nil {
		t.Fatalf("normalized lexical pattern rejected: %v", err)
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("3.00", facets, loc), loc, precisionDecimalPatternValidSpecRef)

	patterns := []string{"3|4", `\d{2}`, `[0-9-[5]]`, `\p{Nd}+`, `\p{IsBasic_Latin}+`, `3?`}
	values := []string{"3", "12", "3", "34", "3", "3"}
	for index, source := range patterns {
		facet := mustPrecisionDecimalPatternFacet(t, source, loc)
		patternFacets := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
			[]PrecisionDecimalPatternFacet{facet}, nil, nil, nil, nil, nil, nil,
		))
		if err := validatePrecisionDecimalFacets(values[index], patternFacets, loc); err != nil {
			t.Fatalf("XML Schema pattern %q rejected %q: %v", source, values[index], err)
		}
	}
	latinSupplement := mustPrecisionDecimalPatternFacet(t, `\p{IsLatin-1_Supplement}`, loc)
	if !latinSupplement.expression.matches("é") || latinSupplement.expression.matches("A") {
		t.Fatal("hyphenated XML block property did not resolve Latin-1 Supplement")
	}
	_, err := ParsePrecisionDecimalPatternFacet(`\p{IsDefinitelyNotAnXMLBlock}`, loc)
	if err == nil {
		t.Fatal("unknown XML block property was accepted")
		return
	}
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalPatternCode, loc, precisionDecimalPatternValueSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalPattern) {
		t.Fatalf("unknown block error lost cause: %v", err)
	}
	dot := mustPrecisionDecimalPatternFacet(t, `.`, loc)
	dotFacets := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
		[]PrecisionDecimalPatternFacet{dot}, nil, nil, nil, nil, nil, nil,
	))
	if !dot.expression.matches("3") {
		t.Fatal("dot pattern did not match an XML character")
	}
	if dot.expression.matches("\n") {
		t.Fatal("dot pattern matched XML newline")
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("33", dotFacets, loc), loc, precisionDecimalPatternValidSpecRef)

	basePattern := mustPrecisionDecimalPatternFacet(t, `3|4`, loc)
	childPattern := mustPrecisionDecimalPatternFacet(t, `3`, mustPrecisionDecimalFacetLoc(t, "child.xsd", 50, 2))
	base := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
		[]PrecisionDecimalPatternFacet{basePattern}, nil, nil, nil, nil, nil, nil,
	))
	child, err := RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalValueFacetDeclarations(
		[]PrecisionDecimalPatternFacet{childPattern}, nil, nil, nil, nil, nil, nil,
	))
	if err != nil {
		t.Fatalf("pattern derivation: %v", err)
	}
	if child.PatternGroupCount() != 2 || child.PatternCount() != 2 {
		t.Fatalf("pattern inheritance counts = (%d,%d), want (2,2)", child.PatternGroupCount(), child.PatternCount())
	}
	if err := validatePrecisionDecimalFacets("3", child, loc); err != nil {
		t.Fatalf("derived pattern rejected 3: %v", err)
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("4", child, loc), loc, precisionDecimalPatternValidSpecRef)
}

func TestPrecisionDecimalValueFacetDerivationIntersectsEnumerationAndRestrictsBounds(t *testing.T) {
	baseLoc := mustPrecisionDecimalFacetLoc(t, "base.xsd", 60, 2)
	childLoc := mustPrecisionDecimalFacetLoc(t, "child.xsd", 70, 2)
	baseMin := precisionDecimalMinInclusive(t, "1", baseLoc)
	baseMax := precisionDecimalMaxInclusive(t, "10", baseLoc)
	base := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalFacetDeclarationsAll(
		nil, nil, nil, nil,
		[]PrecisionDecimalEnumerationFacet{
			mustPrecisionDecimalEnumerationFacet(t, "3", baseLoc),
			mustPrecisionDecimalEnumerationFacet(t, "4", baseLoc),
		}, baseMin, nil, baseMax, nil, nil,
	))
	childMin := precisionDecimalMinExclusive(t, "1", childLoc)
	childMax := precisionDecimalMaxExclusive(t, "10", childLoc)
	child, err := RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarationsAll(
		nil, nil, nil, nil,
		[]PrecisionDecimalEnumerationFacet{
			mustPrecisionDecimalEnumerationFacet(t, "3.0", childLoc),
			mustPrecisionDecimalEnumerationFacet(t, "9", childLoc),
		}, nil, childMin, nil, childMax, nil,
	))
	if err != nil {
		diagnostic := mustDiagnostic(t, err)
		t.Fatalf("derived value facets: %v (%s at %v related %v)", err, diagnostic.SpecRef(), diagnostic.Loc(), diagnostic.Related())
	}
	if child.EnumerationCount() != 1 {
		t.Fatalf("effective enumeration count = %d, want 1", child.EnumerationCount())
	}
	err = validatePrecisionDecimalFacets("3.00", child, childLoc)
	if err != nil {
		t.Fatalf("derived enumeration rejected exact equal value: %v", err)
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("4", child, childLoc), childLoc, precisionDecimalEnumerationValidSpecRef)

	baseFixedMin := precisionDecimalMinInclusive(t, "1", baseLoc).WithFixed(true)
	baseFixed := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{MinInclusive: &baseFixedMin})
	same := precisionDecimalMinInclusive(t, "1.0", childLoc)
	_, err = RestrictPrecisionDecimalFacets(baseFixed, PrecisionDecimalFacetDeclarations{MinInclusive: same})
	if err != nil {
		t.Fatalf("numeric-equal fixed bound rejected: %v", err)
	}
	changed := precisionDecimalMinInclusive(t, "2", childLoc)
	err = expectPrecisionDecimalRestrictionError(t, func() error {
		_, restrictionErr := RestrictPrecisionDecimalFacets(baseFixed, PrecisionDecimalFacetDeclarations{MinInclusive: changed})
		return restrictionErr
	})
	if !errors.Is(err, errInvalidPrecisionDecimalFacetRestriction) {
		t.Fatalf("fixed bound error lost cause: %v", err)
	}

	baseUpper := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{MaxInclusive: precisionDecimalMaxInclusive(t, "10", baseLoc)})
	lessRestrictive := precisionDecimalMaxInclusive(t, "11", childLoc)
	upperRestrictionErr := expectPrecisionDecimalRestrictionError(t, func() error {
		_, restrictionErr := RestrictPrecisionDecimalFacets(baseUpper, PrecisionDecimalFacetDeclarations{MaxInclusive: lessRestrictive})
		return restrictionErr
	})
	if !errors.Is(upperRestrictionErr, errInvalidPrecisionDecimalFacetRestriction) {
		t.Fatalf("upper-bound restriction error lost cause: %v", upperRestrictionErr)
	}
	baseLower := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{MinInclusive: precisionDecimalMinInclusive(t, "1", baseLoc)})
	lessLower := precisionDecimalMinInclusive(t, "0", childLoc)
	lowerRestrictionErr := expectPrecisionDecimalRestrictionError(t, func() error {
		_, restrictionErr := RestrictPrecisionDecimalFacets(baseLower, PrecisionDecimalFacetDeclarations{MinInclusive: lessLower})
		return restrictionErr
	})
	if !errors.Is(lowerRestrictionErr, errInvalidPrecisionDecimalFacetRestriction) {
		t.Fatalf("lower-bound restriction error lost cause: %v", lowerRestrictionErr)
	}
}

func TestPrecisionDecimalValueFacetDeclarationsRejectInvalidCombinationsAndKeepDiagnostics(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "invalid.xsd", 80, 6)
	_, err := ParsePrecisionDecimalPatternFacet("[", loc)
	if err == nil {
		t.Fatal("unterminated pattern was accepted")
		return
	}
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalPatternCode, loc, precisionDecimalPatternValueSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalPattern) {
		t.Fatalf("pattern error lost cause: %v", err)
	}
	_, err = ParsePrecisionDecimalWhiteSpaceFacet("replace", loc)
	if err == nil {
		t.Fatal("invalid whiteSpace was accepted")
		return
	}
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalWhiteSpaceCode, loc, precisionDecimalWhiteSpaceValueSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalWhiteSpace) {
		t.Fatalf("whiteSpace error lost cause: %v", err)
	}
	_, err = ParsePrecisionDecimalMinInclusiveFacet("not-a-decimal", loc)
	if err == nil {
		t.Fatal("invalid bound was accepted")
		return
	}
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCode, loc, precisionDecimalMinInclusiveValueSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalBound) {
		t.Fatalf("bound error lost cause: %v", err)
	}

	minInclusive := precisionDecimalMinInclusive(t, "1", loc)
	minExclusive := precisionDecimalMinExclusive(t, "2", loc)
	local := PrecisionDecimalFacetDeclarations{MinInclusive: minInclusive, MinExclusive: minExclusive}
	_, err = NewPrecisionDecimalFacetsFromDeclarations(local)
	if err == nil {
		t.Fatal("contradictory lower bound declarations were accepted")
	}
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCombinationCode, loc, precisionDecimalMinInclusiveMinExclusiveSpecRef)
	minimum := precisionDecimalMinInclusive(t, "5", loc)
	maximum := precisionDecimalMaxExclusive(t, "5", loc)
	_, err = NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{MinInclusive: minimum, MaxExclusive: maximum})
	if err == nil {
		t.Fatal("empty inclusive/exclusive interval was accepted")
	}

	duplicate := NewPrecisionDecimalFacetDeclarations(nil, nil, nil)
	duplicate.boundRecords = []precisionDecimalBoundRecord{
		{kind: precisionDecimalMinInclusiveBoundKind, value: minInclusive.value, loc: loc},
		{kind: precisionDecimalMinInclusiveBoundKind, value: minInclusive.value, loc: mustPrecisionDecimalFacetLoc(t, "invalid.xsd", 81, 6)},
	}
	_, err = NewPrecisionDecimalFacetsFromDeclarations(duplicate)
	if err == nil {
		t.Fatal("duplicate bound declarations were accepted")
	}
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCombinationCode, duplicate.boundRecords[1].loc, precisionDecimalBoundRestrictionSpecRef)
}

func TestPrecisionDecimalValueFacetDiagnosticsAreDeterministicAndLocated(t *testing.T) {
	valueLoc := mustPrecisionDecimalFacetLoc(t, "instance.xml", 4, 3)
	patternLoc := mustPrecisionDecimalFacetLoc(t, "schema.xsd", 90, 4)
	totalLoc := mustPrecisionDecimalFacetLoc(t, "schema.xsd", 91, 4)
	pattern := mustPrecisionDecimalPatternFacet(t, "4+", patternLoc)
	total := mustPrecisionDecimalTotalFacet(t, "1", totalLoc, false)
	facets := mustPrecisionDecimalFacets(t, NewPrecisionDecimalFacetDeclarationsAll(
		total, nil, nil, []PrecisionDecimalPatternFacet{pattern}, nil, nil, nil, nil, nil, nil,
	))
	err := validatePrecisionDecimalFacets("22", facets, valueLoc)
	assertPrecisionDecimalValueViolation(t, err, valueLoc, precisionDecimalPatternValidSpecRef)
	diagnostic := mustDiagnostic(t, err)
	if related := diagnostic.Related(); len(related) != 1 || related[0] != patternLoc {
		t.Fatalf("pattern related locations = %v, want [%v]", related, patternLoc)
	}
	if err := validatePrecisionDecimalFacets("4", facets, valueLoc); err != nil {
		t.Fatalf("pattern-valid value rejected: %v", err)
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("44", facets, valueLoc), valueLoc, precisionDecimalTotalDigitsSpecRef)
}

func mustPrecisionDecimalValueFacets(t *testing.T, declarations PrecisionDecimalFacetDeclarations) PrecisionDecimalFacets {
	t.Helper()
	facets, err := NewPrecisionDecimalFacetsFromDeclarations(declarations)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacetsFromDeclarations: %v", err)
	}
	return facets
}

func mustPrecisionDecimalFacets(t *testing.T, declarations PrecisionDecimalFacetDeclarations) PrecisionDecimalFacets {
	return mustPrecisionDecimalValueFacets(t, declarations)
}

func mustPrecisionDecimalPatternFacet(t *testing.T, source string, loc Loc) PrecisionDecimalPatternFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalPatternFacet(source, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalPatternFacet(%q): %v", source, err)
	}
	return facet
}

func mustPrecisionDecimalEnumerationFacet(t *testing.T, lexical string, loc Loc) PrecisionDecimalEnumerationFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalEnumerationFacet(lexical, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalEnumerationFacet(%q): %v", lexical, err)
	}
	return facet
}

func precisionDecimalMinInclusive(t *testing.T, lexical string, loc Loc) *PrecisionDecimalMinInclusiveFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalMinInclusiveFacet(lexical, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinInclusiveFacet(%q): %v", lexical, err)
	}
	return &facet
}

func precisionDecimalMinExclusive(t *testing.T, lexical string, loc Loc) *PrecisionDecimalMinExclusiveFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalMinExclusiveFacet(lexical, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinExclusiveFacet(%q): %v", lexical, err)
	}
	return &facet
}

func precisionDecimalMaxInclusive(t *testing.T, lexical string, loc Loc) *PrecisionDecimalMaxInclusiveFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalMaxInclusiveFacet(lexical, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMaxInclusiveFacet(%q): %v", lexical, err)
	}
	return &facet
}

func precisionDecimalMaxExclusive(t *testing.T, lexical string, loc Loc) *PrecisionDecimalMaxExclusiveFacet {
	t.Helper()
	facet, err := ParsePrecisionDecimalMaxExclusiveFacet(lexical, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMaxExclusiveFacet(%q): %v", lexical, err)
	}
	return &facet
}

func assertPrecisionDecimalValueViolation(t *testing.T, err error, valueLoc Loc, specRef string) {
	t.Helper()
	if err == nil {
		t.Fatal("value facet violation was accepted")
	}
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureInvalid)
	}
	if diagnostic.Code() != PrecisionDecimalFacetValueViolationCode {
		t.Fatalf("Code() = %q, want %q", diagnostic.Code(), PrecisionDecimalFacetValueViolationCode)
	}
	if diagnostic.Loc() != valueLoc {
		t.Fatalf("Loc() = %v, want %v", diagnostic.Loc(), valueLoc)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if !errors.Is(err, errPrecisionDecimalFacetValueViolation) {
		t.Fatalf("value violation lost cause: %v", err)
	}
}

func expectPrecisionDecimalRestrictionError(t *testing.T, operation func() error) error {
	t.Helper()
	err := operation()
	if err == nil {
		t.Fatal("invalid restriction was accepted")
	}
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidPrecisionDecimalFacetRestrictionCode {
		t.Fatalf("restriction diagnostic = (%q,%q), want (%q,%q)", diagnostic.Class(), diagnostic.Code(), FailureInvalid, InvalidPrecisionDecimalFacetRestrictionCode)
	}
	return err
}
