package goxsd9

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTotalDigitsUsesThePositiveIntegerDomain(t *testing.T) {
	loc := mustFacetTestLoc(t, "total.xsd", 4, 6)
	for _, test := range []struct {
		name      string
		lexical   string
		canonical string
	}{
		{name: "one", lexical: "1", canonical: "1"},
		{name: "leading plus and zeroes", lexical: "+0001", canonical: "1"},
		{name: "leading zeroes", lexical: "0005", canonical: "5"},
		{name: "collapsed whitespace", lexical: " \t+0005\r\n", canonical: "5"},
		{name: "huge", lexical: strings.Repeat("9", 1024), canonical: strings.Repeat("9", 1024)},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := ParseTotalDigitsFor(XSDVersion10, test.lexical, loc)
			if err != nil {
				t.Fatalf("ParseTotalDigitsFor: %v", err)
			}
			if got := value.Canonical(); got != test.canonical {
				t.Fatalf("Canonical() = %q, want %q", got, test.canonical)
			}
		})
	}

	for _, lexical := range []string{"", "0", "-1", "-0", "+", "1.0", "1 2"} {
		t.Run("invalid "+lexical, func(t *testing.T) {
			_, err := ParseTotalDigitsFor(XSDVersion11, lexical, loc)
			assertFacetDiagnostic(t, err, InvalidTotalDigitsCode, loc, "xsd11-datatypes#rf-totalDigits")
			if !errors.Is(err, errInvalidTotalDigitsValue) {
				t.Fatalf("error does not preserve totalDigits cause: %v", err)
			}
		})
	}
}

func TestParseFractionDigitsUsesTheNonNegativeIntegerDomain(t *testing.T) {
	loc := mustFacetTestLoc(t, "fraction.xsd", 5, 8)
	for _, test := range []struct {
		name      string
		lexical   string
		canonical string
	}{
		{name: "zero", lexical: "0", canonical: "0"},
		{name: "positive zero", lexical: "+000", canonical: "0"},
		{name: "negative zero", lexical: "-0", canonical: "0"},
		{name: "leading zeroes", lexical: "0002", canonical: "2"},
		{name: "huge", lexical: strings.Repeat("8", 1024), canonical: strings.Repeat("8", 1024)},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := ParseFractionDigitsFor(XSDVersion11, test.lexical, loc)
			if err != nil {
				t.Fatalf("ParseFractionDigitsFor: %v", err)
			}
			if got := value.Canonical(); got != test.canonical {
				t.Fatalf("Canonical() = %q, want %q", got, test.canonical)
			}
		})
	}

	for _, lexical := range []string{"", "-1", "-0001", "+", "1.0", "1 2"} {
		t.Run("invalid "+lexical, func(t *testing.T) {
			_, err := ParseFractionDigitsFor(XSDVersion10, lexical, loc)
			assertFacetDiagnostic(t, err, InvalidFractionDigitsCode, loc, "xsd10-datatypes#rf-fractionDigits")
			if !errors.Is(err, errInvalidFractionDigitsValue) {
				t.Fatalf("error does not preserve fractionDigits cause: %v", err)
			}
		})
	}
}

func TestFacetDeclarationsRetainExactValuesAndFixedProperties(t *testing.T) {
	loc := mustFacetTestLoc(t, "facet.xsd", 7, 2)
	total, err := ParseTotalDigitsFacetWithFixed("+0005", loc, true, XSDVersion10)
	if err != nil {
		t.Fatalf("ParseTotalDigitsFacetWithFixed: %v", err)
	}
	if got, want := total.Value().Canonical(), "5"; got != want {
		t.Fatalf("total Value() = %q, want %q", got, want)
	}
	if !total.Fixed() || total.Loc() != loc || total.Version() != XSDVersion10 {
		t.Fatalf("total facet metadata = fixed %t, loc %v, version %q", total.Fixed(), total.Loc(), total.Version())
	}

	returnedValue := total.Value()
	returnedValue.value.SetInt64(99)
	if got, want := total.Value().Canonical(), "5"; got != want {
		t.Fatalf("mutating returned value changed facet to %q, want %q", got, want)
	}

	fraction, err := ParseFractionDigitsFacet("0002", loc, XSDVersion10)
	if err != nil {
		t.Fatalf("ParseFractionDigitsFacet: %v", err)
	}
	if got, want := fraction.Value().Canonical(), "2"; got != want {
		t.Fatalf("fraction Value() = %q, want %q", got, want)
	}
	if fraction.Fixed() || fraction.Loc() != loc || fraction.Version() != XSDVersion10 {
		t.Fatalf("fraction facet metadata = fixed %t, loc %v, version %q", fraction.Fixed(), fraction.Loc(), fraction.Version())
	}
}

