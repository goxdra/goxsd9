package goxsd9

import (
	"errors"
	"fmt"
	"math/big"
)

const (
	// InvalidTotalDigitsCode identifies an invalid totalDigits facet value.
	InvalidTotalDigitsCode = "XSD2004"
	// InvalidFractionDigitsCode identifies an invalid fractionDigits facet value.
	InvalidFractionDigitsCode = "XSD2005"
	// InvalidDigitFacetCombinationCode identifies incompatible digit facets.
	InvalidDigitFacetCombinationCode = "XSD2006"
	// InvalidDigitFacetRestrictionCode identifies an invalid derived restriction.
	InvalidDigitFacetRestrictionCode = "XSD2007"
	// DigitFacetValueViolationCode identifies a value that violates a digit facet.
	DigitFacetValueViolationCode = "XSD2008"
)

var (
	errInvalidTotalDigitsValue      = errors.New("invalid totalDigits facet value")
	errInvalidFractionDigitsValue   = errors.New("invalid fractionDigits facet value")
	errInvalidDigitFacetCombination = errors.New("invalid totalDigits and fractionDigits combination")
	errInvalidDigitFacetRestriction = errors.New("invalid digit facet restriction")
	errDigitFacetValueViolation     = errors.New("digit facet value violation")
	errInvalidDigitFacetState       = errors.New("invalid completed digit facet state")
	errInvalidDigitFacetVersion     = errors.New("incompatible XSD version policy for digit facets")
)

type digitFacetKind uint8

const (
	totalDigitsKind digitFacetKind = iota + 1
	fractionDigitsKind
)

type digitFacetRule uint8

const (
	digitFacetDefinitionRule digitFacetRule = iota + 1
	digitFacetValueRule
	digitFacetCombinationRule
	digitFacetRestrictionRule
	digitFacetFixedRule
	digitFacetIntegerRule
)

// DigitDatatype identifies the exact numeric datatype for a completed digit
// facet set.
type DigitDatatype string

const (
	// DigitDatatypeDecimal identifies decimal and decimal-derived values.
	DigitDatatypeDecimal DigitDatatype = "decimal"
	// DigitDatatypeInteger identifies integer and integer-derived values.
	DigitDatatypeInteger DigitDatatype = "integer"
)

// TotalDigitsFacet is a parsed, immutable totalDigits declaration.
type TotalDigitsFacet struct {
	value   StrictInteger
	loc     Loc
	fixed   bool
	version XSDVersion
}

// Value returns the exact positiveInteger facet value.
func (facet TotalDigitsFacet) Value() StrictInteger {
	return cloneStrictInteger(facet.value)
}

