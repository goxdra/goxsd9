package goxsd9

import (
	"errors"
	"fmt"
)

const (
	// InvalidPrecisionDecimalTotalDigitsCode identifies an invalid
	// precisionDecimal totalDigits declaration.
	InvalidPrecisionDecimalTotalDigitsCode = "XSD2011"
	// InvalidPrecisionDecimalMinScaleCode identifies an invalid
	// precisionDecimal minScale declaration.
	InvalidPrecisionDecimalMinScaleCode = "XSD2012"
	// InvalidPrecisionDecimalMaxScaleCode identifies an invalid
	// precisionDecimal maxScale declaration.
	InvalidPrecisionDecimalMaxScaleCode = "XSD2013"
	// InvalidPrecisionDecimalFacetRestrictionCode identifies an invalid
	// precisionDecimal derived facet declaration.
	InvalidPrecisionDecimalFacetRestrictionCode = "XSD2014"
	// InvalidPrecisionDecimalScaleCombinationCode identifies contradictory
	// precisionDecimal scale bounds.
	InvalidPrecisionDecimalScaleCombinationCode = "XSD2015"
	// InvalidPrecisionDecimalDisallowedFacetCode identifies a facet that is not
	// allowed for precisionDecimal declarations in this layer.
	InvalidPrecisionDecimalDisallowedFacetCode = "XSD2016"
	// UnsupportedPrecisionDecimalFacetCode identifies an applicable
	// precisionDecimal facet that is not implemented in this layer.
	UnsupportedPrecisionDecimalFacetCode = "XSD2017"
	// InvalidPrecisionDecimalUnknownFacetCode identifies an unknown
	// precisionDecimal facet name.
	InvalidPrecisionDecimalUnknownFacetCode = "XSD2018"
)

const (
	precisionDecimalFacetSetSpecRef            = "xsd-precisionDecimal#facets"
	precisionDecimalTotalDigitsSpecRef         = "xsd-precisionDecimal#rf-totalDigits"
	xsd11TotalDigitsValueSpecRef               = "xsd11-datatypes#f-td-value"
	xsd11TotalDigitsFixedSpecRef               = "xsd11-datatypes#f-td-fixed"
	xsd11TotalDigitsRestrictionSpecRef         = "xsd11-datatypes#totalDigits-valid-restriction"
	precisionDecimalMinScaleValueSpecRef       = "xsd-precisionDecimal#f-mns-value"
	precisionDecimalMinScaleFixedSpecRef       = "xsd-precisionDecimal#f-mns-fixed"
	precisionDecimalMinScaleRestrictionSpecRef = "xsd-precisionDecimal#minScale-valid-restriction"
	precisionDecimalMaxScaleValueSpecRef       = "xsd-precisionDecimal#f-ms-value"
	precisionDecimalMaxScaleFixedSpecRef       = "xsd-precisionDecimal#f-ms-fixed"
	precisionDecimalMaxScaleRestrictionSpecRef = "xsd-precisionDecimal#maxScale-valid-restriction"
	precisionDecimalScaleCombinationSpecRef    = "xsd-precisionDecimal#minScale-totalDigits"
)

var (
	errInvalidPrecisionDecimalTotalDigitsValue = errors.New("invalid precisionDecimal totalDigits facet value")
	errInvalidPrecisionDecimalMinScaleValue    = errors.New("invalid precisionDecimal minScale facet value")
	errInvalidPrecisionDecimalMaxScaleValue    = errors.New("invalid precisionDecimal maxScale facet value")
	errInvalidPrecisionDecimalFacetRestriction = errors.New("invalid precisionDecimal facet restriction")
	errInvalidPrecisionDecimalScaleCombination = errors.New("invalid precisionDecimal scale bounds")
	errInvalidPrecisionDecimalDisallowedFacet  = errors.New("disallowed precisionDecimal facet")
	errInvalidPrecisionDecimalUnknownFacet     = errors.New("unknown precisionDecimal facet")
	errInvalidPrecisionDecimalFacetState       = errors.New("invalid completed precisionDecimal facet state")
)

type precisionDecimalFacetKind uint8

const (
	precisionDecimalTotalDigitsKind precisionDecimalFacetKind = iota + 1
	precisionDecimalMinScaleKind
	precisionDecimalMaxScaleKind
)