func TestStrictDecimalDigitFacetValidationUsesValueCounts(t *testing.T) {
	facetLoc := mustFacetTestLoc(t, "decimal-type.xsd", 12, 5)
	valueLoc := mustFacetTestLoc(t, "instance.xml", 3, 11)
	total, err := ParseTotalDigitsFacet("0005", facetLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("ParseTotalDigitsFacet: %v", err)
	}
	fraction, err := ParseFractionDigitsFacet("2", facetLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("ParseFractionDigitsFacet: %v", err)
	}
	facets, err := NewDecimalDigitFacets(&total, &fraction, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets: %v", err)
	}

	for _, lexical := range []string{"123.4500", "-00123.45"} {
		t.Run("accepted "+lexical, func(t *testing.T) {
			value, parseErr := ParseStrictDecimal(lexical, valueLoc)
			if parseErr != nil {
				t.Fatalf("ParseStrictDecimal: %v", parseErr)
			}
			if validateErr := facets.ValidateDecimal(value, valueLoc); validateErr != nil {
				t.Fatalf("ValidateDecimal(%q): %v", lexical, validateErr)
			}
		})
	}

	tooManyTotal, err := ParseStrictDecimal("12345.67", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictDecimal(tooManyTotal): %v", err)
	}
	assertFacetDiagnostic(t, facets.ValidateDecimal(tooManyTotal, valueLoc), DigitFacetValueViolationCode, valueLoc, "xsd11-datatypes#cvc-totalDigits-valid")

	tooManyFraction, err := ParseStrictDecimal("1.234", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictDecimal(tooManyFraction): %v", err)
	}
	fracFacet, err := NewDecimalDigitFacets(nil, &fraction, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets(fraction only): %v", err)
	}
	assertFacetDiagnostic(t, fracFacet.ValidateDecimal(tooManyFraction, valueLoc), DigitFacetValueViolationCode, valueLoc, "xsd11-datatypes#cvc-fractionDigits-valid")

	related := mustDiagnostic(t, facets.ValidateDecimal(tooManyTotal, valueLoc)).Related()
	if len(related) != 1 || related[0] != facetLoc {
		t.Fatalf("value violation Related() = %#v, want [%v]", related, facetLoc)
	}
}

func TestDecimalDigitCountsCoverZeroTrailingZeroesAndExactBoundaries(t *testing.T) {
	valueLoc := mustFacetTestLoc(t, "values.xml", 2, 4)
	cases := []struct {
		name       string
		lexical    string
		total      string
		fraction   string
		shouldPass bool
	}{
		{name: "zero", lexical: "-000.000", total: "1", fraction: "0", shouldPass: true},
		{name: "fraction trailing zeroes ignored", lexical: "12.34000", total: "4", fraction: "2", shouldPass: true},
		{name: "integer trailing zeroes count", lexical: "1200.000", total: "4", fraction: "0", shouldPass: true},
		{name: "small exact value", lexical: "0.0000012300", total: "8", fraction: "8", shouldPass: true},
		{name: "one digit total excess", lexical: "123456", total: "5", fraction: "0", shouldPass: false},
		{name: "one digit fraction excess", lexical: "1.234", total: "4", fraction: "2", shouldPass: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			total := mustTotalFacet(t, test.total, valueLoc)
			fraction := mustFractionFacet(t, test.fraction, valueLoc)
			facets, err := NewDecimalDigitFacets(&total, &fraction)
			if err != nil {
				t.Fatalf("NewDecimalDigitFacets: %v", err)
			}
			value, err := ParseStrictDecimal(test.lexical, valueLoc)
			if err != nil {
				t.Fatalf("ParseStrictDecimal: %v", err)
			}
			validationErr := facets.ValidateDecimal(value, valueLoc)
			if test.shouldPass && validationErr != nil {
				t.Fatalf("ValidateDecimal rejected valid value: %v", validationErr)
			}
			if !test.shouldPass && validationErr == nil {
				t.Fatal("ValidateDecimal accepted invalid value")
			}
		})
	}
}

