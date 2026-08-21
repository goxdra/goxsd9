package goxsd9

import (
	"errors"
	"math/big"
	"strings"
)

const (
	diagnosticPrecisionDecimalLexicalCode        = "XSD2010"
	diagnosticPrecisionDecimalConstructionCode   = "GOXSD9005"
	diagnosticPrecisionDecimalCanonicalLimitCode = "GOXSD9026"
	precisionDecimalLexicalSpecRef               = "xsd-precisionDecimal#f-precDecLexmap"
	precisionDecimalCanonicalLimitSpecRef        = "xsd-precisionDecimal#implementation-limits"
)

// ErrPrecisionDecimalCanonicalOutputLimit reports a canonical output that
// exceeds its per-call byte budget.
var ErrPrecisionDecimalCanonicalOutputLimit = errors.New("precisionDecimal canonical output exceeds byte budget")

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

type precisionDecimalCanonicalForm uint8

const (
	precisionDecimalCanonicalZero precisionDecimalCanonicalForm = iota + 1
	precisionDecimalCanonicalNoDecimal
	precisionDecimalCanonicalDecimal
	precisionDecimalCanonicalScientific
	precisionDecimalCanonicalPositiveInfinity
	precisionDecimalCanonicalNegativeInfinity
	precisionDecimalCanonicalNaN
)

type precisionDecimalCanonicalPlan struct {
	form        precisionDecimalCanonicalForm
	length      uint64
	negative    bool
	coefficient *big.Int
	scale       *big.Int
	exponent    *big.Int
}

// canonicalPrecisionDecimal returns a complete canonical lexical form when
// its exact ASCII-byte length fits budget. It never stores the result on the
// value.
func canonicalPrecisionDecimal(value precisionDecimalValue, budget uint64, loc Loc) (string, error) {
	validatePrecisionDecimalValue(value)
	limit := precisionDecimalCanonicalLengthLimit(budget)
	if limit == 0 {
		return "", newPrecisionDecimalCanonicalLimitDiagnostic(loc)
	}

	plan, ok := planPrecisionDecimalCanonical(value, limit)
	if !ok {
		return "", newPrecisionDecimalCanonicalLimitDiagnostic(loc)
	}
	return materializePrecisionDecimalCanonical(plan, loc)
}

func precisionDecimalCanonicalLengthLimit(budget uint64) uint64 {
	maxInt := uint64(^uint(0) >> 1)
	if budget < maxInt {
		return budget
	}
	return maxInt
}

func newPrecisionDecimalCanonicalLimitDiagnostic(loc Loc) Diagnostic {
	return Diagnostic{
		class:   FailureInvalid,
		code:    diagnosticPrecisionDecimalCanonicalLimitCode,
		loc:     loc,
		message: "precisionDecimal canonical output exceeds its byte budget",
		specRef: precisionDecimalCanonicalLimitSpecRef,
		cause:   ErrPrecisionDecimalCanonicalOutputLimit,
	}
}

func planPrecisionDecimalCanonical(value precisionDecimalValue, limit uint64) (precisionDecimalCanonicalPlan, bool) {
	switch typed := value.(type) {
	case precisionDecimalFinite:
		return planPrecisionDecimalFiniteCanonical(typed, limit)
	case precisionDecimalPositiveInfinity:
		return planPrecisionDecimalFixedCanonical(precisionDecimalCanonicalPositiveInfinity, 3, limit)
	case precisionDecimalNegativeInfinity:
		return planPrecisionDecimalFixedCanonical(precisionDecimalCanonicalNegativeInfinity, 4, limit)
	case precisionDecimalNaN:
		return planPrecisionDecimalFixedCanonical(precisionDecimalCanonicalNaN, 3, limit)
	default:
		panic("precisionDecimal canonicalization: invalid value variant")
	}
}

func planPrecisionDecimalFixedCanonical(form precisionDecimalCanonicalForm, length uint64, limit uint64) (precisionDecimalCanonicalPlan, bool) {
	if length > limit {
		return precisionDecimalCanonicalPlan{}, false
	}
	return precisionDecimalCanonicalPlan{form: form, length: length}, true
}

