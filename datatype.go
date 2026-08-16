package goxsd9

import (
	"fmt"
	"math/big"
	"strings"
)

const (
	// InvalidIntegerLexicalCode identifies an invalid xs:integer lexical form.
	InvalidIntegerLexicalCode = "XSD2001"
	// InvalidDecimalLexicalCode identifies an invalid xs:decimal lexical form.
	InvalidDecimalLexicalCode = "XSD2002"
	// InvalidXSDVersionCode identifies an unsupported datatype version policy.
	InvalidXSDVersionCode = "XSD2003"
)

// XSDVersion selects the specification version for version-sensitive
// canonical representations.
type XSDVersion string

const (
	// XSDVersion10 selects XSD 1.0 canonical mappings.
	XSDVersion10 XSDVersion = "1.0"
	// XSDVersion11 selects XSD 1.1 canonical mappings.
	XSDVersion11 XSDVersion = "1.1"
	// XSD10 is a short name for XSDVersion10.
	XSD10 = XSDVersion10
	// XSD11 is a short name for XSDVersion11.
	XSD11 = XSDVersion11
)

// StrictInteger is an arbitrary-precision value in the XSD integer value
// space. Its zero value is the integer zero.
type StrictInteger struct {
	value *big.Int
}

// Integer is the strict XSD integer value type.
type Integer = StrictInteger

// ParseStrictInteger applies XSD whitespace normalization, validates an
// integer lexical form, and constructs its exact value.
func ParseStrictInteger(lexical string, loc Loc) (StrictInteger, error) {
	lexeme := collapseXMLWhitespace(lexical)
	parsed, ok := scanIntegerLexical(lexeme)
	if !ok {
		return StrictInteger{}, newDiagnostic(
			FailureInvalid,
			InvalidIntegerLexicalCode,
			loc,
			"invalid xs:integer lexical representation",
			nil,
		)
	}

	value, ok := newBigInteger(parsed.digits, parsed.negative)
	if !ok {
		return StrictInteger{}, newDiagnostic(
			FailureInternal,
			"GOXSD9003",
			loc,
			"valid integer lexical representation could not be constructed",
			nil,
		)
	}
	return StrictInteger{value: value}, nil
}

// ParseInteger parses an exact strict XSD integer value.
func ParseInteger(lexical string, loc Loc) (Integer, error) {
	return ParseStrictInteger(lexical, loc)
}

// String returns the canonical XSD representation of the integer.
func (value StrictInteger) String() string {
	return value.Canonical()
}

// Canonical returns the canonical XSD representation of the integer.
func (value StrictInteger) Canonical() string {
	if value.value == nil {
		return "0"
	}
	return value.value.String()
}

func (value StrictInteger) integerCopy() *big.Int {
	if value.value == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(value.value)
}

// Sign reports whether the integer is negative, zero, or positive.
func (value StrictInteger) Sign() int {
	if value.value == nil {
		return 0
	}
	return value.value.Sign()
}

// IsZero reports whether the integer is zero.
func (value StrictInteger) IsZero() bool {
	return value.Sign() == 0
}

// Compare compares the integer with another integer and returns -1, 0, or 1.
func (value StrictInteger) Compare(other StrictInteger) int {
	return value.integerCopy().Cmp(other.integerCopy())
}

// Equal reports whether two integers represent the same value.
func (value StrictInteger) Equal(other StrictInteger) bool {
	return value.Compare(other) == 0
}

type integerLexeme struct {
	digits   string
	negative bool
}

func scanIntegerLexical(lexical string) (integerLexeme, bool) {
	if lexical == "" {
		return integerLexeme{}, false
	}

	start := 0
	negative := false
	if lexical[0] == '+' || lexical[0] == '-' {
		negative = lexical[0] == '-'
		start++
	}
	if start == len(lexical) {
		return integerLexeme{}, false
	}
	for index := start; index < len(lexical); index++ {
		if !isASCIIDigit(lexical[index]) {
			return integerLexeme{}, false
		}
	}
	return integerLexeme{digits: lexical[start:], negative: negative}, true
}

func newBigInteger(digits string, negative bool) (*big.Int, bool) {
	value, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, false
	}
	if negative {
		value.Neg(value)
	}
	return value, true
}

// StrictDecimal is an arbitrary-precision value in the XSD decimal value
// space. Precision is not retained: lexically distinct values such as 2.0
// and 2.00 compare equal. Its zero value is decimal zero.
type StrictDecimal struct {
	coefficient *big.Int
	scale       int
	negative    bool
}

// Decimal is the strict XSD decimal value type.
type Decimal = StrictDecimal