func TestStrictIntegerDigitFacetValidationUsesExactMagnitude(t *testing.T) {
	facetLoc := mustFacetTestLoc(t, "integer-type.xsd", 15, 9)
	valueLoc := mustFacetTestLoc(t, "integer.xml", 5, 3)
	total := mustTotalFacet(t, "4", facetLoc)
	fraction := mustFractionFacet(t, "0", facetLoc)
	facets, err := NewDigitFacets(DigitDatatypeInteger, &total, &fraction, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDigitFacets(integer): %v", err)
	}

	for _, lexical := range []string{"0", "-000", "+001200", "-9999"} {
		t.Run("accepted "+lexical, func(t *testing.T) {
			value, parseErr := ParseStrictInteger(lexical, valueLoc)
			if parseErr != nil {
				t.Fatalf("ParseStrictInteger: %v", parseErr)
			}
			if validateErr := facets.ValidateInteger(value, valueLoc); validateErr != nil {
				t.Fatalf("ValidateInteger(%q): %v", lexical, validateErr)
			}
		})
	}

	tooMany, err := ParseStrictInteger("-10000", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(tooMany): %v", err)
	}
	assertFacetDiagnostic(t, facets.ValidateInteger(tooMany, valueLoc), DigitFacetValueViolationCode, valueLoc, "xsd11-datatypes#cvc-totalDigits-valid")

	hugeLimit := mustTotalFacetFor(t, strings.Repeat("7", 600), facetLoc, XSDVersion10)
	hugeFacets, err := NewIntegerDigitFacets(&hugeLimit, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerDigitFacets(huge): %v", err)
	}
	value, err := ParseStrictInteger(strings.Repeat("8", 2048), valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(large): %v", err)
	}
	if err := hugeFacets.ValidateInteger(value, valueLoc); err != nil {
		t.Fatalf("ValidateInteger(large under huge exact limit): %v", err)
	}
}

