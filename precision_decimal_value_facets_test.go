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
	if err = validatePrecisionDecimalFacets("NaN", facets, loc); err != nil {
		t.Fatalf("NaN did not enumerate itself: %v", err)
	}
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
	assertPrecisionDecimalValueViolation(t, err, loc, precisionDecimalTotalDigitsValidSpecRef)

	minScaleFacet := mustPrecisionDecimalMinFacet(t, "2", loc, false)
	maxScaleFacet := mustPrecisionDecimalMaxFacet(t, "2", loc, false)
	scaleFacets := mustPrecisionDecimalFacets(t, NewPrecisionDecimalFacetDeclarations(nil, minScaleFacet, maxScaleFacet))
	for _, value := range []string{"1.23", "0.00", "+INF", "-INF", "NaN"} {
		err = validatePrecisionDecimalFacets(value, scaleFacets, loc)
		if err != nil {
			t.Fatalf("scale facets rejected %q: %v", value, err)
		}
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("1.2", scaleFacets, loc), loc, precisionDecimalMinScaleValidSpecRef)
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("1.234", scaleFacets, loc), loc, precisionDecimalMaxScaleValidSpecRef)
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("0.000", scaleFacets, loc), loc, precisionDecimalMaxScaleValidSpecRef)

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

	patterns := []string{"3|4", `\d{2}`, `[0-9-[5]]`, `\p{Nd}+`, `\p{IsBasicLatin}+`, `3?`}
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
	testPrecisionDecimalPatternPropertyBehavior(t, loc)

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

func testPrecisionDecimalPatternPropertyBehavior(t *testing.T, loc Loc) {
	t.Helper()
	latinSupplement := mustPrecisionDecimalPatternFacet(t, `\p{IsLatin-1Supplement}`, loc)
	if !latinSupplement.matches("é") || latinSupplement.matches("A") {
		t.Fatal("hyphenated XML block property did not resolve Latin-1 Supplement")
	}
	unknown := mustPrecisionDecimalPatternFacet(t, `\p{IsDefinitelyNotAnXMLBlock}`, loc)
	if !unknown.matches("A") || !unknown.matches("\n") {
		t.Fatal("unknown XML block property did not use any-character default")
	}
	unknownComplement := mustPrecisionDecimalPatternFacet(t, `\P{IsDefinitelyNotAnXMLBlock}`, loc)
	if !unknownComplement.matches("A") || !unknownComplement.matches("\n") {
		t.Fatal("unknown XML block complement did not use any-character default")
	}
	knownComplement := mustPrecisionDecimalPatternFacet(t, `\P{IsBasicLatin}`, loc)
	if knownComplement.matches("A") || !knownComplement.matches("é") {
		t.Fatal("recognized Basic Latin block complement was not exact")
	}
	wrongCase := mustPrecisionDecimalPatternFacet(t, `\P{Isbasiclatin}`, loc)
	if !wrongCase.matches("A") || !wrongCase.matches("é") {
		t.Fatal("wrong-case block name was folded instead of treated as unknown")
	}
	missingHyphen := mustPrecisionDecimalPatternFacet(t, `\P{IsLatin1Supplement}`, loc)
	if !missingHyphen.matches("A") || !missingHyphen.matches("é") {
		t.Fatal("missing-hyphen block name was normalized instead of treated as unknown")
	}
	dot := mustPrecisionDecimalPatternFacet(t, `.`, loc)
	dotFacets := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
		[]PrecisionDecimalPatternFacet{dot}, nil, nil, nil, nil, nil, nil,
	))
	if !dot.matches("3") {
		t.Fatal("dot pattern did not match an XML character")
	}
	if dot.matches("\n") {
		t.Fatal("dot pattern matched XML newline")
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("33", dotFacets, loc), loc, precisionDecimalPatternValidSpecRef)
}