func planPrecisionDecimalFiniteCanonical(value precisionDecimalFinite, limit uint64) (precisionDecimalCanonicalPlan, bool) {
	if value.coefficient.Sign() == 0 {
		length := uint64(5)
		if value.sign == precisionDecimalSignNegative {
			length++
		}
		if length > limit {
			return precisionDecimalCanonicalPlan{}, false
		}
		return precisionDecimalCanonicalPlan{
			form:     precisionDecimalCanonicalZero,
			length:   length,
			negative: value.sign == precisionDecimalSignNegative,
		}, true
	}

	digitCount, ok := precisionDecimalDecimalDigitCountAtMost(value.coefficient, limit)
	if !ok {
		return precisionDecimalCanonicalPlan{}, false
	}
	adjustedExponent := new(big.Int).SetUint64(digitCount - 1)
	adjustedExponent.Sub(adjustedExponent, value.scale)
	inRange := precisionDecimalMagnitudeInCanonicalRange(adjustedExponent, value.coefficient)
	negative := value.sign == precisionDecimalSignNegative

	if value.scale.Sign() == 0 && inRange {
		length, fits := precisionDecimalCanonicalAddLength(precisionDecimalSignLength(negative), digitCount, limit)
		if !fits {
			return precisionDecimalCanonicalPlan{}, false
		}
		return precisionDecimalCanonicalPlan{
			form:        precisionDecimalCanonicalNoDecimal,
			length:      length,
			negative:    negative,
			coefficient: value.coefficient,
		}, true
	}
	if value.scale.Sign() > 0 && inRange {
		length, fits := precisionDecimalCanonicalDecimalLength(negative, value.scale, digitCount, limit)
		if !fits {
			return precisionDecimalCanonicalPlan{}, false
		}
		return precisionDecimalCanonicalPlan{
			form:        precisionDecimalCanonicalDecimal,
			length:      length,
			negative:    negative,
			coefficient: value.coefficient,
			scale:       value.scale,
		}, true
	}

	length, fits := precisionDecimalCanonicalScientificLength(negative, adjustedExponent, digitCount, limit)
	if !fits {
		return precisionDecimalCanonicalPlan{}, false
	}
	return precisionDecimalCanonicalPlan{
		form:        precisionDecimalCanonicalScientific,
		length:      length,
		negative:    negative,
		coefficient: value.coefficient,
		exponent:    adjustedExponent,
	}, true
}

func precisionDecimalSignLength(negative bool) uint64 {
	if negative {
		return 1
	}
	return 0
}

func precisionDecimalCanonicalAddLength(length, addition, limit uint64) (uint64, bool) {
	if length > limit || addition > limit-length {
		return 0, false
	}
	return length + addition, true
}

func precisionDecimalCanonicalDecimalLength(negative bool, scale *big.Int, digits uint64, limit uint64) (uint64, bool) {
	length := precisionDecimalSignLength(negative)
	digitCount := new(big.Int).SetUint64(digits)
	if scale.Cmp(digitCount) < 0 {
		return precisionDecimalCanonicalAddLength(length, digits+1, limit)
	}

	var ok bool
	length, ok = precisionDecimalCanonicalAddLength(length, 2, limit)
	if !ok {
		return 0, false
	}
	return precisionDecimalCanonicalAddBigLength(length, scale, limit)
}

func precisionDecimalCanonicalAddBigLength(length uint64, addition *big.Int, limit uint64) (uint64, bool) {
	if addition.Sign() < 0 {
		panic("precisionDecimal canonicalization: negative length addition")
	}
	if length > limit {
		return 0, false
	}
	remaining := new(big.Int).SetUint64(limit - length)
	if addition.Cmp(remaining) > 0 {
		return 0, false
	}

	result := length
	count := new(big.Int).Set(addition)
	one := big.NewInt(1)
	for count.Sign() > 0 {
		result++
		count.Sub(count, one)
	}
	return result, true
}

func precisionDecimalCanonicalScientificLength(negative bool, exponent *big.Int, digits uint64, limit uint64) (uint64, bool) {
	length := precisionDecimalSignLength(negative)
	mantissaLength := digits
	if digits > 1 {
		mantissaLength++
	}
	var ok bool
	length, ok = precisionDecimalCanonicalAddLength(length, mantissaLength, limit)
	if !ok {
		return 0, false
	}
	length, ok = precisionDecimalCanonicalAddLength(length, 1, limit)
	if !ok {
		return 0, false
	}
	if exponent.Sign() < 0 {
		length, ok = precisionDecimalCanonicalAddLength(length, 1, limit)
		if !ok {
			return 0, false
		}
	}

	exponentDigits := new(big.Int).Set(exponent)
	if exponentDigits.Sign() < 0 {
		exponentDigits.Neg(exponentDigits)
	}
	if exponentDigits.Sign() == 0 {
		return precisionDecimalCanonicalAddLength(length, 1, limit)
	}
	remaining := limit - length
	digitCount, ok := precisionDecimalDecimalDigitCountAtMost(exponentDigits, remaining)
	if !ok {
		return 0, false
	}
	return precisionDecimalCanonicalAddLength(length, digitCount, limit)
}