func TestDigitFacetConstructionInheritsAndRestrictsDeterministically(t *testing.T) {
	baseTotalLoc := mustFacetTestLoc(t, "base.xsd", 10, 3)
	baseFractionLoc := mustFacetTestLoc(t, "base.xsd", 11, 3)
	childTotalLoc := mustFacetTestLoc(t, "child.xsd", 20, 4)
	childFractionLoc := mustFacetTestLoc(t, "child.xsd", 21, 4)
	baseTotal := mustTotalFacetFor(t, "5", baseTotalLoc, XSDVersion10)
	baseFraction := mustFractionFacetFor(t, "3", baseFractionLoc, XSDVersion10)
	base, err := NewDecimalDigitFacets(&baseTotal, &baseFraction, XSDVersion10)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets(base): %v", err)
	}

	childFraction := mustFractionFacetFor(t, "2", childFractionLoc, XSDVersion10)
	child, err := RestrictDigitFacets(base, NewDigitFacetDeclarations(nil, &childFraction))
	if err != nil {
		t.Fatalf("RestrictDigitFacets(inherit total): %v", err)
	}
	if total, ok := child.TotalDigits(); !ok || total.Canonical() != "5" {
		t.Fatalf("inherited totalDigits = (%v, %t), want 5,true", total, ok)
	}
	if fraction, ok := child.FractionDigits(); !ok || fraction.Canonical() != "2" {
		t.Fatalf("local fractionDigits = (%v, %t), want 2,true", fraction, ok)
	}
	if got, ok := child.TotalDigitsLoc(); !ok || got != baseTotalLoc {
		t.Fatalf("inherited totalDigits Loc() = (%v, %t), want %v,true", got, ok, baseTotalLoc)
	}

	tooBroadTotal := mustTotalFacetFor(t, "6", childTotalLoc, XSDVersion10)
	_, err = RestrictDigitFacets(base, NewDigitFacetDeclarations(&tooBroadTotal, nil))
	assertFacetDiagnostic(t, err, InvalidDigitFacetRestrictionCode, childTotalLoc, "xsd10-datatypes#totalDigits-valid-restriction")
	if related := mustDiagnostic(t, err).Related(); len(related) != 1 || related[0] != baseTotalLoc {
		t.Fatalf("total restriction Related() = %#v, want [%v]", related, baseTotalLoc)
	}

	tooBroadFraction := mustFractionFacetFor(t, "4", childFractionLoc, XSDVersion10)
	_, err = RestrictDigitFacets(base, NewDigitFacetDeclarations(nil, &tooBroadFraction))
	assertFacetDiagnostic(t, err, InvalidDigitFacetRestrictionCode, childFractionLoc, "xsd10-datatypes#fractionDigits-valid-restriction")
	if related := mustDiagnostic(t, err).Related(); len(related) != 1 || related[0] != baseFractionLoc {
		t.Fatalf("fraction restriction Related() = %#v, want [%v]", related, baseFractionLoc)
	}

	hugeBase := mustTotalFacetFor(t, "1"+strings.Repeat("0", 512), baseTotalLoc, XSDVersion10)
	hugeChild := mustTotalFacetFor(t, "9"+strings.Repeat("9", 511), childTotalLoc, XSDVersion10)
	hugeBaseFacets, err := NewDecimalDigitFacets(&hugeBase, nil, XSDVersion10)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets(huge base): %v", err)
	}
	if _, restrictionErr := RestrictDigitFacets(hugeBaseFacets, NewDigitFacetDeclarations(&hugeChild, nil)); restrictionErr != nil {
		t.Fatalf("RestrictDigitFacets accepted exact huge comparison input: %v", restrictionErr)
	}
	hugeTooBroad := mustTotalFacetFor(t, "1"+strings.Repeat("0", 512)+"0", childTotalLoc, XSDVersion10)
	_, err = RestrictDigitFacets(hugeBaseFacets, NewDigitFacetDeclarations(&hugeTooBroad, nil))
	assertFacetDiagnostic(t, err, InvalidDigitFacetRestrictionCode, childTotalLoc, "xsd10-datatypes#totalDigits-valid-restriction")
}

func TestDigitFacetConstructionRejectsFractionTotalAndHonorsFixed(t *testing.T) {
	totalLoc := mustFacetTestLoc(t, "combination.xsd", 4, 2)
	fractionLoc := mustFacetTestLoc(t, "combination.xsd", 5, 2)
	total := mustTotalFacet(t, "2", totalLoc)
	fraction := mustFractionFacet(t, "3", fractionLoc)
	_, err := NewDecimalDigitFacets(&total, &fraction, XSDVersion11)
	assertFacetDiagnostic(t, err, InvalidDigitFacetCombinationCode, fractionLoc, "xsd11-datatypes#fractionDigits-totalDigits")
	if related := mustDiagnostic(t, err).Related(); len(related) != 1 || related[0] != totalLoc {
		t.Fatalf("combination Related() = %#v, want [%v]", related, totalLoc)
	}

	fixedTotal := mustTotalFacet(t, "5", totalLoc).WithFixed(true)
	base, err := NewDecimalDigitFacets(&fixedTotal, nil, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets(fixed): %v", err)
	}
	equal := mustTotalFacet(t, "5", fractionLoc)
	child, err := RestrictDigitFacets(base, NewDigitFacetDeclarations(&equal, nil))
	if err != nil {
		t.Fatalf("equal fixed restriction: %v", err)
	}
	if fixed, ok := child.TotalDigitsFixed(); !ok || !fixed {
		t.Fatalf("fixed totalDigits = (%t, %t), want true,true", fixed, ok)
	}

	changed := mustTotalFacet(t, "4", fractionLoc)
	_, err = RestrictDigitFacets(base, NewDigitFacetDeclarations(&changed, nil))
	assertFacetDiagnostic(t, err, InvalidDigitFacetRestrictionCode, fractionLoc, "xsd11-datatypes#f-td-fixed")
}

