package goxsd9

import (
	"errors"
	"fmt"
)

const (
	// InvalidBoundCode identifies an invalid ordered bound declaration.
	InvalidBoundCode = "XSD2028"
	// InvalidBoundRestrictionCode identifies an invalid derived ordered bound.
	InvalidBoundRestrictionCode = "XSD2029"
	// InvalidBoundCombinationCode identifies contradictory ordered bounds.
	InvalidBoundCombinationCode = "XSD2030"
	// BoundValueViolationCode identifies a value outside ordered bounds.
	BoundValueViolationCode = "XSD2031"
)

const (
	diagnosticBoundEffectiveVersionCode = "GOXSD9041"
)

var (
	errInvalidBoundValue       = errors.New("invalid ordered bound value")
	errInvalidBoundRestriction = errors.New("invalid ordered bound restriction")
	errInvalidBoundCombination = errors.New("invalid ordered bound combination")
	errBoundValueViolation     = errors.New("ordered bound value violation")
	errInvalidBoundState       = errors.New("invalid completed ordered bound state")
	errInvalidBoundVersion     = errors.New("incompatible XSD version policy for ordered bounds")
)

type boundRule uint8

const (
	boundDefinitionRule boundRule = iota + 1
	boundValueRule
	boundRestrictionRule
	boundFixedRule
)

// BoundKind identifies one of the four ordered XML Schema bound facets.
type BoundKind uint8

const (
	// BoundMinInclusive identifies an inclusive lower bound.
	BoundMinInclusive BoundKind = iota + 1
	// BoundMinExclusive identifies an exclusive lower bound.
	BoundMinExclusive
	// BoundMaxInclusive identifies an inclusive upper bound.
	BoundMaxInclusive
	// BoundMaxExclusive identifies an exclusive upper bound.
	BoundMaxExclusive
)

// MinInclusiveBound is a short alternate name for BoundMinInclusive.
const MinInclusiveBound = BoundMinInclusive

// MinExclusiveBound is a short alternate name for BoundMinExclusive.
const MinExclusiveBound = BoundMinExclusive

// MaxInclusiveBound is a short alternate name for BoundMaxInclusive.
const MaxInclusiveBound = BoundMaxInclusive

// MaxExclusiveBound is a short alternate name for BoundMaxExclusive.
const MaxExclusiveBound = BoundMaxExclusive

// String returns the XML Schema name of the bound kind.
func (kind BoundKind) String() string {
	switch kind {
	case BoundMinInclusive:
		return "minInclusive"
	case BoundMinExclusive:
		return "minExclusive"
	case BoundMaxInclusive:
		return "maxInclusive"
	case BoundMaxExclusive:
		return "maxExclusive"
	default:
		return ""
	}
}

// IsLower reports whether the bound is a lower bound.
func (kind BoundKind) IsLower() bool {
	return kind == BoundMinInclusive || kind == BoundMinExclusive
}

// IsUpper reports whether the bound is an upper bound.
func (kind BoundKind) IsUpper() bool {
	return kind == BoundMaxInclusive || kind == BoundMaxExclusive
}

// Inclusive reports whether the bound includes its endpoint.
func (kind BoundKind) Inclusive() bool {
	return kind == BoundMinInclusive || kind == BoundMaxInclusive
}

func (kind BoundKind) valid() bool {
	return kind >= BoundMinInclusive && kind <= BoundMaxExclusive
}

// IntegerBoundFacet is one immutable exact integer ordered-bound declaration.
// Its kind identifies the XML Schema facet represented by the declaration.
type IntegerBoundFacet struct {
	kind    BoundKind
	value   StrictInteger
	loc     Loc
	fixed   bool
	version XSDVersion
}

// IntegerMinInclusiveFacet is an alternate name for an integer bound facet.
type IntegerMinInclusiveFacet = IntegerBoundFacet

// IntegerMinExclusiveFacet is an alternate name for an integer bound facet.
type IntegerMinExclusiveFacet = IntegerBoundFacet

// IntegerMaxInclusiveFacet is an alternate name for an integer bound facet.
type IntegerMaxInclusiveFacet = IntegerBoundFacet

// IntegerMaxExclusiveFacet is an alternate name for an integer bound facet.
type IntegerMaxExclusiveFacet = IntegerBoundFacet

// Kind reports which ordered bound facet was declared.
func (facet IntegerBoundFacet) Kind() BoundKind {
	return facet.kind
}

// Value returns the owned exact integer endpoint.
func (facet IntegerBoundFacet) Value() StrictInteger {
	return cloneStrictInteger(facet.value)
}

