package goxsd9

import (
	"errors"
	"fmt"
	"math/big"
)

const (
	// PrecisionDecimalFacetValueViolationCode identifies a value that fails an
	// effective precisionDecimal value facet.
	PrecisionDecimalFacetValueViolationCode = "XSD2020"
	// InvalidPrecisionDecimalPatternCode identifies an invalid XML Schema
	// regular-expression facet value.
	InvalidPrecisionDecimalPatternCode = "XSD2021"
	// InvalidPrecisionDecimalWhiteSpaceCode identifies an invalid fixed
	// precisionDecimal whiteSpace declaration.
	InvalidPrecisionDecimalWhiteSpaceCode = "XSD2022"
	// InvalidPrecisionDecimalBoundCombinationCode identifies contradictory
	// precisionDecimal ordered bound declarations.
	InvalidPrecisionDecimalBoundCombinationCode = "XSD2023"
	// InvalidPrecisionDecimalEnumerationCode identifies an invalid enumeration
	// declaration value.
	InvalidPrecisionDecimalEnumerationCode = "XSD2024"
	// InvalidPrecisionDecimalBoundCode identifies an invalid ordered bound
	// declaration value.
	InvalidPrecisionDecimalBoundCode = "XSD2025"
)

const (
	precisionDecimalPatternValueSpecRef             = "xsd11-datatypes#f-pattern-value"
	precisionDecimalEnumerationValueSpecRef         = "xsd11-datatypes#f-enumeration-value"
	precisionDecimalEnumerationValidSpecRef         = "xsd11-datatypes#cvc-enumeration-valid"
	precisionDecimalPatternValidSpecRef             = "xsd11-datatypes#cvc-pattern-valid"
	precisionDecimalMinInclusiveValueSpecRef        = "xsd11-datatypes#f-minInclusive-value"
	precisionDecimalMinExclusiveValueSpecRef        = "xsd11-datatypes#f-minExclusive-value"
	precisionDecimalMaxInclusiveValueSpecRef        = "xsd11-datatypes#f-maxInclusive-value"
	precisionDecimalMaxExclusiveValueSpecRef        = "xsd11-datatypes#f-maxExclusive-value"
	precisionDecimalMinInclusiveFixedSpecRef        = "xsd11-datatypes#f-minInclusive-fixed"
	precisionDecimalMinExclusiveFixedSpecRef        = "xsd11-datatypes#f-minExclusive-fixed"
	precisionDecimalMaxInclusiveFixedSpecRef        = "xsd11-datatypes#f-maxInclusive-fixed"
	precisionDecimalMaxExclusiveFixedSpecRef        = "xsd11-datatypes#f-maxExclusive-fixed"
	precisionDecimalMinInclusiveRestrictionSpecRef  = "xsd11-datatypes#minInclusive-valid-restriction"
	precisionDecimalMinExclusiveRestrictionSpecRef  = "xsd11-datatypes#minExclusive-valid-restriction"
	precisionDecimalMaxInclusiveRestrictionSpecRef  = "xsd11-datatypes#maxInclusive-valid-restriction"
	precisionDecimalMaxExclusiveRestrictionSpecRef  = "xsd11-datatypes#maxExclusive-valid-restriction"
	precisionDecimalMinInclusiveMaxInclusiveSpecRef = "xsd11-datatypes#minInclusive-maxInclusive"
	precisionDecimalMinInclusiveMaxExclusiveSpecRef = "xsd11-datatypes#minInclusive-maxExclusive"
	precisionDecimalMinExclusiveMaxInclusiveSpecRef = "xsd11-datatypes#minExclusive-maxInclusive"
	precisionDecimalMinExclusiveMaxExclusiveSpecRef = "xsd11-datatypes#minExclusive-maxExclusive"
	precisionDecimalMinInclusiveMinExclusiveSpecRef = "xsd11-datatypes#minInclusive-minExclusive"
	precisionDecimalMaxInclusiveMaxExclusiveSpecRef = "xsd11-datatypes#maxInclusive-maxExclusive"
	precisionDecimalMinInclusiveValidSpecRef        = "xsd11-datatypes#cvc-minInclusive-valid"
	precisionDecimalMinExclusiveValidSpecRef        = "xsd11-datatypes#cvc-minExclusive-valid"
	precisionDecimalMaxInclusiveValidSpecRef        = "xsd11-datatypes#cvc-maxInclusive-valid"
	precisionDecimalMaxExclusiveValidSpecRef        = "xsd11-datatypes#cvc-maxExclusive-valid"
	precisionDecimalWhiteSpaceValueSpecRef          = "xsd-precisionDecimal#f-whiteSpace-value"
	precisionDecimalWhiteSpaceFixedSpecRef          = "xsd-precisionDecimal#f-whiteSpace-fixed"
	precisionDecimalBoundRestrictionSpecRef         = "xsd11-datatypes#ordered-facet-valid-restriction"
	precisionDecimalEnumerationRestrictionSpecRef   = "xsd11-datatypes#enumeration-valid-restriction"
)

var (
	errPrecisionDecimalFacetValueViolation     = errors.New("precisionDecimal facet value violation")
	errInvalidPrecisionDecimalPattern          = errors.New("invalid precisionDecimal pattern")
	errInvalidPrecisionDecimalWhiteSpace       = errors.New("invalid precisionDecimal whiteSpace facet")
	errInvalidPrecisionDecimalBound            = errors.New("invalid precisionDecimal ordered bound")
	errInvalidPrecisionDecimalEnumeration      = errors.New("invalid precisionDecimal enumeration facet")
	errInvalidPrecisionDecimalBoundCombination = errors.New("invalid precisionDecimal ordered bound combination")
)

// PrecisionDecimalPatternFacet is a parsed XML Schema regular-expression
// pattern declaration. Patterns in one declaration step are alternatives.
type PrecisionDecimalPatternFacet struct {
	source     string
	expression *precisionDecimalXMLRegex
	loc        Loc
}

// Value returns the declared XML Schema regular-expression source.
func (facet PrecisionDecimalPatternFacet) Value() string {
	return facet.source
}

// Loc returns the source location of the pattern declaration.
func (facet PrecisionDecimalPatternFacet) Loc() Loc {
	return facet.loc
}

// PrecisionDecimalEnumerationFacet is a parsed exact enumeration member.
// Its value representation remains private to the datatype layer.
type PrecisionDecimalEnumerationFacet struct {
	value precisionDecimalValue
	loc   Loc
}