func TestIntegerFractionDigitsIsFixedAtZero(t *testing.T) {
	totalLoc := mustFacetTestLoc(t, "integer-base.xsd", 8, 2)
	fractionLoc := mustFacetTestLoc(t, "integer-child.xsd", 14, 3)
	total := mustTotalFacet(t, "6", totalLoc)
	base, err := NewIntegerDigitFacets(&total, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerDigitFacets: %v", err)
	}
	fraction, ok := base.FractionDigits()
	if !ok || fraction.Canonical() != "0" {
		t.Fatalf("integer effective fractionDigits = (%v, %t), want 0,true", fraction, ok)
	}
	if fixed, ok := base.FractionDigitsFixed(); !ok || !fixed {
		t.Fatalf("integer fractionDigits fixed = (%t, %t), want true,true", fixed, ok)
	}

	nonzero := mustFractionFacet(t, "1", fractionLoc)
	_, err = RestrictDigitFacets(base, NewDigitFacetDeclarations(nil, &nonzero))
	assertFacetDiagnostic(t, err, InvalidDigitFacetRestrictionCode, fractionLoc, "xsd11-datatypes#integer.fractionDigits")

	zero := mustFractionFacet(t, "-0", fractionLoc)
	child, err := RestrictDigitFacets(base, NewDigitFacetDeclarations(nil, &zero))
	if err != nil {
		t.Fatalf("explicit integer fractionDigits=0: %v", err)
	}
	if fixed, ok := child.FractionDigitsFixed(); !ok || !fixed {
		t.Fatalf("explicit integer fractionDigits fixed = (%t, %t), want true,true", fixed, ok)
	}
}

func TestDigitFacetVersionReferencesAreExplicit(t *testing.T) {
	loc := mustFacetTestLoc(t, "version.xsd", 2, 1)
	for _, version := range []XSDVersion{XSDVersion10, XSDVersion11} {
		t.Run(string(version), func(t *testing.T) {
			assertFacetVersionReference(t, loc, version)
		})
	}
}

func assertFacetVersionReference(t *testing.T, loc Loc, version XSDVersion) {
	t.Helper()
	total, err := ParseTotalDigitsFacetFor(version, "3", loc)
	if err != nil {
		t.Fatalf("ParseTotalDigitsFacetFor: %v", err)
	}
	fraction, err := ParseFractionDigitsFacetFor(version, "1", loc)
	if err != nil {
		t.Fatalf("ParseFractionDigitsFacetFor: %v", err)
	}
	facets, err := NewDecimalDigitFacets(&total, &fraction, version)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets: %v", err)
	}
	value, err := ParseStrictDecimal("1.23", loc, version)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	validationErr := facets.ValidateDecimal(value, loc)
	if validationErr == nil {
		t.Fatal("ValidateDecimal accepted a value with too many fraction digits")
	}
	diagnostic := mustDiagnostic(t, validationErr)
	want := "xsd" + string(version[0]) + string(version[2]) + "-datatypes#cvc-fractionDigits-valid"
	if diagnostic.SpecRef() != want {
		t.Fatalf("SpecRef() = %q, want %q", diagnostic.SpecRef(), want)
	}
}