// Loc returns the source location of the bound declaration.
func (facet IntegerBoundFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the bound is fixed for derived restrictions.
func (facet IntegerBoundFacet) Fixed() bool {
	return facet.fixed
}

// Version reports the XSD version policy used for the declaration.
func (facet IntegerBoundFacet) Version() XSDVersion {
	return facet.version
}

// WithFixed returns an owned copy with the requested fixed property.
func (facet IntegerBoundFacet) WithFixed(fixed bool) IntegerBoundFacet {
	facet.value = cloneStrictInteger(facet.value)
	facet.fixed = fixed
	return facet
}

// DecimalBoundFacet is one immutable exact decimal ordered-bound declaration.
// Its kind identifies the XML Schema facet represented by the declaration.
type DecimalBoundFacet struct {
	kind    BoundKind
	value   StrictDecimal
	loc     Loc
	fixed   bool
	version XSDVersion
}

// DecimalMinInclusiveFacet is an alternate name for a decimal bound facet.
type DecimalMinInclusiveFacet = DecimalBoundFacet

// DecimalMinExclusiveFacet is an alternate name for a decimal bound facet.
type DecimalMinExclusiveFacet = DecimalBoundFacet

// DecimalMaxInclusiveFacet is an alternate name for a decimal bound facet.
type DecimalMaxInclusiveFacet = DecimalBoundFacet

// DecimalMaxExclusiveFacet is an alternate name for a decimal bound facet.
type DecimalMaxExclusiveFacet = DecimalBoundFacet

// Kind reports which ordered bound facet was declared.
func (facet DecimalBoundFacet) Kind() BoundKind {
	return facet.kind
}

// Value returns the owned exact decimal endpoint.
func (facet DecimalBoundFacet) Value() StrictDecimal {
	return cloneStrictDecimal(facet.value)
}

// Loc returns the source location of the bound declaration.
func (facet DecimalBoundFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the bound is fixed for derived restrictions.
func (facet DecimalBoundFacet) Fixed() bool {
	return facet.fixed
}

// Version reports the XSD version policy used for the declaration.
func (facet DecimalBoundFacet) Version() XSDVersion {
	return facet.version
}

// WithFixed returns an owned copy with the requested fixed property.
func (facet DecimalBoundFacet) WithFixed(fixed bool) DecimalBoundFacet {
	facet.value = cloneStrictDecimal(facet.value)
	facet.fixed = fixed
	return facet
}

// ParseIntegerBoundFacet parses one exact integer ordered-bound declaration.
func ParseIntegerBoundFacet(kind BoundKind, lexical string, loc Loc, versions ...XSDVersion) (IntegerBoundFacet, error) {
	version, err := selectBoundVersion(versions)
	if err != nil {
		return IntegerBoundFacet{}, invalidBoundVersionDiagnostic(loc, err)
	}
	if !kind.valid() {
		return IntegerBoundFacet{}, invalidBoundKindDiagnostic(kind, loc, version)
	}
	value, err := ParseStrictInteger(lexical, loc)
	if err != nil {
		return IntegerBoundFacet{}, invalidBoundLexicalDiagnostic(kind, loc, version, err)
	}
	return newIntegerBoundFacet(kind, value, loc, false, version)
}

// ParseIntegerBoundFacetFor parses an integer bound with an explicit version.
func ParseIntegerBoundFacetFor(version XSDVersion, kind BoundKind, lexical string, loc Loc) (IntegerBoundFacet, error) {
	return ParseIntegerBoundFacet(kind, lexical, loc, version)
}

// ParseIntegerBoundFacetWithFixed parses an integer bound including fixed.
func ParseIntegerBoundFacetWithFixed(kind BoundKind, lexical string, loc Loc, fixed bool, versions ...XSDVersion) (IntegerBoundFacet, error) {
	facet, err := ParseIntegerBoundFacet(kind, lexical, loc, versions...)
	if err != nil {
		return IntegerBoundFacet{}, err
	}
	facet.fixed = fixed
	return facet, nil
}

// ParseIntegerBoundFacetForWithFixed parses a fixed integer bound explicitly.
func ParseIntegerBoundFacetForWithFixed(version XSDVersion, kind BoundKind, lexical string, loc Loc, fixed bool) (IntegerBoundFacet, error) {
	return ParseIntegerBoundFacetWithFixed(kind, lexical, loc, fixed, version)
}

// NewIntegerBoundFacet constructs an integer bound from an exact value.
func NewIntegerBoundFacet(kind BoundKind, value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (IntegerBoundFacet, error) {
	version, err := selectBoundVersion(versions)
	if err != nil {
		return IntegerBoundFacet{}, invalidBoundVersionDiagnostic(loc, err)
	}
	return newIntegerBoundFacet(kind, value, loc, fixed, version)
}

// ParseDecimalBoundFacet parses one exact decimal ordered-bound declaration.
func ParseDecimalBoundFacet(kind BoundKind, lexical string, loc Loc, versions ...XSDVersion) (DecimalBoundFacet, error) {
	version, err := selectBoundVersion(versions)
	if err != nil {
		return DecimalBoundFacet{}, invalidBoundVersionDiagnostic(loc, err)
	}
	if !kind.valid() {
		return DecimalBoundFacet{}, invalidBoundKindDiagnostic(kind, loc, version)
	}
	value, err := ParseStrictDecimal(lexical, loc, version)
	if err != nil {
		return DecimalBoundFacet{}, invalidBoundLexicalDiagnostic(kind, loc, version, err)
	}
	return newDecimalBoundFacet(kind, value, loc, false, version)
}

// ParseDecimalBoundFacetFor parses a decimal bound with an explicit version.
func ParseDecimalBoundFacetFor(version XSDVersion, kind BoundKind, lexical string, loc Loc) (DecimalBoundFacet, error) {
	return ParseDecimalBoundFacet(kind, lexical, loc, version)
}

// ParseDecimalBoundFacetWithFixed parses a decimal bound including fixed.
func ParseDecimalBoundFacetWithFixed(kind BoundKind, lexical string, loc Loc, fixed bool, versions ...XSDVersion) (DecimalBoundFacet, error) {
	facet, err := ParseDecimalBoundFacet(kind, lexical, loc, versions...)
	if err != nil {
		return DecimalBoundFacet{}, err
	}
	facet.fixed = fixed
	return facet, nil
}

// ParseDecimalBoundFacetForWithFixed parses a fixed decimal bound explicitly.
func ParseDecimalBoundFacetForWithFixed(version XSDVersion, kind BoundKind, lexical string, loc Loc, fixed bool) (DecimalBoundFacet, error) {
	return ParseDecimalBoundFacetWithFixed(kind, lexical, loc, fixed, version)
}

// NewDecimalBoundFacet constructs a decimal bound from an exact value.
func NewDecimalBoundFacet(kind BoundKind, value StrictDecimal, loc Loc, fixed bool, versions ...XSDVersion) (DecimalBoundFacet, error) {
	version, err := selectBoundVersion(versions)
	if err != nil {
		return DecimalBoundFacet{}, invalidBoundVersionDiagnostic(loc, err)
	}
	return newDecimalBoundFacet(kind, value, loc, fixed, version)
}

// ParseIntegerMinInclusiveFacet parses an integer minInclusive declaration.
func ParseIntegerMinInclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (IntegerMinInclusiveFacet, error) {
	return ParseIntegerBoundFacet(BoundMinInclusive, lexical, loc, versions...)
}

// ParseIntegerMinInclusiveFacetFor parses minInclusive with an explicit version.
func ParseIntegerMinInclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (IntegerMinInclusiveFacet, error) {
	return ParseIntegerMinInclusiveFacet(lexical, loc, version)
}

// ParseIntegerMinInclusiveFacetWithFixed parses minInclusive including fixed.
func ParseIntegerMinInclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMinInclusiveFacet, error) {
	return ParseIntegerBoundFacetWithFixed(BoundMinInclusive, lexical, loc, fixed, versions...)
}

// ParseIntegerMinExclusiveFacet parses an integer minExclusive declaration.
func ParseIntegerMinExclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (IntegerMinExclusiveFacet, error) {
	return ParseIntegerBoundFacet(BoundMinExclusive, lexical, loc, versions...)
}

// ParseIntegerMinExclusiveFacetFor parses minExclusive with an explicit version.
func ParseIntegerMinExclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (IntegerMinExclusiveFacet, error) {
	return ParseIntegerMinExclusiveFacet(lexical, loc, version)
}

// ParseIntegerMinExclusiveFacetWithFixed parses minExclusive including fixed.
func ParseIntegerMinExclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMinExclusiveFacet, error) {
	return ParseIntegerBoundFacetWithFixed(BoundMinExclusive, lexical, loc, fixed, versions...)
}

// ParseIntegerMaxInclusiveFacet parses an integer maxInclusive declaration.
func ParseIntegerMaxInclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (IntegerMaxInclusiveFacet, error) {
	return ParseIntegerBoundFacet(BoundMaxInclusive, lexical, loc, versions...)
}

// ParseIntegerMaxInclusiveFacetFor parses maxInclusive with an explicit version.
func ParseIntegerMaxInclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (IntegerMaxInclusiveFacet, error) {
	return ParseIntegerMaxInclusiveFacet(lexical, loc, version)
}

// ParseIntegerMaxInclusiveFacetWithFixed parses maxInclusive including fixed.
func ParseIntegerMaxInclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMaxInclusiveFacet, error) {
	return ParseIntegerBoundFacetWithFixed(BoundMaxInclusive, lexical, loc, fixed, versions...)
}

// ParseIntegerMaxExclusiveFacet parses an integer maxExclusive declaration.
func ParseIntegerMaxExclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (IntegerMaxExclusiveFacet, error) {
	return ParseIntegerBoundFacet(BoundMaxExclusive, lexical, loc, versions...)
}

// ParseIntegerMaxExclusiveFacetFor parses maxExclusive with an explicit version.
func ParseIntegerMaxExclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (IntegerMaxExclusiveFacet, error) {
	return ParseIntegerMaxExclusiveFacet(lexical, loc, version)
}

// ParseIntegerMaxExclusiveFacetWithFixed parses maxExclusive including fixed.
func ParseIntegerMaxExclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMaxExclusiveFacet, error) {
	return ParseIntegerBoundFacetWithFixed(BoundMaxExclusive, lexical, loc, fixed, versions...)
}

// ParseDecimalMinInclusiveFacet parses a decimal minInclusive declaration.
func ParseDecimalMinInclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (DecimalMinInclusiveFacet, error) {
	return ParseDecimalBoundFacet(BoundMinInclusive, lexical, loc, versions...)
}

// ParseDecimalMinInclusiveFacetFor parses minInclusive with an explicit version.
func ParseDecimalMinInclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (DecimalMinInclusiveFacet, error) {
	return ParseDecimalMinInclusiveFacet(lexical, loc, version)
}

// ParseDecimalMinInclusiveFacetWithFixed parses minInclusive including fixed.
func ParseDecimalMinInclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMinInclusiveFacet, error) {
	return ParseDecimalBoundFacetWithFixed(BoundMinInclusive, lexical, loc, fixed, versions...)
}

// ParseDecimalMinExclusiveFacet parses a decimal minExclusive declaration.
func ParseDecimalMinExclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (DecimalMinExclusiveFacet, error) {
	return ParseDecimalBoundFacet(BoundMinExclusive, lexical, loc, versions...)
}

// ParseDecimalMinExclusiveFacetFor parses minExclusive with an explicit version.
func ParseDecimalMinExclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (DecimalMinExclusiveFacet, error) {
	return ParseDecimalMinExclusiveFacet(lexical, loc, version)
}

// ParseDecimalMinExclusiveFacetWithFixed parses minExclusive including fixed.
func ParseDecimalMinExclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMinExclusiveFacet, error) {
	return ParseDecimalBoundFacetWithFixed(BoundMinExclusive, lexical, loc, fixed, versions...)
}

// ParseDecimalMaxInclusiveFacet parses a decimal maxInclusive declaration.
func ParseDecimalMaxInclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (DecimalMaxInclusiveFacet, error) {
	return ParseDecimalBoundFacet(BoundMaxInclusive, lexical, loc, versions...)
}

// ParseDecimalMaxInclusiveFacetFor parses maxInclusive with an explicit version.
func ParseDecimalMaxInclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (DecimalMaxInclusiveFacet, error) {
	return ParseDecimalMaxInclusiveFacet(lexical, loc, version)
}

// ParseDecimalMaxInclusiveFacetWithFixed parses maxInclusive including fixed.
func ParseDecimalMaxInclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMaxInclusiveFacet, error) {
	return ParseDecimalBoundFacetWithFixed(BoundMaxInclusive, lexical, loc, fixed, versions...)
}

// ParseDecimalMaxExclusiveFacet parses a decimal maxExclusive declaration.
func ParseDecimalMaxExclusiveFacet(lexical string, loc Loc, versions ...XSDVersion) (DecimalMaxExclusiveFacet, error) {
	return ParseDecimalBoundFacet(BoundMaxExclusive, lexical, loc, versions...)
}

// ParseDecimalMaxExclusiveFacetFor parses maxExclusive with an explicit version.
func ParseDecimalMaxExclusiveFacetFor(version XSDVersion, lexical string, loc Loc) (DecimalMaxExclusiveFacet, error) {
	return ParseDecimalMaxExclusiveFacet(lexical, loc, version)
}