// Loc returns the source location of the enumeration declaration.
func (facet PrecisionDecimalEnumerationFacet) Loc() Loc {
	return facet.loc
}

// PrecisionDecimalMinInclusiveFacet is a parsed inclusive lower bound.
type PrecisionDecimalMinInclusiveFacet struct {
	value precisionDecimalValue
	loc   Loc
	fixed bool
}

// Loc returns the source location of the bound declaration.
func (facet PrecisionDecimalMinInclusiveFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the bound is fixed for derived restrictions.
func (facet PrecisionDecimalMinInclusiveFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalMinInclusiveFacet) WithFixed(fixed bool) PrecisionDecimalMinInclusiveFacet {
	facet.fixed = fixed
	return facet
}

// PrecisionDecimalMinExclusiveFacet is a parsed exclusive lower bound.
type PrecisionDecimalMinExclusiveFacet struct {
	value precisionDecimalValue
	loc   Loc
	fixed bool
}

// Loc returns the source location of the bound declaration.
func (facet PrecisionDecimalMinExclusiveFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the bound is fixed for derived restrictions.
func (facet PrecisionDecimalMinExclusiveFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalMinExclusiveFacet) WithFixed(fixed bool) PrecisionDecimalMinExclusiveFacet {
	facet.fixed = fixed
	return facet
}

// PrecisionDecimalMaxInclusiveFacet is a parsed inclusive upper bound.
type PrecisionDecimalMaxInclusiveFacet struct {
	value precisionDecimalValue
	loc   Loc
	fixed bool
}

// Loc returns the source location of the bound declaration.
func (facet PrecisionDecimalMaxInclusiveFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the bound is fixed for derived restrictions.
func (facet PrecisionDecimalMaxInclusiveFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalMaxInclusiveFacet) WithFixed(fixed bool) PrecisionDecimalMaxInclusiveFacet {
	facet.fixed = fixed
	return facet
}

// PrecisionDecimalMaxExclusiveFacet is a parsed exclusive upper bound.
type PrecisionDecimalMaxExclusiveFacet struct {
	value precisionDecimalValue
	loc   Loc
	fixed bool
}

// Loc returns the source location of the bound declaration.
func (facet PrecisionDecimalMaxExclusiveFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the bound is fixed for derived restrictions.
func (facet PrecisionDecimalMaxExclusiveFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet PrecisionDecimalMaxExclusiveFacet) WithFixed(fixed bool) PrecisionDecimalMaxExclusiveFacet {
	facet.fixed = fixed
	return facet
}

// PrecisionDecimalWhiteSpaceFacet is the fixed collapse whiteSpace
// declaration for precisionDecimal.
type PrecisionDecimalWhiteSpaceFacet struct {
	value string
	loc   Loc
	fixed bool
}

// Value returns the declared whiteSpace value.
func (facet PrecisionDecimalWhiteSpaceFacet) Value() string {
	return facet.value
}

// Loc returns the source location of the whiteSpace declaration.
func (facet PrecisionDecimalWhiteSpaceFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the whiteSpace facet is fixed.
func (facet PrecisionDecimalWhiteSpaceFacet) Fixed() bool {
	return facet.fixed
}

type precisionDecimalBoundKind uint8

const (
	precisionDecimalMinInclusiveBoundKind precisionDecimalBoundKind = iota + 1
	precisionDecimalMinExclusiveBoundKind
	precisionDecimalMaxInclusiveBoundKind
	precisionDecimalMaxExclusiveBoundKind
)

type precisionDecimalBoundRecord struct {
	kind  precisionDecimalBoundKind
	value precisionDecimalValue
	loc   Loc
}

// ParsePrecisionDecimalPatternFacet parses an XML Schema regular expression.
func ParsePrecisionDecimalPatternFacet(pattern string, loc Loc) (PrecisionDecimalPatternFacet, error) {
	expression, err := parsePrecisionDecimalXMLRegex(pattern)
	if err == nil {
		return PrecisionDecimalPatternFacet{
			source:     pattern,
			expression: expression,
			loc:        loc,
		}, nil
	}
	return PrecisionDecimalPatternFacet{}, newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalPatternCode,
		loc,
		precisionDecimalPatternValueSpecRef,
		"invalid precisionDecimal XML Schema pattern",
		nil,
		fmt.Errorf("%w: %w", errInvalidPrecisionDecimalPattern, err),
	)
}

// ParsePrecisionDecimalEnumerationFacet parses one exact enumeration member.
func ParsePrecisionDecimalEnumerationFacet(lexical string, loc Loc) (PrecisionDecimalEnumerationFacet, error) {
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		return PrecisionDecimalEnumerationFacet{}, newPrecisionDecimalEnumerationDiagnostic(loc, err)
	}
	return PrecisionDecimalEnumerationFacet{value: clonePrecisionDecimalValue(value), loc: loc}, nil
}

// ParsePrecisionDecimalMinInclusiveFacet parses an inclusive lower bound.
func ParsePrecisionDecimalMinInclusiveFacet(lexical string, loc Loc) (PrecisionDecimalMinInclusiveFacet, error) {
	return parsePrecisionDecimalMinInclusiveFacet(lexical, loc, false)
}

// ParsePrecisionDecimalMinInclusiveFacetWithFixed parses an inclusive lower
// bound including its fixed property.
func ParsePrecisionDecimalMinInclusiveFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinInclusiveFacet, error) {
	return parsePrecisionDecimalMinInclusiveFacet(lexical, loc, fixed)
}

// ParsePrecisionDecimalMinExclusiveFacet parses an exclusive lower bound.
func ParsePrecisionDecimalMinExclusiveFacet(lexical string, loc Loc) (PrecisionDecimalMinExclusiveFacet, error) {
	return parsePrecisionDecimalMinExclusiveFacet(lexical, loc, false)
}

// ParsePrecisionDecimalMinExclusiveFacetWithFixed parses an exclusive lower
// bound including its fixed property.
func ParsePrecisionDecimalMinExclusiveFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinExclusiveFacet, error) {
	return parsePrecisionDecimalMinExclusiveFacet(lexical, loc, fixed)
}

// ParsePrecisionDecimalMaxInclusiveFacet parses an inclusive upper bound.
func ParsePrecisionDecimalMaxInclusiveFacet(lexical string, loc Loc) (PrecisionDecimalMaxInclusiveFacet, error) {
	return parsePrecisionDecimalMaxInclusiveFacet(lexical, loc, false)
}

// ParsePrecisionDecimalMaxInclusiveFacetWithFixed parses an inclusive upper
// bound including its fixed property.
func ParsePrecisionDecimalMaxInclusiveFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxInclusiveFacet, error) {
	return parsePrecisionDecimalMaxInclusiveFacet(lexical, loc, fixed)
}

