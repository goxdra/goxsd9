package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // The table covers the four endpoint kinds and equality matrix.
func TestIntegerBoundFacetsCoverEachBoundAndCompatiblePairs(t *testing.T) {
	facetLoc := mustFacetTestLoc(t, "integer-bounds.xsd", 4, 3)
	valueLoc := mustFacetTestLoc(t, "integer-bounds.xml", 8, 7)
	tests := []struct {
		name string
		kind BoundKind
		want string
	}{
		{name: "minInclusive", kind: BoundMinInclusive, want: "5"},
		{name: "minExclusive", kind: BoundMinExclusive, want: "5"},
		{name: "maxInclusive", kind: BoundMaxInclusive, want: "5"},
		{name: "maxExclusive", kind: BoundMaxExclusive, want: "5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facet := mustIntegerBound(t, test.kind, test.want, facetLoc, false, XSDVersion10)
			facets, err := NewIntegerBoundFacets([]IntegerBoundFacet{facet}, XSDVersion10)
			if err != nil {
				t.Fatalf("NewIntegerBoundFacets: %v", err)
			}
			if facets.Version() != XSDVersion10 {
				t.Fatalf("Version() = %q, want %q", facets.Version(), XSDVersion10)
			}
			bounds := facets.Bounds()
			if len(bounds) != 1 || bounds[0].Kind() != test.kind || bounds[0].Value().Canonical() != test.want {
				t.Fatalf("Bounds() = %#v, want one %s=%s", bounds, test.kind, test.want)
			}
			if err := facets.ValidateInteger(StrictInteger{}, valueLoc); err == nil && test.kind == BoundMinExclusive {
				t.Fatal("minExclusive accepted its excluded endpoint")
			}
		})
	}

	for _, test := range []struct {
		name      string
		lower     BoundKind
		upper     BoundKind
		wantError bool
	}{
		{name: "inclusive singleton", lower: BoundMinInclusive, upper: BoundMaxInclusive},
		{name: "exclusive singleton", lower: BoundMinExclusive, upper: BoundMaxExclusive},
		{name: "lower inclusive upper exclusive equality", lower: BoundMinInclusive, upper: BoundMaxExclusive, wantError: true},
		{name: "lower exclusive upper inclusive equality", lower: BoundMinExclusive, upper: BoundMaxInclusive, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			lower := mustIntegerBound(t, test.lower, "5", facetLoc, false, XSDVersion11)
			upper := mustIntegerBound(t, test.upper, "5", facetLoc, false, XSDVersion11)
			facets, err := NewIntegerBoundFacets([]IntegerBoundFacet{lower, upper}, XSDVersion11)
			if test.wantError {
				assertBoundDiagnostic(t, err, InvalidBoundCombinationCode, facetLoc, boundCombinationSpecRef(XSDVersion11, test.lower, test.upper))
				assertNoIntegerBounds(t, facets)
				return
			}
			if err != nil {
				t.Fatalf("NewIntegerBoundFacets: %v", err)
			}
			if len(facets.Bounds()) != 2 {
				t.Fatalf("Bounds() length = %d, want 2", len(facets.Bounds()))
			}
		})
	}
}

