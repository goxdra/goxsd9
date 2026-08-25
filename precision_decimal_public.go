package goxsd9

import "math/big"

// StrictPrecisionDecimal is an immutable exact value in the optional
// precisionDecimal value space. It includes signed zero, infinities, and
// NaN. Its zero value is positive zero.
type StrictPrecisionDecimal struct {
	value precisionDecimalValue
}

// PrecisionDecimal is the short name for StrictPrecisionDecimal.
type PrecisionDecimal = StrictPrecisionDecimal

// PrecisionDecimalOrder is the partial comparison result for two
// precisionDecimal values. NaN makes the result unordered, including when
// compared with itself.
type PrecisionDecimalOrder uint8

const (
	// PrecisionDecimalLess reports that the left value is less than the right.
	PrecisionDecimalLess PrecisionDecimalOrder = iota + 1
	// PrecisionDecimalEqual reports numerical equality, including signed zero.
	PrecisionDecimalEqual
	// PrecisionDecimalGreater reports that the left value is greater than the right.
	PrecisionDecimalGreater
	// PrecisionDecimalUnordered reports that at least one value is NaN.
	PrecisionDecimalUnordered
)

// String returns the stable name of the comparison result.
func (order PrecisionDecimalOrder) String() string {
	switch order {
	case PrecisionDecimalLess:
		return "less"
	case PrecisionDecimalEqual:
		return "equal"
	case PrecisionDecimalGreater:
		return "greater"
	case PrecisionDecimalUnordered:
		return "unordered"
	default:
		return ""
	}
}

// ParseStrictPrecisionDecimal applies precisionDecimal whitespace collapse,
// validates the lexical mapping, and constructs one complete exact value.
func ParseStrictPrecisionDecimal(lexical string, loc Loc) (StrictPrecisionDecimal, error) {
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		return StrictPrecisionDecimal{}, err
	}
	return StrictPrecisionDecimal{value: clonePrecisionDecimalValue(value)}, nil
}

// ParsePrecisionDecimal parses an exact optional precisionDecimal value.
func ParsePrecisionDecimal(lexical string, loc Loc) (PrecisionDecimal, error) {
	return ParseStrictPrecisionDecimal(lexical, loc)
}

// Compare returns the exact partial ordering between two values.
func (value StrictPrecisionDecimal) Compare(other StrictPrecisionDecimal) PrecisionDecimalOrder {
	order := comparePrecisionDecimal(value.privateValue(), other.privateValue())
	return PrecisionDecimalOrder(order)
}

// Equal reports numerical equality. NaN is not equal to any value, including
// itself; positive and negative zero are equal.
func (value StrictPrecisionDecimal) Equal(other StrictPrecisionDecimal) bool {
	return value.Compare(other) == PrecisionDecimalEqual
}

// Sign reports whether the value is negative, zero, or positive. Both signed
// zero values report zero.
func (value StrictPrecisionDecimal) Sign() int {
	finite, ok := value.privateValue().(precisionDecimalFinite)
	if !ok || finite.coefficient.Sign() == 0 {
		return 0
	}
	if finite.sign == precisionDecimalSignNegative {
		return -1
	}
	return 1
}

// IsZero reports whether the value is either signed zero.
func (value StrictPrecisionDecimal) IsZero() bool {
	finite, ok := value.privateValue().(precisionDecimalFinite)
	return ok && finite.coefficient.Sign() == 0
}

// IsNegativeZero reports whether the value is negative zero.
func (value StrictPrecisionDecimal) IsNegativeZero() bool {
	finite, ok := value.privateValue().(precisionDecimalFinite)
	return ok && finite.coefficient.Sign() == 0 && finite.sign == precisionDecimalSignNegative
}

// IsPositiveZero reports whether the value is positive zero.
func (value StrictPrecisionDecimal) IsPositiveZero() bool {
	finite, ok := value.privateValue().(precisionDecimalFinite)
	return ok && finite.coefficient.Sign() == 0 && finite.sign == precisionDecimalSignPositive
}

// IsNaN reports whether the value is NaN.
func (value StrictPrecisionDecimal) IsNaN() bool {
	_, ok := value.privateValue().(precisionDecimalNaN)
	return ok
}

// IsInfinite reports whether the value is positive or negative infinity.
func (value StrictPrecisionDecimal) IsInfinite() bool {
	switch value.privateValue().(type) {
	case precisionDecimalPositiveInfinity, precisionDecimalNegativeInfinity:
		return true
	default:
		return false
	}
}

// IsPositiveInfinity reports whether the value is positive infinity.
func (value StrictPrecisionDecimal) IsPositiveInfinity() bool {
	_, ok := value.privateValue().(precisionDecimalPositiveInfinity)
	return ok
}

// IsNegativeInfinity reports whether the value is negative infinity.
func (value StrictPrecisionDecimal) IsNegativeInfinity() bool {
	_, ok := value.privateValue().(precisionDecimalNegativeInfinity)
	return ok
}

// Canonical returns the project-defined canonical lexical form when it fits
// budget bytes. The exact limit is checked before output is materialized.
func (value StrictPrecisionDecimal) Canonical(budget uint64, loc Loc) (string, error) {
	return canonicalPrecisionDecimal(value.privateValue(), budget, loc)
}

// CanonicalWithBudget is an explicit name for Canonical's bounded operation.
func (value StrictPrecisionDecimal) CanonicalWithBudget(budget uint64, loc Loc) (string, error) {
	return value.Canonical(budget, loc)
}

// Validate applies the complete effective precisionDecimal facet set to one
// lexical value. Pattern evaluation uses the normalized lexical form, while
// all other supported facets use the newly constructed exact value.
func (facets PrecisionDecimalFacets) Validate(lexical string, valueLoc Loc) error {
	return validatePrecisionDecimalFacets(lexical, facets, valueLoc)
}

// ValidatePrecisionDecimalFacets applies complete effective precisionDecimal
// facets to one lexical value.
func ValidatePrecisionDecimalFacets(lexical string, facets PrecisionDecimalFacets, valueLoc Loc) error {
	return facets.Validate(lexical, valueLoc)
}

// PatternDeclarations returns effective pattern declarations in their stable
// declaration order.
func (facets PrecisionDecimalFacets) PatternDeclarations() []PrecisionDecimalPatternFacet {
	if len(facets.patterns) == 0 {
		return nil
	}
	count := facets.PatternCount()
	patterns := make([]PrecisionDecimalPatternFacet, 0, count)
	for _, group := range facets.patterns {
		patterns = append(patterns, group...)
	}
	return patterns
}

// EnumerationDeclarations returns effective enumeration declarations in
// their stable declaration order.
func (facets PrecisionDecimalFacets) EnumerationDeclarations() []PrecisionDecimalEnumerationFacet {
	return clonePrecisionDecimalEnumerationFacets(facets.enumeration)
}

func (value StrictPrecisionDecimal) privateValue() precisionDecimalValue {
	if value.value == nil {
		return precisionDecimalFinite{
			coefficient: new(big.Int),
			scale:       new(big.Int),
			sign:        precisionDecimalSignPositive,
		}
	}
	return value.value
}