// ParsePrecisionDecimalMaxExclusiveFacet parses an exclusive upper bound.
func ParsePrecisionDecimalMaxExclusiveFacet(lexical string, loc Loc) (PrecisionDecimalMaxExclusiveFacet, error) {
	return parsePrecisionDecimalMaxExclusiveFacet(lexical, loc, false)
}

// ParsePrecisionDecimalMaxExclusiveFacetWithFixed parses an exclusive upper
// bound including its fixed property.
func ParsePrecisionDecimalMaxExclusiveFacetWithFixed(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxExclusiveFacet, error) {
	return parsePrecisionDecimalMaxExclusiveFacet(lexical, loc, fixed)
}

// ParsePrecisionDecimalWhiteSpaceFacet parses the fixed collapse declaration.
func ParsePrecisionDecimalWhiteSpaceFacet(lexical string, loc Loc) (PrecisionDecimalWhiteSpaceFacet, error) {
	value := collapseXMLWhitespace(lexical)
	if value == "collapse" {
		return PrecisionDecimalWhiteSpaceFacet{value: value, loc: loc, fixed: true}, nil
	}
	return PrecisionDecimalWhiteSpaceFacet{}, newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalWhiteSpaceCode,
		loc,
		precisionDecimalWhiteSpaceValueSpecRef,
		"precisionDecimal whiteSpace must be collapse",
		nil,
		fmt.Errorf("%w: %q", errInvalidPrecisionDecimalWhiteSpace, value),
	)
}

// ValidatePrecisionDecimalFacetValue validates one declaration value for a
// supported precisionDecimal facet name.
func ValidatePrecisionDecimalFacetValue(name, lexical string, loc Loc) error {
	switch name {
	case "totalDigits":
		_, err := ParsePrecisionDecimalTotalDigitsFacet(lexical, loc)
		return err
	case "minScale":
		_, err := ParsePrecisionDecimalMinScaleFacet(lexical, loc)
		return err
	case "maxScale":
		_, err := ParsePrecisionDecimalMaxScaleFacet(lexical, loc)
		return err
	case "pattern":
		_, err := ParsePrecisionDecimalPatternFacet(lexical, loc)
		return err
	case "enumeration":
		_, err := ParsePrecisionDecimalEnumerationFacet(lexical, loc)
		return err
	case "minInclusive":
		_, err := ParsePrecisionDecimalMinInclusiveFacet(lexical, loc)
		return err
	case "minExclusive":
		_, err := ParsePrecisionDecimalMinExclusiveFacet(lexical, loc)
		return err
	case "maxInclusive":
		_, err := ParsePrecisionDecimalMaxInclusiveFacet(lexical, loc)
		return err
	case "maxExclusive":
		_, err := ParsePrecisionDecimalMaxExclusiveFacet(lexical, loc)
		return err
	case "whiteSpace":
		_, err := ParsePrecisionDecimalWhiteSpaceFacet(lexical, loc)
		return err
	default:
		return ValidatePrecisionDecimalFacetName(name, loc)
	}
}

func parsePrecisionDecimalMinInclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinInclusiveFacet, error) {
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		return PrecisionDecimalMinInclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("minInclusive", loc, precisionDecimalMinInclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMinInclusiveFacet{value: clonePrecisionDecimalValue(value), loc: loc, fixed: fixed}, nil
}

func parsePrecisionDecimalMinExclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinExclusiveFacet, error) {
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		return PrecisionDecimalMinExclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("minExclusive", loc, precisionDecimalMinExclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMinExclusiveFacet{value: clonePrecisionDecimalValue(value), loc: loc, fixed: fixed}, nil
}

func parsePrecisionDecimalMaxInclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxInclusiveFacet, error) {
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		return PrecisionDecimalMaxInclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("maxInclusive", loc, precisionDecimalMaxInclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMaxInclusiveFacet{value: clonePrecisionDecimalValue(value), loc: loc, fixed: fixed}, nil
}

func parsePrecisionDecimalMaxExclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxExclusiveFacet, error) {
	value, err := parsePrecisionDecimal(lexical, loc)
	if err != nil {
		return PrecisionDecimalMaxExclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("maxExclusive", loc, precisionDecimalMaxExclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMaxExclusiveFacet{value: clonePrecisionDecimalValue(value), loc: loc, fixed: fixed}, nil
}

func newPrecisionDecimalEnumerationDiagnostic(loc Loc, cause error) Diagnostic {
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalEnumerationCode,
		loc,
		precisionDecimalEnumerationValueSpecRef,
		"invalid precisionDecimal enumeration value",
		nil,
		fmt.Errorf("%w: %w", errInvalidPrecisionDecimalEnumeration, cause),
	)
}

func newPrecisionDecimalBoundDiagnostic(name string, loc Loc, specRef string, cause error) Diagnostic {
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalBoundCode,
		loc,
		specRef,
		"invalid precisionDecimal "+name+" value",
		nil,
		fmt.Errorf("%w: %w", errInvalidPrecisionDecimalBound, cause),
	)
}

func clonePrecisionDecimalValue(value precisionDecimalValue) precisionDecimalValue {
	switch typed := value.(type) {
	case precisionDecimalFinite:
		return precisionDecimalFinite{
			coefficient: typed.coefficientCopy(),
			scale:       typed.scaleCopy(),
			sign:        typed.sign,
		}
	case precisionDecimalPositiveInfinity:
		return typed
	case precisionDecimalNegativeInfinity:
		return typed
	case precisionDecimalNaN:
		return typed
	default:
		panic("precisionDecimal value clone: invalid value variant")
	}
}

func clonePrecisionDecimalPatternFacets(facets []PrecisionDecimalPatternFacet) []PrecisionDecimalPatternFacet {
	if facets == nil {
		return nil
	}
	return append([]PrecisionDecimalPatternFacet(nil), facets...)
}

func clonePrecisionDecimalPatternGroups(groups [][]PrecisionDecimalPatternFacet) [][]PrecisionDecimalPatternFacet {
	if groups == nil {
		return nil
	}
	result := make([][]PrecisionDecimalPatternFacet, len(groups))
	for index, group := range groups {
		result[index] = clonePrecisionDecimalPatternFacets(group)
	}
	return result
}