func TestDecimalBoundsCompareExactValuesAndSignedZero(t *testing.T) {
	facetLoc := mustFacetTestLoc(t, "decimal-bounds.xsd", 5, 2)
	valueLoc := mustFacetTestLoc(t, "decimal-bounds.xml", 9, 4)
	lower := mustDecimalBound(t, BoundMinInclusive, "1.2300", facetLoc, XSDVersion11)
	upper := mustDecimalBound(t, BoundMaxInclusive, "1.23", facetLoc, XSDVersion11)
	facets, err := NewDecimalBoundFacets([]DecimalBoundFacet{lower, upper}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalBoundFacets(singleton): %v", err)
	}
	value, err := ParseStrictDecimal("1.230", valueLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	validationErr := facets.ValidateDecimal(value, valueLoc)
	if validationErr != nil {
		t.Fatalf("ValidateDecimal rejected equal value with another scale: %v", validationErr)
	}

	negativeZero := mustDecimalBound(t, BoundMinInclusive, "-0.000", facetLoc, XSDVersion10)
	positiveZero := mustDecimalBound(t, BoundMaxInclusive, "+0.0", facetLoc, XSDVersion10)
	zeroFacets, err := NewDecimalBoundFacets([]DecimalBoundFacet{negativeZero, positiveZero}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewDecimalBoundFacets(zero): %v", err)
	}
	zero, err := ParseStrictDecimal("0.000000", valueLoc, XSDVersion10)
	if err != nil {
		t.Fatalf("ParseStrictDecimal(zero): %v", err)
	}
	validationErr = zeroFacets.ValidateDecimal(zero, valueLoc)
	if validationErr != nil {
		t.Fatalf("ValidateDecimal rejected signed-zero equality: %v", validationErr)
	}

	exclusiveLower := mustDecimalBound(t, BoundMinExclusive, "1.2300", facetLoc, XSDVersion11)
	exclusiveUpper := mustDecimalBound(t, BoundMaxExclusive, "1.23", facetLoc, XSDVersion11)
	exclusive, err := NewDecimalBoundFacets([]DecimalBoundFacet{exclusiveLower, exclusiveUpper}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalBoundFacets(exclusive singleton): %v", err)
	}
	if err := exclusive.ValidateDecimal(value, valueLoc); err == nil {
		t.Fatal("exclusive equality unexpectedly accepted its excluded endpoint")
	}
}

func TestDecimalBoundFacetsCoverEachBoundAndRestrictions(t *testing.T) {
	loc := mustFacetTestLoc(t, "decimal-bound-rules.xsd", 4, 3)
	for _, test := range []struct {
		name string
		kind BoundKind
	}{
		{name: "minInclusive", kind: BoundMinInclusive},
		{name: "minExclusive", kind: BoundMinExclusive},
		{name: "maxInclusive", kind: BoundMaxInclusive},
		{name: "maxExclusive", kind: BoundMaxExclusive},
	} {
		t.Run(test.name, func(t *testing.T) {
			facet := mustDecimalBound(t, test.kind, "5.00", loc, XSDVersion11)
			facets, err := NewDecimalBoundFacets([]DecimalBoundFacet{facet}, XSDVersion11)
			if err != nil {
				t.Fatalf("NewDecimalBoundFacets: %v", err)
			}
			bounds := facets.Bounds()
			if len(bounds) != 1 || bounds[0].Kind() != test.kind || !bounds[0].Value().Equal(facet.Value()) {
				t.Fatalf("Bounds() = %#v, want one %s endpoint", bounds, test.kind)
			}
		})
	}

	baseMinLoc := mustFacetTestLoc(t, "decimal-base.xsd", 10, 4)
	baseMaxLoc := mustFacetTestLoc(t, "decimal-base.xsd", 11, 4)
	childLoc := mustFacetTestLoc(t, "decimal-child.xsd", 20, 4)
	baseMin, err := ParseDecimalMinInclusiveFacetWithFixed("1.0", baseMinLoc, true, XSDVersion11)
	if err != nil {
		t.Fatalf("ParseDecimalMinInclusiveFacetWithFixed: %v", err)
	}
	baseMax := mustDecimalBound(t, BoundMaxInclusive, "10.00", baseMaxLoc, XSDVersion11)
	base, err := NewDecimalBoundFacets([]DecimalBoundFacet{baseMin, baseMax}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalBoundFacets(base): %v", err)
	}

	equal := mustDecimalBound(t, BoundMinInclusive, "1.000", childLoc, XSDVersion11)
	child, err := RestrictDecimalBoundFacets(base, NewDecimalBoundFacetDeclarations([]DecimalBoundFacet{equal}))
	if err != nil {
		t.Fatalf("equal fixed decimal restatement: %v", err)
	}
	if fixed, ok := child.MinInclusiveFixed(); !ok || !fixed {
		t.Fatalf("equal decimal restatement fixed = %t/%t, want true/true", fixed, ok)
	}

	narrower := mustDecimalBound(t, BoundMinExclusive, "2.0", childLoc, XSDVersion11)
	if _, restrictErr := RestrictDecimalBoundFacets(base, NewDecimalBoundFacetDeclarations([]DecimalBoundFacet{narrower})); restrictErr != nil {
		t.Fatalf("narrower decimal opposite bound: %v", restrictErr)
	}

	outOfBase := mustDecimalBound(t, BoundMaxInclusive, "11.0", childLoc, XSDVersion11)
	invalid, err := RestrictDecimalBoundFacets(base, NewDecimalBoundFacetDeclarations([]DecimalBoundFacet{outOfBase}))
	assertBoundDiagnostic(t, err, InvalidBoundRestrictionCode, childLoc, "xsd11-datatypes#maxInclusive-valid-restriction")
	assertNoDecimalBounds(t, invalid)

	lower := mustDecimalBound(t, BoundMinInclusive, "2.0", loc, XSDVersion11)
	upper := mustDecimalBound(t, BoundMaxExclusive, "2.0", loc, XSDVersion11)
	invalid, err = NewDecimalBoundFacets([]DecimalBoundFacet{lower, upper}, XSDVersion11)
	assertBoundDiagnostic(t, err, InvalidBoundCombinationCode, loc, boundCombinationSpecRef(XSDVersion11, BoundMinInclusive, BoundMaxExclusive))
	assertNoDecimalBounds(t, invalid)
}

func TestBoundFacetParsingUsesSelectedValueSpaceAndPreservesCause(t *testing.T) {
	loc := mustFacetTestLoc(t, "invalid-bounds.xsd", 7, 6)
	_, err := ParseDecimalMinInclusiveFacet(".5", loc, XSDVersion10)
	if err == nil {
		t.Fatal("XSD 1.0 accepted a decimal lexical form outside its grammar")
	}
	assertBoundDiagnostic(t, err, InvalidBoundCode, loc, "xsd10-datatypes#rf-minInclusive")
	if !errors.Is(err, errInvalidBoundValue) {
		t.Fatalf("invalid lexical form lost bound cause: %v", err)
	}
	var nested Diagnostic
	if !errors.As(errors.Unwrap(err), &nested) || nested.Code() != InvalidDecimalLexicalCode {
		t.Fatalf("invalid lexical form did not preserve decimal diagnostic: %v", err)
	}
	if _, parseErr := ParseDecimalMinInclusiveFacet(".5", loc, XSDVersion11); parseErr != nil {
		t.Fatalf("XSD 1.1 rejected its decimal lexical form: %v", parseErr)
	}
	_, err = ParseIntegerMaxExclusiveFacet("1.2", loc, XSDVersion11)
	if err == nil {
		t.Fatal("integer bound accepted a non-integer lexical form")
	}
	if !errors.Is(err, errInvalidBoundValue) {
		t.Fatalf("invalid integer lexical form lost bound cause: %v", err)
	}
}

func TestBoundRestrictionInheritsFixedValuesAndAllowsOppositeKinds(t *testing.T) {
	baseMinLoc := mustFacetTestLoc(t, "base-bounds.xsd", 10, 4)
	baseMaxLoc := mustFacetTestLoc(t, "base-bounds.xsd", 11, 4)
	childMinLoc := mustFacetTestLoc(t, "child-bounds.xsd", 20, 4)
	childMaxLoc := mustFacetTestLoc(t, "child-bounds.xsd", 21, 4)
	baseMin := mustIntegerBound(t, BoundMinInclusive, "5", baseMinLoc, true, XSDVersion11)
	baseMax := mustIntegerBound(t, BoundMaxInclusive, "10", baseMaxLoc, false, XSDVersion11)
	base, err := NewIntegerBoundFacets([]IntegerBoundFacet{baseMin, baseMax}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets(base): %v", err)
	}

	equalRestatement := mustIntegerBound(t, BoundMinInclusive, "+0005", childMinLoc, false, XSDVersion11)
	child, err := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{equalRestatement}))
	if err != nil {
		t.Fatalf("equal fixed restatement: %v", err)
	}
	if value, ok := child.MinInclusive(); !ok || value.Canonical() != "5" {
		t.Fatalf("inherited minInclusive = %q/%t, want 5/true", value.Canonical(), ok)
	}
	if fixed, ok := child.MinInclusiveFixed(); !ok || !fixed {
		t.Fatalf("equal restatement fixed = %t/%t, want true/true", fixed, ok)
	}
	if loc, ok := child.MinInclusiveLoc(); !ok || loc != childMinLoc {
		t.Fatalf("equal restatement Loc = %v/%t, want %v/true", loc, ok, childMinLoc)
	}

	opposite := mustIntegerBound(t, BoundMinExclusive, "6", childMinLoc, false, XSDVersion11)
	changedKind, err := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{opposite}))
	if err != nil {
		t.Fatalf("opposite bound kind: %v", err)
	}
	if !changedKind.HasMinExclusive() || changedKind.HasMinInclusive() {
		t.Fatalf("opposite bound overlay = minInclusive %t, minExclusive %t", changedKind.HasMinInclusive(), changedKind.HasMinExclusive())
	}

	changedFixed := mustIntegerBound(t, BoundMinInclusive, "6", childMinLoc, false, XSDVersion11)
	invalid, err := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{changedFixed}))
	assertBoundDiagnostic(t, err, InvalidBoundRestrictionCode, childMinLoc, "xsd11-datatypes#f-mii-fixed")
	assertNoIntegerBounds(t, invalid)
	if !errors.Is(err, errInvalidBoundRestriction) {
		t.Fatalf("fixed restriction lost cause: %v", err)
	}

	maxOpposite := mustIntegerBound(t, BoundMaxExclusive, "9", childMaxLoc, false, XSDVersion11)
	if _, err := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{maxOpposite})); err != nil {
		t.Fatalf("changing opposite upper kind should remain allowed: %v", err)
	}
}