// PrecisionDecimalTotalDigitsFacet is a parsed, immutable totalDigits
// declaration for precisionDecimal.
type PrecisionDecimalTotalDigitsFacet struct {
	value StrictInteger
	loc   Loc
	fixed bool
}

// Value returns the exact positive totalDigits value.
func (facet PrecisionDecimalTotalDigitsFacet) Value() StrictInteger {
	return precisionDecimalIntegerCopy(facet.value)
}

// Loc returns the source location of the facet declaration.
func (facet PrecisionDecimalTotalDigitsFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the facet is fixed for derived restrictions.
func (facet PrecisionDecimalTotalDigitsFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalTotalDigitsFacet) WithFixed(fixed bool) PrecisionDecimalTotalDigitsFacet {
	facet.fixed = fixed
	return facet
}

// PrecisionDecimalMinScaleFacet is a parsed, immutable signed minScale
// declaration for precisionDecimal.
type PrecisionDecimalMinScaleFacet struct {
	value StrictInteger
	loc   Loc
	fixed bool
}

// Value returns the exact signed minScale value.
func (facet PrecisionDecimalMinScaleFacet) Value() StrictInteger {
	return precisionDecimalIntegerCopy(facet.value)
}

// Loc returns the source location of the facet declaration.
func (facet PrecisionDecimalMinScaleFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the facet is fixed for derived restrictions.
func (facet PrecisionDecimalMinScaleFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalMinScaleFacet) WithFixed(fixed bool) PrecisionDecimalMinScaleFacet {
	facet.fixed = fixed
	return facet
}

// PrecisionDecimalMaxScaleFacet is a parsed, immutable signed maxScale
// declaration for precisionDecimal.
type PrecisionDecimalMaxScaleFacet struct {
	value StrictInteger
	loc   Loc
	fixed bool
}

// Value returns the exact signed maxScale value.
func (facet PrecisionDecimalMaxScaleFacet) Value() StrictInteger {
	return precisionDecimalIntegerCopy(facet.value)
}

// Loc returns the source location of the facet declaration.
func (facet PrecisionDecimalMaxScaleFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the facet is fixed for derived restrictions.
func (facet PrecisionDecimalMaxScaleFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalMaxScaleFacet) WithFixed(fixed bool) PrecisionDecimalMaxScaleFacet {
	facet.fixed = fixed
	return facet
}

// ParsePrecisionDecimalTotalDigits parses an exact positive totalDigits
// value without constructing a precisionDecimal value.
func ParsePrecisionDecimalTotalDigits(lexical string, loc Loc) (StrictInteger, error) {
	return parsePrecisionDecimalFacetInteger(precisionDecimalTotalDigitsKind, lexical, loc)
}

// ParsePrecisionDecimalMinScale parses an exact signed minScale value without
// constructing a precisionDecimal value.
func ParsePrecisionDecimalMinScale(lexical string, loc Loc) (StrictInteger, error) {
	return parsePrecisionDecimalFacetInteger(precisionDecimalMinScaleKind, lexical, loc)
}

// ParsePrecisionDecimalMaxScale parses an exact signed maxScale value without
// constructing a precisionDecimal value.
func ParsePrecisionDecimalMaxScale(lexical string, loc Loc) (StrictInteger, error) {
	return parsePrecisionDecimalFacetInteger(precisionDecimalMaxScaleKind, lexical, loc)
}

// ParsePrecisionDecimalTotalDigitsFacet parses a totalDigits declaration with
// fixed=false.
func ParsePrecisionDecimalTotalDigitsFacet(lexical string, loc Loc) (PrecisionDecimalTotalDigitsFacet, error) {
	return parsePrecisionDecimalTotalDigitsFacet(lexical, loc, false)
}

// ParsePrecisionDecimalTotalDigitsFacetWithFixed parses a totalDigits
// declaration including its fixed property.
func ParsePrecisionDecimalTotalDigitsFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalTotalDigitsFacet, error) {
	return parsePrecisionDecimalTotalDigitsFacet(lexical, loc, fixed)
}

// ParsePrecisionDecimalMinScaleFacet parses a minScale declaration with
// fixed=false.
func ParsePrecisionDecimalMinScaleFacet(lexical string, loc Loc) (PrecisionDecimalMinScaleFacet, error) {
	return parsePrecisionDecimalMinScaleFacet(lexical, loc, false)
}

// ParsePrecisionDecimalMinScaleFacetWithFixed parses a minScale declaration
// including its fixed property.
func ParsePrecisionDecimalMinScaleFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinScaleFacet, error) {
	return parsePrecisionDecimalMinScaleFacet(lexical, loc, fixed)
}

// ParsePrecisionDecimalMaxScaleFacet parses a maxScale declaration with
// fixed=false.
func ParsePrecisionDecimalMaxScaleFacet(lexical string, loc Loc) (PrecisionDecimalMaxScaleFacet, error) {
	return parsePrecisionDecimalMaxScaleFacet(lexical, loc, false)
}

// ParsePrecisionDecimalMaxScaleFacetWithFixed parses a maxScale declaration
// including its fixed property.
func ParsePrecisionDecimalMaxScaleFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxScaleFacet, error) {
	return parsePrecisionDecimalMaxScaleFacet(lexical, loc, fixed)
}