// ParseStrictDecimal applies XSD whitespace normalization, validates a
// decimal lexical form, and constructs its exact value. With no version it
// uses the XSD 1.1 lexical grammar; pass a version to select an explicit
// policy.
func ParseStrictDecimal(lexical string, loc Loc, versions ...XSDVersion) (StrictDecimal, error) {
	version, err := selectXSDVersion(versions)
	if err != nil {
		return StrictDecimal{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	return parseStrictDecimal(lexical, loc, version)
}

// ParseStrictDecimalFor parses a decimal using an explicit XSD version.
func ParseStrictDecimalFor(version XSDVersion, lexical string, loc Loc) (StrictDecimal, error) {
	return ParseStrictDecimal(lexical, loc, version)
}

func parseStrictDecimal(lexical string, loc Loc, version XSDVersion) (StrictDecimal, error) {
	lexeme := collapseXMLWhitespace(lexical)
	parsed, ok := scanDecimalLexical(lexeme, version)
	if !ok {
		return StrictDecimal{}, newDiagnostic(
			FailureInvalid,
			InvalidDecimalLexicalCode,
			loc,
			"invalid xs:decimal lexical representation",
			nil,
		)
	}

	value, ok := newStrictDecimal(parsed)
	if !ok {
		return StrictDecimal{}, newDiagnostic(
			FailureInternal,
			"GOXSD9004",
			loc,
			"valid decimal lexical representation could not be constructed",
			nil,
		)
	}
	return value, nil
}

// ParseDecimal parses an exact strict XSD decimal value.
func ParseDecimal(lexical string, loc Loc, versions ...XSDVersion) (Decimal, error) {
	return ParseStrictDecimal(lexical, loc, versions...)
}

// ParseDecimalFor parses a decimal using an explicit XSD version.
func ParseDecimalFor(version XSDVersion, lexical string, loc Loc) (Decimal, error) {
	return ParseStrictDecimalFor(version, lexical, loc)
}

// String returns the canonical XSD representation of the decimal.
func (value StrictDecimal) String() string {
	return value.Canonical()
}

// Canonical returns the canonical XSD representation of the decimal.
// With no argument it uses the XSD 1.0 mapping for compatibility. XSD 1.1
// omits the decimal point when the value is an integer.
func (value StrictDecimal) Canonical(versions ...XSDVersion) string {
	version := XSDVersion10
	if len(versions) > 0 {
		version = versions[0]
	}
	if len(versions) > 1 || (version != XSDVersion10 && version != XSDVersion11) {
		return ""
	}
	if version == XSDVersion11 && value.Scale() == 0 {
		coefficient := value.coefficientCopy().String()
		return signedDecimal(value.negative, coefficient)
	}
	return value.canonicalXSD10()
}

// CanonicalFor returns a canonical representation for a supported XSD
// version or an error for an unknown version.
func (value StrictDecimal) CanonicalFor(version XSDVersion) (string, error) {
	if version != XSDVersion10 && version != XSDVersion11 {
		return "", fmt.Errorf("unsupported XSD version %q", version)
	}
	return value.Canonical(version), nil
}

func (value StrictDecimal) canonicalXSD10() string {
	coefficient := value.coefficientCopy()
	if coefficient.Sign() == 0 {
		return "0.0"
	}

	digits := coefficient.String()
	if value.scale == 0 {
		return signedDecimal(value.negative, digits+".0")
	}
	if len(digits) > value.scale {
		point := len(digits) - value.scale
		return signedDecimal(value.negative, digits[:point]+"."+digits[point:])
	}
	return signedDecimal(
		value.negative,
		"0."+strings.Repeat("0", value.scale-len(digits))+digits,
	)
}

// Scale returns the non-negative decimal scale used by the exact value.
func (value StrictDecimal) Scale() int {
	if value.coefficient == nil || value.coefficient.Sign() == 0 {
		return 0
	}
	return value.scale
}

// FractionDigits returns the smallest number of fractional digits needed to
// represent the value exactly.
func (value StrictDecimal) FractionDigits() int {
	return value.Scale()
}

// TotalDigits returns the smallest totalDigits facet value that admits the
// exact value.
func (value StrictDecimal) TotalDigits() int {
	digits := len(value.coefficientCopy().String())
	if value.Scale() > digits {
		return value.Scale()
	}
	return digits
}

// Sign reports whether the decimal is negative, zero, or positive.
func (value StrictDecimal) Sign() int {
	if value.coefficient == nil || value.coefficient.Sign() == 0 {
		return 0
	}
	if value.negative {
		return -1
	}
	return 1
}

// IsZero reports whether the decimal is zero.
func (value StrictDecimal) IsZero() bool {
	return value.Sign() == 0
}

// Compare compares the decimal with another decimal and returns -1, 0, or 1.
func (value StrictDecimal) Compare(other StrictDecimal) int {
	leftSign := value.Sign()
	rightSign := other.Sign()
	if leftSign != rightSign {
		if leftSign < rightSign {
			return -1
		}
		return 1
	}
	if leftSign == 0 {
		return 0
	}

	comparison := compareDecimalMagnitude(value, other)
	if leftSign < 0 {
		return -comparison
	}
	return comparison
}

// Equal reports whether two decimals represent the same value.
func (value StrictDecimal) Equal(other StrictDecimal) bool {
	return value.Compare(other) == 0
}

type decimalLexeme struct {
	integerDigits  string
	fractionDigits string
	negative       bool
}

func scanDecimalLexical(lexical string, version XSDVersion) (decimalLexeme, bool) {
	if lexical == "" {
		return decimalLexeme{}, false
	}

	start, negative := decimalSign(lexical)
	if start == len(lexical) {
		return decimalLexeme{}, false
	}

	integerDigits, fractionDigits, hasPoint, ok := splitDecimalDigits(lexical[start:])
	if !ok {
		return decimalLexeme{}, false
	}
	if hasPoint && version == XSDVersion10 && (integerDigits == "" || fractionDigits == "") {
		return decimalLexeme{}, false
	}
	return decimalLexeme{
		integerDigits:  integerDigits,
		fractionDigits: fractionDigits,
		negative:       negative,
	}, true
}

func decimalSign(lexical string) (int, bool) {
	if lexical[0] == '+' || lexical[0] == '-' {
		return 1, lexical[0] == '-'
	}
	return 0, false
}

func splitDecimalDigits(numeral string) (string, string, bool, bool) {
	point := strings.IndexByte(numeral, '.')
	if point < 0 {
		return numeral, "", false, allASCIIDigits(numeral)
	}
	if strings.IndexByte(numeral[point+1:], '.') >= 0 {
		return "", "", false, false
	}
	integerDigits := numeral[:point]
	fractionDigits := numeral[point+1:]
	if integerDigits == "" && fractionDigits == "" {
		return "", "", false, false
	}
	if integerDigits != "" && !allASCIIDigits(integerDigits) {
		return "", "", false, false
	}
	if fractionDigits != "" && !allASCIIDigits(fractionDigits) {
		return "", "", false, false
	}
	return integerDigits, fractionDigits, true, true
}

func selectXSDVersion(versions []XSDVersion) (XSDVersion, error) {
	if len(versions) == 0 {
		return XSDVersion11, nil
	}
	if len(versions) != 1 {
		return "", fmt.Errorf("exactly one XSD version is required, got %d", len(versions))
	}
	version := versions[0]
	if version != XSDVersion10 && version != XSDVersion11 {
		return "", fmt.Errorf("unsupported XSD version %q", version)
	}
	return version, nil
}

func newStrictDecimal(parsed decimalLexeme) (StrictDecimal, bool) {
	digits := parsed.integerDigits + parsed.fractionDigits
	first := 0
	for first < len(digits) && digits[first] == '0' {
		first++
	}
	if first == len(digits) {
		return StrictDecimal{coefficient: new(big.Int)}, true
	}
	digits = digits[first:]
	scale := len(parsed.fractionDigits)
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = digits[:len(digits)-1]
		scale--
	}
	coefficient, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return StrictDecimal{}, false
	}
	return StrictDecimal{
		coefficient: coefficient,
		scale:       scale,
		negative:    parsed.negative,
	}, true
}