// ParseDecimalMaxExclusiveFacetWithFixed parses maxExclusive including fixed.
func ParseDecimalMaxExclusiveFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMaxExclusiveFacet, error) {
	return ParseDecimalBoundFacetWithFixed(BoundMaxExclusive, lexical, loc, fixed, versions...)
}

// NewIntegerMinInclusiveFacet constructs an integer minInclusive declaration.
func NewIntegerMinInclusiveFacet(value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMinInclusiveFacet, error) {
	return NewIntegerBoundFacet(BoundMinInclusive, value, loc, fixed, versions...)
}

// NewIntegerMinExclusiveFacet constructs an integer minExclusive declaration.
func NewIntegerMinExclusiveFacet(value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMinExclusiveFacet, error) {
	return NewIntegerBoundFacet(BoundMinExclusive, value, loc, fixed, versions...)
}

// NewIntegerMaxInclusiveFacet constructs an integer maxInclusive declaration.
func NewIntegerMaxInclusiveFacet(value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMaxInclusiveFacet, error) {
	return NewIntegerBoundFacet(BoundMaxInclusive, value, loc, fixed, versions...)
}

// NewIntegerMaxExclusiveFacet constructs an integer maxExclusive declaration.
func NewIntegerMaxExclusiveFacet(value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (IntegerMaxExclusiveFacet, error) {
	return NewIntegerBoundFacet(BoundMaxExclusive, value, loc, fixed, versions...)
}

// NewDecimalMinInclusiveFacet constructs a decimal minInclusive declaration.
func NewDecimalMinInclusiveFacet(value StrictDecimal, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMinInclusiveFacet, error) {
	return NewDecimalBoundFacet(BoundMinInclusive, value, loc, fixed, versions...)
}

// NewDecimalMinExclusiveFacet constructs a decimal minExclusive declaration.
func NewDecimalMinExclusiveFacet(value StrictDecimal, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMinExclusiveFacet, error) {
	return NewDecimalBoundFacet(BoundMinExclusive, value, loc, fixed, versions...)
}

// NewDecimalMaxInclusiveFacet constructs a decimal maxInclusive declaration.
func NewDecimalMaxInclusiveFacet(value StrictDecimal, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMaxInclusiveFacet, error) {
	return NewDecimalBoundFacet(BoundMaxInclusive, value, loc, fixed, versions...)
}

// NewDecimalMaxExclusiveFacet constructs a decimal maxExclusive declaration.
func NewDecimalMaxExclusiveFacet(value StrictDecimal, loc Loc, fixed bool, versions ...XSDVersion) (DecimalMaxExclusiveFacet, error) {
	return NewDecimalBoundFacet(BoundMaxExclusive, value, loc, fixed, versions...)
}

// IntegerBoundFacetDeclarations contains ordered local integer bound facets.
// The order is retained for diagnostics. An empty slice omits all bounds.
type IntegerBoundFacetDeclarations struct {
	Bounds []IntegerBoundFacet
}

// IntegerBoundDeclarations is an alternate name for integer declarations.
type IntegerBoundDeclarations = IntegerBoundFacetDeclarations

// NewIntegerBoundFacetDeclarations makes an owned ordered declaration copy.
func NewIntegerBoundFacetDeclarations(bounds []IntegerBoundFacet) IntegerBoundFacetDeclarations {
	return IntegerBoundFacetDeclarations{Bounds: cloneIntegerBoundFacets(bounds)}
}

// NewIntegerBoundDeclarations makes an owned ordered integer declaration copy.
func NewIntegerBoundDeclarations(bounds []IntegerBoundFacet) IntegerBoundDeclarations {
	return NewIntegerBoundFacetDeclarations(bounds)
}

// DecimalBoundFacetDeclarations contains ordered local decimal bound facets.
// The order is retained for diagnostics. An empty slice omits all bounds.
type DecimalBoundFacetDeclarations struct {
	Bounds []DecimalBoundFacet
}

// DecimalBoundDeclarations is an alternate name for decimal declarations.
type DecimalBoundDeclarations = DecimalBoundFacetDeclarations

// NewDecimalBoundFacetDeclarations makes an owned ordered declaration copy.
func NewDecimalBoundFacetDeclarations(bounds []DecimalBoundFacet) DecimalBoundFacetDeclarations {
	return DecimalBoundFacetDeclarations{Bounds: cloneDecimalBoundFacets(bounds)}
}

// NewDecimalBoundDeclarations makes an owned ordered decimal declaration copy.
func NewDecimalBoundDeclarations(bounds []DecimalBoundFacet) DecimalBoundDeclarations {
	return NewDecimalBoundFacetDeclarations(bounds)
}

type integerBoundEndpoint struct {
	value     StrictInteger
	loc       Loc
	fixed     bool
	inclusive bool
}

// IntegerBoundFacets is an immutable effective integer ordered-bound set.
type IntegerBoundFacets struct {
	version XSDVersion
	lower   *integerBoundEndpoint
	upper   *integerBoundEndpoint
}

// IntegerBounds is an alternate name for IntegerBoundFacets.
type IntegerBounds = IntegerBoundFacets

type decimalBoundEndpoint struct {
	value     StrictDecimal
	loc       Loc
	fixed     bool
	inclusive bool
}

// DecimalBoundFacets is an immutable effective decimal ordered-bound set.
type DecimalBoundFacets struct {
	version XSDVersion
	lower   *decimalBoundEndpoint
	upper   *decimalBoundEndpoint
}

// DecimalBounds is an alternate name for DecimalBoundFacets.
type DecimalBounds = DecimalBoundFacets

// NewIntegerBoundFacets constructs effective integer bounds from local facets.
func NewIntegerBoundFacets(bounds []IntegerBoundFacet, versions ...XSDVersion) (IntegerBoundFacets, error) {
	local := NewIntegerBoundFacetDeclarations(bounds)
	version, err := resolveIntegerBoundVersion(versions, local.Bounds)
	if err != nil {
		return IntegerBoundFacets{}, invalidBoundVersionDiagnostic(integerBoundVersionLoc(local.Bounds), err)
	}
	return completeIntegerBoundFacets(IntegerBoundFacets{version: version}, local, false)
}

// NewIntegerBoundFacetsFromDeclarations constructs effective integer bounds.
func NewIntegerBoundFacetsFromDeclarations(local IntegerBoundFacetDeclarations, versions ...XSDVersion) (IntegerBoundFacets, error) {
	return NewIntegerBoundFacets(local.Bounds, versions...)
}

// RestrictIntegerBoundFacets inherits omitted bounds and narrows local bounds.
func RestrictIntegerBoundFacets(base IntegerBoundFacets, local IntegerBoundFacetDeclarations) (IntegerBoundFacets, error) {
	if err := base.validate(); err != nil {
		return IntegerBoundFacets{}, err
	}
	return completeIntegerBoundFacets(base, NewIntegerBoundFacetDeclarations(local.Bounds), true)
}

// ConstructIntegerBoundFacets is the phase-oriented restriction constructor.
func ConstructIntegerBoundFacets(base IntegerBoundFacets, local IntegerBoundFacetDeclarations) (IntegerBoundFacets, error) {
	return RestrictIntegerBoundFacets(base, local)
}

// NewDecimalBoundFacets constructs effective decimal bounds from local facets.
func NewDecimalBoundFacets(bounds []DecimalBoundFacet, versions ...XSDVersion) (DecimalBoundFacets, error) {
	local := NewDecimalBoundFacetDeclarations(bounds)
	version, err := resolveDecimalBoundVersion(versions, local.Bounds)
	if err != nil {
		return DecimalBoundFacets{}, invalidBoundVersionDiagnostic(decimalBoundVersionLoc(local.Bounds), err)
	}
	return completeDecimalBoundFacets(DecimalBoundFacets{version: version}, local, false)
}

// NewDecimalBoundFacetsFromDeclarations constructs effective decimal bounds.
func NewDecimalBoundFacetsFromDeclarations(local DecimalBoundFacetDeclarations, versions ...XSDVersion) (DecimalBoundFacets, error) {
	return NewDecimalBoundFacets(local.Bounds, versions...)
}

// RestrictDecimalBoundFacets inherits omitted bounds and narrows local bounds.
func RestrictDecimalBoundFacets(base DecimalBoundFacets, local DecimalBoundFacetDeclarations) (DecimalBoundFacets, error) {
	if err := base.validate(); err != nil {
		return DecimalBoundFacets{}, err
	}
	return completeDecimalBoundFacets(base, NewDecimalBoundFacetDeclarations(local.Bounds), true)
}

// ConstructDecimalBoundFacets is the phase-oriented restriction constructor.
func ConstructDecimalBoundFacets(base DecimalBoundFacets, local DecimalBoundFacetDeclarations) (DecimalBoundFacets, error) {
	return RestrictDecimalBoundFacets(base, local)
}

// Version reports the XSD version attached to the effective integer bounds.
func (facets IntegerBoundFacets) Version() XSDVersion {
	return facets.version
}

// Version reports the XSD version attached to the effective decimal bounds.
func (facets DecimalBoundFacets) Version() XSDVersion {
	return facets.version
}

// HasMinInclusive reports whether an inclusive lower bound is effective.
func (facets IntegerBoundFacets) HasMinInclusive() bool {
	return facets.lower != nil && facets.lower.inclusive
}

// HasMinExclusive reports whether an exclusive lower bound is effective.
func (facets IntegerBoundFacets) HasMinExclusive() bool {
	return facets.lower != nil && !facets.lower.inclusive
}

// HasMaxInclusive reports whether an inclusive upper bound is effective.
func (facets IntegerBoundFacets) HasMaxInclusive() bool {
	return facets.upper != nil && facets.upper.inclusive
}

// HasMaxExclusive reports whether an exclusive upper bound is effective.
func (facets IntegerBoundFacets) HasMaxExclusive() bool {
	return facets.upper != nil && !facets.upper.inclusive
}

// MinInclusiveFacet returns the effective inclusive integer lower bound.
func (facets IntegerBoundFacets) MinInclusiveFacet() (IntegerMinInclusiveFacet, bool) {
	if !facets.HasMinInclusive() {
		return IntegerMinInclusiveFacet{}, false
	}
	return integerBoundFacetFromEndpoint(facets.lower, BoundMinInclusive, facets.version), true
}

// MinExclusiveFacet returns the effective exclusive integer lower bound.
func (facets IntegerBoundFacets) MinExclusiveFacet() (IntegerMinExclusiveFacet, bool) {
	if !facets.HasMinExclusive() {
		return IntegerMinExclusiveFacet{}, false
	}
	return integerBoundFacetFromEndpoint(facets.lower, BoundMinExclusive, facets.version), true
}

// MaxInclusiveFacet returns the effective inclusive integer upper bound.
func (facets IntegerBoundFacets) MaxInclusiveFacet() (IntegerMaxInclusiveFacet, bool) {
	if !facets.HasMaxInclusive() {
		return IntegerMaxInclusiveFacet{}, false
	}
	return integerBoundFacetFromEndpoint(facets.upper, BoundMaxInclusive, facets.version), true
}

// MaxExclusiveFacet returns the effective exclusive integer upper bound.
func (facets IntegerBoundFacets) MaxExclusiveFacet() (IntegerMaxExclusiveFacet, bool) {
	if !facets.HasMaxExclusive() {
		return IntegerMaxExclusiveFacet{}, false
	}
	return integerBoundFacetFromEndpoint(facets.upper, BoundMaxExclusive, facets.version), true
}

// MinInclusive returns the effective inclusive lower endpoint and presence.
func (facets IntegerBoundFacets) MinInclusive() (StrictInteger, bool) {
	if !facets.HasMinInclusive() {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(facets.lower.value), true
}

// MinExclusive returns the effective exclusive lower endpoint and presence.
func (facets IntegerBoundFacets) MinExclusive() (StrictInteger, bool) {
	if !facets.HasMinExclusive() {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(facets.lower.value), true
}

// MaxInclusive returns the effective inclusive upper endpoint and presence.
func (facets IntegerBoundFacets) MaxInclusive() (StrictInteger, bool) {
	if !facets.HasMaxInclusive() {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(facets.upper.value), true
}

// MaxExclusive returns the effective exclusive upper endpoint and presence.
func (facets IntegerBoundFacets) MaxExclusive() (StrictInteger, bool) {
	if !facets.HasMaxExclusive() {
		return StrictInteger{}, false
	}
	return cloneStrictInteger(facets.upper.value), true
}

// MinInclusiveLoc returns the source location of the effective lower bound.
func (facets IntegerBoundFacets) MinInclusiveLoc() (Loc, bool) {
	if !facets.HasMinInclusive() {
		return Loc{}, false
	}
	return facets.lower.loc, true
}

// MinExclusiveLoc returns the source location of the effective lower bound.
func (facets IntegerBoundFacets) MinExclusiveLoc() (Loc, bool) {
	if !facets.HasMinExclusive() {
		return Loc{}, false
	}
	return facets.lower.loc, true
}

// MaxInclusiveLoc returns the source location of the effective upper bound.
func (facets IntegerBoundFacets) MaxInclusiveLoc() (Loc, bool) {
	if !facets.HasMaxInclusive() {
		return Loc{}, false
	}
	return facets.upper.loc, true
}

// MaxExclusiveLoc returns the source location of the effective upper bound.
func (facets IntegerBoundFacets) MaxExclusiveLoc() (Loc, bool) {
	if !facets.HasMaxExclusive() {
		return Loc{}, false
	}
	return facets.upper.loc, true
}

// MinInclusiveFixed returns the fixed state of the effective lower bound.
func (facets IntegerBoundFacets) MinInclusiveFixed() (bool, bool) {
	if !facets.HasMinInclusive() {
		return false, false
	}
	return facets.lower.fixed, true
}

// MinExclusiveFixed returns the fixed state of the effective lower bound.
func (facets IntegerBoundFacets) MinExclusiveFixed() (bool, bool) {
	if !facets.HasMinExclusive() {
		return false, false
	}
	return facets.lower.fixed, true
}

// MaxInclusiveFixed returns the fixed state of the effective upper bound.
func (facets IntegerBoundFacets) MaxInclusiveFixed() (bool, bool) {
	if !facets.HasMaxInclusive() {
		return false, false
	}
	return facets.upper.fixed, true
}

// MaxExclusiveFixed returns the fixed state of the effective upper bound.
func (facets IntegerBoundFacets) MaxExclusiveFixed() (bool, bool) {
	if !facets.HasMaxExclusive() {
		return false, false
	}
	return facets.upper.fixed, true
}

// Bounds returns owned effective integer declarations in lower-then-upper order.
func (facets IntegerBoundFacets) Bounds() []IntegerBoundFacet {
	if facets.lower == nil && facets.upper == nil {
		return nil
	}
	bounds := make([]IntegerBoundFacet, 0, 2)
	if facets.lower != nil {
		bounds = append(bounds, integerBoundFacetFromEndpoint(facets.lower, integerLowerBoundKind(facets.lower), facets.version))
	}
	if facets.upper != nil {
		bounds = append(bounds, integerBoundFacetFromEndpoint(facets.upper, integerUpperBoundKind(facets.upper), facets.version))
	}
	return bounds
}

// Declarations returns owned effective integer declarations.
func (facets IntegerBoundFacets) Declarations() []IntegerBoundFacet {
	return facets.Bounds()
}

// ValidateInteger validates an exact integer against effective bounds.
func (facets IntegerBoundFacets) ValidateInteger(value StrictInteger, valueLoc Loc) error {
	if err := facets.validate(); err != nil {
		return err
	}
	if facets.lower != nil {
		comparison := value.Compare(facets.lower.value)
		if comparison < 0 || (comparison == 0 && !facets.lower.inclusive) {
			return boundValueViolationIntegerDiagnostic(valueLoc, facets.version, integerLowerBoundKind(facets.lower), facets.lower.loc, "integer")
		}
	}
	if facets.upper != nil {
		comparison := value.Compare(facets.upper.value)
		if comparison > 0 || (comparison == 0 && !facets.upper.inclusive) {
			return boundValueViolationIntegerDiagnostic(valueLoc, facets.version, integerUpperBoundKind(facets.upper), facets.upper.loc, "integer")
		}
	}
	return nil
}

// ValidateIntegerBounds validates an exact integer against ordered bounds.
func ValidateIntegerBounds(value StrictInteger, facets IntegerBoundFacets, valueLoc Loc) error {
	return facets.ValidateInteger(value, valueLoc)
}

// ValidateIntegerBoundFacets validates an exact integer against ordered bounds.
func ValidateIntegerBoundFacets(value StrictInteger, facets IntegerBoundFacets, valueLoc Loc) error {
	return facets.ValidateInteger(value, valueLoc)
}

// HasMinInclusive reports whether an inclusive lower bound is effective.
func (facets DecimalBoundFacets) HasMinInclusive() bool {
	return facets.lower != nil && facets.lower.inclusive
}

// HasMinExclusive reports whether an exclusive lower bound is effective.
func (facets DecimalBoundFacets) HasMinExclusive() bool {
	return facets.lower != nil && !facets.lower.inclusive
}

// HasMaxInclusive reports whether an inclusive upper bound is effective.
func (facets DecimalBoundFacets) HasMaxInclusive() bool {
	return facets.upper != nil && facets.upper.inclusive
}

// HasMaxExclusive reports whether an exclusive upper bound is effective.
func (facets DecimalBoundFacets) HasMaxExclusive() bool {
	return facets.upper != nil && !facets.upper.inclusive
}

// MinInclusiveFacet returns the effective inclusive decimal lower bound.
func (facets DecimalBoundFacets) MinInclusiveFacet() (DecimalMinInclusiveFacet, bool) {
	if !facets.HasMinInclusive() {
		return DecimalMinInclusiveFacet{}, false
	}
	return decimalBoundFacetFromEndpoint(facets.lower, BoundMinInclusive, facets.version), true
}

// MinExclusiveFacet returns the effective exclusive decimal lower bound.
func (facets DecimalBoundFacets) MinExclusiveFacet() (DecimalMinExclusiveFacet, bool) {
	if !facets.HasMinExclusive() {
		return DecimalMinExclusiveFacet{}, false
	}
	return decimalBoundFacetFromEndpoint(facets.lower, BoundMinExclusive, facets.version), true
}

// MaxInclusiveFacet returns the effective inclusive decimal upper bound.
func (facets DecimalBoundFacets) MaxInclusiveFacet() (DecimalMaxInclusiveFacet, bool) {
	if !facets.HasMaxInclusive() {
		return DecimalMaxInclusiveFacet{}, false
	}
	return decimalBoundFacetFromEndpoint(facets.upper, BoundMaxInclusive, facets.version), true
}

// MaxExclusiveFacet returns the effective exclusive decimal upper bound.
func (facets DecimalBoundFacets) MaxExclusiveFacet() (DecimalMaxExclusiveFacet, bool) {
	if !facets.HasMaxExclusive() {
		return DecimalMaxExclusiveFacet{}, false
	}
	return decimalBoundFacetFromEndpoint(facets.upper, BoundMaxExclusive, facets.version), true
}

// MinInclusive returns the effective inclusive lower endpoint and presence.
func (facets DecimalBoundFacets) MinInclusive() (StrictDecimal, bool) {
	if !facets.HasMinInclusive() {
		return StrictDecimal{}, false
	}
	return cloneStrictDecimal(facets.lower.value), true
}

// MinExclusive returns the effective exclusive lower endpoint and presence.
func (facets DecimalBoundFacets) MinExclusive() (StrictDecimal, bool) {
	if !facets.HasMinExclusive() {
		return StrictDecimal{}, false
	}
	return cloneStrictDecimal(facets.lower.value), true
}

// MaxInclusive returns the effective inclusive upper endpoint and presence.
func (facets DecimalBoundFacets) MaxInclusive() (StrictDecimal, bool) {
	if !facets.HasMaxInclusive() {
		return StrictDecimal{}, false
	}
	return cloneStrictDecimal(facets.upper.value), true
}

// MaxExclusive returns the effective exclusive upper endpoint and presence.
func (facets DecimalBoundFacets) MaxExclusive() (StrictDecimal, bool) {
	if !facets.HasMaxExclusive() {
		return StrictDecimal{}, false
	}
	return cloneStrictDecimal(facets.upper.value), true
}

// MinInclusiveLoc returns the source location of the effective lower bound.
func (facets DecimalBoundFacets) MinInclusiveLoc() (Loc, bool) {
	if !facets.HasMinInclusive() {
		return Loc{}, false
	}
	return facets.lower.loc, true
}

// MinExclusiveLoc returns the source location of the effective lower bound.
func (facets DecimalBoundFacets) MinExclusiveLoc() (Loc, bool) {
	if !facets.HasMinExclusive() {
		return Loc{}, false
	}
	return facets.lower.loc, true
}

// MaxInclusiveLoc returns the source location of the effective upper bound.
func (facets DecimalBoundFacets) MaxInclusiveLoc() (Loc, bool) {
	if !facets.HasMaxInclusive() {
		return Loc{}, false
	}
	return facets.upper.loc, true
}

// MaxExclusiveLoc returns the source location of the effective upper bound.
func (facets DecimalBoundFacets) MaxExclusiveLoc() (Loc, bool) {
	if !facets.HasMaxExclusive() {
		return Loc{}, false
	}
	return facets.upper.loc, true
}

// MinInclusiveFixed returns the fixed state of the effective lower bound.
func (facets DecimalBoundFacets) MinInclusiveFixed() (bool, bool) {
	if !facets.HasMinInclusive() {
		return false, false
	}
	return facets.lower.fixed, true
}

// MinExclusiveFixed returns the fixed state of the effective lower bound.
func (facets DecimalBoundFacets) MinExclusiveFixed() (bool, bool) {
	if !facets.HasMinExclusive() {
		return false, false
	}
	return facets.lower.fixed, true
}

// MaxInclusiveFixed returns the fixed state of the effective upper bound.
func (facets DecimalBoundFacets) MaxInclusiveFixed() (bool, bool) {
	if !facets.HasMaxInclusive() {
		return false, false
	}
	return facets.upper.fixed, true
}

// MaxExclusiveFixed returns the fixed state of the effective upper bound.
func (facets DecimalBoundFacets) MaxExclusiveFixed() (bool, bool) {
	if !facets.HasMaxExclusive() {
		return false, false
	}
	return facets.upper.fixed, true
}

// Bounds returns owned effective decimal declarations in lower-then-upper order.
func (facets DecimalBoundFacets) Bounds() []DecimalBoundFacet {
	if facets.lower == nil && facets.upper == nil {
		return nil
	}
	bounds := make([]DecimalBoundFacet, 0, 2)
	if facets.lower != nil {
		bounds = append(bounds, decimalBoundFacetFromEndpoint(facets.lower, decimalLowerBoundKind(facets.lower), facets.version))
	}
	if facets.upper != nil {
		bounds = append(bounds, decimalBoundFacetFromEndpoint(facets.upper, decimalUpperBoundKind(facets.upper), facets.version))
	}
	return bounds
}

// Declarations returns owned effective decimal declarations.
func (facets DecimalBoundFacets) Declarations() []DecimalBoundFacet {
	return facets.Bounds()
}

// ValidateDecimal validates an exact decimal against effective bounds.
func (facets DecimalBoundFacets) ValidateDecimal(value StrictDecimal, valueLoc Loc) error {
	if err := facets.validate(); err != nil {
		return err
	}
	if facets.lower != nil {
		comparison := value.Compare(facets.lower.value)
		if comparison < 0 || (comparison == 0 && !facets.lower.inclusive) {
			return boundValueViolationDecimalDiagnostic(valueLoc, facets.version, decimalLowerBoundKind(facets.lower), facets.lower.loc, "decimal")
		}
	}
	if facets.upper != nil {
		comparison := value.Compare(facets.upper.value)
		if comparison > 0 || (comparison == 0 && !facets.upper.inclusive) {
			return boundValueViolationDecimalDiagnostic(valueLoc, facets.version, decimalUpperBoundKind(facets.upper), facets.upper.loc, "decimal")
		}
	}
	return nil
}

// ValidateDecimalBounds validates an exact decimal against ordered bounds.
func ValidateDecimalBounds(value StrictDecimal, facets DecimalBoundFacets, valueLoc Loc) error {
	return facets.ValidateDecimal(value, valueLoc)
}

// ValidateDecimalBoundFacets validates an exact decimal against ordered bounds.
func ValidateDecimalBoundFacets(value StrictDecimal, facets DecimalBoundFacets, valueLoc Loc) error {
	return facets.ValidateDecimal(value, valueLoc)
}

func newIntegerBoundFacet(kind BoundKind, value StrictInteger, loc Loc, fixed bool, version XSDVersion) (IntegerBoundFacet, error) {
	if !kind.valid() {
		return IntegerBoundFacet{}, invalidBoundKindDiagnostic(kind, loc, version)
	}
	return IntegerBoundFacet{kind: kind, value: cloneStrictInteger(value), loc: loc, fixed: fixed, version: version}, nil
}

func newDecimalBoundFacet(kind BoundKind, value StrictDecimal, loc Loc, fixed bool, version XSDVersion) (DecimalBoundFacet, error) {
	if !kind.valid() {
		return DecimalBoundFacet{}, invalidBoundKindDiagnostic(kind, loc, version)
	}
	return DecimalBoundFacet{kind: kind, value: cloneStrictDecimal(value), loc: loc, fixed: fixed, version: version}, nil
}

func cloneIntegerBoundFacet(facet IntegerBoundFacet) IntegerBoundFacet {
	facet.value = cloneStrictInteger(facet.value)
	return facet
}

func cloneIntegerBoundFacets(bounds []IntegerBoundFacet) []IntegerBoundFacet {
	if bounds == nil {
		return nil
	}
	owned := make([]IntegerBoundFacet, len(bounds))
	for index := range bounds {
		owned[index] = cloneIntegerBoundFacet(bounds[index])
	}
	return owned
}

func cloneDecimalBoundFacet(facet DecimalBoundFacet) DecimalBoundFacet {
	facet.value = cloneStrictDecimal(facet.value)
	return facet
}

func cloneDecimalBoundFacets(bounds []DecimalBoundFacet) []DecimalBoundFacet {
	if bounds == nil {
		return nil
	}
	owned := make([]DecimalBoundFacet, len(bounds))
	for index := range bounds {
		owned[index] = cloneDecimalBoundFacet(bounds[index])
	}
	return owned
}

func cloneIntegerBoundEndpoint(endpoint *integerBoundEndpoint) *integerBoundEndpoint {
	if endpoint == nil {
		return nil
	}
	owned := *endpoint
	owned.value = cloneStrictInteger(endpoint.value)
	return &owned
}

func cloneDecimalBoundEndpoint(endpoint *decimalBoundEndpoint) *decimalBoundEndpoint {
	if endpoint == nil {
		return nil
	}
	owned := *endpoint
	owned.value = cloneStrictDecimal(endpoint.value)
	return &owned
}

func selectBoundVersion(versions []XSDVersion) (XSDVersion, error) {
	return selectXSDVersion(versions)
}

func resolveIntegerBoundVersion(versions []XSDVersion, bounds []IntegerBoundFacet) (XSDVersion, error) {
	if len(versions) != 0 {
		return selectBoundVersion(versions)
	}
	return inferBoundVersion(bounds, func(index int) XSDVersion { return bounds[index].Version() })
}

func resolveDecimalBoundVersion(versions []XSDVersion, bounds []DecimalBoundFacet) (XSDVersion, error) {
	if len(versions) != 0 {
		return selectBoundVersion(versions)
	}
	return inferBoundVersion(bounds, func(index int) XSDVersion { return bounds[index].Version() })
}

func inferBoundVersion[T any](bounds []T, versionAt func(int) XSDVersion) (XSDVersion, error) {
	if len(bounds) == 0 {
		return XSDVersion11, nil
	}
	version := versionAt(0)
	if version != XSDVersion10 && version != XSDVersion11 {
		return "", fmt.Errorf("%w: %q", errInvalidBoundVersion, version)
	}
	for index := 1; index < len(bounds); index++ {
		if versionAt(index) == version {
			continue
		}
		return "", fmt.Errorf("%w: declarations use %q and %q", errInvalidBoundVersion, version, versionAt(index))
	}
	return version, nil
}

func integerBoundVersionLoc(bounds []IntegerBoundFacet) Loc {
	if len(bounds) == 0 {
		return Loc{}
	}
	return bounds[0].Loc()
}

func decimalBoundVersionLoc(bounds []DecimalBoundFacet) Loc {
	if len(bounds) == 0 {
		return Loc{}
	}
	return bounds[0].Loc()
}

func completeIntegerBoundFacets(base IntegerBoundFacets, local IntegerBoundFacetDeclarations, derived bool) (IntegerBoundFacets, error) {
	if err := base.validateForConstruction(); err != nil {
		return IntegerBoundFacets{}, err
	}
	if err := validateIntegerBoundDeclarations(local, base.version); err != nil {
		return IntegerBoundFacets{}, err
	}
	if derived {
		if err := validateIntegerLocalBoundRestrictions(base, local.Bounds); err != nil {
			return IntegerBoundFacets{}, err
		}
	}
	effective := IntegerBoundFacets{
		version: base.version,
		lower:   cloneIntegerBoundEndpoint(base.lower),
		upper:   cloneIntegerBoundEndpoint(base.upper),
	}
	for index := range local.Bounds {
		applyIntegerBound(&effective, local.Bounds[index])
	}
	if err := effective.validate(); err != nil {
		return IntegerBoundFacets{}, err
	}
	return effective, nil
}

func completeDecimalBoundFacets(base DecimalBoundFacets, local DecimalBoundFacetDeclarations, derived bool) (DecimalBoundFacets, error) {
	if err := base.validateForConstruction(); err != nil {
		return DecimalBoundFacets{}, err
	}
	if err := validateDecimalBoundDeclarations(local, base.version); err != nil {
		return DecimalBoundFacets{}, err
	}
	if derived {
		if err := validateDecimalLocalBoundRestrictions(base, local.Bounds); err != nil {
			return DecimalBoundFacets{}, err
		}
	}
	effective := DecimalBoundFacets{
		version: base.version,
		lower:   cloneDecimalBoundEndpoint(base.lower),
		upper:   cloneDecimalBoundEndpoint(base.upper),
	}
	for index := range local.Bounds {
		applyDecimalBound(&effective, local.Bounds[index])
	}
	if err := effective.validate(); err != nil {
		return DecimalBoundFacets{}, err
	}
	return effective, nil
}

func validateIntegerBoundDeclarations(local IntegerBoundFacetDeclarations, version XSDVersion) error {
	return validateBoundDeclarations(local.Bounds, version)
}

func validateDecimalBoundDeclarations(local DecimalBoundFacetDeclarations, version XSDVersion) error {
	return validateBoundDeclarations(local.Bounds, version)
}

type boundDeclaration interface {
	Kind() BoundKind
	Loc() Loc
	Version() XSDVersion
}

func validateBoundDeclarations[T boundDeclaration](bounds []T, version XSDVersion) error {
	seen := make(map[BoundKind]Loc, len(bounds))
	for index := range bounds {
		bound := bounds[index]
		if bound.Version() != version {
			return invalidBoundDeclarationVersionDiagnostic(bound, version)
		}
		if !bound.Kind().valid() {
			return invalidBoundKindDiagnostic(bound.Kind(), bound.Loc(), version)
		}
		if previous, exists := seen[bound.Kind()]; exists {
			return invalidBoundCombinationDiagnostic(bound.Loc(), facetLocations(previous), version, bound.Kind(), bound.Kind(), "ordered bound facet is declared more than once in one derivation step")
		}
		if opposite, exists := seen[oppositeBoundKind(bound.Kind())]; exists {
			return invalidBoundCombinationDiagnostic(bound.Loc(), facetLocations(opposite), version, oppositeBoundKind(bound.Kind()), bound.Kind(), "same-side ordered bound facets cannot be declared together")
		}
		seen[bound.Kind()] = bound.Loc()
	}
	return nil
}

func validateIntegerLocalBoundRestrictions(base IntegerBoundFacets, local []IntegerBoundFacet) error {
	return validateLocalBoundRestrictions(
		base.version,
		local,
		func(kind BoundKind) *integerBoundEndpoint { return integerBaseEndpoint(base, kind) },
		integerBoundMembershipException,
		func(bound IntegerBoundFacet) bool { return integerValueInBase(base, bound.Value()) },
		validateIntegerFixed,
		validateIntegerMonotonicForVersion,
		integerBoundRelatedLocation(base),
		"integer",
		base.lower,
		func(endpoint *integerBoundEndpoint) bool { return endpoint.inclusive },
		integerLowerBoundKind,
		func(endpoint *integerBoundEndpoint) Loc { return endpoint.loc },
	)
}

func validateDecimalLocalBoundRestrictions(base DecimalBoundFacets, local []DecimalBoundFacet) error {
	return validateLocalBoundRestrictions(
		base.version,
		local,
		func(kind BoundKind) *decimalBoundEndpoint { return decimalBaseEndpoint(base, kind) },
		decimalBoundMembershipException,
		func(bound DecimalBoundFacet) bool { return decimalValueInBase(base, bound.Value()) },
		validateDecimalFixed,
		validateDecimalMonotonicForVersion,
		decimalBoundRelatedLocation(base),
		"decimal",
		base.lower,
		func(endpoint *decimalBoundEndpoint) bool { return endpoint.inclusive },
		decimalLowerBoundKind,
		func(endpoint *decimalBoundEndpoint) Loc { return endpoint.loc },
	)
}

//nolint:gocognit // The ordered restriction checks must remain in declaration order.
func validateLocalBoundRestrictions[T boundDeclaration, E any](
	version XSDVersion,
	local []T,
	baseEndpoint func(BoundKind) *E,
	membershipException func(*E, T) bool,
	valueInBase func(T) bool,
	validateFixed func(*E, T, XSDVersion) error,
	validateMonotonic func(*E, T, XSDVersion) error,
	baseRelated []Loc,
	datatype string,
	lower *E,
	endpointInclusive func(*E) bool,
	lowerKind func(*E) BoundKind,
	lowerLoc func(*E) Loc,
) error {
	for index := range local {
		bound := local[index]
		baseBound := baseEndpoint(bound.Kind())
		if baseBound != nil {
			if err := validateFixed(baseBound, bound, version); err != nil {
				return err
			}
		}
		if !membershipException(baseBound, bound) && !valueInBase(bound) {
			return boundRestrictionDiagnostic(bound.Loc(), baseRelated, version, bound.Kind(), "derived "+datatype+" bound is outside the base value space", fmt.Errorf("%w: endpoint is not a value of the base restriction", errInvalidBoundRestriction))
		}
		if baseBound != nil {
			if err := validateMonotonic(baseBound, bound, version); err != nil {
				return err
			}
		}
		if version == XSDVersion10 && bound.Kind().IsLower() && lower != nil && endpointInclusive(lower) != bound.Kind().Inclusive() {
			return invalidBoundCombinationDiagnostic(bound.Loc(), facetLocations(lowerLoc(lower)), version, lowerKind(lower), bound.Kind(), "XSD 1.0 effective lower bounds cannot use both inclusivity kinds")
		}
	}
	return nil
}

func integerBaseEndpoint(base IntegerBoundFacets, kind BoundKind) *integerBoundEndpoint {
	if kind.IsLower() {
		return base.lower
	}
	if kind.IsUpper() {
		return base.upper
	}
	return nil
}

func decimalBaseEndpoint(base DecimalBoundFacets, kind BoundKind) *decimalBoundEndpoint {
	if kind.IsLower() {
		return base.lower
	}
	if kind.IsUpper() {
		return base.upper
	}
	return nil
}

func integerBoundMembershipException(base *integerBoundEndpoint, local IntegerBoundFacet) bool {
	if base == nil {
		return false
	}
	return !base.inclusive && !local.Kind().Inclusive() && local.Value().Equal(base.value)
}

func decimalBoundMembershipException(base *decimalBoundEndpoint, local DecimalBoundFacet) bool {
	if base == nil {
		return false
	}
	return !base.inclusive && !local.Kind().Inclusive() && local.Value().Equal(base.value)
}

func integerBoundRelatedLocation(base IntegerBoundFacets) []Loc {
	locations := make([]Loc, 0, 2)
	if base.lower != nil && !base.lower.loc.IsZero() {
		locations = append(locations, base.lower.loc)
	}
	if base.upper != nil && !base.upper.loc.IsZero() {
		locations = append(locations, base.upper.loc)
	}
	return locations
}

func decimalBoundRelatedLocation(base DecimalBoundFacets) []Loc {
	locations := make([]Loc, 0, 2)
	if base.lower != nil && !base.lower.loc.IsZero() {
		locations = append(locations, base.lower.loc)
	}
	if base.upper != nil && !base.upper.loc.IsZero() {
		locations = append(locations, base.upper.loc)
	}
	return locations
}

func integerValueInBase(base IntegerBoundFacets, value StrictInteger) bool {
	if base.lower != nil {
		comparison := value.Compare(base.lower.value)
		if comparison < 0 || (comparison == 0 && !base.lower.inclusive) {
			return false
		}
	}
	if base.upper != nil {
		comparison := value.Compare(base.upper.value)
		if comparison > 0 || (comparison == 0 && !base.upper.inclusive) {
			return false
		}
	}
	return true
}

func decimalValueInBase(base DecimalBoundFacets, value StrictDecimal) bool {
	if base.lower != nil {
		comparison := value.Compare(base.lower.value)
		if comparison < 0 || (comparison == 0 && !base.lower.inclusive) {
			return false
		}
	}
	if base.upper != nil {
		comparison := value.Compare(base.upper.value)
		if comparison > 0 || (comparison == 0 && !base.upper.inclusive) {
			return false
		}
	}
	return true
}

func validateIntegerMonotonicForVersion(parent *integerBoundEndpoint, child IntegerBoundFacet, version XSDVersion) error {
	comparison := child.Value().Compare(parent.value)
	valid := true
	if child.Kind().IsLower() {
		valid = comparison > 0 || (comparison == 0 && (parent.inclusive || !child.Kind().Inclusive()))
	}
	if child.Kind().IsUpper() {
		valid = comparison < 0 || (comparison == 0 && (parent.inclusive || !child.Kind().Inclusive()))
	}
	if valid {
		return nil
	}
	return boundRestrictionDiagnostic(child.Loc(), facetLocations(parent.loc), version, child.Kind(), "derived integer bound is less restrictive than its base bound", fmt.Errorf("%w: endpoint moves outward", errInvalidBoundRestriction))
}

func validateDecimalMonotonicForVersion(parent *decimalBoundEndpoint, child DecimalBoundFacet, version XSDVersion) error {
	comparison := child.Value().Compare(parent.value)
	valid := true
	if child.Kind().IsLower() {
		valid = comparison > 0 || (comparison == 0 && (parent.inclusive || !child.Kind().Inclusive()))
	}
	if child.Kind().IsUpper() {
		valid = comparison < 0 || (comparison == 0 && (parent.inclusive || !child.Kind().Inclusive()))
	}
	if valid {
		return nil
	}
	return boundRestrictionDiagnostic(child.Loc(), facetLocations(parent.loc), version, child.Kind(), "derived decimal bound is less restrictive than its base bound", fmt.Errorf("%w: endpoint moves outward", errInvalidBoundRestriction))
}

func validateIntegerFixed(parent *integerBoundEndpoint, child IntegerBoundFacet, version XSDVersion) error {
	if !parent.fixed || parent.inclusive != child.Kind().Inclusive() || child.Value().Equal(parent.value) {
		return nil
	}
	return newBoundDiagnostic(
		FailureInvalid,
		InvalidBoundRestrictionCode,
		child.Loc(),
		boundSpecRef(version, child.Kind(), boundFixedRule),
		"derived integer bound changes a fixed base bound",
		facetLocations(parent.loc),
		fmt.Errorf("%w: fixed bound value differs", errInvalidBoundRestriction),
	)
}

func validateDecimalFixed(parent *decimalBoundEndpoint, child DecimalBoundFacet, version XSDVersion) error {
	if !parent.fixed || parent.inclusive != child.Kind().Inclusive() || child.Value().Equal(parent.value) {
		return nil
	}
	return newBoundDiagnostic(
		FailureInvalid,
		InvalidBoundRestrictionCode,
		child.Loc(),
		boundSpecRef(version, child.Kind(), boundFixedRule),
		"derived decimal bound changes a fixed base bound",
		facetLocations(parent.loc),
		fmt.Errorf("%w: fixed bound value differs", errInvalidBoundRestriction),
	)
}

func applyIntegerBound(effective *IntegerBoundFacets, local IntegerBoundFacet) {
	endpoint := &integerBoundEndpoint{value: cloneStrictInteger(local.value), loc: local.loc, fixed: local.fixed, inclusive: local.kind.Inclusive()}
	if local.kind.IsLower() {
		effective.lower = overlayIntegerEndpoint(effective.lower, endpoint, true)
		return
	}
	effective.upper = overlayIntegerEndpoint(effective.upper, endpoint, false)
}

func applyDecimalBound(effective *DecimalBoundFacets, local DecimalBoundFacet) {
	endpoint := &decimalBoundEndpoint{value: cloneStrictDecimal(local.value), loc: local.loc, fixed: local.fixed, inclusive: local.kind.Inclusive()}
	if local.kind.IsLower() {
		effective.lower = overlayDecimalEndpoint(effective.lower, endpoint, true)
		return
	}
	effective.upper = overlayDecimalEndpoint(effective.upper, endpoint, false)
}

func overlayIntegerEndpoint(current, local *integerBoundEndpoint, lower bool) *integerBoundEndpoint {
	if current == nil {
		return cloneIntegerBoundEndpoint(local)
	}
	if current.inclusive == local.inclusive {
		owned := cloneIntegerBoundEndpoint(local)
		owned.fixed = current.fixed || local.fixed
		return owned
	}
	comparison := local.value.Compare(current.value)
	if comparison == 0 {
		if local.inclusive {
			return cloneIntegerBoundEndpoint(current)
		}
		return cloneIntegerBoundEndpoint(local)
	}
	if lower {
		if comparison > 0 {
			return cloneIntegerBoundEndpoint(local)
		}
		return cloneIntegerBoundEndpoint(current)
	}
	if comparison < 0 {
		return cloneIntegerBoundEndpoint(local)
	}
	return cloneIntegerBoundEndpoint(current)
}

func overlayDecimalEndpoint(current, local *decimalBoundEndpoint, lower bool) *decimalBoundEndpoint {
	if current == nil {
		return cloneDecimalBoundEndpoint(local)
	}
	if current.inclusive == local.inclusive {
		owned := cloneDecimalBoundEndpoint(local)
		owned.fixed = current.fixed || local.fixed
		return owned
	}
	comparison := local.value.Compare(current.value)
	if comparison == 0 {
		if local.inclusive {
			return cloneDecimalBoundEndpoint(current)
		}
		return cloneDecimalBoundEndpoint(local)
	}
	if lower {
		if comparison > 0 {
			return cloneDecimalBoundEndpoint(local)
		}
		return cloneDecimalBoundEndpoint(current)
	}
	if comparison < 0 {
		return cloneDecimalBoundEndpoint(local)
	}
	return cloneDecimalBoundEndpoint(current)
}

func (facets IntegerBoundFacets) validateForConstruction() error {
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newBoundDiagnostic(FailureInternal, diagnosticBoundEffectiveVersionCode, Loc{}, "", "cannot construct bounds from an unknown XSD version", nil, errInvalidBoundState)
	}
	return nil
}

func (facets DecimalBoundFacets) validateForConstruction() error {
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newBoundDiagnostic(FailureInternal, diagnosticBoundEffectiveVersionCode, Loc{}, "", "cannot construct bounds from an unknown XSD version", nil, errInvalidBoundState)
	}
	return nil
}

func (facets IntegerBoundFacets) validate() error {
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newBoundDiagnostic(FailureInternal, diagnosticBoundEffectiveVersionCode, Loc{}, "", "completed integer bounds have an unknown XSD version", nil, errInvalidBoundState)
	}
	return validateIntegerInterval(facets.lower, facets.upper, facets.version)
}

func (facets DecimalBoundFacets) validate() error {
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newBoundDiagnostic(FailureInternal, diagnosticBoundEffectiveVersionCode, Loc{}, "", "completed decimal bounds have an unknown XSD version", nil, errInvalidBoundState)
	}
	return validateDecimalInterval(facets.lower, facets.upper, facets.version)
}