func clonePrecisionDecimalEnumerationFacets(facets []PrecisionDecimalEnumerationFacet) []PrecisionDecimalEnumerationFacet {
	if facets == nil {
		return nil
	}
	result := make([]PrecisionDecimalEnumerationFacet, len(facets))
	for index, facet := range facets {
		result[index] = facet
		result[index].value = clonePrecisionDecimalValue(facet.value)
	}
	return result
}

func clonePrecisionDecimalMinInclusiveFacet(facet *PrecisionDecimalMinInclusiveFacet) *PrecisionDecimalMinInclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMinExclusiveFacet(facet *PrecisionDecimalMinExclusiveFacet) *PrecisionDecimalMinExclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMaxInclusiveFacet(facet *PrecisionDecimalMaxInclusiveFacet) *PrecisionDecimalMaxInclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMaxExclusiveFacet(facet *PrecisionDecimalMaxExclusiveFacet) *PrecisionDecimalMaxExclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalWhiteSpaceFacet(facet *PrecisionDecimalWhiteSpaceFacet) *PrecisionDecimalWhiteSpaceFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	return &facetCopy
}

func clonePrecisionDecimalFacetDeclarations(local PrecisionDecimalFacetDeclarations) PrecisionDecimalFacetDeclarations {
	declarations := NewPrecisionDecimalFacetDeclarationsAll(
		local.TotalDigits,
		local.MinScale,
		local.MaxScale,
		local.Patterns,
		local.Enumeration,
		local.MinInclusive,
		local.MinExclusive,
		local.MaxInclusive,
		local.MaxExclusive,
		local.WhiteSpace,
	)
	if local.boundRecords != nil {
		declarations.boundRecords = make([]precisionDecimalBoundRecord, len(local.boundRecords))
		for index, record := range local.boundRecords {
			declarations.boundRecords[index] = record
			declarations.boundRecords[index].value = clonePrecisionDecimalValue(record.value)
		}
	}
	return declarations
}

func precisionDecimalValueFacetHasEnumeration(facets PrecisionDecimalFacets) bool {
	return facets.enumeration != nil
}

// HasPattern reports whether any effective pattern facet exists.
func (facets PrecisionDecimalFacets) HasPattern() bool {
	return len(facets.patterns) != 0
}

// PatternGroupCount returns the number of inherited pattern steps.
func (facets PrecisionDecimalFacets) PatternGroupCount() int {
	return len(facets.patterns)
}

// PatternCount returns the number of effective pattern expressions.
func (facets PrecisionDecimalFacets) PatternCount() int {
	count := 0
	for _, group := range facets.patterns {
		count += len(group)
	}
	return count
}

// HasEnumeration reports whether an effective enumeration facet exists.
func (facets PrecisionDecimalFacets) HasEnumeration() bool {
	return precisionDecimalValueFacetHasEnumeration(facets)
}

// EnumerationCount returns the number of effective enumeration members.
func (facets PrecisionDecimalFacets) EnumerationCount() int {
	return len(facets.enumeration)
}

// MinInclusiveFacet returns the effective inclusive lower bound.
func (facets PrecisionDecimalFacets) MinInclusiveFacet() (PrecisionDecimalMinInclusiveFacet, bool) {
	if facets.minInclusive == nil {
		return PrecisionDecimalMinInclusiveFacet{}, false
	}
	return *clonePrecisionDecimalMinInclusiveFacet(facets.minInclusive), true
}

// MinExclusiveFacet returns the effective exclusive lower bound.
func (facets PrecisionDecimalFacets) MinExclusiveFacet() (PrecisionDecimalMinExclusiveFacet, bool) {
	if facets.minExclusive == nil {
		return PrecisionDecimalMinExclusiveFacet{}, false
	}
	return *clonePrecisionDecimalMinExclusiveFacet(facets.minExclusive), true
}

// MaxInclusiveFacet returns the effective inclusive upper bound.
func (facets PrecisionDecimalFacets) MaxInclusiveFacet() (PrecisionDecimalMaxInclusiveFacet, bool) {
	if facets.maxInclusive == nil {
		return PrecisionDecimalMaxInclusiveFacet{}, false
	}
	return *clonePrecisionDecimalMaxInclusiveFacet(facets.maxInclusive), true
}

// MaxExclusiveFacet returns the effective exclusive upper bound.
func (facets PrecisionDecimalFacets) MaxExclusiveFacet() (PrecisionDecimalMaxExclusiveFacet, bool) {
	if facets.maxExclusive == nil {
		return PrecisionDecimalMaxExclusiveFacet{}, false
	}
	return *clonePrecisionDecimalMaxExclusiveFacet(facets.maxExclusive), true
}

// WhiteSpaceFacet returns the effective whiteSpace declaration.
func (facets PrecisionDecimalFacets) WhiteSpaceFacet() (PrecisionDecimalWhiteSpaceFacet, bool) {
	if facets.whiteSpace == nil {
		return PrecisionDecimalWhiteSpaceFacet{}, false
	}
	return *clonePrecisionDecimalWhiteSpaceFacet(facets.whiteSpace), true
}

func validatePrecisionDecimalPatternState(patterns []PrecisionDecimalPatternFacet) error {
	for _, pattern := range patterns {
		if pattern.expression != nil {
			continue
		}
		return newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			InvalidPrecisionDecimalPatternCode,
			pattern.Loc(),
			precisionDecimalPatternValueSpecRef,
			"completed precisionDecimal pattern has no compiled expression",
			nil,
			errInvalidPrecisionDecimalFacetState,
		)
	}
	return nil
}

func validatePrecisionDecimalPatternGroupsState(groups [][]PrecisionDecimalPatternFacet) error {
	for _, group := range groups {
		if len(group) == 0 {
			return newPrecisionDecimalFacetDiagnostic(
				FailureInternal,
				InvalidPrecisionDecimalPatternCode,
				Loc{},
				precisionDecimalPatternValueSpecRef,
				"completed precisionDecimal pattern group is empty",
				nil,
				errInvalidPrecisionDecimalFacetState,
			)
		}
		if err := validatePrecisionDecimalPatternState(group); err != nil {
			return err
		}
	}
	return nil
}

