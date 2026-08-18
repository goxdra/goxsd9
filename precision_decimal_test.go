package goxsd9

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func TestPrecisionDecimalLexicalMapping(t *testing.T) {
	loc, err := NewLoc("precision-decimal.xsd", 12, 6)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	tests := []struct {
		name        string
		lexical     string
		coefficient string
		scale       string
		sign        precisionDecimalSign
	}{
		{name: "integer", lexical: "3", coefficient: "3", scale: "0", sign: precisionDecimalSignPositive},
		{name: "trailing zeroes", lexical: "3.00", coefficient: "300", scale: "2", sign: precisionDecimalSignPositive},
		{name: "leading zeroes", lexical: "03.00", coefficient: "300", scale: "2", sign: precisionDecimalSignPositive},
		{name: "trailing decimal point", lexical: "1.", coefficient: "1", scale: "0", sign: precisionDecimalSignPositive},
		{name: "leading decimal point", lexical: ".1", coefficient: "1", scale: "1", sign: precisionDecimalSignPositive},
		{name: "positive sign", lexical: "+.5", coefficient: "5", scale: "1", sign: precisionDecimalSignPositive},
		{name: "scientific", lexical: "1.e2", coefficient: "1", scale: "-2", sign: precisionDecimalSignPositive},
		{name: "signed exponent", lexical: "3.0E-2", coefficient: "30", scale: "3", sign: precisionDecimalSignPositive},
		{name: "scientific trailing zero", lexical: "3.0e2", coefficient: "30", scale: "-1", sign: precisionDecimalSignPositive},
		{name: "collapsed whitespace", lexical: " \t\n 3.00e2 \r", coefficient: "300", scale: "0", sign: precisionDecimalSignPositive},
		{name: "negative", lexical: "-12.30", coefficient: "1230", scale: "2", sign: precisionDecimalSignNegative},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertPrecisionDecimalFiniteCase(t, test.lexical, test.coefficient, test.scale, test.sign, loc)
		})
	}
}

func assertPrecisionDecimalFiniteCase(t *testing.T, lexical, coefficient, scale string, sign precisionDecimalSign, loc Loc) {
	t.Helper()
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		t.Fatalf("parsePrecisionDecimal: %v", err)
	}
	finite, ok := value.(precisionDecimalFinite)
	if !ok {
		t.Fatalf("value type = %T, want precisionDecimalFinite", value)
	}
	if got := finite.coefficient.String(); got != coefficient {
		t.Fatalf("coefficient = %q, want %q", got, coefficient)
	}
	if got := finite.scale.String(); got != scale {
		t.Fatalf("scale = %q, want %q", got, scale)
	}
	if finite.sign != sign {
		t.Fatalf("sign = %d, want %d", finite.sign, sign)
	}
}

func TestPrecisionDecimalEquivalentNumericFactsRetainScale(t *testing.T) {
	lexicals := []string{"3.00e2", "300", "30e1", ".30e3"}
	wantNumeric := new(big.Rat).SetInt64(300)
	wantScales := []string{"0", "0", "-1", "-1"}
	for index, lexical := range lexicals {
		value, err := parsePrecisionDecimal(lexical, Loc{})
		if err != nil {
			t.Fatalf("parsePrecisionDecimal(%q): %v", lexical, err)
		}
		finite, ok := value.(precisionDecimalFinite)
		if !ok {
			t.Fatalf("%q value type = %T, want precisionDecimalFinite", lexical, value)
		}
		if got := precisionDecimalNumericValue(finite); got.Cmp(wantNumeric) != 0 {
			t.Fatalf("%q numeric value = %s, want %s", lexical, got, wantNumeric)
		}
		if got := finite.scale.String(); got != wantScales[index] {
			t.Fatalf("%q scale = %q, want %q", lexical, got, wantScales[index])
		}
	}
}