func validateIntegerInterval(lower, upper *integerBoundEndpoint, version XSDVersion) error {
	if lower == nil || upper == nil {
		return nil
	}
	comparison := lower.value.Compare(upper.value)
	if comparison < 0 || (comparison == 0 && lower.inclusive == upper.inclusive) {
		return nil
	}
	return invalidBoundCombinationDiagnostic(lower.loc, facetLocations(upper.loc), version, integerLowerBoundKind(lower), integerUpperBoundKind(upper), "integer lower and upper bounds describe an empty ordered interval")
}

func validateDecimalInterval(lower, upper *decimalBoundEndpoint, version XSDVersion) error {
	if lower == nil || upper == nil {
		return nil
	}
	comparison := lower.value.Compare(upper.value)
	if comparison < 0 || (comparison == 0 && lower.inclusive == upper.inclusive) {
		return nil
	}
	return invalidBoundCombinationDiagnostic(lower.loc, facetLocations(upper.loc), version, decimalLowerBoundKind(lower), decimalUpperBoundKind(upper), "decimal lower and upper bounds describe an empty ordered interval")
}

func integerLowerBoundKind(endpoint *integerBoundEndpoint) BoundKind {
	if endpoint.inclusive {
		return BoundMinInclusive
	}
	return BoundMinExclusive
}