// NewPrecisionDecimalTotalDigitsFacet constructs a totalDigits declaration
// from a completed exact value.
func NewPrecisionDecimalTotalDigitsFacet(value StrictInteger, loc Loc, fixed bool) (PrecisionDecimalTotalDigitsFacet, error) {
	if value.Sign() > 0 {
		return PrecisionDecimalTotalDigitsFacet{
			value: precisionDecimalIntegerCopy(value),
			loc:   loc,
			fixed: fixed,
		}, nil
	}
	return PrecisionDecimalTotalDigitsFacet{}, invalidPrecisionDecimalFacetValue(
		precisionDecimalTotalDigitsKind,
		loc,
		value.Canonical(),
		fmt.Errorf("%w: value is not positive", errInvalidPrecisionDecimalTotalDigitsValue),
	)
}

// NewPrecisionDecimalMinScaleFacet constructs a minScale declaration from a
// completed exact value.
func NewPrecisionDecimalMinScaleFacet(value StrictInteger, loc Loc, fixed bool) (PrecisionDecimalMinScaleFacet, error) {
	return PrecisionDecimalMinScaleFacet{
		value: precisionDecimalIntegerCopy(value),
		loc:   loc,
		fixed: fixed,
	}, nil
}

// NewPrecisionDecimalMaxScaleFacet constructs a maxScale declaration from a
// completed exact value.
func NewPrecisionDecimalMaxScaleFacet(value StrictInteger, loc Loc, fixed bool) (PrecisionDecimalMaxScaleFacet, error) {
	return PrecisionDecimalMaxScaleFacet{
		value: precisionDecimalIntegerCopy(value),
		loc:   loc,
		fixed: fixed,
	}, nil
}

// PrecisionDecimalFacetDeclarations contains only local precisionDecimal
// facet declarations. Nil fields were omitted by the local type.
type PrecisionDecimalFacetDeclarations struct {
	TotalDigits *PrecisionDecimalTotalDigitsFacet
	MinScale    *PrecisionDecimalMinScaleFacet
	MaxScale    *PrecisionDecimalMaxScaleFacet
}

// NewPrecisionDecimalFacetDeclarations makes an owned copy of local
// precisionDecimal facet declarations.
func NewPrecisionDecimalFacetDeclarations(
	totalDigits *PrecisionDecimalTotalDigitsFacet,
	minScale *PrecisionDecimalMinScaleFacet,
	maxScale *PrecisionDecimalMaxScaleFacet,
) PrecisionDecimalFacetDeclarations {
	return PrecisionDecimalFacetDeclarations{
		TotalDigits: clonePrecisionDecimalTotalDigitsFacet(totalDigits),
		MinScale:    clonePrecisionDecimalMinScaleFacet(minScale),
		MaxScale:    clonePrecisionDecimalMaxScaleFacet(maxScale),
	}
}

// PrecisionDecimalFacets is an immutable complete effective set of the
// declaration-time precisionDecimal totalDigits, minScale, and maxScale
// facets.
type PrecisionDecimalFacets struct {
	totalDigits *PrecisionDecimalTotalDigitsFacet
	minScale    *PrecisionDecimalMinScaleFacet
	maxScale    *PrecisionDecimalMaxScaleFacet
}