func TestPrecisionDecimalSignedZeroRetainsSignAndScale(t *testing.T) {
	tests := []struct {
		name    string
		lexical string
		scale   string
		sign    precisionDecimalSign
	}{
		{name: "positive zero", lexical: "+0.000e2", scale: "1", sign: precisionDecimalSignPositive},
		{name: "negative zero", lexical: "-0.000e2", scale: "1", sign: precisionDecimalSignNegative},
		{name: "huge negative zero scale", lexical: "-0e100000000000000000000000", scale: "-100000000000000000000000", sign: precisionDecimalSignNegative},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertPrecisionDecimalSignedZero(t, test.lexical, test.scale, test.sign)
		})
	}
}

func assertPrecisionDecimalSignedZero(t *testing.T, lexical, scale string, sign precisionDecimalSign) {
	t.Helper()
	value, err := parsePrecisionDecimal(lexical, Loc{})
	if err != nil {
		t.Fatalf("parsePrecisionDecimal: %v", err)
	}
	finite, ok := value.(precisionDecimalFinite)
	if !ok {
		t.Fatalf("value type = %T, want precisionDecimalFinite", value)
	}
	if finite.coefficient.Sign() != 0 {
		t.Fatalf("coefficient = %s, want zero", finite.coefficient)
	}
	if got := finite.scale.String(); got != scale {
		t.Fatalf("scale = %q, want %q", got, scale)
	}
	if finite.sign != sign {
		t.Fatalf("sign = %d, want %d", finite.sign, sign)
	}
}

func TestPrecisionDecimalSpecialValues(t *testing.T) {
	tests := []struct {
		name    string
		lexical string
		check   func(t *testing.T, value precisionDecimalValue)
	}{
		{name: "INF", lexical: "INF", check: assertPrecisionDecimalPositiveInfinity},
		{name: "positive INF", lexical: "+INF", check: assertPrecisionDecimalPositiveInfinity},
		{name: "negative INF", lexical: "-INF", check: assertPrecisionDecimalNegativeInfinity},
		{name: "NaN", lexical: "NaN", check: assertPrecisionDecimalNaN},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := parsePrecisionDecimal(test.lexical, Loc{})
			if err != nil {
				t.Fatalf("parsePrecisionDecimal: %v", err)
			}
			test.check(t, value)
		})
	}
}

func assertPrecisionDecimalPositiveInfinity(t *testing.T, value precisionDecimalValue) {
	t.Helper()
	if _, ok := value.(precisionDecimalPositiveInfinity); !ok {
		t.Fatalf("value type = %T, want precisionDecimalPositiveInfinity", value)
	}
}

func assertPrecisionDecimalNegativeInfinity(t *testing.T, value precisionDecimalValue) {
	t.Helper()
	if _, ok := value.(precisionDecimalNegativeInfinity); !ok {
		t.Fatalf("value type = %T, want precisionDecimalNegativeInfinity", value)
	}
}

func assertPrecisionDecimalNaN(t *testing.T, value precisionDecimalValue) {
	t.Helper()
	if _, ok := value.(precisionDecimalNaN); !ok {
		t.Fatalf("value type = %T, want precisionDecimalNaN", value)
	}
}

func TestPrecisionDecimalHugeCoefficientAndExponentAreExact(t *testing.T) {
	coefficientDigits := strings.Repeat("9", 2048)
	exponentDigits := "1234567890123456789012345678901234567890"
	value, err := parsePrecisionDecimal(coefficientDigits+".00e-"+exponentDigits, Loc{})
	if err != nil {
		t.Fatalf("parsePrecisionDecimal: %v", err)
	}
	finite, ok := value.(precisionDecimalFinite)
	if !ok {
		t.Fatalf("value type = %T, want precisionDecimalFinite", value)
	}
	if got, want := finite.coefficient.String(), coefficientDigits+"00"; got != want {
		t.Fatalf("coefficient length/value changed: got %q, want %q", got, want)
	}
	wantScale := new(big.Int)
	wantScale.SetString(exponentDigits, 10)
	wantScale.Add(wantScale, big.NewInt(2))
	if got := finite.scale.String(); got != wantScale.String() {
		t.Fatalf("scale = %q, want %q", got, wantScale)
	}
}