func validatePrecisionDecimalEnumerationState(members []PrecisionDecimalEnumerationFacet) error {
	for _, member := range members {
		if member.value != nil {
			continue
		}
		return newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			InvalidPrecisionDecimalEnumerationCode,
			member.Loc(),
			precisionDecimalEnumerationValueSpecRef,
			"completed precisionDecimal enumeration member has no value",
			nil,
			errInvalidPrecisionDecimalFacetState,
		)
	}
	return nil
}

func validatePrecisionDecimalWhiteSpaceState(facet *PrecisionDecimalWhiteSpaceFacet) error {
	if facet == nil {
		return nil
	}
	if facet.value == "collapse" && facet.fixed {
		return nil
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInternal,
		InvalidPrecisionDecimalWhiteSpaceCode,
		facet.Loc(),
		precisionDecimalWhiteSpaceFixedSpecRef,
		"completed precisionDecimal whiteSpace is not fixed collapse",
		nil,
		errInvalidPrecisionDecimalFacetState,
	)
}

func validatePrecisionDecimalLocalValueFacetState(local PrecisionDecimalFacetDeclarations) error {
	if err := validatePrecisionDecimalBoundRecords(local.boundRecords); err != nil {
		return err
	}
	if local.MinInclusive != nil && local.MinExclusive != nil {
		return invalidPrecisionDecimalBoundCombinationWithSpec(
			local.MinExclusive.Loc(),
			local.MinInclusive.Loc(),
			precisionDecimalMinInclusiveMinExclusiveSpecRef,
			"precisionDecimal declarations cannot contain both minInclusive and minExclusive",
		)
	}
	if local.MaxInclusive != nil && local.MaxExclusive != nil {
		return invalidPrecisionDecimalBoundCombinationWithSpec(
			local.MaxExclusive.Loc(),
			local.MaxInclusive.Loc(),
			precisionDecimalMaxInclusiveMaxExclusiveSpecRef,
			"precisionDecimal declarations cannot contain both maxInclusive and maxExclusive",
		)
	}
	if err := validatePrecisionDecimalPatternState(local.Patterns); err != nil {
		return err
	}
	if err := validatePrecisionDecimalEnumerationState(local.Enumeration); err != nil {
		return err
	}
	if err := validatePrecisionDecimalBoundState(local.MinInclusive, local.MinExclusive, local.MaxInclusive, local.MaxExclusive); err != nil {
		return err
	}
	return validatePrecisionDecimalWhiteSpaceState(local.WhiteSpace)
}

func validatePrecisionDecimalEffectiveValueFacetState(facets PrecisionDecimalFacets) error {
	if err := validatePrecisionDecimalPatternGroupsState(facets.patterns); err != nil {
		return err
	}
	if err := validatePrecisionDecimalEnumerationState(facets.enumeration); err != nil {
		return err
	}
	if err := validatePrecisionDecimalBoundState(facets.minInclusive, facets.minExclusive, facets.maxInclusive, facets.maxExclusive); err != nil {
		return err
	}
	return validatePrecisionDecimalWhiteSpaceState(facets.whiteSpace)
}

func validatePrecisionDecimalBoundRecords(records []precisionDecimalBoundRecord) error {
	var seenMinInclusive, seenMinExclusive bool
	var seenMaxInclusive, seenMaxExclusive bool
	var minInclusiveLoc, minExclusiveLoc, maxInclusiveLoc, maxExclusiveLoc Loc
	for _, record := range records {
		var duplicate bool
		var related Loc
		switch record.kind {
		case precisionDecimalMinInclusiveBoundKind:
			duplicate = seenMinInclusive
			related = minInclusiveLoc
			seenMinInclusive = true
			minInclusiveLoc = record.loc
		case precisionDecimalMinExclusiveBoundKind:
			duplicate = seenMinExclusive
			related = minExclusiveLoc
			seenMinExclusive = true
			minExclusiveLoc = record.loc
		case precisionDecimalMaxInclusiveBoundKind:
			duplicate = seenMaxInclusive
			related = maxInclusiveLoc
			seenMaxInclusive = true
			maxInclusiveLoc = record.loc
		case precisionDecimalMaxExclusiveBoundKind:
			duplicate = seenMaxExclusive
			related = maxExclusiveLoc
			seenMaxExclusive = true
			maxExclusiveLoc = record.loc
		default:
			return newPrecisionDecimalFacetDiagnostic(
				FailureInternal,
				InvalidPrecisionDecimalBoundCombinationCode,
				record.loc,
				precisionDecimalBoundRestrictionSpecRef,
				"completed precisionDecimal bound has an invalid kind",
				nil,
				errInvalidPrecisionDecimalFacetState,
			)
		}
		if !duplicate {
			continue
		}
		return invalidPrecisionDecimalBoundCombination(
			record.loc,
			related,
			"precisionDecimal declarations contain a duplicate ordered bound",
		)
	}
	return nil
}

func validatePrecisionDecimalBoundState(
	minInclusive *PrecisionDecimalMinInclusiveFacet,
	minExclusive *PrecisionDecimalMinExclusiveFacet,
	maxInclusive *PrecisionDecimalMaxInclusiveFacet,
	maxExclusive *PrecisionDecimalMaxExclusiveFacet,
) error {
	if minInclusive != nil && minExclusive != nil {
		return invalidPrecisionDecimalBoundCombinationWithSpec(
			minExclusive.Loc(),
			minInclusive.Loc(),
			precisionDecimalMinInclusiveMinExclusiveSpecRef,
			"completed precisionDecimal facets contain both lower-bound kinds",
		)
	}
	if maxInclusive != nil && maxExclusive != nil {
		return invalidPrecisionDecimalBoundCombinationWithSpec(
			maxExclusive.Loc(),
			maxInclusive.Loc(),
			precisionDecimalMaxInclusiveMaxExclusiveSpecRef,
			"completed precisionDecimal facets contain both upper-bound kinds",
		)
	}
	if minInclusive == nil && minExclusive == nil {
		return nil
	}
	if maxInclusive == nil && maxExclusive == nil {
		return nil
	}
	minValue, minLoc, minInclusiveKind := precisionDecimalLowerBound(minInclusive, minExclusive)
	maxValue, maxLoc, maxInclusiveKind := precisionDecimalUpperBound(maxInclusive, maxExclusive)
	order := comparePrecisionDecimal(minValue, maxValue)
	if order == precisionDecimalOrderUnordered {
		return nil
	}
	valid := order == precisionDecimalOrderLess || (order == precisionDecimalOrderEqual && minInclusiveKind && maxInclusiveKind)
	if valid {
		return nil
	}
	return invalidPrecisionDecimalBoundCombinationWithSpec(
		minLoc,
		maxLoc,
		precisionDecimalBoundCombinationSpecRef(minInclusiveKind, maxInclusiveKind),
		"precisionDecimal lower and upper bounds describe an empty ordered interval",
	)
}