func TestPrecisionDecimalValueFacetDerivationRequiresBaseMembershipAndRestrictsBounds(t *testing.T) {
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
	child, err := RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarationsAll(
		nil, nil, nil, nil,
		[]PrecisionDecimalEnumerationFacet{mustPrecisionDecimalEnumerationFacet(t, "3.0", childLoc)},
		nil, nil, nil, nil, nil,
	))
	if err != nil {
		diagnostic := mustDiagnostic(t, err)
		t.Fatalf("derived value facets: %v (%s at %v related %v)", err, diagnostic.SpecRef(), diagnostic.Loc(), diagnostic.Related())
	}
	if child.EnumerationCount() != 1 {
		t.Fatalf("effective enumeration count = %d, want 1", child.EnumerationCount())
	}
	if child.enumeration[0].normalizedLexical != "" {
		t.Fatal("effective enumeration retained local normalized lexical state")
	}
	err = validatePrecisionDecimalFacets("3.00", child, childLoc)
	if err != nil {
		t.Fatalf("derived enumeration rejected exact equal value: %v", err)
	}
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("4", child, childLoc), childLoc, precisionDecimalEnumerationValidSpecRef)
	_, err = RestrictPrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarationsAll(
		nil, nil, nil, nil,
		[]PrecisionDecimalEnumerationFacet{mustPrecisionDecimalEnumerationFacet(t, "9", childLoc)},
		nil, nil, nil, nil, nil,
	))
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalEnumerationCode, childLoc, precisionDecimalEnumerationRestrictionSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalEnumeration) {
		t.Fatalf("base-membership enumeration error lost cause: %v", err)
	}

	tooBroadBound := precisionDecimalMaxInclusive(t, "11", childLoc)
	_, err = RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{MaxInclusive: tooBroadBound})
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCode, childLoc, precisionDecimalMaxInclusiveRestrictionSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalBound) {
		t.Fatalf("base-membership bound error lost cause: %v", err)
	}

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
	_, upperRestrictionErr := RestrictPrecisionDecimalFacets(baseUpper, PrecisionDecimalFacetDeclarations{MaxInclusive: lessRestrictive})
	assertPrecisionDecimalFacetDiagnostic(t, upperRestrictionErr, InvalidPrecisionDecimalBoundCode, childLoc, precisionDecimalMaxInclusiveRestrictionSpecRef)
	if !errors.Is(upperRestrictionErr, errInvalidPrecisionDecimalBound) {
		t.Fatalf("upper-bound base-membership error lost cause: %v", upperRestrictionErr)
	}
	baseLower := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{MinInclusive: precisionDecimalMinInclusive(t, "1", baseLoc)})
	lessLower := precisionDecimalMinInclusive(t, "0", childLoc)
	_, lowerRestrictionErr := RestrictPrecisionDecimalFacets(baseLower, PrecisionDecimalFacetDeclarations{MinInclusive: lessLower})
	assertPrecisionDecimalFacetDiagnostic(t, lowerRestrictionErr, InvalidPrecisionDecimalBoundCode, childLoc, precisionDecimalMinInclusiveRestrictionSpecRef)
	if !errors.Is(lowerRestrictionErr, errInvalidPrecisionDecimalBound) {
		t.Fatalf("lower-bound base-membership error lost cause: %v", lowerRestrictionErr)
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
}