// Loc returns the source location of the facet declaration.
func (facet TotalDigitsFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the facet is fixed for derived restrictions.
func (facet TotalDigitsFacet) Fixed() bool {
	return facet.fixed
}

// Version reports the XSD version policy used for facet diagnostics.
func (facet TotalDigitsFacet) Version() XSDVersion {
	return facet.version
}

// WithFixed returns a copy with the requested fixed property.
func (facet TotalDigitsFacet) WithFixed(fixed bool) TotalDigitsFacet {
	facet.fixed = fixed
	return facet
}

// FractionDigitsFacet is a parsed, immutable fractionDigits declaration.
type FractionDigitsFacet struct {
	value   StrictInteger
	loc     Loc
	fixed   bool
	version XSDVersion
}

// Value returns the exact nonNegativeInteger facet value.
func (facet FractionDigitsFacet) Value() StrictInteger {
	return cloneStrictInteger(facet.value)
}

// Loc returns the source location of the facet declaration.
func (facet FractionDigitsFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the facet is fixed for derived restrictions.
func (facet FractionDigitsFacet) Fixed() bool {
	return facet.fixed
}

// Version reports the XSD version policy used for facet diagnostics.
func (facet FractionDigitsFacet) Version() XSDVersion {
	return facet.version
}

// WithFixed returns a copy with the requested fixed property.
func (facet FractionDigitsFacet) WithFixed(fixed bool) FractionDigitsFacet {
	facet.fixed = fixed
	return facet
}

// ParseTotalDigits parses an exact positiveInteger totalDigits limit.
func ParseTotalDigits(lexical string, loc Loc, versions ...XSDVersion) (StrictInteger, error) {
	version, err := selectDigitFacetVersion(versions)
	if err != nil {
		return StrictInteger{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	return parseDigitLimit(totalDigitsKind, lexical, loc, version)
}

// ParseTotalDigitsFor parses a totalDigits limit with an explicit XSD version.
func ParseTotalDigitsFor(version XSDVersion, lexical string, loc Loc) (StrictInteger, error) {
	return ParseTotalDigits(lexical, loc, version)
}

// ParseFractionDigits parses an exact nonNegativeInteger fractionDigits limit.
func ParseFractionDigits(lexical string, loc Loc, versions ...XSDVersion) (StrictInteger, error) {
	version, err := selectDigitFacetVersion(versions)
	if err != nil {
		return StrictInteger{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	return parseDigitLimit(fractionDigitsKind, lexical, loc, version)
}

// ParseFractionDigitsFor parses a fractionDigits limit with an explicit XSD
// version.
func ParseFractionDigitsFor(version XSDVersion, lexical string, loc Loc) (StrictInteger, error) {
	return ParseFractionDigits(lexical, loc, version)
}

// ParseTotalDigitsFacet parses a totalDigits declaration with fixed=false.
func ParseTotalDigitsFacet(lexical string, loc Loc, versions ...XSDVersion) (TotalDigitsFacet, error) {
	return parseTotalDigitsFacet(lexical, loc, false, versions...)
}

// ParseTotalDigitsFacetFor parses a totalDigits declaration with an explicit
// XSD version.
func ParseTotalDigitsFacetFor(version XSDVersion, lexical string, loc Loc) (TotalDigitsFacet, error) {
	return parseTotalDigitsFacet(lexical, loc, false, version)
}

// ParseTotalDigitsFacetWithFixed parses a totalDigits declaration including
// its fixed property.
func ParseTotalDigitsFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (TotalDigitsFacet, error) {
	return parseTotalDigitsFacet(lexical, loc, fixed, versions...)
}

// ParseFractionDigitsFacet parses a fractionDigits declaration with fixed=false.
func ParseFractionDigitsFacet(lexical string, loc Loc, versions ...XSDVersion) (FractionDigitsFacet, error) {
	return parseFractionDigitsFacet(lexical, loc, false, versions...)
}

// ParseFractionDigitsFacetFor parses a fractionDigits declaration with an
// explicit XSD version.
func ParseFractionDigitsFacetFor(version XSDVersion, lexical string, loc Loc) (FractionDigitsFacet, error) {
	return parseFractionDigitsFacet(lexical, loc, false, version)
}

// ParseFractionDigitsFacetWithFixed parses a fractionDigits declaration
// including its fixed property.
func ParseFractionDigitsFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (FractionDigitsFacet, error) {
	return parseFractionDigitsFacet(lexical, loc, fixed, versions...)
}

// NewTotalDigitsFacet constructs a totalDigits declaration from a completed
// exact value.
func NewTotalDigitsFacet(value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (TotalDigitsFacet, error) {
	version, err := selectDigitFacetVersion(versions)
	if err != nil {
		return TotalDigitsFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	return newTotalDigitsFacet(value, loc, fixed, version)
}

// NewFractionDigitsFacet constructs a fractionDigits declaration from a
// completed exact value.
func NewFractionDigitsFacet(value StrictInteger, loc Loc, fixed bool, versions ...XSDVersion) (FractionDigitsFacet, error) {
	version, err := selectDigitFacetVersion(versions)
	if err != nil {
		return FractionDigitsFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	return newFractionDigitsFacet(value, loc, fixed, version)
}

func parseTotalDigitsFacet(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (TotalDigitsFacet, error) {
	version, err := selectDigitFacetVersion(versions)
	if err != nil {
		return TotalDigitsFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	value, err := parseDigitLimit(totalDigitsKind, lexical, loc, version)
	if err != nil {
		return TotalDigitsFacet{}, err
	}
	return newTotalDigitsFacet(value, loc, fixed, version)
}

func parseFractionDigitsFacet(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (FractionDigitsFacet, error) {
	version, err := selectDigitFacetVersion(versions)
	if err != nil {
		return FractionDigitsFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			err.Error(),
			err,
		)
	}
	value, err := parseDigitLimit(fractionDigitsKind, lexical, loc, version)
	if err != nil {
		return FractionDigitsFacet{}, err
	}
	return newFractionDigitsFacet(value, loc, fixed, version)
}

func parseDigitLimit(kind digitFacetKind, lexical string, loc Loc, version XSDVersion) (StrictInteger, error) {
	lexeme := collapseXMLWhitespace(lexical)
	parsed, ok := scanIntegerLexical(lexeme)
	if !ok {
		return StrictInteger{}, invalidDigitFacetValue(kind, loc, version, lexeme,
			fmt.Errorf("%w: lexical form is not an integer", digitFacetValueCause(kind)))
	}

	value, ok := newBigInteger(parsed.digits, parsed.negative)
	if !ok {
		return StrictInteger{}, newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9012",
			loc,
			digitFacetSpecRef(version, kind, digitFacetDefinitionRule),
			"valid digit facet value could not be constructed",
			nil,
			fmt.Errorf("%w: arbitrary-precision construction failed", digitFacetValueCause(kind)),
		)
	}

	if kind == totalDigitsKind && value.Sign() <= 0 {
		return StrictInteger{}, invalidDigitFacetValue(kind, loc, version, lexeme,
			fmt.Errorf("%w: value is not positive", errInvalidTotalDigitsValue))
	}
	if kind == fractionDigitsKind && value.Sign() < 0 {
		return StrictInteger{}, invalidDigitFacetValue(kind, loc, version, lexeme,
			fmt.Errorf("%w: value is negative", errInvalidFractionDigitsValue))
	}
	return StrictInteger{value: value}, nil
}

func newTotalDigitsFacet(value StrictInteger, loc Loc, fixed bool, version XSDVersion) (TotalDigitsFacet, error) {
	if value.Sign() <= 0 {
		return TotalDigitsFacet{}, invalidDigitFacetValue(totalDigitsKind, loc, version, value.Canonical(),
			fmt.Errorf("%w: value is not positive", errInvalidTotalDigitsValue))
	}
	return TotalDigitsFacet{value: cloneStrictInteger(value), loc: loc, fixed: fixed, version: version}, nil
}

func newFractionDigitsFacet(value StrictInteger, loc Loc, fixed bool, version XSDVersion) (FractionDigitsFacet, error) {
	if value.Sign() < 0 {
		return FractionDigitsFacet{}, invalidDigitFacetValue(fractionDigitsKind, loc, version, value.Canonical(),
			fmt.Errorf("%w: value is negative", errInvalidFractionDigitsValue))
	}
	return FractionDigitsFacet{value: cloneStrictInteger(value), loc: loc, fixed: fixed, version: version}, nil
}

// DigitFacetDeclarations are the completed local facet components supplied to
// a sequential effective-restriction construction phase. A nil field means
// that the local type omitted that facet.
type DigitFacetDeclarations struct {
	TotalDigits    *TotalDigitsFacet
	FractionDigits *FractionDigitsFacet
}

// NewDigitFacetDeclarations makes an owned copy of local facet declarations.
func NewDigitFacetDeclarations(totalDigits *TotalDigitsFacet, fractionDigits *FractionDigitsFacet) DigitFacetDeclarations {
	return DigitFacetDeclarations{
		TotalDigits:    cloneTotalDigitsFacet(totalDigits),
		FractionDigits: cloneFractionDigitsFacet(fractionDigits),
	}
}

// DigitFacets is an immutable effective totalDigits/fractionDigits set.
type DigitFacets struct {
	kind           DigitDatatype
	version        XSDVersion
	totalDigits    *TotalDigitsFacet
	fractionDigits *FractionDigitsFacet
}

// NewDigitFacets constructs complete effective facets for a decimal or
// integer datatype from completed local declarations.
func NewDigitFacets(kind DigitDatatype, totalDigits *TotalDigitsFacet, fractionDigits *FractionDigitsFacet, versions ...XSDVersion) (DigitFacets, error) {
	version, err := resolveDigitFacetVersion(versions, totalDigits, fractionDigits)
	if err != nil {
		loc := digitFacetVersionErrorLoc(totalDigits, fractionDigits)
		return DigitFacets{}, newDiagnostic(FailureInvalid, InvalidXSDVersionCode, loc, err.Error(), err)
	}
	if kind != DigitDatatypeDecimal && kind != DigitDatatypeInteger {
		return DigitFacets{}, newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9013",
			Loc{},
			"",
			"unknown exact numeric datatype for digit facets",
			nil,
			fmt.Errorf("%w: %q", errInvalidDigitFacetState, kind),
		)
	}
	base := DigitFacets{kind: kind, version: version}
	return completeDigitFacets(base, NewDigitFacetDeclarations(totalDigits, fractionDigits), false)
}

// NewDecimalDigitFacets constructs effective facets for decimal values.
func NewDecimalDigitFacets(totalDigits *TotalDigitsFacet, fractionDigits *FractionDigitsFacet, versions ...XSDVersion) (DigitFacets, error) {
	return NewDigitFacets(DigitDatatypeDecimal, totalDigits, fractionDigits, versions...)
}

// NewIntegerDigitFacets constructs effective facets for integer values. The
// fixed fractionDigits=0 facet is installed even when it was omitted locally.
func NewIntegerDigitFacets(totalDigits *TotalDigitsFacet, versions ...XSDVersion) (DigitFacets, error) {
	return NewDigitFacets(DigitDatatypeInteger, totalDigits, nil, versions...)
}

// RestrictDigitFacets inherits omitted facets from base and validates the
// completed local declarations as a restriction.
func RestrictDigitFacets(base DigitFacets, local DigitFacetDeclarations) (DigitFacets, error) {
	if err := base.validate(); err != nil {
		return DigitFacets{}, err
	}
	return completeDigitFacets(base, NewDigitFacetDeclarations(local.TotalDigits, local.FractionDigits), true)
}

// ConstructDigitFacets is the phase-oriented name for RestrictDigitFacets.
func ConstructDigitFacets(base DigitFacets, local DigitFacetDeclarations) (DigitFacets, error) {
	return RestrictDigitFacets(base, local)
}

// Kind reports whether the effective set belongs to decimal or integer.
func (facets DigitFacets) Kind() DigitDatatype {
	return facets.kind
}

// Version reports the explicit XSD version policy of the effective set.
func (facets DigitFacets) Version() XSDVersion {
	return facets.version
}

// HasTotalDigits reports whether an effective totalDigits facet exists.
func (facets DigitFacets) HasTotalDigits() bool {
	return facets.totalDigits != nil
}

// TotalDigits returns the effective totalDigits value and its presence.
func (facets DigitFacets) TotalDigits() (StrictInteger, bool) {
	if facets.totalDigits == nil {
		return StrictInteger{}, false
	}
	return facets.totalDigits.Value(), true
}

// TotalDigitsLoc returns the source location and presence of totalDigits.
func (facets DigitFacets) TotalDigitsLoc() (Loc, bool) {
	if facets.totalDigits == nil {
		return Loc{}, false
	}
	return facets.totalDigits.Loc(), true
}

// TotalDigitsFixed returns the fixed property and presence of totalDigits.
func (facets DigitFacets) TotalDigitsFixed() (bool, bool) {
	if facets.totalDigits == nil {
		return false, false
	}
	return facets.totalDigits.Fixed(), true
}

// HasFractionDigits reports whether an effective fractionDigits facet exists.
func (facets DigitFacets) HasFractionDigits() bool {
	return facets.fractionDigits != nil
}

// FractionDigits returns the effective fractionDigits value and its presence.
func (facets DigitFacets) FractionDigits() (StrictInteger, bool) {
	if facets.fractionDigits == nil {
		return StrictInteger{}, false
	}
	return facets.fractionDigits.Value(), true
}

// FractionDigitsLoc returns the source location and presence of
// fractionDigits.
func (facets DigitFacets) FractionDigitsLoc() (Loc, bool) {
	if facets.fractionDigits == nil {
		return Loc{}, false
	}
	return facets.fractionDigits.Loc(), true
}

// FractionDigitsFixed returns the fixed property and presence of
// fractionDigits.
func (facets DigitFacets) FractionDigitsFixed() (bool, bool) {
	if facets.fractionDigits == nil {
		return false, false
	}
	return facets.fractionDigits.Fixed(), true
}

// ValidateDecimal validates an exact decimal value against the effective
// digit facets. valueLoc is the primary location for a value violation.
func (facets DigitFacets) ValidateDecimal(value StrictDecimal, valueLoc Loc) error {
	return facets.validateCounts(
		exactDigitCountFromInt(value.TotalDigits()),
		exactDigitCountFromInt(value.FractionDigits()),
		valueLoc,
	)
}

// ValidateInteger validates an exact integer value against the effective
// digit facets. Its exact fractionDigits count is always zero.
func (facets DigitFacets) ValidateInteger(value StrictInteger, valueLoc Loc) error {
	return facets.validateCounts(value.digitCount(), exactDigitCountFromInt(0), valueLoc)
}

// ValidateDecimalFacets validates an exact decimal value against effective
// digit facets.
func ValidateDecimalFacets(value StrictDecimal, facets DigitFacets, valueLoc Loc) error {
	return facets.ValidateDecimal(value, valueLoc)
}

// ValidateIntegerFacets validates an exact integer value against effective
// digit facets.
func ValidateIntegerFacets(value StrictInteger, facets DigitFacets, valueLoc Loc) error {
	return facets.ValidateInteger(value, valueLoc)
}

func (facets DigitFacets) validate() error {
	if facets.kind != DigitDatatypeDecimal && facets.kind != DigitDatatypeInteger {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9014",
			Loc{},
			"",
			"completed digit facets have an unknown datatype",
			nil,
			errInvalidDigitFacetState,
		)
	}
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9015",
			Loc{},
			"",
			"completed digit facets have an unknown XSD version",
			nil,
			errInvalidDigitFacetState,
		)
	}
	if facets.totalDigits != nil {
		if err := validateTotalDigitsFacet(*facets.totalDigits, facets.version); err != nil {
			return err
		}
	}
	if facets.fractionDigits != nil {
		if err := validateFractionDigitsFacet(*facets.fractionDigits, facets.version); err != nil {
			return err
		}
	}
	if facets.kind == DigitDatatypeInteger && facets.fractionDigits == nil {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9016",
			Loc{},
			digitFacetSpecRef(facets.version, fractionDigitsKind, digitFacetIntegerRule),
			"integer digit facets do not contain their fixed fractionDigits facet",
			nil,
			errInvalidDigitFacetState,
		)
	}
	return validateDigitFacetCombination(facets.totalDigits, facets.fractionDigits, facets.version)
}

func (facets DigitFacets) validateCounts(totalDigits, fractionDigits StrictInteger, valueLoc Loc) error {
	if err := facets.validate(); err != nil {
		return err
	}
	if facets.totalDigits != nil && totalDigits.Compare(facets.totalDigits.value) > 0 {
		related := facetRelatedLocations(facets.totalDigits.Loc())
		return newDigitFacetDiagnostic(
			FailureInvalid,
			DigitFacetValueViolationCode,
			valueLoc,
			digitFacetSpecRef(facets.version, totalDigitsKind, digitFacetValueRule),
			"value violates totalDigits",
			related,
			fmt.Errorf("%w: totalDigits value is too large", errDigitFacetValueViolation),
		)
	}
	if facets.fractionDigits != nil && fractionDigits.Compare(facets.fractionDigits.value) > 0 {
		related := facetRelatedLocations(facets.fractionDigits.Loc())
		return newDigitFacetDiagnostic(
			FailureInvalid,
			DigitFacetValueViolationCode,
			valueLoc,
			digitFacetSpecRef(facets.version, fractionDigitsKind, digitFacetValueRule),
			"value violates fractionDigits",
			related,
			fmt.Errorf("%w: fractionDigits value is too large", errDigitFacetValueViolation),
		)
	}
	return nil
}

func completeDigitFacets(base DigitFacets, local DigitFacetDeclarations, derived bool) (DigitFacets, error) {
	if err := base.validateForConstruction(); err != nil {
		return DigitFacets{}, err
	}
	if err := validateLocalFacetVersions(local, base.version); err != nil {
		return DigitFacets{}, err
	}

	effective := DigitFacets{
		kind:    base.kind,
		version: base.version,
	}
	effective.totalDigits = cloneTotalDigitsFacet(base.totalDigits)
	effective.fractionDigits = cloneFractionDigitsFacet(base.fractionDigits)

	if effective.kind == DigitDatatypeInteger && effective.fractionDigits == nil {
		effective.fractionDigits = integerFractionDigitsFacet(base.version)
	}

	if local.TotalDigits != nil {
		if err := applyLocalTotalDigits(&effective, base, *local.TotalDigits, derived); err != nil {
			return DigitFacets{}, err
		}
	}

	if local.FractionDigits != nil {
		if err := applyLocalFractionDigits(&effective, base, *local.FractionDigits, derived); err != nil {
			return DigitFacets{}, err
		}
	}

	if err := validateDigitFacetCombination(effective.totalDigits, effective.fractionDigits, base.version); err != nil {
		return DigitFacets{}, err
	}
	if err := effective.validate(); err != nil {
		return DigitFacets{}, err
	}
	return effective, nil
}

func applyLocalTotalDigits(effective *DigitFacets, base DigitFacets, local TotalDigitsFacet, derived bool) error {
	if err := validateTotalDigitsFacet(local, base.version); err != nil {
		return err
	}
	if derived && base.totalDigits != nil {
		if err := validateFacetRestriction(totalDigitsKind, local, *base.totalDigits, base.version); err != nil {
			return err
		}
	}
	effective.totalDigits = cloneTotalDigitsFacet(&local)
	if base.totalDigits != nil && base.totalDigits.Fixed() {
		effective.totalDigits.fixed = true
	}
	return nil
}

func applyLocalFractionDigits(effective *DigitFacets, base DigitFacets, local FractionDigitsFacet, derived bool) error {
	if err := validateFractionDigitsFacet(local, base.version); err != nil {
		return err
	}
	if effective.kind == DigitDatatypeInteger && local.Value().Sign() != 0 {
		return integerFractionDigitsDiagnostic(local, base.version)
	}
	if derived && base.fractionDigits != nil {
		if err := validateFacetRestriction(fractionDigitsKind, local, *base.fractionDigits, base.version); err != nil {
			return err
		}
	}
	effective.fractionDigits = cloneFractionDigitsFacet(&local)
	if effective.kind == DigitDatatypeInteger {
		effective.fractionDigits.fixed = true
	}
	if base.fractionDigits != nil && base.fractionDigits.Fixed() {
		effective.fractionDigits.fixed = true
	}
	return nil
}

func (facets DigitFacets) validateForConstruction() error {
	if facets.kind != DigitDatatypeDecimal && facets.kind != DigitDatatypeInteger {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9017",
			Loc{},
			"",
			"cannot construct a digit restriction from an unknown datatype",
			nil,
			errInvalidDigitFacetState,
		)
	}
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9018",
			Loc{},
			"",
			"cannot construct a digit restriction from an unknown XSD version",
			nil,
			errInvalidDigitFacetState,
		)
	}
	if facets.totalDigits != nil {
		if err := validateTotalDigitsFacet(*facets.totalDigits, facets.version); err != nil {
			return err
		}
	}
	if facets.fractionDigits != nil {
		if err := validateFractionDigitsFacet(*facets.fractionDigits, facets.version); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalFacetVersions(local DigitFacetDeclarations, version XSDVersion) error {
	if local.TotalDigits != nil && local.TotalDigits.Version() != version {
		return newDigitFacetDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			local.TotalDigits.Loc(),
			digitFacetSpecRef(version, totalDigitsKind, digitFacetDefinitionRule),
			"digit facet version does not match its base type policy",
			nil,
			errInvalidDigitFacetVersion,
		)
	}
	if local.FractionDigits != nil && local.FractionDigits.Version() != version {
		return newDigitFacetDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			local.FractionDigits.Loc(),
			digitFacetSpecRef(version, fractionDigitsKind, digitFacetDefinitionRule),
			"digit facet version does not match its base type policy",
			nil,
			errInvalidDigitFacetVersion,
		)
	}
	return nil
}

func validateTotalDigitsFacet(facet TotalDigitsFacet, version XSDVersion) error {
	if facet.Version() != version || facet.value.Sign() <= 0 {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9019",
			facet.Loc(),
			digitFacetSpecRef(version, totalDigitsKind, digitFacetDefinitionRule),
			"completed totalDigits facet is not a positiveInteger in the selected version",
			nil,
			errInvalidDigitFacetState,
		)
	}
	return nil
}

func validateFractionDigitsFacet(facet FractionDigitsFacet, version XSDVersion) error {
	if facet.Version() != version || facet.value.Sign() < 0 {
		return newDigitFacetDiagnostic(
			FailureInternal,
			"GOXSD9020",
			facet.Loc(),
			digitFacetSpecRef(version, fractionDigitsKind, digitFacetDefinitionRule),
			"completed fractionDigits facet is not a nonNegativeInteger in the selected version",
			nil,
			errInvalidDigitFacetState,
		)
	}
	return nil
}

func validateFacetRestriction(kind digitFacetKind, local, base interface {
	Value() StrictInteger
	Loc() Loc
	Fixed() bool
}, version XSDVersion) error {
	if base.Fixed() && !local.Value().Equal(base.Value()) {
		return fixedFacetDiagnostic(kind, local, base, version)
	}
	if local.Value().Compare(base.Value()) > 0 {
		return newDigitFacetDiagnostic(
			FailureInvalid,
			InvalidDigitFacetRestrictionCode,
			local.Loc(),
			digitFacetSpecRef(version, kind, digitFacetRestrictionRule),
			"derived digit facet is less restrictive than its base facet",
			facetRelatedLocations(base.Loc()),
			fmt.Errorf("%w: derived value exceeds base value", errInvalidDigitFacetRestriction),
		)
	}
	return nil
}

func fixedFacetDiagnostic(kind digitFacetKind, local, base interface {
	Value() StrictInteger
	Loc() Loc
	Fixed() bool
}, version XSDVersion) error {
	return newDigitFacetDiagnostic(
		FailureInvalid,
		InvalidDigitFacetRestrictionCode,
		local.Loc(),
		digitFacetSpecRef(version, kind, digitFacetFixedRule),
		"derived digit facet changes a fixed base facet",
		facetRelatedLocations(base.Loc()),
		fmt.Errorf("%w: fixed facet value differs", errInvalidDigitFacetRestriction),
	)
}

func validateDigitFacetCombination(total *TotalDigitsFacet, fraction *FractionDigitsFacet, version XSDVersion) error {
	if total == nil || fraction == nil {
		return nil
	}
	if fraction.Value().Compare(total.Value()) <= 0 {
		return nil
	}
	primary, related := crossFacetLocations(fraction.Loc(), total.Loc())
	return newDigitFacetDiagnostic(
		FailureInvalid,
		InvalidDigitFacetCombinationCode,
		primary,
		digitFacetSpecRef(version, fractionDigitsKind, digitFacetCombinationRule),
		"fractionDigits is greater than totalDigits",
		related,
		fmt.Errorf("%w: fractionDigits exceeds totalDigits", errInvalidDigitFacetCombination),
	)
}

func integerFractionDigitsDiagnostic(facet FractionDigitsFacet, version XSDVersion) error {
	return newDigitFacetDiagnostic(
		FailureInvalid,
		InvalidDigitFacetRestrictionCode,
		facet.Loc(),
		digitFacetSpecRef(version, fractionDigitsKind, digitFacetIntegerRule),
		"integer fixes fractionDigits to zero",
		nil,
		fmt.Errorf("%w: integer fractionDigits is nonzero", errInvalidDigitFacetRestriction),
	)
}

func integerFractionDigitsFacet(version XSDVersion) *FractionDigitsFacet {
	return &FractionDigitsFacet{
		value:   StrictInteger{value: big.NewInt(0)},
		fixed:   true,
		version: version,
	}
}

func (value StrictInteger) digitCount() StrictInteger {
	magnitude := value.integerCopy()
	if magnitude.Sign() < 0 {
		magnitude.Neg(magnitude)
	}
	if magnitude.Sign() == 0 {
		return StrictInteger{value: big.NewInt(1)}
	}

	count := new(big.Int)
	one := big.NewInt(1)
	ten := big.NewInt(10)
	for magnitude.Sign() > 0 {
		magnitude.Quo(magnitude, ten)
		count.Add(count, one)
	}
	return StrictInteger{value: count}
}

func exactDigitCountFromInt(count int) StrictInteger {
	return StrictInteger{value: big.NewInt(int64(count))}
}

func cloneStrictInteger(value StrictInteger) StrictInteger {
	return StrictInteger{value: value.integerCopy()}
}

func cloneTotalDigitsFacet(facet *TotalDigitsFacet) *TotalDigitsFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = cloneStrictInteger(facet.value)
	return &facetCopy
}