func integerUpperBoundKind(endpoint *integerBoundEndpoint) BoundKind {
	if endpoint.inclusive {
		return BoundMaxInclusive
	}
	return BoundMaxExclusive
}

func decimalLowerBoundKind(endpoint *decimalBoundEndpoint) BoundKind {
	if endpoint.inclusive {
		return BoundMinInclusive
	}
	return BoundMinExclusive
}

func decimalUpperBoundKind(endpoint *decimalBoundEndpoint) BoundKind {
	if endpoint.inclusive {
		return BoundMaxInclusive
	}
	return BoundMaxExclusive
}

func integerBoundFacetFromEndpoint(endpoint *integerBoundEndpoint, kind BoundKind, version XSDVersion) IntegerBoundFacet {
	return IntegerBoundFacet{kind: kind, value: cloneStrictInteger(endpoint.value), loc: endpoint.loc, fixed: endpoint.fixed, version: version}
}

func decimalBoundFacetFromEndpoint(endpoint *decimalBoundEndpoint, kind BoundKind, version XSDVersion) DecimalBoundFacet {
	return DecimalBoundFacet{kind: kind, value: cloneStrictDecimal(endpoint.value), loc: endpoint.loc, fixed: endpoint.fixed, version: version}
}

func oppositeBoundKind(kind BoundKind) BoundKind {
	switch kind {
	case BoundMinInclusive:
		return BoundMinExclusive
	case BoundMinExclusive:
		return BoundMinInclusive
	case BoundMaxInclusive:
		return BoundMaxExclusive
	case BoundMaxExclusive:
		return BoundMaxInclusive
	default:
		return 0
	}
}