func TestBoundRestrictionEnforcesAllEndpointKindCombinations(t *testing.T) {
	loc := mustFacetTestLoc(t, "monotonic.xsd", 4, 2)
	cases := []struct {
		name       string
		parentKind BoundKind
		childKind  BoundKind
		parent     string
		child      string
		wantError  bool
	}{
		{name: "lower inclusive to inclusive equality", parentKind: BoundMinInclusive, childKind: BoundMinInclusive, parent: "5", child: "5"},
		{name: "lower inclusive to exclusive equality", parentKind: BoundMinInclusive, childKind: BoundMinExclusive, parent: "5", child: "5"},
		{name: "lower inclusive to inclusive outward", parentKind: BoundMinInclusive, childKind: BoundMinInclusive, parent: "5", child: "4", wantError: true},
		{name: "lower exclusive to exclusive equality", parentKind: BoundMinExclusive, childKind: BoundMinExclusive, parent: "5", child: "5"},
		{name: "lower exclusive to inclusive equality", parentKind: BoundMinExclusive, childKind: BoundMinInclusive, parent: "5", child: "5", wantError: true},
		{name: "upper inclusive to inclusive equality", parentKind: BoundMaxInclusive, childKind: BoundMaxInclusive, parent: "5", child: "5"},
		{name: "upper inclusive to exclusive equality", parentKind: BoundMaxInclusive, childKind: BoundMaxExclusive, parent: "5", child: "5"},
		{name: "upper inclusive to inclusive outward", parentKind: BoundMaxInclusive, childKind: BoundMaxInclusive, parent: "5", child: "6", wantError: true},
		{name: "upper exclusive to exclusive equality", parentKind: BoundMaxExclusive, childKind: BoundMaxExclusive, parent: "5", child: "5"},
		{name: "upper exclusive to inclusive equality", parentKind: BoundMaxExclusive, childKind: BoundMaxInclusive, parent: "5", child: "5", wantError: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			baseFacet := mustIntegerBound(t, test.parentKind, test.parent, loc, false, XSDVersion11)
			base, err := NewIntegerBoundFacets([]IntegerBoundFacet{baseFacet}, XSDVersion11)
			if err != nil {
				t.Fatalf("NewIntegerBoundFacets(base): %v", err)
			}
			childFacet := mustIntegerBound(t, test.childKind, test.child, loc, false, XSDVersion11)
			child, err := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{childFacet}))
			if test.wantError {
				assertBoundDiagnostic(t, err, InvalidBoundRestrictionCode, loc, "xsd11-datatypes#"+test.childKind.String()+"-valid-restriction")
				assertNoIntegerBounds(t, child)
				return
			}
			if err != nil {
				t.Fatalf("RestrictIntegerBoundFacets: %v", err)
			}
		})
	}
}