func TestPrecisionDecimalFacetDeclarationsRejectZeroValuesWithoutPanics(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "zero.xsd", 100, 3)
	tests := []struct {
		name  string
		local PrecisionDecimalFacetDeclarations
		code  string
		ref   string
		loc   Loc
	}{
		{
			name:  "totalDigits",
			local: PrecisionDecimalFacetDeclarations{TotalDigits: &PrecisionDecimalTotalDigitsFacet{loc: loc}},
			code:  InvalidPrecisionDecimalTotalDigitsCode,
			ref:   xsd11TotalDigitsValueSpecRef,
			loc:   loc,
		},
		{
			name:  "minScale direct",
			local: PrecisionDecimalFacetDeclarations{MinScale: &PrecisionDecimalMinScaleFacet{loc: loc}},
			code:  InvalidPrecisionDecimalMinScaleCode,
			ref:   precisionDecimalMinScaleValueSpecRef,
			loc:   loc,
		},
		{
			name:  "minScale copied",
			local: NewPrecisionDecimalFacetDeclarations(nil, &PrecisionDecimalMinScaleFacet{loc: loc}, nil),
			code:  InvalidPrecisionDecimalMinScaleCode,
			ref:   precisionDecimalMinScaleValueSpecRef,
			loc:   loc,
		},
		{
			name:  "maxScale",
			local: PrecisionDecimalFacetDeclarations{MaxScale: &PrecisionDecimalMaxScaleFacet{loc: loc}},
			code:  InvalidPrecisionDecimalMaxScaleCode,
			ref:   precisionDecimalMaxScaleValueSpecRef,
			loc:   loc,
		},
		{
			name:  "enumeration",
			local: PrecisionDecimalFacetDeclarations{Enumeration: []PrecisionDecimalEnumerationFacet{{loc: loc}}},
			code:  InvalidPrecisionDecimalEnumerationCode,
			ref:   precisionDecimalEnumerationValueSpecRef,
			loc:   loc,
		},
		{
			name:  "minInclusive",
			local: PrecisionDecimalFacetDeclarations{MinInclusive: &PrecisionDecimalMinInclusiveFacet{loc: loc}},
			code:  InvalidPrecisionDecimalBoundCode,
			ref:   precisionDecimalMinInclusiveValueSpecRef,
			loc:   loc,
		},
		{
			name:  "whiteSpace",
			local: PrecisionDecimalFacetDeclarations{WhiteSpace: &PrecisionDecimalWhiteSpaceFacet{loc: loc}},
			code:  InvalidPrecisionDecimalWhiteSpaceCode,
			ref:   precisionDecimalWhiteSpaceValueSpecRef,
			loc:   loc,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := constructPrecisionDecimalFacetsNoPanic(t, test.local)
			assertPrecisionDecimalFacetDiagnostic(t, err, test.code, test.loc, test.ref)
			err = restrictPrecisionDecimalFacetsNoPanic(t, PrecisionDecimalFacets{}, test.local)
			assertPrecisionDecimalFacetDiagnostic(t, err, test.code, test.loc, test.ref)
		})
	}

	err := constructPrecisionDecimalFacetsNoPanic(t, PrecisionDecimalFacetDeclarations{Enumeration: []PrecisionDecimalEnumerationFacet{}})
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalEnumerationCode, Loc{}, precisionDecimalEnumerationValueSpecRef)

	empty := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{})
	whiteSpace, ok := empty.WhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "collapse" || !whiteSpace.Fixed() {
		t.Fatalf("empty effective whiteSpace = (%q, fixed=%t, present=%t), want collapse/true/true", whiteSpace.Value(), whiteSpace.Fixed(), ok)
	}
	var zero PrecisionDecimalFacets
	whiteSpace, ok = zero.WhiteSpaceFacet()
	if !ok || whiteSpace.Value() != "collapse" || !whiteSpace.Fixed() {
		t.Fatalf("zero effective whiteSpace = (%q, fixed=%t, present=%t), want collapse/true/true", whiteSpace.Value(), whiteSpace.Fixed(), ok)
	}
	corrupt := PrecisionDecimalFacets{minScale: &PrecisionDecimalMinScaleFacet{loc: loc}}
	err = corrupt.validate()
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != InvalidPrecisionDecimalMinScaleCode || diagnostic.Loc() != loc {
		t.Fatalf("corrupt effective scale diagnostic = (%q,%q,%v), want internal/%s/%v", diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), InvalidPrecisionDecimalMinScaleCode, loc)
	}
}

func TestPrecisionDecimalBoundNaNIsLegalButProducesEmptyRestrictions(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "nan-bound.xsd", 110, 4)
	nan := precisionDecimalMinInclusive(t, "NaN", loc)
	base, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{MinInclusive: nan})
	if err != nil {
		t.Fatalf("unrestricted NaN bound rejected: %v", err)
	}
	for _, value := range []string{"NaN", "0", "+INF"} {
		assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets(value, base, loc), loc, precisionDecimalMinInclusiveValidSpecRef)
	}

	fixedNaN := nan.WithFixed(true)
	fixedBase := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{MinInclusive: &fixedNaN})
	sameNaN := precisionDecimalMinInclusive(t, "NaN", loc)
	if _, restrictionErr := RestrictPrecisionDecimalFacets(fixedBase, PrecisionDecimalFacetDeclarations{MinInclusive: sameNaN}); restrictionErr != nil {
		t.Fatalf("same fixed NaN bound was rejected: %v", restrictionErr)
	}
	finite := precisionDecimalMinInclusive(t, "0", loc)
	err = expectPrecisionDecimalBoundRestrictionDiagnostic(t, fixedBase, PrecisionDecimalFacetDeclarations{MinInclusive: finite}, loc, precisionDecimalMinInclusiveRestrictionSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalBound) {
		t.Fatalf("finite bound did not preserve empty-base cause: %v", err)
	}

	nonFixedBase := mustPrecisionDecimalValueFacets(t, PrecisionDecimalFacetDeclarations{MinInclusive: precisionDecimalMinInclusive(t, "NaN", loc)})
	err = expectPrecisionDecimalBoundRestrictionDiagnostic(t, nonFixedBase, PrecisionDecimalFacetDeclarations{MinInclusive: precisionDecimalMinInclusive(t, "NaN", loc)}, loc, precisionDecimalMinInclusiveRestrictionSpecRef)
	if !errors.Is(err, errInvalidPrecisionDecimalBound) {
		t.Fatalf("non-fixed NaN redeclaration did not preserve empty-base cause: %v", err)
	}
}