func invalidBoundKindDiagnostic(_ BoundKind, loc Loc, _ XSDVersion) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, InvalidBoundCode, loc, "", "ordered bound declaration has an unknown kind", nil, errInvalidBoundValue)
}

func invalidBoundVersionDiagnostic(loc Loc, cause error) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, InvalidXSDVersionCode, loc, "", "invalid XSD version policy for ordered bounds", nil, fmt.Errorf("%w: %w", errInvalidBoundVersion, cause))
}

func invalidBoundDeclarationVersionDiagnostic(facet interface {
	Kind() BoundKind
	Loc() Loc
	Version() XSDVersion
}, version XSDVersion) Diagnostic {
	cause := fmt.Errorf("%w: declaration uses %q, base uses %q", errInvalidBoundVersion, facet.Version(), version)
	return newBoundDiagnostic(FailureInvalid, InvalidXSDVersionCode, facet.Loc(), boundSpecRef(version, facet.Kind(), boundDefinitionRule), "ordered bound declaration version does not match its base type policy", nil, cause)
}

func invalidBoundLexicalDiagnostic(kind BoundKind, loc Loc, version XSDVersion, cause error) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, InvalidBoundCode, loc, boundSpecRef(version, kind, boundDefinitionRule), "invalid "+kind.String()+" facet value", nil, fmt.Errorf("%w: %w", errInvalidBoundValue, cause))
}