func TestDigitFacetValueViolationPreservesCause(t *testing.T) {
	facetLoc := mustFacetTestLoc(t, "cause-type.xsd", 6, 5)
	valueLoc := mustFacetTestLoc(t, "cause.xml", 9, 7)
	total := mustTotalFacet(t, "1", facetLoc)
	facets, err := NewDecimalDigitFacets(&total, nil)
	if err != nil {
		t.Fatalf("NewDecimalDigitFacets: %v", err)
	}
	value, err := ParseStrictDecimal("12", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	validationErr := facets.ValidateDecimal(value, valueLoc)
	if !errors.Is(validationErr, errDigitFacetValueViolation) {
		t.Fatalf("value violation does not preserve cause: %v", validationErr)
	}
}

func FuzzDigitFacetBoundariesDoNotPanicOrRound(f *testing.F) {
	for _, seed := range []struct {
		total    string
		fraction string
		value    string
	}{
		{total: "1", fraction: "0", value: "0"},
		{total: "+0005", fraction: "0002", value: "123.45"},
		{total: strings.Repeat("9", 128), fraction: strings.Repeat("8", 127), value: "0." + strings.Repeat("1", 127)},
		{total: "-0", fraction: "-0", value: "-0.000"},
	} {
		f.Add(seed.total, seed.fraction, seed.value)
	}
	f.Fuzz(func(_ *testing.T, totalLexical, fractionLexical, valueLexical string) {
		loc := Loc{}
		total, totalErr := ParseTotalDigits(totalLexical, loc)
		fraction, fractionErr := ParseFractionDigits(fractionLexical, loc)
		value, valueErr := ParseStrictDecimal(valueLexical, loc)
		if totalErr != nil || fractionErr != nil || valueErr != nil {
			return
		}
		totalFacet, err := NewTotalDigitsFacet(total, loc, false)
		if err != nil {
			return
		}
		fractionFacet, err := NewFractionDigitsFacet(fraction, loc, false)
		if err != nil {
			return
		}
		facets, err := NewDecimalDigitFacets(&totalFacet, &fractionFacet)
		if err != nil {
			return
		}
		if err := facets.ValidateDecimal(value, loc); err != nil {
			return
		}
	})
}

func FuzzIntegerDigitFacetBoundariesDoNotPanicOrRound(f *testing.F) {
	for _, lexical := range []string{"0", "-000", "+0012", "-9999", strings.Repeat("7", 128), "1e2", "-"} {
		f.Add(lexical)
	}
	f.Fuzz(func(_ *testing.T, lexical string) {
		value, err := ParseStrictInteger(lexical, Loc{})
		if err != nil {
			return
		}
		limit, err := ParseTotalDigitsFacet(strings.Repeat("9", len(lexical)+1), Loc{})
		if err != nil {
			return
		}
		facets, err := NewIntegerDigitFacets(&limit)
		if err != nil {
			return
		}
		if err := facets.ValidateInteger(value, Loc{}); err != nil {
			return
		}
	})
}

func mustFacetTestLoc(t *testing.T, source SourceID, line, column int) Loc {
	t.Helper()
	loc, err := NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func mustTotalFacet(t *testing.T, lexical string, loc Loc) TotalDigitsFacet {
	t.Helper()
	return mustTotalFacetFor(t, lexical, loc, XSDVersion11)
}

func mustTotalFacetFor(t *testing.T, lexical string, loc Loc, version XSDVersion) TotalDigitsFacet {
	t.Helper()
	facet, err := ParseTotalDigitsFacetFor(version, lexical, loc)
	if err != nil {
		t.Fatalf("ParseTotalDigitsFacetFor(%q): %v", lexical, err)
	}
	return facet
}

func mustFractionFacet(t *testing.T, lexical string, loc Loc) FractionDigitsFacet {
	t.Helper()
	return mustFractionFacetFor(t, lexical, loc, XSDVersion11)
}

func mustFractionFacetFor(t *testing.T, lexical string, loc Loc, version XSDVersion) FractionDigitsFacet {
	t.Helper()
	facet, err := ParseFractionDigitsFacetFor(version, lexical, loc)
	if err != nil {
		t.Fatalf("ParseFractionDigitsFacetFor(%q): %v", lexical, err)
	}
	return facet
}

func assertFacetDiagnostic(t *testing.T, err error, code string, loc Loc, specRef string) {
	t.Helper()
	if err == nil {
		t.Fatal("operation accepted invalid facet input")
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

func mustDiagnostic(t *testing.T, err error) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("expected a diagnostic")
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	return diagnostic
}
