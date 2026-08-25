package goxsd9_test

import (
	"errors"
	"testing"

	"github.com/goxdra/goxsd9"
)

type precisionDecimalValueQuery struct {
	name     string
	lexical  string
	sign     int
	zero     bool
	negative bool
	positive bool
	nan      bool
	infinite bool
	posInf   bool
	negInf   bool
}

type precisionDecimalLibraryFacetFixture struct {
	loc          goxsd9.Loc
	total        goxsd9.PrecisionDecimalTotalDigitsFacet
	minScale     goxsd9.PrecisionDecimalMinScaleFacet
	maxScale     goxsd9.PrecisionDecimalMaxScaleFacet
	pattern      goxsd9.PrecisionDecimalPatternFacet
	enumeration  goxsd9.PrecisionDecimalEnumerationFacet
	minInclusive goxsd9.PrecisionDecimalMinInclusiveFacet
	maxExclusive goxsd9.PrecisionDecimalMaxExclusiveFacet
	whiteSpace   goxsd9.PrecisionDecimalWhiteSpaceFacet
	facets       goxsd9.PrecisionDecimalFacets
	constructed  goxsd9.PrecisionDecimalFacets
}

func TestPrecisionDecimalLibraryValueQueries(t *testing.T) {
	loc, err := goxsd9.NewLoc("library.xsd", 3, 4)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	t.Run("value queries", func(t *testing.T) {
		testPrecisionDecimalLibraryValueQueries(t, loc)
	})
	t.Run("canonical", func(t *testing.T) {
		testPrecisionDecimalLibraryCanonical(t, loc)
	})
	t.Run("zero", func(t *testing.T) {
		testPrecisionDecimalLibraryZero(t)
	})
	t.Run("invalid lexical value", func(t *testing.T) {
		testPrecisionDecimalLibraryInvalidLexicalValue(t, loc)
	})
	t.Run("order strings", func(t *testing.T) {
		testPrecisionDecimalLibraryOrderStrings(t)
	})
}

func testPrecisionDecimalLibraryValueQueries(t *testing.T, loc goxsd9.Loc) {
	tests := []precisionDecimalValueQuery{
		{name: "positive finite", lexical: "12.5", sign: 1},
		{name: "negative finite", lexical: "-12.5", sign: -1},
		{name: "positive zero", lexical: "0", zero: true, positive: true},
		{name: "negative zero", lexical: "-0", zero: true, negative: true},
		{name: "NaN", lexical: "NaN", nan: true},
		{name: "positive infinity", lexical: "+INF", infinite: true, posInf: true},
		{name: "negative infinity", lexical: "-INF", infinite: true, negInf: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, parseErr := goxsd9.ParsePrecisionDecimal(test.lexical, loc)
			if parseErr != nil {
				t.Fatalf("ParsePrecisionDecimal(%q): %v", test.lexical, parseErr)
			}
			assertPrecisionDecimalValueQuery(t, value, test)
		})
	}
}

func assertPrecisionDecimalValueQuery(t *testing.T, value goxsd9.PrecisionDecimal, test precisionDecimalValueQuery) {
	if got := value.Sign(); got != test.sign {
		t.Fatalf("Sign() = %d, want %d", got, test.sign)
	}
	if got := value.IsZero(); got != test.zero {
		t.Fatalf("IsZero() = %t, want %t", got, test.zero)
	}
	if got := value.IsNegativeZero(); got != test.negative {
		t.Fatalf("IsNegativeZero() = %t, want %t", got, test.negative)
	}
	if got := value.IsPositiveZero(); got != test.positive {
		t.Fatalf("IsPositiveZero() = %t, want %t", got, test.positive)
	}
	if got := value.IsNaN(); got != test.nan {
		t.Fatalf("IsNaN() = %t, want %t", got, test.nan)
	}
	if got := value.IsInfinite(); got != test.infinite {
		t.Fatalf("IsInfinite() = %t, want %t", got, test.infinite)
	}
	if got := value.IsPositiveInfinity(); got != test.posInf {
		t.Fatalf("IsPositiveInfinity() = %t, want %t", got, test.posInf)
	}
	if got := value.IsNegativeInfinity(); got != test.negInf {
		t.Fatalf("IsNegativeInfinity() = %t, want %t", got, test.negInf)
	}
}