func cloneFractionDigitsFacet(facet *FractionDigitsFacet) *FractionDigitsFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = cloneStrictInteger(facet.value)
	return &facetCopy
}

func selectDigitFacetVersion(versions []XSDVersion) (XSDVersion, error) {
	return selectXSDVersion(versions)
}

func resolveDigitFacetVersion(versions []XSDVersion, total *TotalDigitsFacet, fraction *FractionDigitsFacet) (XSDVersion, error) {
	if len(versions) != 0 {
		return selectDigitFacetVersion(versions)
	}
	if total != nil && fraction != nil && total.Version() != fraction.Version() {
		return "", fmt.Errorf("%w: totalDigits uses %q and fractionDigits uses %q", errInvalidDigitFacetVersion, total.Version(), fraction.Version())
	}
	if total != nil {
		return total.Version(), nil
	}
	if fraction != nil {
		return fraction.Version(), nil
	}
	return XSDVersion11, nil
}

func digitFacetVersionErrorLoc(total *TotalDigitsFacet, fraction *FractionDigitsFacet) Loc {
	if total != nil {
		return total.Loc()
	}
	if fraction != nil {
		return fraction.Loc()
	}
	return Loc{}
}

func digitFacetValueCause(kind digitFacetKind) error {
	if kind == totalDigitsKind {
		return errInvalidTotalDigitsValue
	}
	return errInvalidFractionDigitsValue
}

