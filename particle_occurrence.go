package goxsd9

import "errors"

var (
	errParticleOccurrenceNegative              = errors.New("particle occurrence is negative")
	errParticleOccurrenceMinimumUnbounded      = errors.New("particle minimum occurrence is unbounded")
	errParticleOccurrenceMinimumExceedsMaximum = errors.New("particle minimum occurrence exceeds maximum occurrence")
)

type particleOccurrenceKind uint8

const (
	particleOccurrenceFinite particleOccurrenceKind = iota
	particleOccurrenceUnbounded
)

// particleOccurrence is a completed exact occurrence bound. Its finite
// value is owned and its unbounded variant is available only for maxima.
type particleOccurrence struct {
	kind   particleOccurrenceKind
	finite StrictInteger
}

func newFiniteParticleOccurrence(value StrictInteger) (particleOccurrence, error) {
	if value.Sign() < 0 {
		return particleOccurrence{}, errParticleOccurrenceNegative
	}
	return particleOccurrence{
		kind:   particleOccurrenceFinite,
		finite: cloneStrictInteger(value),
	}, nil
}

func newUnboundedParticleOccurrence() particleOccurrence {
	return particleOccurrence{kind: particleOccurrenceUnbounded}
}

func parseParticleOccurrence(lexical string, allowUnbounded bool, loc Loc) (particleOccurrence, error) {
	lexeme := collapseXMLWhitespace(lexical)
	if allowUnbounded && lexeme == "unbounded" {
		return newUnboundedParticleOccurrence(), nil
	}

	value, err := ParseStrictInteger(lexeme, loc)
	if err != nil {
		return particleOccurrence{}, err
	}
	return newFiniteParticleOccurrence(value)
}

func (value particleOccurrence) isUnbounded() bool {
	return value.kind == particleOccurrenceUnbounded
}

func (value particleOccurrence) finiteValue() (StrictInteger, bool) {
	if value.kind != particleOccurrenceFinite {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(value.finite), true
}

// Compare orders finite bounds before the distinct unbounded bound.
func (value particleOccurrence) Compare(other particleOccurrence) int {
	switch {
	case value.kind == particleOccurrenceFinite && other.kind == particleOccurrenceFinite:
		return value.finite.Compare(other.finite)
	case value.kind == particleOccurrenceUnbounded && other.kind == particleOccurrenceUnbounded:
		return 0
	case value.kind == particleOccurrenceUnbounded:
		return 1
	case other.kind == particleOccurrenceUnbounded:
		return -1
	default:
		return value.finite.Compare(other.finite)
	}
}

// Equal reports whether two bounds have the same finite value or variant.
func (value particleOccurrence) Equal(other particleOccurrence) bool {
	if value.kind != other.kind {
		return false
	}
	if value.kind == particleOccurrenceUnbounded {
		return true
	}
	return value.finite.Equal(other.finite)
}

// String returns the canonical finite value or the unbounded keyword.
func (value particleOccurrence) String() string {
	if value.isUnbounded() {
		return "unbounded"
	}
	return value.finite.Canonical()
}

func (value particleOccurrence) clone() particleOccurrence {
	if value.isUnbounded() {
		return newUnboundedParticleOccurrence()
	}
	return particleOccurrence{
		kind:   value.kind,
		finite: cloneStrictInteger(value.finite),
	}
}

// particleOccurrenceRange is a completed range with an exact finite minimum
// and either an exact finite or unbounded maximum.
type particleOccurrenceRange struct {
	minimum particleOccurrence
	maximum particleOccurrence
}

func newParticleOccurrenceRange(minimum, maximum particleOccurrence) (particleOccurrenceRange, error) {
	if minimum.isUnbounded() {
		return particleOccurrenceRange{}, errParticleOccurrenceMinimumUnbounded
	}
	if !maximum.isUnbounded() && minimum.Compare(maximum) > 0 {
		return particleOccurrenceRange{}, errParticleOccurrenceMinimumExceedsMaximum
	}
	return particleOccurrenceRange{
		minimum: minimum.clone(),
		maximum: maximum.clone(),
	}, nil
}

func (occurrences particleOccurrenceRange) minimumOccurrence() particleOccurrence {
	return occurrences.minimum.clone()
}

func (occurrences particleOccurrenceRange) maximumOccurrence() particleOccurrence {
	return occurrences.maximum.clone()
}

func (occurrences particleOccurrenceRange) mapsToParticle() bool {
	return !occurrences.minimum.Equal(particleOccurrence{}) ||
		!occurrences.maximum.Equal(particleOccurrence{})
}

func (occurrences particleOccurrenceRange) Equal(other particleOccurrenceRange) bool {
	return occurrences.minimum.Equal(other.minimum) && occurrences.maximum.Equal(other.maximum)
}

func (occurrences particleOccurrenceRange) String() string {
	return occurrences.minimum.String() + "/" + occurrences.maximum.String()
}
