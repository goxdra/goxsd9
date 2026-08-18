package goxsd9

import (
	"math/big"
	"strings"
)

const (
	diagnosticPrecisionDecimalLexicalCode      = "XSD2010"
	diagnosticPrecisionDecimalConstructionCode = "GOXSD9005"
	precisionDecimalLexicalSpecRef             = "xsd-precisionDecimal#f-precDecLexmap"
)

// precisionDecimalValue is the sealed value space for the lexical/value core.
type precisionDecimalValue interface {
	isPrecisionDecimalValue()
}

type precisionDecimalFinite struct {
	coefficient *big.Int
	scale       *big.Int
	sign        precisionDecimalSign
}

type precisionDecimalPositiveInfinity struct{}

type precisionDecimalNegativeInfinity struct{}

type precisionDecimalNaN struct{}

func (precisionDecimalFinite) isPrecisionDecimalValue() {}

func (precisionDecimalPositiveInfinity) isPrecisionDecimalValue() {}

func (precisionDecimalNegativeInfinity) isPrecisionDecimalValue() {}

func (precisionDecimalNaN) isPrecisionDecimalValue() {}

type precisionDecimalSign uint8

const (
	precisionDecimalSignPositive precisionDecimalSign = iota
	precisionDecimalSignNegative
)

func (value precisionDecimalFinite) coefficientCopy() *big.Int {
	if value.coefficient == nil {
		return nil
	}
	return new(big.Int).Set(value.coefficient)
}

func (value precisionDecimalFinite) scaleCopy() *big.Int {
	if value.scale == nil {
		return nil
	}
	return new(big.Int).Set(value.scale)
}

type precisionDecimalLexeme interface {
	isPrecisionDecimalLexeme()
}

type precisionDecimalFiniteLexeme struct {
	sign                 precisionDecimalSign
	significand          string
	fractionalDigitCount int
	exponentDigits       string
	exponentNegative     bool
}

type precisionDecimalSpecialLexeme struct {
	kind precisionDecimalSpecialKind
}

func (precisionDecimalFiniteLexeme) isPrecisionDecimalLexeme() {}

func (precisionDecimalSpecialLexeme) isPrecisionDecimalLexeme() {}

type precisionDecimalSpecialKind uint8

const (
	precisionDecimalSpecialPositiveInfinity precisionDecimalSpecialKind = iota + 1
	precisionDecimalSpecialNegativeInfinity
	precisionDecimalSpecialNaN
)

// parsePrecisionDecimal applies XML whitespace collapse, validates the pinned
// lexical mapping, and constructs one complete private value-space variant.
func parsePrecisionDecimal(lexical string, loc Loc) (precisionDecimalValue, error) {
	lexeme := collapseXMLWhitespace(lexical)
	scanned, ok := scanPrecisionDecimalLexical(lexeme)
	if !ok {
		return nil, newPrecisionDecimalLexicalDiagnostic(loc)
	}

	value, ok := constructPrecisionDecimal(scanned)
	if ok {
		return value, nil
	}
	return nil, newDiagnostic(
		FailureInternal,
		diagnosticPrecisionDecimalConstructionCode,
		loc,
		"valid precisionDecimal lexical representation could not be constructed",
		nil,
	)
}

func newPrecisionDecimalLexicalDiagnostic(loc Loc) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticPrecisionDecimalLexicalCode,
		loc:     loc,
		message: "invalid precisionDecimal lexical representation",
		specRef: precisionDecimalLexicalSpecRef,
	}
}

func scanPrecisionDecimalLexical(lexical string) (precisionDecimalLexeme, bool) {
	switch lexical {
	case "INF", "+INF":
		return precisionDecimalSpecialLexeme{
			kind: precisionDecimalSpecialPositiveInfinity,
		}, true
	case "-INF":
		return precisionDecimalSpecialLexeme{
			kind: precisionDecimalSpecialNegativeInfinity,
		}, true
	case "NaN":
		return precisionDecimalSpecialLexeme{
			kind: precisionDecimalSpecialNaN,
		}, true
	}

	if lexical == "" {
		return nil, false
	}

	start, sign := precisionDecimalSignStart(lexical)
	if start == len(lexical) {
		return nil, false
	}

	numeral := lexical[start:]
	exponentIndex := strings.IndexAny(numeral, "eE")
	significand := numeral
	exponentDigits := ""
	exponentNegative := false
	var ok bool
	if exponentIndex >= 0 {
		significand = numeral[:exponentIndex]
		exponentDigits, exponentNegative, ok = scanPrecisionDecimalExponent(numeral[exponentIndex+1:])
		if !ok {
			return nil, false
		}
	}

	integerDigits, fractionalDigits, _, ok := splitDecimalDigits(significand)
	if !ok {
		return nil, false
	}
	return precisionDecimalFiniteLexeme{
		sign:                 sign,
		significand:          integerDigits + fractionalDigits,
		fractionalDigitCount: len(fractionalDigits),
		exponentDigits:       exponentDigits,
		exponentNegative:     exponentNegative,
	}, true
}

func precisionDecimalSignStart(lexical string) (int, precisionDecimalSign) {
	if lexical[0] == '-' {
		return 1, precisionDecimalSignNegative
	}
	if lexical[0] == '+' {
		return 1, precisionDecimalSignPositive
	}
	return 0, precisionDecimalSignPositive
}

func scanPrecisionDecimalExponent(exponent string) (string, bool, bool) {
	if exponent == "" {
		return "", false, false
	}

	start := 0
	negative := false
	if exponent[0] == '+' || exponent[0] == '-' {
		negative = exponent[0] == '-'
		start++
	}
	if start == len(exponent) || !allASCIIDigits(exponent[start:]) {
		return "", false, false
	}
	return exponent[start:], negative, true
}

func constructPrecisionDecimal(lexeme precisionDecimalLexeme) (precisionDecimalValue, bool) {
	switch value := lexeme.(type) {
	case precisionDecimalFiniteLexeme:
		return constructPrecisionDecimalFinite(value)
	case precisionDecimalSpecialLexeme:
		switch value.kind {
		case precisionDecimalSpecialPositiveInfinity:
			return precisionDecimalPositiveInfinity{}, true
		case precisionDecimalSpecialNegativeInfinity:
			return precisionDecimalNegativeInfinity{}, true
		case precisionDecimalSpecialNaN:
			return precisionDecimalNaN{}, true
		}
	}
	return nil, false
}

func constructPrecisionDecimalFinite(lexeme precisionDecimalFiniteLexeme) (precisionDecimalValue, bool) {
	coefficientDigits := strings.TrimLeft(lexeme.significand, "0")
	if coefficientDigits == "" {
		coefficientDigits = "0"
	}
	coefficient, ok := new(big.Int).SetString(coefficientDigits, 10)
	if !ok {
		return nil, false
	}

	exponent := new(big.Int)
	if lexeme.exponentDigits != "" {
		exponent, ok = new(big.Int).SetString(lexeme.exponentDigits, 10)
		if !ok {
			return nil, false
		}
		if lexeme.exponentNegative {
			exponent.Neg(exponent)
		}
	}

	scale := new(big.Int).SetInt64(int64(lexeme.fractionalDigitCount))
	scale.Sub(scale, exponent)
	return precisionDecimalFinite{
		coefficient: coefficient,
		scale:       scale,
		sign:        lexeme.sign,
	}, true
}
