package goxsd9

import (
	"errors"
	"strings"
	"testing"
)

func TestStrictIntegerLexicalMappingAndCanonicalRepresentation(t *testing.T) {
	loc, err := NewLoc("integer.xsd", 4, 9)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	tests := []struct {
		name      string
		lexical   string
		canonical string
	}{
		{name: "zero", lexical: "0", canonical: "0"},
		{name: "negative zero", lexical: "-000", canonical: "0"},
		{name: "positive sign", lexical: "+00042", canonical: "42"},
		{name: "negative", lexical: "-00042", canonical: "-42"},
		{name: "collapsed whitespace", lexical: " \t+00042\r\n", canonical: "42"},
		{name: "large", lexical: strings.Repeat("9", 2048), canonical: strings.Repeat("9", 2048)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkStrictIntegerCase(t, test.lexical, test.canonical, loc)
		})
	}
}

func checkStrictIntegerCase(t *testing.T, lexical, canonical string, loc Loc) {
	t.Helper()
	value, err := ParseStrictInteger(lexical, loc)
	if err != nil {
		t.Fatalf("ParseStrictInteger: %v", err)
	}
	if got := value.Canonical(); got != canonical {
		t.Fatalf("Canonical() = %q, want %q", got, canonical)
	}
	if got := value.String(); got != canonical {
		t.Fatalf("String() = %q, want %q", got, canonical)
	}
	roundTrip, err := ParseStrictInteger(value.Canonical(), Loc{})
	if err != nil {
		t.Fatalf("ParseStrictInteger(canonical): %v", err)
	}
	if !value.Equal(roundTrip) {
		t.Fatal("canonical integer does not round-trip")
	}
}

func TestStrictIntegerRetainsAnImmutableExactValue(t *testing.T) {
	value, err := ParseStrictInteger("123456789012345678901234567890", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictInteger: %v", err)
	}
	if got, want := value.Canonical(), "123456789012345678901234567890"; got != want {
		t.Fatalf("Canonical() = %q, want %q", got, want)
	}
	if got, want := value.Sign(), 1; got != want {
		t.Fatalf("Sign() = %d, want %d", got, want)
	}
}

func TestStrictIntegerRejectsInvalidLexicalFormsWithLocation(t *testing.T) {
	loc, err := NewLoc("integer.xsd", 8, 3)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	for _, lexical := range []string{"", "+", "-", "1.0", "1 2", "１２"} {
		t.Run(lexical, func(t *testing.T) {
			checkInvalidInteger(t, lexical, loc)
		})
	}
}

func checkInvalidInteger(t *testing.T, lexical string, loc Loc) {
	t.Helper()
	_, err := ParseStrictInteger(lexical, loc)
	assertInvalidLexicalDiagnostic(t, err, InvalidIntegerLexicalCode, loc)
}

func TestStrictDecimalLexicalMappingAndCanonicalRepresentation(t *testing.T) {
	tests := []struct {
		name      string
		lexical   string
		canonical string
		scale     int
		digits    int
	}{
		{name: "integer numeral", lexical: "210", canonical: "210.0", scale: 0, digits: 3},
		{name: "leading decimal point", lexical: ".5", canonical: "0.5", scale: 1, digits: 1},
		{name: "trailing decimal point", lexical: "1.", canonical: "1.0", scale: 0, digits: 1},
		{name: "sign and zeroes", lexical: "+0001.2300", canonical: "1.23", scale: 2, digits: 3},
		{name: "small value", lexical: "0.00100", canonical: "0.001", scale: 3, digits: 3},
		{name: "negative", lexical: "-1.2300", canonical: "-1.23", scale: 2, digits: 3},
		{name: "zero", lexical: "-000.000", canonical: "0.0", scale: 0, digits: 1},
		{name: "collapsed whitespace", lexical: " \n-0001.2300\t", canonical: "-1.23", scale: 2, digits: 3},
		{name: "large", lexical: "123456789012345678901234567890.00000000000000000001", canonical: "123456789012345678901234567890.00000000000000000001", scale: 20, digits: 50},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkStrictDecimalCase(t, test.lexical, test.canonical, test.scale, test.digits)
		})
	}
}

func checkStrictDecimalCase(t *testing.T, lexical, canonical string, scale, digits int) {
	t.Helper()
	value, err := ParseStrictDecimal(lexical, Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal: %v", err)
	}
	if got := value.Canonical(); got != canonical {
		t.Fatalf("Canonical() = %q, want %q", got, canonical)
	}
	if got := value.Scale(); got != scale {
		t.Fatalf("Scale() = %d, want %d", got, scale)
	}
	if got := value.FractionDigits(); got != scale {
		t.Fatalf("FractionDigits() = %d, want %d", got, scale)
	}
	if got := value.TotalDigits(); got != digits {
		t.Fatalf("TotalDigits() = %d, want %d", got, digits)
	}
	roundTrip, err := ParseStrictDecimal(value.Canonical(), Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal(canonical): %v", err)
	}
	if !value.Equal(roundTrip) {
		t.Fatal("canonical decimal does not round-trip")
	}
}