// NewPrecisionDecimalFacets constructs complete effective facets for local
// declarations on precisionDecimal.
func NewPrecisionDecimalFacets(
	totalDigits *PrecisionDecimalTotalDigitsFacet,
	minScale *PrecisionDecimalMinScaleFacet,
	maxScale *PrecisionDecimalMaxScaleFacet,
) (PrecisionDecimalFacets, error) {
	local := NewPrecisionDecimalFacetDeclarations(totalDigits, minScale, maxScale)
	return completePrecisionDecimalFacets(PrecisionDecimalFacets{}, local, false)
}

// NewPrecisionDecimalFacetsFromDeclarations constructs complete effective
// facets from local declarations.
func NewPrecisionDecimalFacetsFromDeclarations(local PrecisionDecimalFacetDeclarations) (PrecisionDecimalFacets, error) {
	return completePrecisionDecimalFacets(PrecisionDecimalFacets{}, NewPrecisionDecimalFacetDeclarations(
		local.TotalDigits,
		local.MinScale,
		local.MaxScale,
	), false)
}

// RestrictPrecisionDecimalFacets inherits omitted declarations from base and
// validates the complete local overlay as a restriction.
func RestrictPrecisionDecimalFacets(base PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations) (PrecisionDecimalFacets, error) {
	if err := base.validate(); err != nil {
		return PrecisionDecimalFacets{}, err
	}
	return completePrecisionDecimalFacets(base, NewPrecisionDecimalFacetDeclarations(
		local.TotalDigits,
		local.MinScale,
		local.MaxScale,
	), true)
}

// ConstructPrecisionDecimalFacets is the phase-oriented name for
// RestrictPrecisionDecimalFacets.
func ConstructPrecisionDecimalFacets(base PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations) (PrecisionDecimalFacets, error) {
	return RestrictPrecisionDecimalFacets(base, local)
}

// HasTotalDigits reports whether an effective totalDigits facet exists.
func (facets PrecisionDecimalFacets) HasTotalDigits() bool {
	return facets.totalDigits != nil
}

// TotalDigits returns the effective totalDigits value and its presence.
func (facets PrecisionDecimalFacets) TotalDigits() (StrictInteger, bool) {
	if facets.totalDigits == nil {
		return StrictInteger{}, false
	}
	return facets.totalDigits.Value(), true
}

// TotalDigitsLoc returns the source location and presence of totalDigits.
func (facets PrecisionDecimalFacets) TotalDigitsLoc() (Loc, bool) {
	if facets.totalDigits == nil {
		return Loc{}, false
	}
	return facets.totalDigits.Loc(), true
}

// TotalDigitsFixed returns the fixed property and presence of totalDigits.
func (facets PrecisionDecimalFacets) TotalDigitsFixed() (bool, bool) {
	if facets.totalDigits == nil {
		return false, false
	}
	return facets.totalDigits.Fixed(), true
}

// HasMinScale reports whether an effective minScale facet exists.
func (facets PrecisionDecimalFacets) HasMinScale() bool {
	return facets.minScale != nil
}

// MinScale returns the effective minScale value and its presence.
func (facets PrecisionDecimalFacets) MinScale() (StrictInteger, bool) {
	if facets.minScale == nil {
		return StrictInteger{}, false
	}
	return facets.minScale.Value(), true
}

// MinScaleLoc returns the source location and presence of minScale.
func (facets PrecisionDecimalFacets) MinScaleLoc() (Loc, bool) {
	if facets.minScale == nil {
		return Loc{}, false
	}
	return facets.minScale.Loc(), true
}

// MinScaleFixed returns the fixed property and presence of minScale.
func (facets PrecisionDecimalFacets) MinScaleFixed() (bool, bool) {
	if facets.minScale == nil {
		return false, false
	}
	return facets.minScale.Fixed(), true
}

// HasMaxScale reports whether an effective maxScale facet exists.
func (facets PrecisionDecimalFacets) HasMaxScale() bool {
	return facets.maxScale != nil
}

// MaxScale returns the effective maxScale value and its presence.
func (facets PrecisionDecimalFacets) MaxScale() (StrictInteger, bool) {
	if facets.maxScale == nil {
		return StrictInteger{}, false
	}
	return facets.maxScale.Value(), true
}