func validatePrecisionDecimalBounds(facets PrecisionDecimalFacets) error {
	return validatePrecisionDecimalBoundState(facets.minInclusive, facets.minExclusive, facets.maxInclusive, facets.maxExclusive)
}

func precisionDecimalLowerBound(minInclusive *PrecisionDecimalMinInclusiveFacet, minExclusive *PrecisionDecimalMinExclusiveFacet) (precisionDecimalValue, Loc, bool) {
	if minInclusive != nil {
		return minInclusive.value, minInclusive.Loc(), true
	}
	return minExclusive.value, minExclusive.Loc(), false
}

func precisionDecimalUpperBound(maxInclusive *PrecisionDecimalMaxInclusiveFacet, maxExclusive *PrecisionDecimalMaxExclusiveFacet) (precisionDecimalValue, Loc, bool) {
	if maxInclusive != nil {
		return maxInclusive.value, maxInclusive.Loc(), true
	}
	return maxExclusive.value, maxExclusive.Loc(), false
}

func invalidPrecisionDecimalBoundCombination(primary, related Loc, message string) Diagnostic {
	return invalidPrecisionDecimalBoundCombinationWithSpec(primary, related, precisionDecimalBoundRestrictionSpecRef, message)
}

func invalidPrecisionDecimalBoundCombinationWithSpec(primary, related Loc, specRef, message string) Diagnostic {
	primary, relatedLocations := precisionDecimalCrossFacetLocations(primary, related)
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalBoundCombinationCode,
		primary,
		specRef,
		message,
		relatedLocations,
		fmt.Errorf("%w: %s", errInvalidPrecisionDecimalBoundCombination, message),
	)
}

func precisionDecimalBoundCombinationSpecRef(minInclusive, maxInclusive bool) string {
	if minInclusive && maxInclusive {
		return precisionDecimalMinInclusiveMaxInclusiveSpecRef
	}
	if minInclusive {
		return precisionDecimalMinInclusiveMaxExclusiveSpecRef
	}
	if maxInclusive {
		return precisionDecimalMinExclusiveMaxInclusiveSpecRef
	}
	return precisionDecimalMinExclusiveMaxExclusiveSpecRef
}

func applyPrecisionDecimalValueFacets(effective, base *PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations, derived bool) error {
	if len(local.Patterns) != 0 {
		effective.patterns = append(effective.patterns, clonePrecisionDecimalPatternFacets(local.Patterns))
	}
	applyPrecisionDecimalEnumerationFacets(effective, base, local.Enumeration)
	if err := applyPrecisionDecimalWhiteSpaceFacet(effective, base, local.WhiteSpace, derived); err != nil {
		return err
	}
	return applyPrecisionDecimalBoundFacets(effective, base, local, derived)
}

func applyPrecisionDecimalEnumerationFacets(effective, base *PrecisionDecimalFacets, local []PrecisionDecimalEnumerationFacet) {
	if local == nil {
		return
	}
	if base.enumeration == nil {
		effective.enumeration = clonePrecisionDecimalEnumerationFacets(local)
		return
	}
	effective.enumeration = intersectPrecisionDecimalEnumerations(base.enumeration, local)
}

func applyPrecisionDecimalWhiteSpaceFacet(effective, base *PrecisionDecimalFacets, local *PrecisionDecimalWhiteSpaceFacet, derived bool) error {
	if local == nil {
		return nil
	}
	if derived && base.whiteSpace != nil && local.Value() != base.whiteSpace.Value() {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalFacetRestrictionCode,
			local.Loc(),
			precisionDecimalWhiteSpaceFixedSpecRef,
			"derived precisionDecimal whiteSpace changes a fixed base facet",
			precisionDecimalRelatedLocation(base.whiteSpace.Loc()),
			fmt.Errorf("%w: fixed whiteSpace differs", errInvalidPrecisionDecimalFacetRestriction),
		)
	}
	effective.whiteSpace = clonePrecisionDecimalWhiteSpaceFacet(local)
	if base.whiteSpace != nil && base.whiteSpace.Fixed() {
		effective.whiteSpace.fixed = true
	}
	return nil
}

func applyPrecisionDecimalBoundFacets(effective, base *PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations, derived bool) error {
	if local.MinInclusive != nil {
		if err := applyPrecisionDecimalMinInclusive(effective, base, *local.MinInclusive, derived); err != nil {
			return err
		}
	}
	if local.MinExclusive != nil {
		if err := applyPrecisionDecimalMinExclusive(effective, base, *local.MinExclusive, derived); err != nil {
			return err
		}
	}
	if local.MaxInclusive != nil {
		if err := applyPrecisionDecimalMaxInclusive(effective, base, *local.MaxInclusive, derived); err != nil {
			return err
		}
	}
	if local.MaxExclusive != nil {
		if err := applyPrecisionDecimalMaxExclusive(effective, base, *local.MaxExclusive, derived); err != nil {
			return err
		}
	}
	return nil
}

func intersectPrecisionDecimalEnumerations(base, local []PrecisionDecimalEnumerationFacet) []PrecisionDecimalEnumerationFacet {
	result := make([]PrecisionDecimalEnumerationFacet, 0, len(local))
	for _, localMember := range local {
		for _, baseMember := range base {
			if comparePrecisionDecimal(localMember.value, baseMember.value) != precisionDecimalOrderEqual {
				continue
			}
			result = append(result, PrecisionDecimalEnumerationFacet{
				value: clonePrecisionDecimalValue(localMember.value),
				loc:   localMember.Loc(),
			})
			break
		}
	}
	return result
}

func applyPrecisionDecimalMinInclusive(effective, base *PrecisionDecimalFacets, local PrecisionDecimalMinInclusiveFacet, derived bool) error {
	if derived {
		if base.minInclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.minInclusive.value, base.minInclusive.Loc(), base.minInclusive.fixed, true, true, true); err != nil {
				return err
			}
		}
		if base.minExclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.minExclusive.value, base.minExclusive.Loc(), base.minExclusive.fixed, true, false, true); err != nil {
				return err
			}
		}
	}
	effective.minInclusive = clonePrecisionDecimalMinInclusiveFacet(&local)
	effective.minExclusive = nil
	if base.minInclusive != nil && base.minInclusive.fixed {
		effective.minInclusive.fixed = true
	}
	return nil
}