func compareDecimalMagnitude(left, right StrictDecimal) int {
	leftCoefficient := left.coefficientCopy()
	rightCoefficient := right.coefficientCopy()
	scale := left.Scale()
	if right.Scale() > scale {
		scale = right.Scale()
	}
	if left.Scale() < scale {
		leftCoefficient.Mul(leftCoefficient, decimalPowerOfTen(scale-left.Scale()))
	}
	if right.Scale() < scale {
		rightCoefficient.Mul(rightCoefficient, decimalPowerOfTen(scale-right.Scale()))
	}
	return leftCoefficient.Cmp(rightCoefficient)
}

func decimalPowerOfTen(power int) *big.Int {
	if power == 0 {
		return big.NewInt(1)
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(power)), nil)
}

func (value StrictDecimal) coefficientCopy() *big.Int {
	if value.coefficient == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(value.coefficient)
}

func signedDecimal(negative bool, digits string) string {
	if !negative {
		return digits
	}
	return "-" + digits
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if !isASCIIDigit(value[index]) {
			return false
		}
	}
	return true
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func collapseXMLWhitespace(input string) string {
	var output strings.Builder
	output.Grow(len(input))
	pendingSpace := false
	for index := 0; index < len(input); index++ {
		if isXMLWhitespace(input[index]) {
			if output.Len() > 0 {
				pendingSpace = true
			}
			continue
		}
		if pendingSpace {
			output.WriteByte(' ')
			pendingSpace = false
		}
		output.WriteByte(input[index])
	}
	return output.String()
}

func isXMLWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}