func TestPrecisionDecimalFixedBoundAllowsStricterOppositeKind(t *testing.T) {
	baseLoc := mustPrecisionDecimalFacetLoc(t, "fixed-kind-base.xsd", 120, 2)
	childLoc := mustPrecisionDecimalFacetLoc(t, "fixed-kind-child.xsd", 121, 2)
	baseMin := precisionDecimalMinInclusive(t, "1", baseLoc)
	baseMin.fixed = true
	base, err := NewPrecisionDecimalFacetsFromDeclarations(PrecisionDecimalFacetDeclarations{MinInclusive: baseMin})
	if err != nil {
		t.Fatalf("fixed base construction: %v", err)
	}
	childMin := precisionDecimalMinExclusive(t, "2", childLoc)
	if _, err := RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{MinExclusive: childMin}); err != nil {
		t.Fatalf("stricter opposite bound kind rejected: %v", err)
	}
}

func TestPrecisionDecimalExclusiveEndpointMembershipSkipsOtherBaseFacets(t *testing.T) {
	baseLoc := mustPrecisionDecimalFacetLoc(t, "exclusive-base.xsd", 122, 2)
	childLoc := mustPrecisionDecimalFacetLoc(t, "exclusive-child.xsd", 123, 2)
	basePattern := mustPrecisionDecimalPatternFacet(t, "0", baseLoc)
	baseMax := precisionDecimalMaxExclusive(t, "1", baseLoc)
	base := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalFacetDeclarationsAll(
		nil, nil, nil, []PrecisionDecimalPatternFacet{basePattern}, nil,
		nil, nil, nil, baseMax, nil,
	))
	childMax := precisionDecimalMaxExclusive(t, "1.0", childLoc)
	child, err := RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{MaxExclusive: childMax})
	if err != nil {
		t.Fatalf("same exclusive endpoint was rejected by another base facet: %v", err)
	}
	if child.maxExclusive.normalizedLexical != "" {
		t.Fatal("effective exclusive bound retained local normalized lexical state")
	}

	fixedMin := precisionDecimalMinInclusive(t, "1", baseLoc)
	fixedMin.fixed = true
	fixedBase := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalFacetDeclarationsAll(
		nil, nil, nil, []PrecisionDecimalPatternFacet{basePattern}, nil,
		fixedMin, nil, nil, nil, nil,
	))
	_, err = RestrictPrecisionDecimalFacets(fixedBase, PrecisionDecimalFacetDeclarations{MinInclusive: precisionDecimalMinInclusive(t, "1.0", childLoc)})
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCode, childLoc, precisionDecimalMinInclusiveRestrictionSpecRef)
}

func TestPrecisionDecimalBoundEqualityDependsOnInclusivity(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "bound-equality.xsd", 124, 2)
	for _, test := range []struct {
		name string
		min  PrecisionDecimalFacetDeclarations
		ref  string
	}{
		{
			name: "inclusive inclusive",
			min: PrecisionDecimalFacetDeclarations{
				MinInclusive: precisionDecimalMinInclusive(t, "1", loc),
				MaxInclusive: precisionDecimalMaxInclusive(t, "1", loc),
			},
		},
		{
			name: "exclusive exclusive",
			min: PrecisionDecimalFacetDeclarations{
				MinExclusive: precisionDecimalMinExclusive(t, "1", loc),
				MaxExclusive: precisionDecimalMaxExclusive(t, "1", loc),
			},
		},
		{
			name: "inclusive exclusive",
			min: PrecisionDecimalFacetDeclarations{
				MinInclusive: precisionDecimalMinInclusive(t, "1", loc),
				MaxExclusive: precisionDecimalMaxExclusive(t, "1", loc),
			},
			ref: precisionDecimalMinInclusiveMaxExclusiveSpecRef,
		},
		{
			name: "exclusive inclusive",
			min: PrecisionDecimalFacetDeclarations{
				MinExclusive: precisionDecimalMinExclusive(t, "1", loc),
				MaxInclusive: precisionDecimalMaxInclusive(t, "1", loc),
			},
			ref: precisionDecimalMinExclusiveMaxInclusiveSpecRef,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewPrecisionDecimalFacetsFromDeclarations(test.min)
			if test.ref == "" {
				if err != nil {
					t.Fatalf("equal same-kind bounds rejected: %v", err)
				}
				return
			}
			assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCombinationCode, loc, test.ref)
		})
	}
}