func applyPrecisionDecimalMinExclusive(effective, base *PrecisionDecimalFacets, local PrecisionDecimalMinExclusiveFacet, derived bool) error {
	if derived {
		if base.minInclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.minInclusive.value, base.minInclusive.Loc(), base.minInclusive.fixed, false, true, true); err != nil {
				return err
			}
		}
		if base.minExclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.minExclusive.value, base.minExclusive.Loc(), base.minExclusive.fixed, false, false, true); err != nil {
				return err
			}
		}
	}
	effective.minExclusive = clonePrecisionDecimalMinExclusiveFacet(&local)
	effective.minInclusive = nil
	if base.minExclusive != nil && base.minExclusive.fixed {
		effective.minExclusive.fixed = true
	}
	return nil
}

func applyPrecisionDecimalMaxInclusive(effective, base *PrecisionDecimalFacets, local PrecisionDecimalMaxInclusiveFacet, derived bool) error {
	if derived {
		if base.maxInclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.maxInclusive.value, base.maxInclusive.Loc(), base.maxInclusive.fixed, true, true, false); err != nil {
				return err
			}
		}
		if base.maxExclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.maxExclusive.value, base.maxExclusive.Loc(), base.maxExclusive.fixed, true, false, false); err != nil {
				return err
			}
		}
	}
	effective.maxInclusive = clonePrecisionDecimalMaxInclusiveFacet(&local)
	effective.maxExclusive = nil
	if base.maxInclusive != nil && base.maxInclusive.fixed {
		effective.maxInclusive.fixed = true
	}
	return nil
}

func applyPrecisionDecimalMaxExclusive(effective, base *PrecisionDecimalFacets, local PrecisionDecimalMaxExclusiveFacet, derived bool) error {
	if derived {
		if base.maxInclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.maxInclusive.value, base.maxInclusive.Loc(), base.maxInclusive.fixed, false, true, false); err != nil {
				return err
			}
		}
		if base.maxExclusive != nil {
			if err := validatePrecisionDecimalBoundRestriction(local.value, local.Loc(), base.maxExclusive.value, base.maxExclusive.Loc(), base.maxExclusive.fixed, false, false, false); err != nil {
				return err
			}
		}
	}
	effective.maxExclusive = clonePrecisionDecimalMaxExclusiveFacet(&local)
	effective.maxInclusive = nil
	if base.maxExclusive != nil && base.maxExclusive.fixed {
		effective.maxExclusive.fixed = true
	}
	return nil
}

func validatePrecisionDecimalBoundRestriction(localValue precisionDecimalValue, localLoc Loc, baseValue precisionDecimalValue, baseLoc Loc, baseFixed bool, localInclusive, baseInclusive, lower bool) error {
	if baseFixed && comparePrecisionDecimal(localValue, baseValue) != precisionDecimalOrderEqual {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalFacetRestrictionCode,
			localLoc,
			precisionDecimalBoundFixedSpecRef(localInclusive, lower),
			"derived precisionDecimal bound changes a fixed base facet",
			precisionDecimalRelatedLocation(baseLoc),
			fmt.Errorf("%w: fixed ordered bound differs", errInvalidPrecisionDecimalFacetRestriction),
		)
	}
	order := comparePrecisionDecimal(localValue, baseValue)
	if order == precisionDecimalOrderUnordered {
		return nil
	}
	violation := order == precisionDecimalOrderLess
	if !lower {
		violation = order == precisionDecimalOrderGreater
	}
	if localInclusive != baseInclusive {
		if lower && localInclusive && !baseInclusive {
			violation = order == precisionDecimalOrderLess || order == precisionDecimalOrderEqual
		}
		if !lower && localInclusive && !baseInclusive {
			violation = order == precisionDecimalOrderGreater || order == precisionDecimalOrderEqual
		}
	}
	if !violation {
		return nil
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalFacetRestrictionCode,
		localLoc,
		precisionDecimalBoundRestrictionSpecRefForKind(localInclusive, lower),
		"derived precisionDecimal bound is less restrictive than its base facet",
		precisionDecimalRelatedLocation(baseLoc),
		fmt.Errorf("%w: ordered bound monotonicity is violated", errInvalidPrecisionDecimalFacetRestriction),
	)
}

func precisionDecimalBoundFixedSpecRef(inclusive, lower bool) string {
	if lower && inclusive {
		return precisionDecimalMinInclusiveFixedSpecRef
	}
	if lower {
		return precisionDecimalMinExclusiveFixedSpecRef
	}
	if inclusive {
		return precisionDecimalMaxInclusiveFixedSpecRef
	}
	return precisionDecimalMaxExclusiveFixedSpecRef
}

func precisionDecimalBoundRestrictionSpecRefForKind(inclusive, lower bool) string {
	if lower && inclusive {
		return precisionDecimalMinInclusiveRestrictionSpecRef
	}
	if lower {
		return precisionDecimalMinExclusiveRestrictionSpecRef
	}
	if inclusive {
		return precisionDecimalMaxInclusiveRestrictionSpecRef
	}
	return precisionDecimalMaxExclusiveRestrictionSpecRef
}

func precisionDecimalFacetValueViolation(valueLoc Loc, related []Loc, specRef, message string) error {
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		PrecisionDecimalFacetValueViolationCode,
		valueLoc,
		specRef,
		message,
		related,
		errPrecisionDecimalFacetValueViolation,
	)
}

func precisionDecimalFacetRelatedLocationsFromPatterns(groups [][]PrecisionDecimalPatternFacet) []Loc {
	var locations []Loc
	for _, group := range groups {
		for _, pattern := range group {
			if pattern.Loc().IsZero() {
				continue
			}
			locations = append(locations, pattern.Loc())
		}
	}
	return locations
}

func precisionDecimalFacetRelatedLocationsFromEnumeration(members []PrecisionDecimalEnumerationFacet) []Loc {
	locations := make([]Loc, 0, len(members))
	for _, member := range members {
		if member.Loc().IsZero() {
			continue
		}
		locations = append(locations, member.Loc())
	}
	return locations
}

