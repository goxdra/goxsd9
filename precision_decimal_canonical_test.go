package goxsd9

import (
	"errors"
	"math/big"
	"strings"
	"testing"
)

func TestPrecisionDecimalCanonicalForms(t *testing.T) {
	tests := []struct {
		name      string
		lexical   string
		canonical string
	}{
		{name: "positive infinity", lexical: "+INF", canonical: "INF"},
		{name: "negative infinity", lexical: "-INF", canonical: "-INF"},
		{name: "NaN", lexical: "NaN", canonical: "NaN"},
		{name: "positive zero", lexical: "0.000e2", canonical: "0.0E0"},
		{name: "negative zero", lexical: "-0.000e2", canonical: "-0.0E0"},
		{name: "positive zero huge scale", lexical: "0e-999999999999999999999999", canonical: "0.0E0"},
		{name: "negative zero huge scale", lexical: "-0e999999999999999999999999", canonical: "-0.0E0"},
		{name: "ordinary decimal", lexical: "3.00", canonical: "3.00"},
		{name: "ordinary integer", lexical: "3.00e2", canonical: "300"},
		{name: "retained scientific zero", lexical: "3.0e2", canonical: "3.0E2"},
		{name: "lower magnitude boundary", lexical: "1e-6", canonical: "0.000001"},
		{name: "below lower magnitude boundary", lexical: "1e-7", canonical: "1E-7"},
		{name: "upper magnitude boundary", lexical: "1000000", canonical: "1000000"},
		{name: "above upper magnitude boundary", lexical: "1000001", canonical: "1.000001E6"},
		{name: "negative upper boundary", lexical: "-1000000", canonical: "-1000000"},
		{name: "negative below lower boundary", lexical: "-1e-7", canonical: "-1E-7"},
		{name: "retained decimal zeroes", lexical: "12.3400e2", canonical: "1234.00"},
		{name: "retained fractional zeroes", lexical: "0.0012300", canonical: "0.0012300"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := mustParsePrecisionDecimalCanonicalValue(t, test.lexical)
			got, err := canonicalPrecisionDecimal(value, uint64(len(test.canonical)), Loc{})
			if err != nil {
				t.Fatalf("canonicalPrecisionDecimal(%q): %v", test.lexical, err)
			}
			if got != test.canonical {
				t.Fatalf("canonicalPrecisionDecimal(%q) = %q, want %q", test.lexical, got, test.canonical)
			}
		})
	}
}

func TestPrecisionDecimalCanonicalEquivalentValuesKeepRepresentationPolicy(t *testing.T) {
	tests := []struct {
		lexical   string
		canonical string
	}{
		{lexical: "3.00e2", canonical: "300"},
		{lexical: "300", canonical: "300"},
		{lexical: "30e1", canonical: "3.0E2"},
		{lexical: ".30e3", canonical: "3.0E2"},
	}
	for _, test := range tests {
		t.Run(test.lexical, func(t *testing.T) {
			value := mustParsePrecisionDecimalCanonicalValue(t, test.lexical)
			got, err := canonicalPrecisionDecimal(value, uint64(len(test.canonical)), Loc{})
			if err != nil {
				t.Fatalf("canonicalPrecisionDecimal(%q): %v", test.lexical, err)
			}
			if got != test.canonical {
				t.Fatalf("canonicalPrecisionDecimal(%q) = %q, want %q", test.lexical, got, test.canonical)
			}
		})
	}
}

func TestPrecisionDecimalCanonicalExactAndOverBudget(t *testing.T) {
	tests := []struct {
		name      string
		lexical   string
		canonical string
	}{
		{name: "special", lexical: "INF", canonical: "INF"},
		{name: "signed zero", lexical: "-0", canonical: "-0.0E0"},
		{name: "decimal", lexical: "3.00", canonical: "3.00"},
		{name: "scientific", lexical: "1e-7", canonical: "1E-7"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := mustParsePrecisionDecimalCanonicalValue(t, test.lexical)
			got, err := canonicalPrecisionDecimal(value, uint64(len(test.canonical)), Loc{})
			if err != nil {
				t.Fatalf("exact budget: %v", err)
			}
			if got != test.canonical {
				t.Fatalf("exact budget output = %q, want %q", got, test.canonical)
			}

			overBudget := uint64(len(test.canonical))
			overBudget--
			got, err = canonicalPrecisionDecimal(value, overBudget, Loc{})
			assertPrecisionDecimalCanonicalLimit(t, got, err)
		})
	}
}

func TestPrecisionDecimalCanonicalZeroBudgetRejectsEveryValue(t *testing.T) {
	for _, lexical := range []string{"0", "-0", "1", "INF", "-INF", "NaN"} {
		t.Run(lexical, func(t *testing.T) {
			value := mustParsePrecisionDecimalCanonicalValue(t, lexical)
			got, err := canonicalPrecisionDecimal(value, 0, Loc{})
			assertPrecisionDecimalCanonicalLimit(t, got, err)
		})
	}
}

