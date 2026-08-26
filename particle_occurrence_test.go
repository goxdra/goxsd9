package goxsd9

import (
	"errors"
	"testing"
)

type particleOccurrenceTableError uint8

const (
	particleOccurrenceNoError particleOccurrenceTableError = iota
	particleOccurrenceInvalidLexical
	particleOccurrenceNegative
	particleOccurrenceInvalidRange
)

//nolint:funlen // Keep the normative occurrence rows together as one proof table.
func TestParticleOccurrenceNormativeRows(t *testing.T) {
	tests := []struct {
		name       string
		version    XSDVersion
		minPresent bool
		min        string
		maxPresent bool
		max        string
		wantMin    string
		wantMax    string
		wantPublic bool
		wantError  particleOccurrenceTableError
	}{
		{
			name:       "xsd10 omitted bounds default to one",
			version:    XSDVersion10,
			wantMin:    "1",
			wantMax:    "1",
			wantPublic: true,
		},
		{
			name:       "xsd11 omitted bounds default to one",
			version:    XSDVersion11,
			wantMin:    "1",
			wantMax:    "1",
			wantPublic: true,
		},
		{
			name:       "xsd10 zero minimum is exact",
			version:    XSDVersion10,
			minPresent: true,
			min:        "0",
			wantMin:    "0",
			wantMax:    "1",
			wantPublic: true,
		},
		{
			name:       "xsd11 zero minimum is exact",
			version:    XSDVersion11,
			minPresent: true,
			min:        "0",
			wantMin:    "0",
			wantMax:    "1",
			wantPublic: true,
		},
		{
			name:       "xsd10 zero-zero has no mapped particle",
			version:    XSDVersion10,
			minPresent: true,
			min:        "0",
			maxPresent: true,
			max:        "0",
			wantMin:    "0",
			wantMax:    "0",
		},
		{
			name:       "xsd11 zero-zero has no mapped particle",
			version:    XSDVersion11,
			minPresent: true,
			min:        "0",
			maxPresent: true,
			max:        "0",
			wantMin:    "0",
			wantMax:    "0",
		},
		{
			name:       "xsd10 negative zero canonicalizes to zero",
			version:    XSDVersion10,
			minPresent: true,
			min:        "-0",
			maxPresent: true,
			max:        "0",
			wantMin:    "0",
			wantMax:    "0",
		},
		{
			name:       "xsd11 negative zero canonicalizes to zero",
			version:    XSDVersion11,
			minPresent: true,
			min:        "-0",
			maxPresent: true,
			max:        "0",
			wantMin:    "0",
			wantMax:    "0",
		},
		{
			name:       "xsd10 arbitrary finite values exceed uint64",
			version:    XSDVersion10,
			minPresent: true,
			min:        "18446744073709551615",
			maxPresent: true,
			max:        "18446744073709551616",
			wantMin:    "18446744073709551615",
			wantMax:    "18446744073709551616",
			wantPublic: true,
		},
		{
			name:       "xsd11 arbitrary finite values exceed uint64",
			version:    XSDVersion11,
			minPresent: true,
			min:        "18446744073709551615",
			maxPresent: true,
			max:        "18446744073709551616",
			wantMin:    "18446744073709551615",
			wantMax:    "18446744073709551616",
			wantPublic: true,
		},
		{
			name:       "xsd10 unbounded is a max-only variant",
			version:    XSDVersion10,
			minPresent: true,
			min:        "18446744073709551616",
			maxPresent: true,
			max:        "unbounded",
			wantMin:    "18446744073709551616",
			wantMax:    "unbounded",
			wantPublic: true,
		},
		{
			name:       "xsd11 unbounded is a max-only variant",
			version:    XSDVersion11,
			minPresent: true,
			min:        "18446744073709551616",
			maxPresent: true,
			max:        "unbounded",
			wantMin:    "18446744073709551616",
			wantMax:    "unbounded",
			wantPublic: true,
		},
		{
			name:       "xsd10 malformed minimum is invalid",
			version:    XSDVersion10,
			minPresent: true,
			min:        "maybe",
			wantError:  particleOccurrenceInvalidLexical,
		},
		{
			name:       "xsd11 malformed maximum is invalid",
			version:    XSDVersion11,
			maxPresent: true,
			max:        "1.0",
			wantError:  particleOccurrenceInvalidLexical,
		},
		{
			name:       "xsd10 negative minimum is invalid",
			version:    XSDVersion10,
			minPresent: true,
			min:        "-1",
			wantError:  particleOccurrenceNegative,
		},
		{
			name:       "xsd11 negative maximum is invalid",
			version:    XSDVersion11,
			maxPresent: true,
			max:        "-1",
			wantError:  particleOccurrenceNegative,
		},
		{
			name:       "xsd10 finite minimum above maximum is invalid",
			version:    XSDVersion10,
			minPresent: true,
			min:        "2",
			maxPresent: true,
			max:        "1",
			wantError:  particleOccurrenceInvalidRange,
		},
		{
			name:       "xsd11 finite minimum above maximum is invalid",
			version:    XSDVersion11,
			minPresent: true,
			min:        "2",
			maxPresent: true,
			max:        "1",
			wantError:  particleOccurrenceInvalidRange,
		},
		{
			name:       "xsd11 unbounded bypasses numeric comparison",
			version:    XSDVersion11,
			minPresent: true,
			min:        "999999999999999999999999999999",
			maxPresent: true,
			max:        "unbounded",
			wantMin:    "999999999999999999999999999999",
			wantMax:    "unbounded",
			wantPublic: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loc := mustTestLoc(t, "particle-occurrence.xsd", 4, 3)
			element := particleOccurrenceSyntaxElement(t, loc, test.minPresent, test.min, test.maxPresent, test.max)
			occurrences, err := schemaParticleOccurrenceRange(element)
			if test.wantError != particleOccurrenceNoError {
				assertParticleOccurrenceTableError(t, err, test.wantError, loc)
				return
			}
			if err != nil {
				t.Fatalf("schemaParticleOccurrenceRange: %v", err)
			}
			if got := occurrences.String(); got != test.wantMin+"/"+test.wantMax {
				t.Fatalf("range = %q, want %q", got, test.wantMin+"/"+test.wantMax)
			}
			if got := occurrences.mapsToParticle(); got != test.wantPublic {
				t.Fatalf("mapsToParticle() = %t, want %t", got, test.wantPublic)
			}
		})
	}
}