// MaxScaleLoc returns the source location and presence of maxScale.
func (facets PrecisionDecimalFacets) MaxScaleLoc() (Loc, bool) {
	if facets.maxScale == nil {
		return Loc{}, false
	}
	return facets.maxScale.Loc(), true
}

// MaxScaleFixed returns the fixed property and presence of maxScale.
func (facets PrecisionDecimalFacets) MaxScaleFixed() (bool, bool) {
	if facets.maxScale == nil {
		return false, false
	}
	return facets.maxScale.Fixed(), true
}

// ValidatePrecisionDecimalFacetName classifies a precisionDecimal facet name
// for this declaration layer. Inapplicable facets are invalid; applicable
// facets outside this layer are reported as unsupported. In particular,
// fractionDigits is not represented.
func ValidatePrecisionDecimalFacetName(name string, loc Loc) error {
	switch name {
	case "totalDigits", "minScale", "maxScale":
		return nil
	case "pattern", "enumeration", "minInclusive", "minExclusive", "maxInclusive", "maxExclusive", "assertions", "whiteSpace":
		feature, ok := LookupUnsupportedFeature(FeaturePrecisionDecimal)
		if !ok {
			return newDiagnostic(
				FailureInternal,
				diagnosticUnregisteredFeatureCode,
				loc,
				"precisionDecimal feature is not registered",
				fmt.Errorf("%w: precisionDecimal feature", errInvalidPrecisionDecimalFacetState),
			)
		}
		return newUnsupported(
			feature,
			UnsupportedPrecisionDecimalFacetCode,
			loc,
			fmt.Sprintf("precisionDecimal facet %q is not implemented", name),
		)
	case "fractionDigits", "length", "minLength", "maxLength":
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalDisallowedFacetCode,
			loc,
			precisionDecimalFacetSetSpecRef,
			fmt.Sprintf("facet %q is not allowed for precisionDecimal declarations", name),
			nil,
			fmt.Errorf("%w: %q", errInvalidPrecisionDecimalDisallowedFacet, name),
		)
	default:
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalUnknownFacetCode,
			loc,
			precisionDecimalFacetSetSpecRef,
			fmt.Sprintf("unknown precisionDecimal facet %q", name),
			nil,
			fmt.Errorf("%w: %q", errInvalidPrecisionDecimalUnknownFacet, name),
		)
	}
}

func parsePrecisionDecimalTotalDigitsFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalTotalDigitsFacet, error) {
	value, err := ParsePrecisionDecimalTotalDigits(lexical, loc)
	if err != nil {
		return PrecisionDecimalTotalDigitsFacet{}, err
	}
	return PrecisionDecimalTotalDigitsFacet{
		value: value,
		loc:   loc,
		fixed: fixed,
	}, nil
}

func parsePrecisionDecimalMinScaleFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinScaleFacet, error) {
	value, err := ParsePrecisionDecimalMinScale(lexical, loc)
	if err != nil {
		return PrecisionDecimalMinScaleFacet{}, err
	}
	return PrecisionDecimalMinScaleFacet{
		value: value,
		loc:   loc,
		fixed: fixed,
	}, nil
}

func parsePrecisionDecimalMaxScaleFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxScaleFacet, error) {
	value, err := ParsePrecisionDecimalMaxScale(lexical, loc)
	if err != nil {
		return PrecisionDecimalMaxScaleFacet{}, err
	}
	return PrecisionDecimalMaxScaleFacet{
		value: value,
		loc:   loc,
		fixed: fixed,
	}, nil
}

func parsePrecisionDecimalFacetInteger(kind precisionDecimalFacetKind, lexical string, loc Loc) (StrictInteger, error) {
	lexeme := collapseXMLWhitespace(lexical)
	parsed, ok := scanIntegerLexical(lexeme)
	if !ok {
		return StrictInteger{}, invalidPrecisionDecimalFacetValue(
			kind,
			loc,
			lexeme,
			fmt.Errorf("%w: lexical form is not an integer", precisionDecimalFacetValueCause(kind)),
		)
	}

	value, ok := newBigInteger(parsed.digits, parsed.negative)
	if !ok {
		return StrictInteger{}, newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			precisionDecimalFacetValueCode(kind),
			loc,
			precisionDecimalFacetValueSpecRef(kind),
			"valid precisionDecimal facet value could not be constructed",
			nil,
			fmt.Errorf("%w: arbitrary-precision construction failed", precisionDecimalFacetValueCause(kind)),
		)
	}

	if kind == precisionDecimalTotalDigitsKind && value.Sign() <= 0 {
		return StrictInteger{}, invalidPrecisionDecimalFacetValue(
			kind,
			loc,
			lexeme,
			fmt.Errorf("%w: value is not positive", errInvalidPrecisionDecimalTotalDigitsValue),
		)
	}
	return StrictInteger{value: value}, nil
}