func TestBoundVersionDuplicateRulesAndDeclarationOrder(t *testing.T) {
	firstLoc := mustFacetTestLoc(t, "duplicate.xsd", 4, 5)
	secondLoc := mustFacetTestLoc(t, "duplicate.xsd", 5, 5)
	first := mustIntegerBound(t, BoundMinInclusive, "1", firstLoc, false, XSDVersion11)
	second := mustIntegerBound(t, BoundMinExclusive, "2", secondLoc, false, XSDVersion11)
	_, err := NewIntegerBoundFacets([]IntegerBoundFacet{first, second}, XSDVersion11)
	assertBoundDiagnostic(t, err, InvalidBoundCombinationCode, secondLoc, "xsd11-datatypes#minInclusive-minExclusive")
	if related := mustDiagnostic(t, err).Related(); !reflect.DeepEqual(related, []Loc{firstLoc}) {
		t.Fatalf("duplicate related locations = %#v, want [%v]", related, firstLoc)
	}
	if !errors.Is(err, errInvalidBoundCombination) {
		t.Fatalf("duplicate bound lost cause: %v", err)
	}

	maxInclusive := mustIntegerBound(t, BoundMaxInclusive, "10", firstLoc, false, XSDVersion10)
	maxExclusive := mustIntegerBound(t, BoundMaxExclusive, "9", secondLoc, false, XSDVersion10)
	base, err := NewIntegerBoundFacets([]IntegerBoundFacet{maxInclusive}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets(max base): %v", err)
	}
	if _, restrictErr := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{maxExclusive})); restrictErr != nil {
		t.Fatalf("XSD 1.0 upper opposite kind across derivation: %v", restrictErr)
	}

	minInclusive10 := mustIntegerBound(t, BoundMinInclusive, "5", firstLoc, false, XSDVersion10)
	minExclusive10 := mustIntegerBound(t, BoundMinExclusive, "6", secondLoc, false, XSDVersion10)
	minBase10, err := NewIntegerBoundFacets([]IntegerBoundFacet{minInclusive10}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets(min 1.0 base): %v", err)
	}
	if _, restrictErr := RestrictIntegerBoundFacets(minBase10, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{minExclusive10})); restrictErr != nil {
		t.Fatalf("XSD 1.0 lower opposite kind across derivation: %v", restrictErr)
	}
	_, err = NewIntegerBoundFacets([]IntegerBoundFacet{minInclusive10, minExclusive10}, XSDVersion10)
	assertBoundDiagnostic(t, err, InvalidBoundCombinationCode, secondLoc, "xsd10-datatypes#minInclusive-minExclusive")

	minInclusive11 := mustIntegerBound(t, BoundMinInclusive, "5", firstLoc, false, XSDVersion11)
	minExclusive11 := mustIntegerBound(t, BoundMinExclusive, "6", secondLoc, false, XSDVersion11)
	minBase11, err := NewIntegerBoundFacets([]IntegerBoundFacet{minInclusive11}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets(min 1.1 base): %v", err)
	}
	if _, err := RestrictIntegerBoundFacets(minBase11, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{minExclusive11})); err != nil {
		t.Fatalf("XSD 1.1 lower opposite kind across derivation: %v", err)
	}
}