func assertParticleOccurrenceTableError(t *testing.T, err error, want particleOccurrenceTableError, loc Loc) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want table error %d", want)
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v, want located diagnostic", err)
	}
	if diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic code = %s, want %s", diagnostic.Code(), invalidSchemaCompositionCode)
	}
	if want == particleOccurrenceInvalidRange && diagnostic.Loc() != loc {
		t.Fatalf("range diagnostic location = %s, want %s", diagnostic.Loc(), loc)
	}
	switch want {
	case particleOccurrenceInvalidLexical:
		var cause Diagnostic
		if !errors.As(diagnostic.Unwrap(), &cause) {
			t.Fatalf("diagnostic cause = %v, want lexical diagnostic", diagnostic.Unwrap())
		}
		if cause.Code() != InvalidIntegerLexicalCode {
			t.Fatalf("lexical cause code = %s, want %s", cause.Code(), InvalidIntegerLexicalCode)
		}
	case particleOccurrenceNegative:
		if !errors.Is(err, errParticleOccurrenceNegative) {
			t.Fatalf("error = %v, want negative occurrence cause", err)
		}
	case particleOccurrenceInvalidRange:
		if diagnostic.Message() != "particle minOccurs cannot exceed maxOccurs" {
			t.Fatalf("range diagnostic message = %q", diagnostic.Message())
		}
	case particleOccurrenceNoError:
		t.Fatalf("unexpected no-error table classification")
	default:
		t.Fatalf("unknown table error %d", want)
	}
}

