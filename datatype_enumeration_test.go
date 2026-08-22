package goxsd9

import (
	"errors"
	"math/big"
	"reflect"
	"testing"
)

func TestIntegerEnumerationFacetsUseExactValuesAndOrderedLocations(t *testing.T) {
	firstLoc := mustEnumerationLoc(t, "integer-enum.xsd", 4, 8)
	secondLoc := mustEnumerationLoc(t, "integer-enum.xsd", 5, 8)
	thirdLoc := mustEnumerationLoc(t, "integer-enum.xsd", 6, 8)
	valueLoc := mustEnumerationLoc(t, "integer-enum.xml", 12, 4)

	first := mustIntegerEnumerationFacet(t, "+0007", firstLoc, XSDVersion10)
	second := mustIntegerEnumerationFacet(t, "-0", secondLoc, XSDVersion10)
	third := mustIntegerEnumerationFacet(t, "123456789012345678901234567890", thirdLoc, XSDVersion10)
	values := []IntegerEnumerationFacet{first, second, third}
	facets, err := NewIntegerEnumerationFacets(values, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerEnumerationFacets: %v", err)
	}
	if !facets.HasEnumeration() || facets.Len() != len(values) {
		t.Fatalf("enumeration presence/length = (%t, %d), want true,%d", facets.HasEnumeration(), facets.Len(), len(values))
	}
	if facets.Version() != XSDVersion10 {
		t.Fatalf("Version() = %q, want %q", facets.Version(), XSDVersion10)
	}
	if got, want := integerEnumerationCanonicals(facets.Values()), []string{"7", "0", "123456789012345678901234567890"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
	if got, want := facets.Locations(), []Loc{firstLoc, secondLoc, thirdLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Locations() = %#v, want %#v", got, want)
	}

	accepted, err := ParseStrictInteger("0000007", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(accepted): %v", err)
	}
	acceptedErr := facets.ValidateInteger(accepted, valueLoc)
	if acceptedErr != nil {
		t.Fatalf("ValidateInteger accepted equivalent lexical form: %v", acceptedErr)
	}
	zero, err := ParseStrictInteger("+000", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(zero): %v", err)
	}
	zeroErr := facets.ValidateInteger(zero, valueLoc)
	if zeroErr != nil {
		t.Fatalf("ValidateInteger rejected integer zero equivalence: %v", zeroErr)
	}

	candidate, err := ParseStrictInteger("8", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictInteger(candidate): %v", err)
	}
	validationErr := facets.ValidateInteger(candidate, valueLoc)
	assertEnumerationDiagnostic(t, validationErr, EnumerationValueViolationCode, valueLoc, "xsd10-datatypes#cvc-enumeration-valid")
	if related := mustDiagnostic(t, validationErr).Related(); !reflect.DeepEqual(related, []Loc{firstLoc, secondLoc, thirdLoc}) {
		t.Fatalf("value violation Related() = %#v, want declaration order", related)
	}
	if !errors.Is(validationErr, errEnumerationValueViolation) {
		t.Fatalf("value violation does not preserve cause: %v", validationErr)
	}
}

func TestDecimalEnumerationFacetsUseValueSpaceEqualityAndZeroEquivalence(t *testing.T) {
	firstLoc := mustEnumerationLoc(t, "decimal-enum.xsd", 4, 8)
	secondLoc := mustEnumerationLoc(t, "decimal-enum.xsd", 5, 8)
	zeroLoc := mustEnumerationLoc(t, "decimal-enum.xsd", 6, 8)
	valueLoc := mustEnumerationLoc(t, "decimal-enum.xml", 12, 4)

	first := mustDecimalEnumerationFacet(t, "1.2300", firstLoc, XSDVersion11)
	second := mustDecimalEnumerationFacet(t, "1.23", secondLoc, XSDVersion11)
	zero := mustDecimalEnumerationFacet(t, "-0.000", zeroLoc, XSDVersion11)
	facets, err := NewDecimalEnumerationFacets([]DecimalEnumerationFacet{first, second, zero}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalEnumerationFacets: %v", err)
	}
	if got, want := decimalEnumerationCanonicals(facets.Values()), []string{"1.23", "1.23", "0.0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}

	for _, lexical := range []string{"1.230", "1.23000000000000000000000000000000000000000000000001"} {
		candidate, parseErr := ParseStrictDecimal(lexical, valueLoc)
		if parseErr != nil {
			t.Fatalf("ParseStrictDecimal(%q): %v", lexical, parseErr)
		}
		wantAccepted := lexical == "1.230"
		validationErr := facets.ValidateDecimal(candidate, valueLoc)
		if wantAccepted && validationErr != nil {
			t.Fatalf("ValidateDecimal rejected %q: %v", lexical, validationErr)
		}
		if !wantAccepted {
			assertEnumerationDiagnostic(t, validationErr, EnumerationValueViolationCode, valueLoc, "xsd11-datatypes#cvc-enumeration-valid")
		}
	}

	positiveZero, err := ParseStrictDecimal("+0.00", valueLoc)
	if err != nil {
		t.Fatalf("ParseStrictDecimal(positive zero): %v", err)
	}
	if err := facets.ValidateDecimal(positiveZero, valueLoc); err != nil {
		t.Fatalf("ValidateDecimal rejected positive zero for negative-zero declaration: %v", err)
	}
}

func TestEnumerationInheritanceNarrowingAndBaseMembership(t *testing.T) {
	baseFirstLoc := mustEnumerationLoc(t, "base.xsd", 10, 3)
	baseSecondLoc := mustEnumerationLoc(t, "base.xsd", 11, 3)
	baseThirdLoc := mustEnumerationLoc(t, "base.xsd", 12, 3)
	childFirstLoc := mustEnumerationLoc(t, "child.xsd", 20, 3)
	childSecondLoc := mustEnumerationLoc(t, "child.xsd", 21, 3)

	base, err := NewIntegerEnumerationFacets([]IntegerEnumerationFacet{
		mustIntegerEnumerationFacet(t, "1", baseFirstLoc, XSDVersion10),
		mustIntegerEnumerationFacet(t, "2", baseSecondLoc, XSDVersion10),
		mustIntegerEnumerationFacet(t, "3", baseThirdLoc, XSDVersion10),
	}, XSDVersion10)
	if err != nil {
		t.Fatalf("NewIntegerEnumerationFacets(base): %v", err)
	}

	inherited, err := RestrictIntegerEnumerationFacets(base, IntegerEnumerationFacetDeclarations{})
	if err != nil {
		t.Fatalf("RestrictIntegerEnumerationFacets(inherited): %v", err)
	}
	if got, want := inherited.Locations(), []Loc{baseFirstLoc, baseSecondLoc, baseThirdLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inherited Locations() = %#v, want %#v", got, want)
	}

	narrowed, err := RestrictIntegerEnumerationFacets(base, NewIntegerEnumerationFacetDeclarations([]IntegerEnumerationFacet{
		mustIntegerEnumerationFacet(t, "3", childFirstLoc, XSDVersion10),
		mustIntegerEnumerationFacet(t, "+0003", childSecondLoc, XSDVersion10),
	}))
	if err != nil {
		t.Fatalf("RestrictIntegerEnumerationFacets(narrowed): %v", err)
	}
	if got, want := narrowed.Locations(), []Loc{childFirstLoc, childSecondLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("narrowed Locations() = %#v, want local declaration order", got)
	}
	narrowedErr := narrowed.ValidateInteger(StrictInteger{}, childFirstLoc)
	if narrowedErr == nil {
		t.Fatal("narrowed enumeration unexpectedly accepted omitted integer zero")
	}

	outOfBaseLoc := mustEnumerationLoc(t, "child.xsd", 22, 3)
	_, err = RestrictIntegerEnumerationFacets(base, NewIntegerEnumerationFacetDeclarations([]IntegerEnumerationFacet{
		mustIntegerEnumerationFacet(t, "4", outOfBaseLoc, XSDVersion10),
		mustIntegerEnumerationFacet(t, "2", childSecondLoc, XSDVersion10),
	}))
	assertEnumerationDiagnostic(t, err, InvalidEnumerationRestrictionCode, outOfBaseLoc, "xsd10-datatypes#enumeration-valid-restriction")
	if related := mustDiagnostic(t, err).Related(); !reflect.DeepEqual(related, []Loc{baseFirstLoc, baseSecondLoc, baseThirdLoc}) {
		t.Fatalf("restriction Related() = %#v, want base declaration order", related)
	}
	if !errors.Is(err, errInvalidEnumerationRestriction) {
		t.Fatalf("restriction does not preserve cause: %v", err)
	}
}

func TestEnumerationAllowsLocalValuesWhenBaseOmitsFacet(t *testing.T) {
	loc := mustEnumerationLoc(t, "unconstrained.xsd", 4, 3)
	base, err := NewIntegerEnumerationFacets(nil, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerEnumerationFacets(empty): %v", err)
	}
	if base.HasEnumeration() {
		t.Fatal("nil local values unexpectedly created an enumeration")
	}
	child, err := RestrictIntegerEnumerationFacets(base, NewIntegerEnumerationFacetDeclarations([]IntegerEnumerationFacet{
		mustIntegerEnumerationFacet(t, "9", loc, XSDVersion11),
	}))
	if err != nil {
		t.Fatalf("RestrictIntegerEnumerationFacets(unconstrained base): %v", err)
	}
	if !child.HasEnumeration() || child.Len() != 1 {
		t.Fatalf("child enumeration presence/length = (%t, %d), want true,1", child.HasEnumeration(), child.Len())
	}
}

func TestEnumerationRejectsInvalidLocalDeclarationsBeforeCompletion(t *testing.T) {
	loc := mustEnumerationLoc(t, "invalid-enum.xsd", 8, 5)
	_, err := ParseDecimalEnumerationFacetFor(XSDVersion10, ".5", loc)
	if err == nil {
		t.Fatal("XSD 1.0 decimal enumeration accepted an invalid lexical form")
	}
	assertEnumerationDiagnostic(t, err, InvalidEnumerationCode, loc, "xsd10-datatypes#rf-enumeration")
	if errors.Is(err, ErrUnsupported) {
		t.Fatal("invalid enumeration lexical form was classified as unsupported")
	}
	if !errors.Is(err, errInvalidEnumerationValue) {
		t.Fatalf("invalid enumeration lexical form does not preserve cause: %v", err)
	}

	_, err = NewIntegerEnumerationFacets([]IntegerEnumerationFacet{}, XSDVersion11)
	if err == nil {
		t.Fatal("present empty integer enumeration was accepted")
	}
	assertEnumerationDiagnostic(t, err, InvalidEnumerationCode, Loc{}, "xsd11-datatypes#rf-enumeration")
	if !errors.Is(err, errInvalidEnumerationValue) {
		t.Fatalf("empty enumeration does not preserve cause: %v", err)
	}
}

func TestEnumerationValuesAreOwnedAtEveryBoundary(t *testing.T) {
	facetLoc := mustEnumerationLoc(t, "ownership.xsd", 4, 3)
	integerValue := StrictInteger{value: new(big.Int).Exp(big.NewInt(10), big.NewInt(300), nil)}
	integerFacet, err := NewIntegerEnumerationFacet(integerValue, facetLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerEnumerationFacet: %v", err)
	}
	input := []IntegerEnumerationFacet{integerFacet}
	declarations := NewIntegerEnumerationFacetDeclarations(input)
	facets, err := NewIntegerEnumerationFacetsFromDeclarations(declarations, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerEnumerationFacetsFromDeclarations: %v", err)
	}
	wantInteger := integerValue.Canonical()

	integerValue.value.SetInt64(1)
	input[0].value.value.SetInt64(2)
	declarations.Values[0].value.value.SetInt64(3)
	returned := facets.Values()
	returned[0].value.SetInt64(4)
	if got := facets.Values()[0].Canonical(); got != wantInteger {
		t.Fatalf("integer effective value changed through caller state: %q, want %q", got, wantInteger)
	}

	decimalValue, err := ParseStrictDecimal("12345678901234567890.123400", facetLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	decimalFacet, err := NewDecimalEnumerationFacet(decimalValue, facetLoc, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalEnumerationFacet: %v", err)
	}
	decimalFacets, err := NewDecimalEnumerationFacets([]DecimalEnumerationFacet{decimalFacet}, XSDVersion11)
	if err != nil {
		t.Fatalf("NewDecimalEnumerationFacets: %v", err)
	}
	wantDecimal := decimalValue.Canonical()
	decimalValue.coefficient.SetInt64(1)
	decimalFacet.value.coefficient.SetInt64(2)
	decimalReturned := decimalFacets.Values()
	decimalReturned[0].coefficient.SetInt64(3)
	if got := decimalFacets.Values()[0].Canonical(); got != wantDecimal {
		t.Fatalf("decimal effective value changed through caller state: %q, want %q", got, wantDecimal)
	}
}

func TestEnumerationDiagnosticsUseBothVersionedReferences(t *testing.T) {
	for _, version := range []XSDVersion{XSDVersion10, XSDVersion11} {
		t.Run(string(version), func(t *testing.T) {
			baseLoc := mustEnumerationLoc(t, "version-base.xsd", 4, 3)
			candidateLoc := mustEnumerationLoc(t, "version.xml", 9, 2)
			base := mustIntegerEnumerationFacet(t, "1", baseLoc, version)
			facets, err := NewIntegerEnumerationFacets([]IntegerEnumerationFacet{base}, version)
			if err != nil {
				t.Fatalf("NewIntegerEnumerationFacets: %v", err)
			}
			candidate := StrictInteger{}
			candidateErr := facets.ValidateInteger(candidate, candidateLoc)
			if candidateErr == nil {
				t.Fatal("value violation was accepted")
			}
			assertEnumerationDiagnostic(t, candidateErr, EnumerationValueViolationCode, candidateLoc, versionedEnumerationSpecRef(version, "cvc-enumeration-valid"))

			outsideLoc := mustEnumerationLoc(t, "version-child.xsd", 8, 3)
			outside := mustIntegerEnumerationFacet(t, "2", outsideLoc, version)
			_, err = RestrictIntegerEnumerationFacets(facets, NewIntegerEnumerationFacetDeclarations([]IntegerEnumerationFacet{outside}))
			assertEnumerationDiagnostic(t, err, InvalidEnumerationRestrictionCode, outsideLoc, versionedEnumerationSpecRef(version, "enumeration-valid-restriction"))
		})
	}
}

func mustIntegerEnumerationFacet(t *testing.T, lexical string, loc Loc, version XSDVersion) IntegerEnumerationFacet {
	t.Helper()
	facet, err := ParseIntegerEnumerationFacetFor(version, lexical, loc)
	if err != nil {
		t.Fatalf("ParseIntegerEnumerationFacetFor(%q): %v", lexical, err)
	}
	return facet
}

func mustDecimalEnumerationFacet(t *testing.T, lexical string, loc Loc, version XSDVersion) DecimalEnumerationFacet {
	t.Helper()
	facet, err := ParseDecimalEnumerationFacetFor(version, lexical, loc)
	if err != nil {
		t.Fatalf("ParseDecimalEnumerationFacetFor(%q): %v", lexical, err)
	}
	return facet
}

func mustEnumerationLoc(t *testing.T, source SourceID, line, column int) Loc {
	t.Helper()
	loc, err := NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func integerEnumerationCanonicals(values []StrictInteger) []string {
	canonicals := make([]string, len(values))
	for index := range values {
		canonicals[index] = values[index].Canonical()
	}
	return canonicals
}

func decimalEnumerationCanonicals(values []StrictDecimal) []string {
	canonicals := make([]string, len(values))
	for index := range values {
		canonicals[index] = values[index].Canonical()
	}
	return canonicals
}

func assertEnumerationDiagnostic(t *testing.T, err error, code string, loc Loc, specRef string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected enumeration diagnostic")
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

func versionedEnumerationSpecRef(version XSDVersion, suffix string) string {
	return "xsd" + string(version[0]) + string(version[2]) + "-datatypes#" + suffix
}