func completePrecisionDecimalFacets(base PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations, derived bool) (PrecisionDecimalFacets, error) {
	if err := base.validate(); err != nil {
		return PrecisionDecimalFacets{}, err
	}
	if err := validatePrecisionDecimalLocalDeclarations(local); err != nil {
		return PrecisionDecimalFacets{}, err
	}

	effective := PrecisionDecimalFacets{
		totalDigits: clonePrecisionDecimalTotalDigitsFacet(base.totalDigits),
		minScale:    clonePrecisionDecimalMinScaleFacet(base.minScale),
		maxScale:    clonePrecisionDecimalMaxScaleFacet(base.maxScale),
	}

	if local.TotalDigits != nil {
		if err := applyPrecisionDecimalTotalDigits(&effective, base, *local.TotalDigits, derived); err != nil {
			return PrecisionDecimalFacets{}, err
		}
	}
	if local.MinScale != nil {
		if err := applyPrecisionDecimalMinScale(&effective, base, *local.MinScale, derived); err != nil {
			return PrecisionDecimalFacets{}, err
		}
	}
	if local.MaxScale != nil {
		if err := applyPrecisionDecimalMaxScale(&effective, base, *local.MaxScale, derived); err != nil {
			return PrecisionDecimalFacets{}, err
		}
	}

	if err := validatePrecisionDecimalScaleBounds(effective, local, derived); err != nil {
		return PrecisionDecimalFacets{}, err
	}
	if err := effective.validate(); err != nil {
		return PrecisionDecimalFacets{}, err
	}
	return effective, nil
}

func applyPrecisionDecimalTotalDigits(effective *PrecisionDecimalFacets, base PrecisionDecimalFacets, local PrecisionDecimalTotalDigitsFacet, derived bool) error {
	if derived && base.totalDigits != nil {
		if err := validatePrecisionDecimalRestriction(
			precisionDecimalTotalDigitsKind,
			local.Value(),
			local.Loc(),
			base.totalDigits.Value(),
			base.totalDigits.Loc(),
			base.totalDigits.Fixed(),
			xsd11TotalDigitsRestrictionSpecRef,
			local.Value().Compare(base.totalDigits.Value()) > 0,
		); err != nil {
			return err
		}
	}
	effective.totalDigits = clonePrecisionDecimalTotalDigitsFacet(&local)
	if base.totalDigits != nil && base.totalDigits.Fixed() {
		effective.totalDigits.fixed = true
	}
	return nil
}

func applyPrecisionDecimalMinScale(effective *PrecisionDecimalFacets, base PrecisionDecimalFacets, local PrecisionDecimalMinScaleFacet, derived bool) error {
	if derived && base.minScale != nil {
		if err := validatePrecisionDecimalRestriction(
			precisionDecimalMinScaleKind,
			local.Value(),
			local.Loc(),
			base.minScale.Value(),
			base.minScale.Loc(),
			base.minScale.Fixed(),
			precisionDecimalMinScaleRestrictionSpecRef,
			local.Value().Compare(base.minScale.Value()) < 0,
		); err != nil {
			return err
		}
	}
	effective.minScale = clonePrecisionDecimalMinScaleFacet(&local)
	if base.minScale != nil && base.minScale.Fixed() {
		effective.minScale.fixed = true
	}
	return nil
}