func TestXSD10LowerInclusiveToExclusiveRestrictionsValidateExactValues(t *testing.T) {
	baseLoc := mustFacetTestLoc(t, "xsd10-lower-base.xsd", 10, 4)
	childLoc := mustFacetTestLoc(t, "xsd10-lower-child.xsd", 20, 4)
	for _, test := range []struct {
		name       string
		childValue string
		invalid    string
		valid      string
	}{
		{name: "equal endpoint", childValue: "10", invalid: "10", valid: "11"},
		{name: "stricter endpoint", childValue: "11", invalid: "11", valid: "12"},
	} {
		t.Run("integer/"+test.name, func(t *testing.T) {
			assertXSD10IntegerLowerRestriction(t, baseLoc, childLoc, test.childValue, test.invalid, test.valid)
		})

		t.Run("decimal/"+test.name, func(t *testing.T) {
			assertXSD10DecimalLowerRestriction(t, baseLoc, childLoc, test.childValue, test.invalid, test.valid)
		})
	}
}

func assertXSD10IntegerLowerRestriction(t *testing.T, baseLoc, childLoc Loc, childValue, invalidLexical, validLexical string) {
	t.Helper()
	baseFacet := mustIntegerBound(t, BoundMinInclusive, "10", baseLoc, false, XSDVersion10)
	base, err := NewIntegerBoundFacets([]IntegerBoundFacet{baseFacet}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets: %v", err)
	}
	childFacet := mustIntegerBound(t, BoundMinExclusive, childValue, childLoc, false, XSDVersion10)
	child, err := RestrictIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations([]IntegerBoundFacet{childFacet}))
	if err != nil {
		t.Fatalf("RestrictIntegerBoundFacets: %v", err)
	}
	invalidValue, err := ParseStrictInteger(invalidLexical, childLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(invalid): %v", err)
	}
	validationErr := child.ValidateInteger(invalidValue, childLoc)
	if validationErr == nil {
		t.Fatal("child accepted its exclusive endpoint")
	}
	validValue, err := ParseStrictInteger(validLexical, childLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(valid): %v", err)
	}
	if validationErr = child.ValidateInteger(validValue, childLoc); validationErr != nil {
		t.Fatalf("child rejected valid integer: %v", validationErr)
	}
}