func TestPrecisionDecimalCanonicalHugeScalesRejectBeforeExpansion(t *testing.T) {
	hugeScaleDigits := strings.Repeat("9", 4096)
	positiveScale, ok := new(big.Int).SetString(hugeScaleDigits, 10)
	if !ok {
		t.Fatal("SetString positive scale failed")
	}
	negativeScale := new(big.Int).Neg(positiveScale)
	tests := []struct {
		name  string
		scale *big.Int
	}{
		{name: "huge positive scale", scale: positiveScale},
		{name: "huge negative scale", scale: negativeScale},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := precisionDecimalFinite{
				coefficient: big.NewInt(1),
				scale:       new(big.Int).Set(test.scale),
				sign:        precisionDecimalSignPositive,
			}
			coefficientBefore := new(big.Int).Set(value.coefficient)
			scaleBefore := new(big.Int).Set(value.scale)
			got, err := canonicalPrecisionDecimal(value, 8, Loc{})
			assertPrecisionDecimalCanonicalLimit(t, got, err)
			if value.coefficient.Cmp(coefficientBefore) != 0 {
				t.Fatalf("coefficient changed after rejection")
			}
			if value.scale.Cmp(scaleBefore) != 0 {
				t.Fatalf("scale changed after rejection")
			}
		})
	}
}

func TestPrecisionDecimalCanonicalHugeScalesHaveExactBoundaries(t *testing.T) {
	scaleDigits := "1234567890123456789012345678901234567890"
	positiveScale, ok := new(big.Int).SetString(scaleDigits, 10)
	if !ok {
		t.Fatal("SetString positive scale failed")
	}
	negativeScale := new(big.Int).Neg(positiveScale)
	tests := []struct {
		name      string
		scale     *big.Int
		canonical string
	}{
		{name: "positive scale", scale: positiveScale, canonical: "1E-" + scaleDigits},
		{name: "negative scale", scale: negativeScale, canonical: "1E" + scaleDigits},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := precisionDecimalFinite{
				coefficient: big.NewInt(1),
				scale:       new(big.Int).Set(test.scale),
				sign:        precisionDecimalSignPositive,
			}
			budget := uint64(len(test.canonical))
			got, err := canonicalPrecisionDecimal(value, budget, Loc{})
			if err != nil {
				t.Fatalf("exact huge-scale budget: %v", err)
			}
			if got != test.canonical {
				t.Fatalf("canonical output = %q, want %q", got, test.canonical)
			}
			got, err = canonicalPrecisionDecimal(value, budget-1, Loc{})
			assertPrecisionDecimalCanonicalLimit(t, got, err)
		})
	}
}

func TestPrecisionDecimalCanonicalRepeatedCallsAreDeterministicAndUncached(t *testing.T) {
	value := mustParsePrecisionDecimalCanonicalValue(t, "3.00")
	finite, ok := value.(precisionDecimalFinite)
	if !ok {
		t.Fatalf("value type = %T, want precisionDecimalFinite", value)
	}

	first, err := canonicalPrecisionDecimal(value, 4, Loc{})
	if err != nil {
		t.Fatalf("first canonical call: %v", err)
	}
	second, err := canonicalPrecisionDecimal(value, 4, Loc{})
	if err != nil {
		t.Fatalf("second canonical call: %v", err)
	}
	if first != second || first != "3.00" {
		t.Fatalf("repeated output = %q and %q", first, second)
	}

	finite.coefficient.SetInt64(4)
	updated, err := canonicalPrecisionDecimal(value, 4, Loc{})
	if err != nil {
		t.Fatalf("updated canonical call: %v", err)
	}
	if updated != "0.04" {
		t.Fatalf("updated output = %q, want %q", updated, "0.04")
	}
}

func TestPrecisionDecimalCanonicalLimitDiagnosticIsLocatedAndCausePreserving(t *testing.T) {
	loc, err := NewLoc("canonical.xsd", 8, 13)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	value := mustParsePrecisionDecimalCanonicalValue(t, "123")
	got, err := canonicalPrecisionDecimal(value, 2, loc)
	assertPrecisionDecimalCanonicalLimit(t, got, err)

	if !errors.Is(err, ErrPrecisionDecimalCanonicalOutputLimit) {
		t.Fatalf("errors.Is(limit) = false: %v", err)
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("limit error was classified as unsupported: %v", err)
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), diagnosticPrecisionDecimalCanonicalLimitCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Loc(), loc; got != want {
		t.Fatalf("Loc() = %#v, want %#v", got, want)
	}
	if got, want := diagnostic.SpecRef(), precisionDecimalCanonicalLimitSpecRef; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
	if !errors.Is(diagnostic.Unwrap(), ErrPrecisionDecimalCanonicalOutputLimit) {
		t.Fatalf("Unwrap() = %v, want %v", diagnostic.Unwrap(), ErrPrecisionDecimalCanonicalOutputLimit)
	}
}

func mustParsePrecisionDecimalCanonicalValue(t *testing.T, lexical string) precisionDecimalValue {
	t.Helper()
	value, err := parsePrecisionDecimal(lexical, Loc{})
	if err != nil {
		t.Fatalf("parsePrecisionDecimal(%q): %v", lexical, err)
	}
	return value
}

func assertPrecisionDecimalCanonicalLimit(t *testing.T, got string, err error) {
	t.Helper()
	if got != "" {
		t.Fatalf("over-budget output = %q, want empty", got)
	}
	if err == nil {
		t.Fatal("over-budget call returned no error")
	}
	if !errors.Is(err, ErrPrecisionDecimalCanonicalOutputLimit) {
		t.Fatalf("errors.Is(limit) = false: %v", err)
	}
}