func precisionDecimalMagnitudeInCanonicalRange(adjustedExponent, coefficient *big.Int) bool {
	if adjustedExponent.Cmp(big.NewInt(-6)) < 0 {
		return false
	}
	if adjustedExponent.Cmp(big.NewInt(6)) < 0 {
		return true
	}
	if adjustedExponent.Cmp(big.NewInt(6)) > 0 {
		return false
	}
	return precisionDecimalIsPowerOfTen(coefficient)
}

func precisionDecimalIsPowerOfTen(value *big.Int) bool {
	if value.Sign() <= 0 {
		return false
	}

	quotient := new(big.Int).Set(value)
	remainder := new(big.Int)
	base := big.NewInt(1_000_000_000)
	for quotient.Cmp(base) >= 0 {
		quotient.QuoRem(quotient, base, remainder)
		if remainder.Sign() != 0 {
			return false
		}
	}

	small := quotient.Uint64()
	if small == 0 {
		return false
	}
	for small%10 == 0 {
		small /= 10
	}
	return small == 1
}

func precisionDecimalDecimalDigitCountAtMost(value *big.Int, limit uint64) (uint64, bool) {
	if value == nil || value.Sign() <= 0 {
		panic("precisionDecimal canonicalization: digit count requires a positive value")
	}
	if limit == 0 {
		return 0, false
	}

	quotient := new(big.Int).Set(value)
	base := big.NewInt(1_000_000_000)
	remainder := new(big.Int)
	var chunks uint64
	for quotient.Cmp(base) >= 0 {
		if limit < 10 || chunks > (limit-10)/9 {
			return 0, false
		}
		quotient.QuoRem(quotient, base, remainder)
		chunks++
	}

	tail := precisionDecimalSmallDecimalDigitCount(quotient.Uint64())
	if tail > limit {
		return 0, false
	}
	if chunks > (limit-tail)/9 {
		return 0, false
	}
	return chunks*9 + tail, true
}

func precisionDecimalSmallDecimalDigitCount(value uint64) uint64 {
	switch {
	case value >= 100_000_000:
		return 9
	case value >= 10_000_000:
		return 8
	case value >= 1_000_000:
		return 7
	case value >= 100_000:
		return 6
	case value >= 10_000:
		return 5
	case value >= 1_000:
		return 4
	case value >= 100:
		return 3
	case value >= 10:
		return 2
	default:
		return 1
	}
}

func materializePrecisionDecimalCanonical(plan precisionDecimalCanonicalPlan, loc Loc) (string, error) {
	switch plan.form {
	case precisionDecimalCanonicalPositiveInfinity:
		return "INF", nil
	case precisionDecimalCanonicalNegativeInfinity:
		return "-INF", nil
	case precisionDecimalCanonicalNaN:
		return "NaN", nil
	case precisionDecimalCanonicalZero:
		if plan.negative {
			return "-0.0E0", nil
		}
		return "0.0E0", nil
	case precisionDecimalCanonicalNoDecimal, precisionDecimalCanonicalDecimal, precisionDecimalCanonicalScientific:
		// These forms are materialized below.
	default:
		panic("precisionDecimal canonicalization: invalid planned form")
	}

	var writer precisionDecimalCanonicalWriter
	writer.builder.Grow(precisionDecimalCanonicalLengthAsInt(plan.length))
	digits := plan.coefficient.String()
	if plan.negative {
		writer.writeByte('-')
	}
	switch plan.form {
	case precisionDecimalCanonicalNoDecimal:
		writer.writeString(digits)
	case precisionDecimalCanonicalDecimal:
		writePrecisionDecimalCanonicalDecimal(&writer, plan.scale, digits)
	case precisionDecimalCanonicalScientific:
		writePrecisionDecimalCanonicalScientific(&writer, plan.exponent, digits)
	case precisionDecimalCanonicalZero, precisionDecimalCanonicalPositiveInfinity, precisionDecimalCanonicalNegativeInfinity, precisionDecimalCanonicalNaN:
		panic("precisionDecimal canonicalization: special form reached materialization")
	default:
		panic("precisionDecimal canonicalization: invalid materialization form")
	}
	if writer.err != nil {
		return "", newDiagnostic(
			FailureInternal,
			diagnosticPrecisionDecimalConstructionCode,
			loc,
			"precisionDecimal canonical output could not be materialized",
			writer.err,
		)
	}
	result := writer.builder.String()
	if uint64(len(result)) != plan.length {
		return "", newDiagnostic(
			FailureInternal,
			diagnosticPrecisionDecimalConstructionCode,
			loc,
			"precisionDecimal canonical output length plan was inconsistent",
			nil,
		)
	}
	return result, nil
}