func particleOccurrenceSyntaxElement(t *testing.T, loc Loc, minPresent bool, minimumLexical string, maxPresent bool, maximumLexical string) *syntaxElement {
	t.Helper()
	element := &syntaxElement{loc: loc}
	if minPresent {
		element.attrs = append(element.attrs, syntaxAttribute{
			name:  syntaxName{local: "minOccurs"},
			value: minimumLexical,
			loc:   mustTestLoc(t, "particle-occurrence.xsd", 4, 12),
		})
	}
	if maxPresent {
		element.attrs = append(element.attrs, syntaxAttribute{
			name:  syntaxName{local: "maxOccurs"},
			value: maximumLexical,
			loc:   mustTestLoc(t, "particle-occurrence.xsd", 4, 24),
		})
	}
	return element
}

//nolint:funlen,gocognit // Keep the exact operations and ownership proof together.
func TestParticleOccurrenceExactOperationsAndOwnership(t *testing.T) {
	source, err := ParseStrictInteger("123456789012345678901234567890", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictInteger: %v", err)
	}
	owned, err := newFiniteParticleOccurrence(source)
	if err != nil {
		t.Fatalf("newFiniteParticleOccurrence: %v", err)
	}
	source.value.SetInt64(7)
	if got, want := owned.String(), "123456789012345678901234567890"; got != want {
		t.Fatalf("owned value changed after source mutation: got %q, want %q", got, want)
	}

	copyValue, ok := owned.finiteValue()
	if !ok {
		t.Fatal("finiteValue() reported an owned finite value as unbounded")
	}
	copyValue.value.SetInt64(8)
	if got, want := owned.String(), "123456789012345678901234567890"; got != want {
		t.Fatalf("finiteValue() exposed mutable storage: got %q, want %q", got, want)
	}

	zero, err := parseParticleOccurrence("0", false, Loc{})
	if err != nil {
		t.Fatalf("parse zero: %v", err)
	}
	one, err := parseParticleOccurrence("+0001", false, Loc{})
	if err != nil {
		t.Fatalf("parse one: %v", err)
	}
	unbounded, err := parseParticleOccurrence("unbounded", true, Loc{})
	if err != nil {
		t.Fatalf("parse unbounded: %v", err)
	}
	anotherUnbounded := newUnboundedParticleOccurrence()
	if zero.Compare(one) >= 0 || one.Compare(owned) >= 0 {
		t.Fatal("finite occurrence ordering is not exact")
	}
	if owned.Compare(unbounded) >= 0 || unbounded.Compare(owned) <= 0 {
		t.Fatal("unbounded occurrence ordering is not distinct")
	}
	if !unbounded.Equal(anotherUnbounded) || zero.Equal(unbounded) {
		t.Fatal("finite and unbounded equality is ambiguous")
	}
	if got, want := one.String(), "1"; got != want {
		t.Fatalf("canonical one = %q, want %q", got, want)
	}
	if got, want := unbounded.String(), "unbounded"; got != want {
		t.Fatalf("unbounded string = %q, want %q", got, want)
	}

	rangeValue, err := newParticleOccurrenceRange(zero, unbounded)
	if err != nil {
		t.Fatalf("newParticleOccurrenceRange: %v", err)
	}
	rangeMinimum := rangeValue.minimumOccurrence()
	rangeMinimum.finite.value.SetInt64(9)
	if got, want := rangeValue.String(), "0/unbounded"; got != want {
		t.Fatalf("range changed after accessor mutation: got %q, want %q", got, want)
	}
	if !rangeValue.mapsToParticle() {
		t.Fatal("zero/unbounded range was incorrectly treated as absent")
	}

	zeroRange, err := newParticleOccurrenceRange(zero, zero)
	if err != nil {
		t.Fatalf("new zero range: %v", err)
	}
	if zeroRange.mapsToParticle() {
		t.Fatal("zero/zero range mapped to a public particle")
	}
	if _, err := newParticleOccurrenceRange(unbounded, zero); !errors.Is(err, errParticleOccurrenceMinimumUnbounded) {
		t.Fatalf("unbounded minimum error = %v", err)
	}
	if _, err := newParticleOccurrenceRange(one, zero); !errors.Is(err, errParticleOccurrenceMinimumExceedsMaximum) {
		t.Fatalf("minimum-greater-than-maximum error = %v", err)
	}
}
