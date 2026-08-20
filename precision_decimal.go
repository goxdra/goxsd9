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

type precisionDecimalOrder uint8

const (
	precisionDecimalOrderLess precisionDecimalOrder = iota + 1
	precisionDecimalOrderEqual
	precisionDecimalOrderGreater
	precisionDecimalOrderUnordered
)

func comparePrecisionDecimal(left, right precisionDecimalValue) precisionDecimalOrder {
	validatePrecisionDecimalValue(left)
	validatePrecisionDecimalValue(right)

	leftNaN := precisionDecimalIsNaN(left)
	rightNaN := precisionDecimalIsNaN(right)
	if leftNaN || rightNaN {
		return precisionDecimalOrderUnordered
	}

	switch leftValue := left.(type) {
	case precisionDecimalFinite:
		switch rightValue := right.(type) {
		case precisionDecimalFinite:
			return comparePrecisionDecimalFinite(leftValue, rightValue)
		case precisionDecimalPositiveInfinity:
			return precisionDecimalOrderLess
		case precisionDecimalNegativeInfinity:
			return precisionDecimalOrderGreater
		default:
			panic("precisionDecimal comparison: invalid right value variant")
		}
	case precisionDecimalPositiveInfinity:
		switch right.(type) {
		case precisionDecimalFinite, precisionDecimalNegativeInfinity:
			return precisionDecimalOrderGreater
		case precisionDecimalPositiveInfinity:
			return precisionDecimalOrderEqual
		default:
			panic("precisionDecimal comparison: invalid right value variant")
		}
	case precisionDecimalNegativeInfinity:
		switch right.(type) {
		case precisionDecimalFinite, precisionDecimalPositiveInfinity:
			return precisionDecimalOrderLess
		case precisionDecimalNegativeInfinity:
			return precisionDecimalOrderEqual
		default:
			panic("precisionDecimal comparison: invalid right value variant")
		}
	case precisionDecimalNaN:
		panic("precisionDecimal comparison: NaN was not classified")
	default:
		panic("precisionDecimal comparison: invalid left value variant")
	}
}

func validatePrecisionDecimalValue(value precisionDecimalValue) {
	switch typed := value.(type) {
	case precisionDecimalFinite:
		validatePrecisionDecimalFinite(typed)
	case precisionDecimalPositiveInfinity, precisionDecimalNegativeInfinity, precisionDecimalNaN:
		return
	default:
		panic("precisionDecimal comparison: invalid value variant")
	}
}

func precisionDecimalIsNaN(value precisionDecimalValue) bool {
	switch value.(type) {
	case precisionDecimalFinite, precisionDecimalPositiveInfinity, precisionDecimalNegativeInfinity:
		return false
	case precisionDecimalNaN:
		return true
	default:
		panic("precisionDecimal comparison: invalid value variant")
	}
}

func comparePrecisionDecimalFinite(left, right precisionDecimalFinite) precisionDecimalOrder {
	validatePrecisionDecimalFinite(left)
	validatePrecisionDecimalFinite(right)

	leftZero := left.coefficient.Sign() == 0
	rightZero := right.coefficient.Sign() == 0
	if leftZero && rightZero {
		return precisionDecimalOrderEqual
	}
	if leftZero {
		if right.sign == precisionDecimalSignNegative {
			return precisionDecimalOrderGreater
		}
		return precisionDecimalOrderLess
	}
	if rightZero {
		if left.sign == precisionDecimalSignNegative {
			return precisionDecimalOrderLess
		}
		return precisionDecimalOrderGreater
	}
	if left.sign != right.sign {
		if left.sign == precisionDecimalSignNegative {
			return precisionDecimalOrderLess
		}
		return precisionDecimalOrderGreater
	}

	order := comparePrecisionDecimalMagnitude(left, right)
	if left.sign != precisionDecimalSignNegative {
		return order
	}
	return reversePrecisionDecimalOrder(order)
}

func validatePrecisionDecimalFinite(value precisionDecimalFinite) {
	if value.coefficient == nil {
		panic("precisionDecimal comparison: finite coefficient is nil")
	}
	if value.scale == nil {
		panic("precisionDecimal comparison: finite scale is nil")
	}
	if value.coefficient.Sign() < 0 {
		panic("precisionDecimal comparison: finite coefficient is negative")
	}
	switch value.sign {
	case precisionDecimalSignPositive, precisionDecimalSignNegative:
		return
	default:
		panic("precisionDecimal comparison: invalid finite sign")
	}
}