func invalidDigitFacetValue(kind digitFacetKind, loc Loc, version XSDVersion, lexical string, cause error) Diagnostic {
	return newDigitFacetDiagnostic(
		FailureInvalid,
		digitFacetCode(kind),
		loc,
		digitFacetSpecRef(version, kind, digitFacetDefinitionRule),
		"invalid "+digitFacetName(kind)+" facet value",
		nil,
		fmt.Errorf("%w: %q", cause, lexical),
	)
}

func digitFacetCode(kind digitFacetKind) string {
	if kind == totalDigitsKind {
		return InvalidTotalDigitsCode
	}
	return InvalidFractionDigitsCode
}

func digitFacetName(kind digitFacetKind) string {
	if kind == totalDigitsKind {
		return "totalDigits"
	}
	return "fractionDigits"
}

func digitFacetSpecRef(version XSDVersion, kind digitFacetKind, rule digitFacetRule) string {
	if version == XSDVersion10 {
		return digitFacetSpecRef10(kind, rule)
	}
	if version == XSDVersion11 {
		return digitFacetSpecRef11(kind, rule)
	}
	return ""
}

func digitFacetSpecRef10(kind digitFacetKind, rule digitFacetRule) string {
	if kind == totalDigitsKind {
		switch rule {
		case digitFacetDefinitionRule:
			return "xsd10-datatypes#rf-totalDigits"
		case digitFacetValueRule:
			return "xsd10-datatypes#cvc-totalDigits-valid"
		case digitFacetRestrictionRule:
			return "xsd10-datatypes#totalDigits-valid-restriction"
		case digitFacetFixedRule:
			return "xsd10-datatypes#totalDigits-fixed"
		case digitFacetCombinationRule, digitFacetIntegerRule:
			return "xsd10-datatypes#rf-totalDigits"
		}
		return "xsd10-datatypes#rf-totalDigits"
	}
	switch rule {
	case digitFacetDefinitionRule:
		return "xsd10-datatypes#rf-fractionDigits"
	case digitFacetValueRule:
		return "xsd10-datatypes#cvc-fractionDigits-valid"
	case digitFacetCombinationRule:
		return "xsd10-datatypes#fractionDigits-totalDigits"
	case digitFacetRestrictionRule:
		return "xsd10-datatypes#fractionDigits-valid-restriction"
	case digitFacetFixedRule:
		return "xsd10-datatypes#fractionDigits-fixed"
	case digitFacetIntegerRule:
		return "xsd10-datatypes#integer.fractionDigits"
	}
	return "xsd10-datatypes#rf-fractionDigits"
}