func TestPrecisionDecimalRejectsInvalidLexicalForms(t *testing.T) {
	loc, err := NewLoc("precision-decimal.xsd", 21, 4)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	for _, lexical := range []string{
		"", "+", "-", ".", "+.", "-.", "1e", "1e+", "1e-", "1e.2", "1e2e3",
		"1..0", "1 2", "1\t2", "1E 2", "inf", "Infinity", "+NaN", "-NaN", "１２",
	} {
		t.Run(lexical, func(t *testing.T) {
			value, parseErr := parsePrecisionDecimal(lexical, loc)
			if value != nil {
				t.Fatalf("invalid lexical form returned value %T", value)
			}
			assertPrecisionDecimalLexicalDiagnostic(t, parseErr, loc)
		})
	}
}

func assertPrecisionDecimalLexicalDiagnostic(t *testing.T, err error, loc Loc) {
	t.Helper()
	if err == nil {
		t.Fatal("invalid lexical form returned no error")
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), diagnosticPrecisionDecimalLexicalCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Loc(), loc; got != want {
		t.Fatalf("Loc() = %#v, want %#v", got, want)
	}
	if got, want := diagnostic.SpecRef(), precisionDecimalLexicalSpecRef; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
}

func TestPrecisionDecimalCopiesNumericInternals(t *testing.T) {
	value, err := parsePrecisionDecimal("123.4500e2", Loc{})
	if err != nil {
		t.Fatalf("parsePrecisionDecimal: %v", err)
	}
	finite, ok := value.(precisionDecimalFinite)
	if !ok {
		t.Fatalf("value type = %T, want precisionDecimalFinite", value)
	}
	coefficient := finite.coefficientCopy()
	scale := finite.scaleCopy()
	coefficient.SetInt64(0)
	scale.SetInt64(0)
	if got, want := finite.coefficient.String(), "1234500"; got != want {
		t.Fatalf("coefficient changed through copy = %q, want %q", got, want)
	}
	if got, want := finite.scale.String(), "2"; got != want {
		t.Fatalf("scale changed through copy = %q, want %q", got, want)
	}
}

func precisionDecimalNumericValue(value precisionDecimalFinite) *big.Rat {
	coefficient := value.coefficientCopy()
	scale := value.scaleCopy()
	if scale.Sign() < 0 {
		power := new(big.Int).Neg(scale)
		coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(10), power, nil))
		return new(big.Rat).SetInt(coefficient)
	}
	denominator := new(big.Int).Exp(big.NewInt(10), scale, nil)
	return new(big.Rat).SetFrac(coefficient, denominator)
}

func FuzzPrecisionDecimalLexicalMapping(f *testing.F) {
	for _, lexical := range []string{
		"0", "+0012.30", "-0012.30", ".5", "1.", "1.e2", "3.0e-2",
		"-0.000", "INF", "+INF", "-INF", "NaN", "1e", "1 2", "１２",
		strings.Repeat("8", 128) + "." + strings.Repeat("1", 128),
	} {
		f.Add(lexical)
	}
	f.Fuzz(func(t *testing.T, lexical string) {
		first, firstErr := parsePrecisionDecimal(lexical, Loc{})
		second, secondErr := parsePrecisionDecimal(lexical, Loc{})
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("repeated parse changed error result: first=%v second=%v", firstErr, secondErr)
		}
		if firstErr != nil {
			if first != nil || second != nil {
				t.Fatalf("invalid input returned partial value: first=%T second=%T", first, second)
			}
			return
		}
		if first == nil || second == nil {
			t.Fatalf("valid input returned nil value: first=%T second=%T", first, second)
		}
		if got, want := precisionDecimalValueSnapshot(first), precisionDecimalValueSnapshot(second); got != want {
			t.Fatalf("repeated parse changed value: first=%q second=%q", got, want)
		}
	})
}

func precisionDecimalValueSnapshot(value precisionDecimalValue) string {
	switch typed := value.(type) {
	case precisionDecimalFinite:
		return strings.Join([]string{
			"finite",
			typed.coefficient.String(),
			typed.scale.String(),
			strconv.Itoa(int(typed.sign)),
		}, ":")
	case precisionDecimalPositiveInfinity:
		return "positive-infinity"
	case precisionDecimalNegativeInfinity:
		return "negative-infinity"
	case precisionDecimalNaN:
		return "nan"
	default:
		return "unknown"
	}
}