func assertXSD10DecimalLowerRestriction(t *testing.T, baseLoc, childLoc Loc, childValue, invalidLexical, validLexical string) {
	t.Helper()
	baseFacet := mustDecimalBound(t, BoundMinInclusive, "10.0", baseLoc, XSDVersion10)
	base, err := NewDecimalBoundFacets([]DecimalBoundFacet{baseFacet}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewDecimalBoundFacets: %v", err)
	}
	childFacet := mustDecimalBound(t, BoundMinExclusive, childValue+".0", childLoc, XSDVersion10)
	child, err := RestrictDecimalBoundFacets(base, NewDecimalBoundFacetDeclarations([]DecimalBoundFacet{childFacet}))
	if err != nil {
		t.Fatalf("RestrictDecimalBoundFacets: %v", err)
	}
	invalidValue, err := ParseStrictDecimal(invalidLexical+".0", childLoc, XSDVersion10)
	if err != nil {
		t.Fatalf("ParseStrictDecimal(invalid): %v", err)
	}
	validationErr := child.ValidateDecimal(invalidValue, childLoc)
	if validationErr == nil {
		t.Fatal("child accepted its exclusive endpoint")
	}
	validValue, err := ParseStrictDecimal(validLexical+".0", childLoc, XSDVersion10)
	if err != nil {
		t.Fatalf("ParseStrictDecimal(valid): %v", err)
	}
	if validationErr = child.ValidateDecimal(validValue, childLoc); validationErr != nil {
		t.Fatalf("child rejected valid decimal: %v", validationErr)
	}
}