func applyPrecisionDecimalMaxScale(effective *PrecisionDecimalFacets, base PrecisionDecimalFacets, local PrecisionDecimalMaxScaleFacet, derived bool) error {
	if derived && base.maxScale != nil {
		if err := validatePrecisionDecimalRestriction(
			precisionDecimalMaxScaleKind,
			local.Value(),
			local.Loc(),
			base.maxScale.Value(),
			base.maxScale.Loc(),
			base.maxScale.Fixed(),
			precisionDecimalMaxScaleRestrictionSpecRef,
			local.Value().Compare(base.maxScale.Value()) > 0,
		); err != nil {
			return err
		}
	}
	effective.maxScale = clonePrecisionDecimalMaxScaleFacet(&local)
	if base.maxScale != nil && base.maxScale.Fixed() {
		effective.maxScale.fixed = true
	}
	return nil
}

func validatePrecisionDecimalRestriction(
	kind precisionDecimalFacetKind,
	localValue StrictInteger,
	localLoc Loc,
	baseValue StrictInteger,
	baseLoc Loc,
	baseFixed bool,
	restrictionSpecRef string,
	monotonicityViolation bool,
) error {
	if baseFixed && !localValue.Equal(baseValue) {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalFacetRestrictionCode,
			localLoc,
			precisionDecimalFixedSpecRef(kind),
			"derived precisionDecimal facet changes a fixed base facet",
			precisionDecimalRelatedLocation(baseLoc),
			fmt.Errorf("%w: fixed facet value differs", errInvalidPrecisionDecimalFacetRestriction),
		)
	}
	if !monotonicityViolation {
		return nil
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalFacetRestrictionCode,
		localLoc,
		restrictionSpecRef,
		"derived precisionDecimal facet is less restrictive than its base facet",
		precisionDecimalRelatedLocation(baseLoc),
		fmt.Errorf("%w: derived value violates %s restriction", errInvalidPrecisionDecimalFacetRestriction, precisionDecimalFacetName(kind)),
	)
}

func validatePrecisionDecimalScaleBounds(facets PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations, derived bool) error {
	if facets.minScale == nil || facets.maxScale == nil {
		return nil
	}
	if facets.minScale.Value().Compare(facets.maxScale.Value()) <= 0 {
		return nil
	}

	primary := facets.minScale.Loc()
	related := facets.maxScale.Loc()
	if derived && local.MinScale == nil && local.MaxScale != nil {
		primary = facets.maxScale.Loc()
		related = facets.minScale.Loc()
	}
	primary, relatedLocations := precisionDecimalCrossFacetLocations(primary, related)
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalScaleCombinationCode,
		primary,
		precisionDecimalScaleCombinationSpecRef,
		"precisionDecimal minScale is greater than maxScale",
		relatedLocations,
		fmt.Errorf("%w: minScale exceeds maxScale", errInvalidPrecisionDecimalScaleCombination),
	)
}

func (facets PrecisionDecimalFacets) validate() error {
	if facets.totalDigits != nil {
		if err := validatePrecisionDecimalTotalDigitsState(*facets.totalDigits); err != nil {
			return err
		}
	}
	if facets.minScale == nil || facets.maxScale == nil {
		return nil
	}
	if facets.minScale.Value().Compare(facets.maxScale.Value()) <= 0 {
		return nil
	}
	primary, related := precisionDecimalCrossFacetLocations(facets.minScale.Loc(), facets.maxScale.Loc())
	return newPrecisionDecimalFacetDiagnostic(
		FailureInternal,
		InvalidPrecisionDecimalScaleCombinationCode,
		primary,
		precisionDecimalScaleCombinationSpecRef,
		"completed precisionDecimal facets have contradictory scale bounds",
		related,
		errInvalidPrecisionDecimalFacetState,
	)
}

func validatePrecisionDecimalLocalDeclarations(local PrecisionDecimalFacetDeclarations) error {
	if local.TotalDigits != nil {
		if err := validatePrecisionDecimalTotalDigitsState(*local.TotalDigits); err != nil {
			return err
		}
	}
	return nil
}

func validatePrecisionDecimalTotalDigitsState(facet PrecisionDecimalTotalDigitsFacet) error {
	if facet.value.Sign() > 0 {
		return nil
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInternal,
		InvalidPrecisionDecimalTotalDigitsCode,
		facet.Loc(),
		xsd11TotalDigitsValueSpecRef,
		"completed precisionDecimal totalDigits is not positive",
		nil,
		errInvalidPrecisionDecimalFacetState,
	)
}