func TestPrecisionDecimalLocalMembersValidateAllBaseFacets(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "base-members.xsd", 130, 2)
	tests := []struct {
		name  string
		base  PrecisionDecimalFacetDeclarations
		value string
	}{
		{
			name:  "totalDigits",
			base:  PrecisionDecimalFacetDeclarations{TotalDigits: mustPrecisionDecimalTotalFacet(t, "2", loc, false)},
			value: "123",
		},
		{
			name:  "minScale",
			base:  PrecisionDecimalFacetDeclarations{MinScale: mustPrecisionDecimalMinFacet(t, "2", loc, false)},
			value: "1.0",
		},
		{
			name:  "pattern",
			base:  PrecisionDecimalFacetDeclarations{Patterns: []PrecisionDecimalPatternFacet{mustPrecisionDecimalPatternFacet(t, "3", loc)}},
			value: "4",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := mustPrecisionDecimalValueFacets(t, test.base)
			member := mustPrecisionDecimalEnumerationFacet(t, test.value, loc)
			_, err := RestrictPrecisionDecimalFacets(base, PrecisionDecimalFacetDeclarations{Enumeration: []PrecisionDecimalEnumerationFacet{member}})
			assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalEnumerationCode, loc, precisionDecimalEnumerationRestrictionSpecRef)
			if !errors.Is(err, errInvalidPrecisionDecimalEnumeration) {
				t.Fatalf("base member error lost cause: %v", err)
			}
		})
	}
}

func TestPrecisionDecimalXMLRegexUsesExactXSDPropertiesAndClasses(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "regex-exact.xsd", 140, 2)
	testPrecisionDecimalRegexCategories(t, loc)
	testPrecisionDecimalRegexClasses(t, loc)
	testPrecisionDecimalRegexSurrogateBlocks(t, loc)
}

func testPrecisionDecimalRegexCategories(t *testing.T, loc Loc) {
	t.Helper()
	word := mustPrecisionDecimalPatternFacet(t, `\w`, loc)
	if !word.matches("$") || word.matches("_") || word.matches("\ufdd0") {
		t.Fatal("XSD \\w did not use the complement of P/Z/C")
	}
	unassigned := mustPrecisionDecimalPatternFacet(t, `\p{Cn}`, loc)
	if !unassigned.matches("\ufdd0") {
		t.Fatal("XSD Cn did not include an unassigned XML character")
	}
	categoryC := mustPrecisionDecimalPatternFacet(t, `\p{C}`, loc)
	if !categoryC.matches("\ufdd0") {
		t.Fatal("XSD C did not include Cn")
	}
	assignedComplement := mustPrecisionDecimalPatternFacet(t, `\P{Cn}`, loc)
	if assignedComplement.matches("\ufdd0") || !assignedComplement.matches("A") {
		t.Fatal("XSD \\P{Cn} did not complement Cn")
	}
	for _, source := range []string{`\p{LC}`, `\p{Cs}`, `\p{Letter}`} {
		if _, err := ParsePrecisionDecimalPatternFacet(source, loc); err == nil {
			t.Fatalf("non-XSD category %q was accepted", source)
		}
	}
}

func testPrecisionDecimalRegexClasses(t *testing.T, loc Loc) {
	t.Helper()
	for _, source := range []string{`[--z]`, `[a--b]`, `[^[a-b]]`, `[^]`, `\p{IsLatin-1_Supplement}`} {
		if _, err := ParsePrecisionDecimalPatternFacet(source, loc); err == nil {
			t.Fatalf("invalid XSD regex %q was accepted", source)
		}
	}
	for _, source := range []string{`[-]`, `[-a]+`, `[a-]*`, `[\--z]`, `[a^]`, `[^^]`, `[a-z-[b-z]]`, `[a--[b]]`, `[a-z--[b-z]]`} {
		if _, err := ParsePrecisionDecimalPatternFacet(source, loc); err != nil {
			t.Fatalf("valid XSD regex %q was rejected: %v", source, err)
		}
	}
}