func TestStrictDecimalEqualityOrderingAndImmutability(t *testing.T) {
	first, err := ParseStrictDecimal("1.2300", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal(first): %v", err)
	}
	second, err := ParseStrictDecimal("1.23", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal(second): %v", err)
	}
	if !first.Equal(second) || first.Compare(second) != 0 {
		t.Fatal("equivalent decimal values are not equal")
	}
	less, err := ParseStrictDecimal("0.00000000000000000000000000000000000001", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal(less): %v", err)
	}
	if less.Compare(first) >= 0 || first.Compare(less) <= 0 {
		t.Fatal("decimal ordering is incorrect")
	}
	if got, want := first.Canonical(), "1.23"; got != want {
		t.Fatalf("Canonical() changed after comparison = %q, want %q", got, want)
	}
}

func TestStrictDecimalCanonicalVersionPolicy(t *testing.T) {
	integer, err := ParseStrictDecimal("0012.00", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal(integer): %v", err)
	}
	canonical10, versionErr := integer.CanonicalFor(XSDVersion10)
	if versionErr != nil {
		t.Fatalf("CanonicalFor(XSD 1.0): %v", versionErr)
	}
	if got, want := canonical10, "12.0"; got != want {
		t.Fatalf("XSD 1.0 CanonicalFor() = %q, want %q", got, want)
	}
	canonical11, versionErr := integer.CanonicalFor(XSDVersion11)
	if versionErr != nil {
		t.Fatalf("CanonicalFor(XSD 1.1): %v", versionErr)
	}
	if got, want := canonical11, "12"; got != want {
		t.Fatalf("XSD 1.1 CanonicalFor() = %q, want %q", got, want)
	}
	if got, want := integer.String(), "12.0"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if _, invalidVersionErr := integer.CanonicalFor("2.0"); invalidVersionErr == nil {
		t.Fatal("CanonicalFor accepted an unsupported XSD version")
	}

	fraction, err := ParseStrictDecimal("12.50", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimal(fraction): %v", err)
	}
	canonical11, versionErr = fraction.CanonicalFor(XSDVersion11)
	if versionErr != nil {
		t.Fatalf("CanonicalFor(XSD 1.1 fraction): %v", versionErr)
	}
	if got, want := canonical11, "12.5"; got != want {
		t.Fatalf("XSD 1.1 fractional CanonicalFor() = %q, want %q", got, want)
	}
}

func TestStrictDecimalLexicalVersionPolicy(t *testing.T) {
	loc := Loc{}
	for _, lexical := range []string{".5", "1."} {
		if _, err := ParseStrictDecimalFor(XSDVersion10, lexical, loc); err == nil {
			t.Fatalf("XSD 1.0 accepted %q", lexical)
		}
		if _, err := ParseStrictDecimalFor(XSDVersion11, lexical, loc); err != nil {
			t.Fatalf("XSD 1.1 rejected %q: %v", lexical, err)
		}
	}
	if _, err := ParseStrictDecimal("1.0", loc, XSDVersion10, XSDVersion11); err == nil {
		t.Fatal("parser accepted multiple XSD versions")
	}
	_, err := ParseStrictDecimal("1.0", loc, XSDVersion("2.0"))
	assertInvalidLexicalDiagnostic(t, err, InvalidXSDVersionCode, loc)
}

func TestStrictDecimalRejectsInvalidLexicalFormsWithLocation(t *testing.T) {
	loc, err := NewLoc("decimal.xsd", 9, 4)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	for _, lexical := range []string{"", "+", "-", ".", "+.", "1..0", "1e2", "1 2", "１２"} {
		t.Run(lexical, func(t *testing.T) {
			checkInvalidDecimal(t, lexical, loc)
		})
	}
}

func checkInvalidDecimal(t *testing.T, lexical string, loc Loc) {
	t.Helper()
	_, err := ParseStrictDecimal(lexical, loc)
	assertInvalidLexicalDiagnostic(t, err, InvalidDecimalLexicalCode, loc)
}

func assertInvalidLexicalDiagnostic(t *testing.T, err error, code string, loc Loc) {
	t.Helper()
	if err == nil {
		t.Fatal("parser accepted invalid lexical form")
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), code; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got := diagnostic.Loc(); got != loc {
		t.Fatalf("Loc() = %#v, want %#v", got, loc)
	}
}

func FuzzStrictIntegerCanonicalRoundTrip(f *testing.F) {
	for _, lexical := range []string{"0", "+0012", "-0012", strings.Repeat("8", 128), "1e2", "-"} {
		f.Add(lexical)
	}
	f.Fuzz(func(t *testing.T, lexical string) {
		value, err := ParseStrictInteger(lexical, Loc{})
		if err != nil {
			return
		}
		canonical := value.Canonical()
		roundTrip, err := ParseStrictInteger(canonical, Loc{})
		if err != nil {
			t.Fatalf("ParseStrictInteger(canonical): %v", err)
		}
		if !value.Equal(roundTrip) {
			t.Fatal("integer canonical round trip changed the value")
		}
	})
}

func FuzzStrictDecimalCanonicalRoundTrip(f *testing.F) {
	for _, lexical := range []string{"0", "+0012.30", "-0012.30", ".5", "1.", "-0.000000", strings.Repeat("8", 128) + "." + strings.Repeat("1", 128), "1e2", "-", "１２"} {
		f.Add(lexical)
	}
	f.Fuzz(func(t *testing.T, lexical string) {
		value, err := ParseStrictDecimal(lexical, Loc{})
		if err != nil {
			return
		}
		canonical := value.Canonical()
		roundTrip, err := ParseStrictDecimal(canonical, Loc{})
		if err != nil {
			t.Fatalf("ParseStrictDecimal(canonical): %v", err)
		}
		if !value.Equal(roundTrip) {
			t.Fatal("decimal canonical round trip changed the value")
		}
	})
}