func testPrecisionDecimalLibraryCanonical(t *testing.T, loc goxsd9.Loc) {
	positive, err := goxsd9.ParsePrecisionDecimal("12.5", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimal(12.5): %v", err)
	}
	canonical, err := positive.CanonicalWithBudget(4, loc)
	if err != nil || canonical != "12.5" {
		t.Fatalf("CanonicalWithBudget() = %q, %v; want 12.5", canonical, err)
	}
}

func testPrecisionDecimalLibraryZero(t *testing.T) {
	var zero goxsd9.StrictPrecisionDecimal
	if zero.Sign() != 0 || !zero.IsZero() || !zero.IsPositiveZero() || zero.IsNegativeZero() {
		t.Fatal("zero StrictPrecisionDecimal is not positive zero")
	}
}

func testPrecisionDecimalLibraryInvalidLexicalValue(t *testing.T, loc goxsd9.Loc) {
	_, err := goxsd9.ParseStrictPrecisionDecimal("not-a-precisionDecimal", loc)
	if err == nil {
		t.Fatal("ParseStrictPrecisionDecimal accepted invalid lexical input")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Loc() != loc {
		t.Fatalf("invalid lexical diagnostic = %v, want location %s", err, loc)
	}
}

func testPrecisionDecimalLibraryOrderStrings(t *testing.T) {
	for _, test := range []struct {
		order goxsd9.PrecisionDecimalOrder
		name  string
	}{
		{order: goxsd9.PrecisionDecimalLess, name: "less"},
		{order: goxsd9.PrecisionDecimalEqual, name: "equal"},
		{order: goxsd9.PrecisionDecimalGreater, name: "greater"},
		{order: goxsd9.PrecisionDecimalUnordered, name: "unordered"},
		{order: goxsd9.PrecisionDecimalOrder(99), name: ""},
	} {
		if got := test.order.String(); got != test.name {
			t.Fatalf("PrecisionDecimalOrder(%d).String() = %q, want %q", test.order, got, test.name)
		}
	}
}

func TestPrecisionDecimalLibraryFacetViews(t *testing.T) {
	fixture := newPrecisionDecimalLibraryFacetFixture(t)
	t.Run("construction", func(t *testing.T) {
		if fixture.constructed.EnumerationCount() != fixture.facets.EnumerationCount() {
			t.Fatalf("constructed enumeration count = %d, want %d", fixture.constructed.EnumerationCount(), fixture.facets.EnumerationCount())
		}
	})
	t.Run("declarations", func(t *testing.T) {
		assertPrecisionDecimalLibraryFacetDeclarations(t, fixture)
	})
	t.Run("facet accessors", func(t *testing.T) {
		assertPrecisionDecimalLibraryFacetAccessors(t, fixture)
	})
	t.Run("effective views", func(t *testing.T) {
		assertPrecisionDecimalLibraryFacetViews(t, fixture)
	})
	t.Run("validation", func(t *testing.T) {
		assertPrecisionDecimalLibraryFacetValidation(t, fixture)
	})
}

func newPrecisionDecimalLibraryFacetFixture(t *testing.T) precisionDecimalLibraryFacetFixture {
	t.Helper()
	loc, err := goxsd9.NewLoc("library-facets.xsd", 7, 2)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	total, err := goxsd9.ParsePrecisionDecimalTotalDigitsFacetWithFixed("4", loc, true)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalTotalDigitsFacet: %v", err)
	}
	minScale, err := goxsd9.ParsePrecisionDecimalMinScaleFacet("-2", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinScaleFacet: %v", err)
	}
	maxScale, err := goxsd9.ParsePrecisionDecimalMaxScaleFacet("2", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMaxScaleFacet: %v", err)
	}
	pattern, err := goxsd9.ParsePrecisionDecimalPatternFacet(`[0-9]+\.[0-9]+`, loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalPatternFacet: %v", err)
	}
	enumeration, err := goxsd9.ParsePrecisionDecimalEnumerationFacet("12.3", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalEnumerationFacet: %v", err)
	}
	minInclusive, err := goxsd9.ParsePrecisionDecimalMinInclusiveFacetWithFixed("1", loc, true)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMinInclusiveFacetWithFixed: %v", err)
	}
	maxExclusive, err := goxsd9.ParsePrecisionDecimalMaxExclusiveFacetWithFixed("20", loc, true)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalMaxExclusiveFacetWithFixed: %v", err)
	}
	whiteSpace, err := goxsd9.ParsePrecisionDecimalWhiteSpaceFacet(" \tcollapse\n", loc)
	if err != nil {
		t.Fatalf("ParsePrecisionDecimalWhiteSpaceFacet: %v", err)
	}

	declarations := goxsd9.NewPrecisionDecimalFacetDeclarationsAll(
		&total,
		&minScale,
		&maxScale,
		[]goxsd9.PrecisionDecimalPatternFacet{pattern},
		[]goxsd9.PrecisionDecimalEnumerationFacet{enumeration},
		&minInclusive,
		nil,
		nil,
		&maxExclusive,
		&whiteSpace,
	)
	facets, err := goxsd9.NewPrecisionDecimalFacetsFromDeclarations(declarations)
	if err != nil {
		t.Fatalf("NewPrecisionDecimalFacetsFromDeclarations: %v", err)
	}
	constructed, err := goxsd9.ConstructPrecisionDecimalFacets(goxsd9.PrecisionDecimalFacets{}, declarations)
	if err != nil {
		t.Fatalf("ConstructPrecisionDecimalFacets: %v", err)
	}
	return precisionDecimalLibraryFacetFixture{
		loc:          loc,
		total:        total,
		minScale:     minScale,
		maxScale:     maxScale,
		pattern:      pattern,
		enumeration:  enumeration,
		minInclusive: minInclusive,
		maxExclusive: maxExclusive,
		whiteSpace:   whiteSpace,
		facets:       facets,
		constructed:  constructed,
	}
}