func precisionDecimalTotalDigitsValue(value precisionDecimalValue) *big.Int {
	finite, ok := value.(precisionDecimalFinite)
	if !ok || finite.coefficient.Sign() == 0 {
		return nil
	}
	return big.NewInt(int64(precisionDecimalDecimalDigitCount(finite.coefficient)))
}

// validatePrecisionDecimalFacets parses one lexical value and evaluates it
// against complete effective value facets. Lexical text is retained only in
// the phase-specific input pairing.
func validatePrecisionDecimalFacets(lexical string, facets PrecisionDecimalFacets, valueLoc Loc) error {
	input, err := parsePrecisionDecimalFacetInput(lexical, valueLoc)
	if err != nil {
		return err
	}
	return validatePrecisionDecimalFacetInput(input, facets, valueLoc)
}

func validatePrecisionDecimalFacetInput(input precisionDecimalFacetInput, facets PrecisionDecimalFacets, valueLoc Loc) error {
	if err := facets.validate(); err != nil {
		return err
	}
	if err := validatePrecisionDecimalPatterns(input.normalizedLexical, facets, valueLoc); err != nil {
		return err
	}
	if err := validatePrecisionDecimalTotalDigitsValue(input.value, facets, valueLoc); err != nil {
		return err
	}
	if err := validatePrecisionDecimalScaleValue(input.value, facets, valueLoc); err != nil {
		return err
	}
	if err := validatePrecisionDecimalEnumerationValue(input.value, facets, valueLoc); err != nil {
		return err
	}
	return validatePrecisionDecimalBoundsValue(input.value, facets, valueLoc)
}

func validatePrecisionDecimalPatterns(normalizedLexical string, facets PrecisionDecimalFacets, valueLoc Loc) error {
	for _, group := range facets.patterns {
		matched := false
		for _, pattern := range group {
			if !pattern.expression.matches(normalizedLexical) {
				continue
			}
			matched = true
			break
		}
		if matched {
			continue
		}
		return precisionDecimalFacetValueViolation(
			valueLoc,
			precisionDecimalFacetRelatedLocationsFromPatterns([][]PrecisionDecimalPatternFacet{group}),
			precisionDecimalPatternValidSpecRef,
			"precisionDecimal value does not satisfy its pattern facet",
		)
	}
	return nil
}

func validatePrecisionDecimalTotalDigitsValue(value precisionDecimalValue, facets PrecisionDecimalFacets, valueLoc Loc) error {
	if facets.totalDigits == nil {
		return nil
	}
	digits := precisionDecimalTotalDigitsValue(value)
	if digits == nil || digits.Cmp(facets.totalDigits.value.value) <= 0 {
		return nil
	}
	return precisionDecimalFacetValueViolation(
		valueLoc,
		precisionDecimalRelatedLocation(facets.totalDigits.Loc()),
		precisionDecimalTotalDigitsSpecRef,
		"precisionDecimal value exceeds totalDigits",
	)
}

func validatePrecisionDecimalScaleValue(value precisionDecimalValue, facets PrecisionDecimalFacets, valueLoc Loc) error {
	finite, ok := value.(precisionDecimalFinite)
	if !ok {
		return nil
	}
	if facets.minScale != nil && finite.scale.Cmp(facets.minScale.value.value) < 0 {
		return precisionDecimalFacetValueViolation(
			valueLoc,
			precisionDecimalRelatedLocation(facets.minScale.Loc()),
			precisionDecimalMinScaleValueSpecRef,
			"precisionDecimal value is below minScale",
		)
	}
	if facets.maxScale != nil && finite.scale.Cmp(facets.maxScale.value.value) > 0 {
		return precisionDecimalFacetValueViolation(
			valueLoc,
			precisionDecimalRelatedLocation(facets.maxScale.Loc()),
			precisionDecimalMaxScaleValueSpecRef,
			"precisionDecimal value exceeds maxScale",
		)
	}
	return nil
}

func validatePrecisionDecimalEnumerationValue(value precisionDecimalValue, facets PrecisionDecimalFacets, valueLoc Loc) error {
	if facets.enumeration == nil {
		return nil
	}
	for _, member := range facets.enumeration {
		if comparePrecisionDecimal(value, member.value) == precisionDecimalOrderEqual {
			return nil
		}
	}
	return precisionDecimalFacetValueViolation(
		valueLoc,
		precisionDecimalFacetRelatedLocationsFromEnumeration(facets.enumeration),
		precisionDecimalEnumerationValidSpecRef,
		"precisionDecimal value is not an enumeration member",
	)
}

func validatePrecisionDecimalBoundsValue(value precisionDecimalValue, facets PrecisionDecimalFacets, valueLoc Loc) error {
	if facets.minInclusive != nil {
		order := comparePrecisionDecimal(value, facets.minInclusive.value)
		if order != precisionDecimalOrderGreater && order != precisionDecimalOrderEqual {
			return precisionDecimalFacetValueViolation(
				valueLoc,
				precisionDecimalRelatedLocation(facets.minInclusive.Loc()),
				precisionDecimalMinInclusiveValidSpecRef,
				"precisionDecimal value is below minInclusive",
			)
		}
	}
	if facets.minExclusive != nil {
		if comparePrecisionDecimal(value, facets.minExclusive.value) != precisionDecimalOrderGreater {
			return precisionDecimalFacetValueViolation(
				valueLoc,
				precisionDecimalRelatedLocation(facets.minExclusive.Loc()),
				precisionDecimalMinExclusiveValidSpecRef,
				"precisionDecimal value does not exceed minExclusive",
			)
		}
	}
	if facets.maxInclusive != nil {
		order := comparePrecisionDecimal(value, facets.maxInclusive.value)
		if order != precisionDecimalOrderLess && order != precisionDecimalOrderEqual {
			return precisionDecimalFacetValueViolation(
				valueLoc,
				precisionDecimalRelatedLocation(facets.maxInclusive.Loc()),
				precisionDecimalMaxInclusiveValidSpecRef,
				"precisionDecimal value exceeds maxInclusive",
			)
		}
	}
	if facets.maxExclusive != nil {
		if comparePrecisionDecimal(value, facets.maxExclusive.value) != precisionDecimalOrderLess {
			return precisionDecimalFacetValueViolation(
				valueLoc,
				precisionDecimalRelatedLocation(facets.maxExclusive.Loc()),
				precisionDecimalMaxExclusiveValidSpecRef,
				"precisionDecimal value does not satisfy maxExclusive",
			)
		}
	}
	return nil
}