func testPrecisionDecimalRegexSurrogateBlocks(t *testing.T, loc Loc) {
	t.Helper()
	surrogates := mustPrecisionDecimalPatternFacet(t, `\p{IsHighSurrogates}`, loc)
	if surrogates.matches("A") {
		t.Fatal("surrogate block unexpectedly matched XML characters")
	}
	surrogateComplement := mustPrecisionDecimalPatternFacet(t, `\P{IsHighSurrogates}`, loc)
	if !surrogateComplement.matches("A") {
		t.Fatal("surrogate block complement did not match XML characters")
	}
}

func TestPrecisionDecimalXMLRegexResourceLimitsAreStable(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "regex-resource.xsd", 150, 2)
	deepSource := strings.Repeat("(", precisionDecimalXMLRegexMaxDepth+1) + "a" + strings.Repeat(")", precisionDecimalXMLRegexMaxDepth+1)
	flatSource := strings.Repeat("a", precisionDecimalXMLRegexMaxPieces+1)
	overByteSource := strings.Repeat("a", precisionDecimalXMLRegexMaxSourceBytes+1)
	for _, source := range []string{deepSource, flatSource, overByteSource} {
		_, err := ParsePrecisionDecimalPatternFacet(source, loc)
		if err == nil {
			t.Fatalf("resource-heavy pattern %q was accepted", source[:min(len(source), 20)])
		}
		diagnostic := mustDiagnostic(t, err)
		if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedPrecisionDecimalPatternCode || diagnostic.Loc() != loc || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("resource diagnostic = (%q,%q,%v), want unsupported/%s/%v: %v", diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), UnsupportedPrecisionDecimalPatternCode, loc, err)
		}
	}
	hugeQuantifier := mustPrecisionDecimalPatternFacet(t, `a{`+strings.Repeat("9", 4096)+`}`, loc)
	if hugeQuantifier.matches("a") {
		t.Fatal("huge quantifier unexpectedly matched a short source")
	}
	ambiguous := mustPrecisionDecimalPatternFacet(t, `(a?)*`, loc)
	if !ambiguous.matches(strings.Repeat("a", 32)) {
		t.Fatal("ambiguous repetition failed to match without panic")
	}
}

func TestPrecisionDecimalXMLRegexMatchResourceLimitIsUnsupported(t *testing.T) {
	loc := mustPrecisionDecimalFacetLoc(t, "regex-match-resource.xsd", 151, 2)
	pattern := mustPrecisionDecimalPatternFacet(t, strings.Repeat("0*", 128), loc)
	facets := mustPrecisionDecimalValueFacets(t, NewPrecisionDecimalValueFacetDeclarations(
		[]PrecisionDecimalPatternFacet{pattern}, nil, nil, nil, nil, nil, nil,
	))
	err := validatePrecisionDecimalFacets(strings.Repeat("0", 256), facets, loc)
	diagnostic := mustDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedPrecisionDecimalPatternCode || diagnostic.Loc() != loc || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("match resource diagnostic = (%q,%q,%v), want unsupported/%s/%v: %v", diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), UnsupportedPrecisionDecimalPatternCode, loc, err)
	}
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
	assertPrecisionDecimalValueViolation(t, validatePrecisionDecimalFacets("44", facets, valueLoc), valueLoc, precisionDecimalTotalDigitsValidSpecRef)
}

func mustPrecisionDecimalValueFacets(t *testing.T, declarations PrecisionDecimalFacetDeclarations) PrecisionDecimalFacets {
	t.Helper()
	facets, err := NewPrecisionDecimalFacetsFromDeclarations(declarations)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacetsFromDeclarations: %v", err)
	}
	return facets
}

func constructPrecisionDecimalFacetsNoPanic(t *testing.T, declarations PrecisionDecimalFacetDeclarations) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("precisionDecimal facet construction panicked: %v", recovered)
		}
	}()
	_, err = NewPrecisionDecimalFacetsFromDeclarations(declarations)
	return err
}

func restrictPrecisionDecimalFacetsNoPanic(t *testing.T, base PrecisionDecimalFacets, declarations PrecisionDecimalFacetDeclarations) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("precisionDecimal facet restriction panicked: %v", recovered)
		}
	}()
	_, err = RestrictPrecisionDecimalFacets(base, declarations)
	return err
}

func expectPrecisionDecimalBoundRestrictionDiagnostic(t *testing.T, base PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations, loc Loc, specRef string) error {
	t.Helper()
	_, err := RestrictPrecisionDecimalFacets(base, local)
	assertPrecisionDecimalFacetDiagnostic(t, err, InvalidPrecisionDecimalBoundCode, loc, specRef)
	return err
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