func assertPrecisionDecimalLibraryFacetDeclarations(t *testing.T, fixture precisionDecimalLibraryFacetFixture) {
	if !fixture.facets.HasPattern() || fixture.facets.PatternGroupCount() != 1 || fixture.facets.PatternCount() != 1 {
		t.Fatalf("pattern facts = (%t, %d, %d), want true/1/1", fixture.facets.HasPattern(), fixture.facets.PatternGroupCount(), fixture.facets.PatternCount())
	}
	patterns := fixture.facets.PatternDeclarations()
	if len(patterns) != 1 || patterns[0].Value() != fixture.pattern.Value() || patterns[0].Loc() != fixture.loc {
		t.Fatalf("pattern declarations = %#v, want the parsed pattern", patterns)
	}
	if !fixture.facets.HasEnumeration() || fixture.facets.EnumerationCount() != 1 {
		t.Fatalf("enumeration facts = (%t, %d), want true/1", fixture.facets.HasEnumeration(), fixture.facets.EnumerationCount())
	}
	members := fixture.facets.EnumerationDeclarations()
	if len(members) != 1 || members[0].Value().Compare(fixture.enumeration.Value()) != goxsd9.PrecisionDecimalEqual || members[0].Loc() != fixture.loc {
		t.Fatalf("enumeration declarations = %#v, want the parsed member", members)
	}
}

func assertPrecisionDecimalLibraryFacetAccessors(t *testing.T, fixture precisionDecimalLibraryFacetFixture) {
	if fixture.total.Value().Canonical() != "4" || !fixture.total.Fixed() || fixture.total.WithFixed(false).Fixed() {
		t.Fatalf("totalDigits accessors lost value or fixed state")
	}
	if fixture.minScale.Value().Canonical() != "-2" || fixture.maxScale.Value().Canonical() != "2" {
		t.Fatal("scale facet accessors lost exact values")
	}
	if !fixture.minInclusive.Fixed() || fixture.minInclusive.WithFixed(false).Fixed() || !fixture.maxExclusive.Fixed() || fixture.maxExclusive.WithFixed(false).Fixed() {
		t.Fatal("bound facet accessors lost fixed state")
	}
	if fixture.whiteSpace.Value() != "collapse" || fixture.whiteSpace.Loc() != fixture.loc {
		t.Fatalf("whiteSpace accessors = (%q, %s), want collapse and %s", fixture.whiteSpace.Value(), fixture.whiteSpace.Loc(), fixture.loc)
	}
}