func boundRestrictionDiagnostic(primary Loc, related []Loc, version XSDVersion, kind BoundKind, message string, cause error) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, InvalidBoundRestrictionCode, primary, boundSpecRef(version, kind, boundRestrictionRule), message, related, cause)
}

func invalidBoundCombinationDiagnostic(primary Loc, related []Loc, version XSDVersion, lower, upper BoundKind, message string) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, InvalidBoundCombinationCode, primary, boundCombinationSpecRef(version, lower, upper), message, related, fmt.Errorf("%w: %s", errInvalidBoundCombination, message))
}

func boundValueViolationIntegerDiagnostic(valueLoc Loc, version XSDVersion, kind BoundKind, related Loc, datatype string) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, BoundValueViolationCode, valueLoc, boundSpecRef(version, kind, boundValueRule), datatype+" value violates "+kind.String(), facetLocations(related), fmt.Errorf("%w: value violates %s", errBoundValueViolation, kind.String()))
}

func boundValueViolationDecimalDiagnostic(valueLoc Loc, version XSDVersion, kind BoundKind, related Loc, datatype string) Diagnostic {
	return newBoundDiagnostic(FailureInvalid, BoundValueViolationCode, valueLoc, boundSpecRef(version, kind, boundValueRule), datatype+" value violates "+kind.String(), facetLocations(related), fmt.Errorf("%w: value violates %s", errBoundValueViolation, kind.String()))
}