func digitFacetSpecRef11(kind digitFacetKind, rule digitFacetRule) string {
	if kind == totalDigitsKind {
		switch rule {
		case digitFacetDefinitionRule:
			return "xsd11-datatypes#rf-totalDigits"
		case digitFacetValueRule:
			return "xsd11-datatypes#cvc-totalDigits-valid"
		case digitFacetRestrictionRule:
			return "xsd11-datatypes#totalDigits-valid-restriction"
		case digitFacetFixedRule:
			return "xsd11-datatypes#f-td-fixed"
		case digitFacetCombinationRule, digitFacetIntegerRule:
			return "xsd11-datatypes#rf-totalDigits"
		}
		return "xsd11-datatypes#rf-totalDigits"
	}
	switch rule {
	case digitFacetDefinitionRule:
		return "xsd11-datatypes#rf-fractionDigits"
	case digitFacetValueRule:
		return "xsd11-datatypes#cvc-fractionDigits-valid"
	case digitFacetCombinationRule:
		return "xsd11-datatypes#fractionDigits-totalDigits"
	case digitFacetRestrictionRule:
		return "xsd11-datatypes#fractionDigits-valid-restriction"
	case digitFacetFixedRule:
		return "xsd11-datatypes#f-fd-fixed"
	case digitFacetIntegerRule:
		return "xsd11-datatypes#integer.fractionDigits"
	}
	return "xsd11-datatypes#rf-fractionDigits"
}

func newDigitFacetDiagnostic(class FailureClass, code string, loc Loc, specRef, message string, related []Loc, cause error) Diagnostic {
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

func facetRelatedLocations(loc Loc) []Loc {
	if loc.IsZero() {
		return nil
	}
	return []Loc{loc}
}

func crossFacetLocations(primary, related Loc) (Loc, []Loc) {
	if primary.IsZero() {
		return related, nil
	}
	if related.IsZero() || related == primary {
		return primary, nil
	}
	return primary, []Loc{related}
}