func precisionDecimalCanonicalLengthAsInt(length uint64) int {
	maxInt := uint64(^uint(0) >> 1)
	if length > maxInt {
		panic("precisionDecimal canonicalization: planned length does not fit int")
	}
	// The range check above proves this conversion is representable.
	return int(length) // #nosec G115 -- length is checked against maxInt
}

type precisionDecimalCanonicalWriter struct {
	builder strings.Builder
	err     error
}

func (writer *precisionDecimalCanonicalWriter) writeByte(value byte) {
	if writer.err != nil {
		return
	}
	writer.err = writer.builder.WriteByte(value)
}

func (writer *precisionDecimalCanonicalWriter) writeString(value string) {
	if writer.err != nil {
		return
	}
	_, writer.err = writer.builder.WriteString(value)
}

func (writer *precisionDecimalCanonicalWriter) writeZeroes(count *big.Int) {
	remaining := new(big.Int).Set(count)
	one := big.NewInt(1)
	for remaining.Sign() > 0 {
		writer.writeByte('0')
		if writer.err != nil {
			return
		}
		remaining.Sub(remaining, one)
	}
}

func writePrecisionDecimalCanonicalDecimal(writer *precisionDecimalCanonicalWriter, scale *big.Int, digits string) {
	digitCount := big.NewInt(int64(len(digits)))
	if scale.Cmp(digitCount) < 0 {
		integerDigits := precisionDecimalDecimalPointIndex(scale, len(digits))
		writer.writeString(digits[:integerDigits])
		writer.writeByte('.')
		writer.writeString(digits[integerDigits:])
		return
	}

	writer.writeString("0.")
	padding := new(big.Int).Sub(scale, digitCount)
	writer.writeZeroes(padding)
	writer.writeString(digits)
}

func precisionDecimalDecimalPointIndex(scale *big.Int, digits int) int {
	index := digits
	remaining := new(big.Int).Set(scale)
	one := big.NewInt(1)
	for remaining.Sign() > 0 {
		index--
		remaining.Sub(remaining, one)
	}
	return index
}

func writePrecisionDecimalCanonicalScientific(writer *precisionDecimalCanonicalWriter, exponent *big.Int, digits string) {
	if len(digits) == 1 {
		writer.writeString(digits)
	}
	if len(digits) > 1 {
		writer.writeByte(digits[0])
		writer.writeByte('.')
		writer.writeString(digits[1:])
	}
	writer.writeByte('E')
	if exponent.Sign() < 0 {
		writer.writeByte('-')
	}
	exponentDigits := new(big.Int).Set(exponent)
	if exponentDigits.Sign() < 0 {
		exponentDigits.Neg(exponentDigits)
	}
	writer.writeString(exponentDigits.String())
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

// precisionDecimalFacetInput keeps the phase-specific lexical/value pairing
// used by value facets. The value itself never retains lexical text.
type precisionDecimalFacetInput struct {
	normalizedLexical string
	value             precisionDecimalValue
}

// parsePrecisionDecimal applies XML whitespace collapse, validates the pinned
// lexical mapping, and constructs one complete private value-space variant.
func parsePrecisionDecimal(lexical string, loc Loc) (precisionDecimalValue, error) {
	input, err := parsePrecisionDecimalFacetInput(lexical, loc)
	if err != nil {
		return nil, err
	}
	return input.value, nil
}

func parsePrecisionDecimalFacetInput(lexical string, loc Loc) (precisionDecimalFacetInput, error) {
	lexeme := collapseXMLWhitespace(lexical)
	scanned, ok := scanPrecisionDecimalLexical(lexeme)
	if !ok {
		return precisionDecimalFacetInput{}, newPrecisionDecimalLexicalDiagnostic(loc)
	}

	value, ok := constructPrecisionDecimal(scanned)
	if ok {
		return precisionDecimalFacetInput{
			normalizedLexical: lexeme,
			value:             value,
		}, nil
	}
	return precisionDecimalFacetInput{}, newDiagnostic(
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