func assertPrecisionDecimalLibraryFacetViews(t *testing.T, fixture precisionDecimalLibraryFacetFixture) {
	if value, ok := fixture.facets.TotalDigits(); !ok || value.Canonical() != "4" {
		t.Fatalf("TotalDigits() = (%s, %t), want 4,true", value.Canonical(), ok)
	}
	if value, ok := fixture.facets.MinScale(); !ok || value.Canonical() != "-2" {
		t.Fatalf("MinScale() = (%s, %t), want -2,true", value.Canonical(), ok)
	}
	if value, ok := fixture.facets.MaxScale(); !ok || value.Canonical() != "2" {
		t.Fatalf("MaxScale() = (%s, %t), want 2,true", value.Canonical(), ok)
	}
	if _, ok := fixture.facets.MinInclusiveFacet(); !ok {
		t.Fatal("MinInclusiveFacet() omitted the effective lower bound")
	}
	if _, ok := fixture.facets.MaxExclusiveFacet(); !ok {
		t.Fatal("MaxExclusiveFacet() omitted the effective upper bound")
	}
	if _, ok := fixture.facets.MinExclusiveFacet(); ok {
		t.Fatal("MinExclusiveFacet() returned an omitted bound")
	}
	if _, ok := fixture.facets.MaxInclusiveFacet(); ok {
		t.Fatal("MaxInclusiveFacet() returned an omitted bound")
	}
	if value, ok := fixture.facets.WhiteSpaceFacet(); !ok || value.Value() != "collapse" || !value.Fixed() {
		t.Fatalf("WhiteSpaceFacet() = (%q, %t, %t), want collapse,true,true", value.Value(), value.Fixed(), ok)
	}
}

func assertPrecisionDecimalLibraryFacetValidation(t *testing.T, fixture precisionDecimalLibraryFacetFixture) {
	if err := fixture.facets.Validate(" \t12.3\n", fixture.loc); err != nil {
		t.Fatalf("PrecisionDecimalFacets.Validate(valid): %v", err)
	}
	violationErr := goxsd9.ValidatePrecisionDecimalFacets("12.4", fixture.facets, fixture.loc)
	if violationErr == nil {
		t.Fatal("ValidatePrecisionDecimalFacets accepted a non-enumerated value")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(violationErr, &diagnostic) || diagnostic.Code() != goxsd9.PrecisionDecimalFacetValueViolationCode || diagnostic.Loc() != fixture.loc {
		t.Fatalf("facet violation = %v, want located XSD2020 diagnostic", violationErr)
	}

	var empty goxsd9.PrecisionDecimalFacets
	if empty.PatternDeclarations() != nil || empty.EnumerationDeclarations() != nil {
		t.Fatal("empty facet declarations returned non-nil slices")
	}
	for _, test := range []struct {
		name    string
		lexical string
	}{
		{name: "totalDigits", lexical: "4"},
		{name: "minScale", lexical: "-2"},
		{name: "maxScale", lexical: "2"},
		{name: "pattern", lexical: `[0-9]+`},
		{name: "enumeration", lexical: "12.3"},
		{name: "minInclusive", lexical: "1"},
		{name: "minExclusive", lexical: "1"},
		{name: "maxInclusive", lexical: "20"},
		{name: "maxExclusive", lexical: "20"},
		{name: "whiteSpace", lexical: "collapse"},
	} {
		if err := goxsd9.ValidatePrecisionDecimalFacetValue(test.name, test.lexical, fixture.loc); err != nil {
			t.Fatalf("ValidatePrecisionDecimalFacetValue(%q): %v", test.name, err)
		}
	}
	if err := goxsd9.ValidatePrecisionDecimalFacetValue("unknown", "1", fixture.loc); err == nil {
		t.Fatal("ValidatePrecisionDecimalFacetValue accepted an unknown facet")
	}
}