func newBoundDiagnostic(class FailureClass, code string, loc Loc, specRef, message string, related []Loc, cause error) Diagnostic {
	return Diagnostic{class: class, code: code, loc: loc, message: message, related: append([]Loc(nil), related...), specRef: specRef, cause: cause}
}

func facetLocations(loc Loc) []Loc {
	if loc.IsZero() {
		return nil
	}
	return []Loc{loc}
}

func boundSpecRef(version XSDVersion, kind BoundKind, rule boundRule) string {
	prefix := "xsd11"
	if version == XSDVersion10 {
		prefix = "xsd10"
	}
	name := kind.String()
	switch rule {
	case boundDefinitionRule:
		return prefix + "-datatypes#rf-" + name
	case boundValueRule:
		return prefix + "-datatypes#cvc-" + name + "-valid"
	case boundRestrictionRule:
		return prefix + "-datatypes#" + name + "-valid-restriction"
	case boundFixedRule:
		if version == XSDVersion11 {
			return prefix + "-datatypes#" + boundFixedAnchor(kind)
		}
		return prefix + "-datatypes#" + name + "-fixed"
	default:
		return prefix + "-datatypes#rf-" + name
	}
}

func boundFixedAnchor(kind BoundKind) string {
	switch kind {
	case BoundMinInclusive:
		return "f-mii-fixed"
	case BoundMinExclusive:
		return "f-mie-fixed"
	case BoundMaxInclusive:
		return "f-mai-fixed"
	case BoundMaxExclusive:
		return "f-mae-fixed"
	default:
		return "ordered-bound-fixed"
	}
}

func boundCombinationSpecRef(version XSDVersion, lower, upper BoundKind) string {
	prefix := "xsd11"
	if version == XSDVersion10 {
		prefix = "xsd10"
	}
	if lower.IsLower() && upper.IsLower() {
		return prefix + "-datatypes#minInclusive-minExclusive"
	}
	if lower.IsUpper() && upper.IsUpper() {
		return prefix + "-datatypes#maxInclusive-maxExclusive"
	}
	if lower == BoundMinInclusive && upper == BoundMaxInclusive {
		return prefix + "-datatypes#minInclusive-less-than-equal-to-maxInclusive"
	}
	if lower == BoundMinInclusive && upper == BoundMaxExclusive {
		return prefix + "-datatypes#minInclusive-less-than-maxExclusive"
	}
	if lower == BoundMinExclusive && upper == BoundMaxInclusive {
		return prefix + "-datatypes#minExclusive-less-than-maxInclusive"
	}
	if lower == BoundMinExclusive && upper == BoundMaxExclusive {
		return prefix + "-datatypes#minExclusive-less-than-equal-to-maxExclusive"
	}
	return prefix + "-datatypes#rf-minInclusive"
}