func comparePrecisionDecimalMagnitude(left, right precisionDecimalFinite) precisionDecimalOrder {
	leftDigits := precisionDecimalDecimalDigitCount(left.coefficient)
	rightDigits := precisionDecimalDecimalDigitCount(right.coefficient)
	leftOrder := precisionDecimalAdjustedOrder(left.scale, leftDigits)
	rightOrder := precisionDecimalAdjustedOrder(right.scale, rightDigits)
	comparison := leftOrder.Cmp(rightOrder)
	if comparison != 0 {
		return precisionDecimalOrderFromComparison(comparison)
	}

	// Equal leading orders make the alignment power no larger than the
	// coefficient digit-count difference, rather than the arbitrary scale.
	digitDifference := leftDigits - rightDigits
	if digitDifference > 0 {
		rightCoefficient := precisionDecimalScaledCoefficient(right.coefficient, digitDifference)
		return precisionDecimalOrderFromComparison(left.coefficient.Cmp(rightCoefficient))
	}
	if digitDifference < 0 {
		leftCoefficient := precisionDecimalScaledCoefficient(left.coefficient, -digitDifference)
		return precisionDecimalOrderFromComparison(leftCoefficient.Cmp(right.coefficient))
	}
	return precisionDecimalOrderFromComparison(left.coefficient.Cmp(right.coefficient))
}

func precisionDecimalDecimalDigitCount(coefficient *big.Int) int {
	if coefficient == nil || coefficient.Sign() <= 0 {
		panic("precisionDecimal comparison: digit count requires a positive coefficient")
	}

	// Since 2^3 is below 10, this upper bound keeps digit-count powers tied to
	// the coefficient size rather than to its arbitrary value-space scale.
	bitLength := coefficient.BitLen()
	high := bitLength / 3
	if bitLength%3 != 0 {
		high++
	}
	low := 0
	for high-low > 1 {
		middle := low + (high-low)/2
		if precisionDecimalPowerOfTen(middle).Cmp(coefficient) <= 0 {
			low = middle
			continue
		}
		high = middle
	}
	return low + 1
}

func precisionDecimalAdjustedOrder(scale *big.Int, digits int) *big.Int {
	order := big.NewInt(int64(digits - 1))
	return order.Sub(order, scale)
}

func precisionDecimalScaledCoefficient(coefficient *big.Int, power int) *big.Int {
	if power < 0 {
		panic("precisionDecimal comparison: negative coefficient alignment power")
	}
	result := new(big.Int).Set(coefficient)
	if power == 0 {
		return result
	}
	return result.Mul(result, precisionDecimalPowerOfTen(power))
}

func precisionDecimalPowerOfTen(power int) *big.Int {
	if power < 0 {
		panic("precisionDecimal comparison: negative power of ten")
	}
	result := big.NewInt(1)
	base := big.NewInt(10)
	for power > 0 {
		if power%2 == 1 {
			result.Mul(result, base)
		}
		power /= 2
		if power == 0 {
			break
		}
		base.Mul(base, base)
	}
	return result
}

func precisionDecimalOrderFromComparison(comparison int) precisionDecimalOrder {
	switch {
	case comparison < 0:
		return precisionDecimalOrderLess
	case comparison > 0:
		return precisionDecimalOrderGreater
	case comparison == 0:
		return precisionDecimalOrderEqual
	default:
		panic("precisionDecimal comparison: invalid integer comparison")
	}
}

func reversePrecisionDecimalOrder(order precisionDecimalOrder) precisionDecimalOrder {
	switch order {
	case precisionDecimalOrderLess:
		return precisionDecimalOrderGreater
	case precisionDecimalOrderEqual:
		return precisionDecimalOrderEqual
	case precisionDecimalOrderGreater:
		return precisionDecimalOrderLess
	case precisionDecimalOrderUnordered:
		panic("precisionDecimal comparison: unordered finite magnitude")
	default:
		panic("precisionDecimal comparison: invalid finite magnitude order")
	}
}

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