func invalidPrecisionDecimalFacetValue(kind precisionDecimalFacetKind, loc Loc, lexical string, cause error) Diagnostic {
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		precisionDecimalFacetValueCode(kind),
		loc,
		precisionDecimalFacetValueSpecRef(kind),
		"invalid precisionDecimal "+precisionDecimalFacetName(kind)+" facet value",
		nil,
		fmt.Errorf("%w: %q", cause, lexical),
	)
}

func precisionDecimalIntegerCopy(value StrictInteger) StrictInteger {
	return StrictInteger{value: value.integerCopy()}
}

func clonePrecisionDecimalTotalDigitsFacet(facet *PrecisionDecimalTotalDigitsFacet) *PrecisionDecimalTotalDigitsFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = precisionDecimalIntegerCopy(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMinScaleFacet(facet *PrecisionDecimalMinScaleFacet) *PrecisionDecimalMinScaleFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = precisionDecimalIntegerCopy(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMaxScaleFacet(facet *PrecisionDecimalMaxScaleFacet) *PrecisionDecimalMaxScaleFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = precisionDecimalIntegerCopy(facet.value)
	return &facetCopy
}

func precisionDecimalFacetValueCause(kind precisionDecimalFacetKind) error {
	switch kind {
	case precisionDecimalTotalDigitsKind:
		return errInvalidPrecisionDecimalTotalDigitsValue
	case precisionDecimalMinScaleKind:
		return errInvalidPrecisionDecimalMinScaleValue
	case precisionDecimalMaxScaleKind:
		return errInvalidPrecisionDecimalMaxScaleValue
	default:
		return errInvalidPrecisionDecimalFacetState
	}
}

func precisionDecimalFacetValueCode(kind precisionDecimalFacetKind) string {
	switch kind {
	case precisionDecimalTotalDigitsKind:
		return InvalidPrecisionDecimalTotalDigitsCode
	case precisionDecimalMinScaleKind:
		return InvalidPrecisionDecimalMinScaleCode
	case precisionDecimalMaxScaleKind:
		return InvalidPrecisionDecimalMaxScaleCode
	default:
		return InvalidPrecisionDecimalDisallowedFacetCode
	}
}

func precisionDecimalFacetValueSpecRef(kind precisionDecimalFacetKind) string {
	switch kind {
	case precisionDecimalTotalDigitsKind:
		return precisionDecimalTotalDigitsSpecRef
	case precisionDecimalMinScaleKind:
		return precisionDecimalMinScaleValueSpecRef
	case precisionDecimalMaxScaleKind:
		return precisionDecimalMaxScaleValueSpecRef
	default:
		return precisionDecimalFacetSetSpecRef
	}
}

func precisionDecimalFixedSpecRef(kind precisionDecimalFacetKind) string {
	switch kind {
	case precisionDecimalTotalDigitsKind:
		return xsd11TotalDigitsFixedSpecRef
	case precisionDecimalMinScaleKind:
		return precisionDecimalMinScaleFixedSpecRef
	case precisionDecimalMaxScaleKind:
		return precisionDecimalMaxScaleFixedSpecRef
	default:
		return precisionDecimalFacetSetSpecRef
	}
}

func precisionDecimalFacetName(kind precisionDecimalFacetKind) string {
	switch kind {
	case precisionDecimalTotalDigitsKind:
		return "totalDigits"
	case precisionDecimalMinScaleKind:
		return "minScale"
	case precisionDecimalMaxScaleKind:
		return "maxScale"
	default:
		return "facet"
	}
}

func newPrecisionDecimalFacetDiagnostic(class FailureClass, code string, loc Loc, specRef, message string, related []Loc, cause error) Diagnostic {
	return Diagnostic{
		class:   class,
		code:    code,
		loc:     loc,
		message: message,
		related: append([]Loc(nil), related...),
		specRef: specRef,
		cause:   cause,
	}
}

func precisionDecimalRelatedLocation(loc Loc) []Loc {
	if loc.IsZero() {
		return nil
	}
	return []Loc{loc}
}

func precisionDecimalCrossFacetLocations(primary, related Loc) (Loc, []Loc) {
	if primary.IsZero() {
		return related, nil
	}
	if related.IsZero() || related == primary {
		return primary, nil
	}
	return primary, []Loc{related}
}