func TestBoundConstructionRejectsContradictionsWithoutReturningState(t *testing.T) {
	loc := mustFacetTestLoc(t, "contradiction.xsd", 3, 3)
	for _, test := range []struct {
		name  string
		lower BoundKind
		upper BoundKind
	}{
		{name: "reverse inclusive", lower: BoundMinInclusive, upper: BoundMaxInclusive},
		{name: "mixed lower exclusive", lower: BoundMinExclusive, upper: BoundMaxInclusive},
		{name: "mixed upper exclusive", lower: BoundMinInclusive, upper: BoundMaxExclusive},
		{name: "reverse exclusive", lower: BoundMinExclusive, upper: BoundMaxExclusive},
	} {
		t.Run(test.name, func(t *testing.T) {
			lowerValue, upperValue := "8", "2"
			if strings.Contains(test.name, "mixed") {
				lowerValue, upperValue = "5", "5"
			}
			lower := mustIntegerBound(t, test.lower, lowerValue, loc, false, XSDVersion11)
			upper := mustIntegerBound(t, test.upper, upperValue, loc, false, XSDVersion11)
			facets, err := NewIntegerBoundFacets([]IntegerBoundFacet{lower, upper}, XSDVersion11)
			assertBoundDiagnostic(t, err, InvalidBoundCombinationCode, loc, boundCombinationSpecRef(XSDVersion11, test.lower, test.upper))
			assertNoIntegerBounds(t, facets)
		})
	}

	valid, err := NewIntegerBoundFacets([]IntegerBoundFacet{
		mustIntegerBound(t, BoundMinInclusive, "5", loc, false, XSDVersion11),
		mustIntegerBound(t, BoundMaxInclusive, "5", loc, false, XSDVersion11),
	}, XSDVersion11)
	if err != nil {
		t.Fatalf("inclusive singleton: %v", err)
	}
	if value, ok := valid.MinInclusive(); !ok || value.Canonical() != "5" {
		t.Fatalf("valid singleton lower = %q/%t", value.Canonical(), ok)
	}
}

func TestBoundValueDiagnosticsUseCandidateAndRelatedLocations(t *testing.T) {
	lowerLoc := mustFacetTestLoc(t, "value-bounds.xsd", 10, 3)
	upperLoc := mustFacetTestLoc(t, "value-bounds.xsd", 11, 3)
	valueLoc := mustFacetTestLoc(t, "value.xml", 2, 9)
	lower := mustIntegerBound(t, BoundMinInclusive, "5", lowerLoc, false, XSDVersion10)
	upper := mustIntegerBound(t, BoundMaxExclusive, "10", upperLoc, false, XSDVersion10)
	facets, err := NewIntegerBoundFacets([]IntegerBoundFacet{lower, upper}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets: %v", err)
	}
	candidate, err := ParseStrictInteger("10", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger: %v", err)
	}
	validationErr := facets.ValidateInteger(candidate, valueLoc)
	assertBoundDiagnostic(t, validationErr, BoundValueViolationCode, valueLoc, "xsd10-datatypes#cvc-maxExclusive-valid")
	if related := mustDiagnostic(t, validationErr).Related(); !reflect.DeepEqual(related, []Loc{upperLoc}) {
		t.Fatalf("value related locations = %#v, want [%v]", related, upperLoc)
	}
	if !errors.Is(validationErr, errBoundValueViolation) {
		t.Fatalf("value violation lost cause: %v", validationErr)
	}
}

func TestBoundValuesAreOwnedAtEveryBoundary(t *testing.T) {
	facetLoc := mustFacetTestLoc(t, "ownership-bounds.xsd", 4, 4)
	huge := strings.Repeat("9", 512)
	integerValue, err := ParseStrictInteger(huge, facetLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger: %v", err)
	}
	facet, err := NewIntegerMinInclusiveFacet(integerValue, facetLoc, true, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerMinInclusiveFacet: %v", err)
	}
	input := []IntegerBoundFacet{facet}
	declarations := NewIntegerBoundFacetDeclarations(input)
	facets, err := NewIntegerBoundFacetsFromDeclarations(declarations, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacetsFromDeclarations: %v", err)
	}
	integerValue.value.SetInt64(1)
	input[0].value.value.SetInt64(2)
	declarations.Bounds[0].value.value.SetInt64(3)
	returned := facets.Bounds()
	returned[0].value.value.SetInt64(4)
	if got := facets.Bounds()[0].Value().Canonical(); got != huge {
		t.Fatalf("integer bound ownership changed value to %q", got)
	}

	decimalValue, err := ParseStrictDecimal("12345678901234567890.123400", facetLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	decimalFacet, err := NewDecimalMaxExclusiveFacet(decimalValue, facetLoc, false, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalMaxExclusiveFacet: %v", err)
	}
	decimalFacets, err := NewDecimalBoundFacets([]DecimalBoundFacet{decimalFacet}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalBoundFacets: %v", err)
	}
	wantDecimal := decimalValue.Canonical()
	decimalValue.coefficient.SetInt64(1)
	decimalFacet.value.coefficient.SetInt64(2)
	decimalReturned := decimalFacets.Bounds()
	decimalReturned[0].value.coefficient.SetInt64(3)
	if got := decimalFacets.Bounds()[0].Value().Canonical(); got != wantDecimal {
		t.Fatalf("decimal bound ownership changed value to %q, want %q", got, wantDecimal)
	}
	if !facets.Bounds()[0].Fixed() {
		t.Fatal("integer fixed state was not retained")
	}
}

func TestBoundVersionMismatchAndInvalidStateReturnNoCompletedSet(t *testing.T) {
	loc := mustFacetTestLoc(t, "version-bounds.xsd", 6, 2)
	facet := mustIntegerBound(t, BoundMinInclusive, "1", loc, false, XSDVersion10)
	facets, err := NewIntegerBoundFacets([]IntegerBoundFacet{facet}, XSDVersion11)
	assertBoundDiagnostic(t, err, InvalidXSDVersionCode, loc, "xsd11-datatypes#rf-minInclusive")
	assertNoIntegerBounds(t, facets)
	if !errors.Is(err, errInvalidBoundVersion) {
		t.Fatalf("version mismatch lost cause: %v", err)
	}

	_, err = NewIntegerBoundFacets([]IntegerBoundFacet{facet}, XSDVersion("2.0"))
	if err == nil {
		t.Fatal("unsupported XSD version was accepted")
	}
	assertBoundDiagnostic(t, err, InvalidXSDVersionCode, loc, "")
}

func mustIntegerBound(t *testing.T, kind BoundKind, lexical string, loc Loc, fixed bool, version XSDVersion) IntegerBoundFacet {
	t.Helper()
	facet, err := ParseIntegerBoundFacetWithFixed(kind, lexical, loc, fixed, version)
	if err != nil {
		t.Fatalf("ParseIntegerBoundFacetWithFixed(%s, %q): %v", kind, lexical, err)
	}
	return facet
}

func mustDecimalBound(t *testing.T, kind BoundKind, lexical string, loc Loc, version XSDVersion) DecimalBoundFacet {
	t.Helper()
	facet, err := ParseDecimalBoundFacet(kind, lexical, loc, version)
	if err != nil {
		t.Fatalf("ParseDecimalBoundFacet(%s, %q): %v", kind, lexical, err)
	}
	return facet
}

func assertBoundDiagnostic(t *testing.T, err error, code string, loc Loc, specRef string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected ordered bound diagnostic")
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

func assertNoIntegerBounds(t *testing.T, facets IntegerBoundFacets) {
	t.Helper()
	if facets.Version() != "" || facets.Bounds() != nil {
		t.Fatalf("failed construction returned completed bounds: version=%q bounds=%#v", facets.Version(), facets.Bounds())
	}
}

func assertNoDecimalBounds(t *testing.T, facets DecimalBoundFacets) {
	t.Helper()
	if facets.Version() != "" || facets.Bounds() != nil {
		t.Fatalf("failed construction returned completed decimal bounds: version=%q bounds=%#v", facets.Version(), facets.Bounds())
	}
}
