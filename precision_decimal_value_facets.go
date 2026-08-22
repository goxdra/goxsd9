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
	// UnsupportedPrecisionDecimalPatternCode identifies a pattern that is
	// valid XML Schema syntax but exceeds the implementation's resource
	// limits.
	UnsupportedPrecisionDecimalPatternCode = "XSD2026"
)

const (
	precisionDecimalPatternValueSpecRef             = "xsd11-datatypes#f-p-value"
	precisionDecimalEnumerationValueSpecRef         = "xsd11-datatypes#f-e-value"
	precisionDecimalEnumerationValidSpecRef         = "xsd11-datatypes#cvc-enumeration-valid"
	precisionDecimalPatternValidSpecRef             = "xsd11-datatypes#cvc-pattern-valid"
	precisionDecimalMinInclusiveValueSpecRef        = "xsd11-datatypes#f-mii-value"
	precisionDecimalMinExclusiveValueSpecRef        = "xsd11-datatypes#f-mie-value"
	precisionDecimalMaxInclusiveValueSpecRef        = "xsd11-datatypes#f-mai-value"
	precisionDecimalMaxExclusiveValueSpecRef        = "xsd11-datatypes#f-mae-value"
	precisionDecimalMinInclusiveFixedSpecRef        = "xsd11-datatypes#f-mii-fixed"
	precisionDecimalMinExclusiveFixedSpecRef        = "xsd11-datatypes#f-mie-fixed"
	precisionDecimalMaxInclusiveFixedSpecRef        = "xsd11-datatypes#f-mai-fixed"
	precisionDecimalMaxExclusiveFixedSpecRef        = "xsd11-datatypes#f-mae-fixed"
	precisionDecimalMinInclusiveRestrictionSpecRef  = "xsd11-datatypes#minInclusive-valid-restriction"
	precisionDecimalMinExclusiveRestrictionSpecRef  = "xsd11-datatypes#minExclusive-valid-restriction"
	precisionDecimalMaxInclusiveRestrictionSpecRef  = "xsd11-datatypes#maxInclusive-valid-restriction"
	precisionDecimalMaxExclusiveRestrictionSpecRef  = "xsd11-datatypes#maxExclusive-valid-restriction"
	precisionDecimalMinInclusiveMaxInclusiveSpecRef = "xsd11-datatypes#minInclusive-less-than-equal-to-maxInclusive"
	precisionDecimalMinInclusiveMaxExclusiveSpecRef = "xsd11-datatypes#minInclusive-less-than-maxExclusive"
	precisionDecimalMinExclusiveMaxInclusiveSpecRef = "xsd11-datatypes#minExclusive-less-than-maxInclusive"
	precisionDecimalMinExclusiveMaxExclusiveSpecRef = "xsd11-datatypes#minExclusive-less-than-equal-to-maxExclusive"
	precisionDecimalMinInclusiveMinExclusiveSpecRef = "xsd11-datatypes#minInclusive-minExclusive"
	precisionDecimalMaxInclusiveMaxExclusiveSpecRef = "xsd11-datatypes#maxInclusive-maxExclusive"
	precisionDecimalMinInclusiveValidSpecRef        = "xsd11-datatypes#cvc-minInclusive-valid"
	precisionDecimalMinExclusiveValidSpecRef        = "xsd11-datatypes#cvc-minExclusive-valid"
	precisionDecimalMaxInclusiveValidSpecRef        = "xsd11-datatypes#cvc-maxInclusive-valid"
	precisionDecimalMaxExclusiveValidSpecRef        = "xsd11-datatypes#cvc-maxExclusive-valid"
	precisionDecimalTotalDigitsValidSpecRef         = "xsd-precisionDecimal#cvc-totalDigits-valid"
	precisionDecimalMinScaleValidSpecRef            = "xsd-precisionDecimal#cvc-minScale-valid"
	precisionDecimalMaxScaleValidSpecRef            = "xsd-precisionDecimal#cvc-maxScale-valid"
	precisionDecimalWhiteSpaceValueSpecRef          = "xsd-precisionDecimal#precisionDecimal.whiteSpace"
	precisionDecimalWhiteSpaceFixedSpecRef          = "xsd-precisionDecimal#precisionDecimal.whiteSpace"
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
	source string
	loc    Loc
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
	normalizedLexical string
	value             precisionDecimalValue
	loc               Loc
}

// Loc returns the source location of the enumeration declaration.
func (facet PrecisionDecimalEnumerationFacet) Loc() Loc {
	return facet.loc
}

// PrecisionDecimalMinInclusiveFacet is a parsed inclusive lower bound.
type PrecisionDecimalMinInclusiveFacet struct {
	normalizedLexical string
	value             precisionDecimalValue
	loc               Loc
	fixed             bool
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
	normalizedLexical string
	value             precisionDecimalValue
	loc               Loc
	fixed             bool
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
	normalizedLexical string
	value             precisionDecimalValue
	loc               Loc
	fixed             bool
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
	normalizedLexical string
	value             precisionDecimalValue
	loc               Loc
	fixed             bool
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

// ParsePrecisionDecimalPatternFacet parses an XML Schema regular expression.
func ParsePrecisionDecimalPatternFacet(pattern string, loc Loc) (PrecisionDecimalPatternFacet, error) {
	_, err := parsePrecisionDecimalXMLRegex(pattern)
	if err == nil {
		return PrecisionDecimalPatternFacet{source: pattern, loc: loc}, nil
	}
	if errors.Is(err, errPrecisionDecimalXMLRegexResourceLimit) {
		return PrecisionDecimalPatternFacet{}, newPrecisionDecimalPatternResourceDiagnostic(loc, err)
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
	input, err := parsePrecisionDecimalFacetInput(lexical, loc)
	if err != nil {
		return PrecisionDecimalEnumerationFacet{}, newPrecisionDecimalEnumerationDiagnostic(loc, err)
	}
	return PrecisionDecimalEnumerationFacet{
		normalizedLexical: input.normalizedLexical,
		value:             clonePrecisionDecimalValue(input.value),
		loc:               loc,
	}, nil
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
		return PrecisionDecimalWhiteSpaceFacet{value: value, loc: loc}, nil
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
	input, err := parsePrecisionDecimalFacetInput(lexical, loc)
	if err != nil {
		return PrecisionDecimalMinInclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("minInclusive", loc, precisionDecimalMinInclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMinInclusiveFacet{
		normalizedLexical: input.normalizedLexical,
		value:             clonePrecisionDecimalValue(input.value),
		loc:               loc,
		fixed:             fixed,
	}, nil
}

func parsePrecisionDecimalMinExclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMinExclusiveFacet, error) {
	input, err := parsePrecisionDecimalFacetInput(lexical, loc)
	if err != nil {
		return PrecisionDecimalMinExclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("minExclusive", loc, precisionDecimalMinExclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMinExclusiveFacet{
		normalizedLexical: input.normalizedLexical,
		value:             clonePrecisionDecimalValue(input.value),
		loc:               loc,
		fixed:             fixed,
	}, nil
}

func parsePrecisionDecimalMaxInclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxInclusiveFacet, error) {
	input, err := parsePrecisionDecimalFacetInput(lexical, loc)
	if err != nil {
		return PrecisionDecimalMaxInclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("maxInclusive", loc, precisionDecimalMaxInclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMaxInclusiveFacet{
		normalizedLexical: input.normalizedLexical,
		value:             clonePrecisionDecimalValue(input.value),
		loc:               loc,
		fixed:             fixed,
	}, nil
}

func parsePrecisionDecimalMaxExclusiveFacet(lexical string, loc Loc, fixed bool) (PrecisionDecimalMaxExclusiveFacet, error) {
	input, err := parsePrecisionDecimalFacetInput(lexical, loc)
	if err != nil {
		return PrecisionDecimalMaxExclusiveFacet{}, newPrecisionDecimalBoundDiagnostic("maxExclusive", loc, precisionDecimalMaxExclusiveValueSpecRef, err)
	}
	return PrecisionDecimalMaxExclusiveFacet{
		normalizedLexical: input.normalizedLexical,
		value:             clonePrecisionDecimalValue(input.value),
		loc:               loc,
		fixed:             fixed,
	}, nil
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

func newPrecisionDecimalPatternResourceDiagnostic(loc Loc, cause error) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeaturePrecisionDecimal)
	if !ok {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			UnsupportedPrecisionDecimalPatternCode,
			loc,
			precisionDecimalPatternValueSpecRef,
			"precisionDecimal unsupported feature is not registered",
			nil,
			fmt.Errorf("%w: %w", errInvalidPrecisionDecimalFacetState, cause),
		)
	}
	diagnostic := newUnsupported(
		feature,
		UnsupportedPrecisionDecimalPatternCode,
		loc,
		"precisionDecimal XML Schema pattern exceeds implementation resource limits",
	)
	diagnostic.specRef = precisionDecimalPatternValueSpecRef
	diagnostic.cause = fmt.Errorf("%w: %w", ErrUnsupported, cause)
	return diagnostic
}

func clonePrecisionDecimalValue(value precisionDecimalValue) precisionDecimalValue {
	if value == nil {
		return nil
	}
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

func (facet PrecisionDecimalPatternFacet) matches(source string) bool {
	expression, err := parsePrecisionDecimalXMLRegex(facet.source)
	if err != nil {
		return false
	}
	return expression.matches(source)
}

func clonePrecisionDecimalPatternFacets(facets []PrecisionDecimalPatternFacet) []PrecisionDecimalPatternFacet {
	if facets == nil {
		return nil
	}
	result := make([]PrecisionDecimalPatternFacet, len(facets))
	copy(result, facets)
	return result
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

func clonePrecisionDecimalEnumerationFacetsForEffective(facets []PrecisionDecimalEnumerationFacet) []PrecisionDecimalEnumerationFacet {
	result := clonePrecisionDecimalEnumerationFacets(facets)
	for index := range result {
		result[index].normalizedLexical = ""
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

func clonePrecisionDecimalMinInclusiveFacetForEffective(facet *PrecisionDecimalMinInclusiveFacet) *PrecisionDecimalMinInclusiveFacet {
	result := clonePrecisionDecimalMinInclusiveFacet(facet)
	if result != nil {
		result.normalizedLexical = ""
	}
	return result
}

func clonePrecisionDecimalMinExclusiveFacet(facet *PrecisionDecimalMinExclusiveFacet) *PrecisionDecimalMinExclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMinExclusiveFacetForEffective(facet *PrecisionDecimalMinExclusiveFacet) *PrecisionDecimalMinExclusiveFacet {
	result := clonePrecisionDecimalMinExclusiveFacet(facet)
	if result != nil {
		result.normalizedLexical = ""
	}
	return result
}

func clonePrecisionDecimalMaxInclusiveFacet(facet *PrecisionDecimalMaxInclusiveFacet) *PrecisionDecimalMaxInclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMaxInclusiveFacetForEffective(facet *PrecisionDecimalMaxInclusiveFacet) *PrecisionDecimalMaxInclusiveFacet {
	result := clonePrecisionDecimalMaxInclusiveFacet(facet)
	if result != nil {
		result.normalizedLexical = ""
	}
	return result
}

func clonePrecisionDecimalMaxExclusiveFacet(facet *PrecisionDecimalMaxExclusiveFacet) *PrecisionDecimalMaxExclusiveFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	facetCopy.value = clonePrecisionDecimalValue(facet.value)
	return &facetCopy
}

func clonePrecisionDecimalMaxExclusiveFacetForEffective(facet *PrecisionDecimalMaxExclusiveFacet) *PrecisionDecimalMaxExclusiveFacet {
	result := clonePrecisionDecimalMaxExclusiveFacet(facet)
	if result != nil {
		result.normalizedLexical = ""
	}
	return result
}

func clonePrecisionDecimalWhiteSpaceFacet(facet *PrecisionDecimalWhiteSpaceFacet) *PrecisionDecimalWhiteSpaceFacet {
	if facet == nil {
		return nil
	}
	facetCopy := *facet
	return &facetCopy
}

func precisionDecimalDefaultWhiteSpaceFacet() *PrecisionDecimalWhiteSpaceFacet {
	return &PrecisionDecimalWhiteSpaceFacet{value: "collapse", fixed: true}
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
		return *precisionDecimalDefaultWhiteSpaceFacet(), true
	}
	return *clonePrecisionDecimalWhiteSpaceFacet(facets.whiteSpace), true
}

func validatePrecisionDecimalPatternState(patterns []PrecisionDecimalPatternFacet) error {
	for _, pattern := range patterns {
		_, err := parsePrecisionDecimalXMLRegex(pattern.source)
		if err == nil {
			continue
		}
		if errors.Is(err, errPrecisionDecimalXMLRegexResourceLimit) {
			return newPrecisionDecimalPatternResourceDiagnostic(pattern.Loc(), err)
		}
		return newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			InvalidPrecisionDecimalPatternCode,
			pattern.Loc(),
			precisionDecimalPatternValueSpecRef,
			"completed precisionDecimal pattern is not valid XML Schema syntax",
			nil,
			fmt.Errorf("%w: %w", errInvalidPrecisionDecimalFacetState, err),
		)
	}
	return nil
}

func validatePrecisionDecimalLocalPatternState(patterns []PrecisionDecimalPatternFacet) error {
	if patterns != nil && len(patterns) == 0 {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalPatternCode,
			Loc{},
			precisionDecimalPatternValueSpecRef,
			"precisionDecimal pattern declaration is empty",
			nil,
			errInvalidPrecisionDecimalPattern,
		)
	}
	return validatePrecisionDecimalPatternState(patterns)
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
	if members != nil && len(members) == 0 {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			InvalidPrecisionDecimalEnumerationCode,
			Loc{},
			precisionDecimalEnumerationValueSpecRef,
			"completed precisionDecimal enumeration is empty",
			nil,
			errInvalidPrecisionDecimalFacetState,
		)
	}
	for _, member := range members {
		if precisionDecimalValueIsWellFormed(member.value) {
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

func validatePrecisionDecimalLocalEnumerationState(members []PrecisionDecimalEnumerationFacet) error {
	if members != nil && len(members) == 0 {
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalEnumerationCode,
			Loc{},
			precisionDecimalEnumerationValueSpecRef,
			"precisionDecimal enumeration declaration is empty",
			nil,
			errInvalidPrecisionDecimalEnumeration,
		)
	}
	for _, member := range members {
		if precisionDecimalValueIsWellFormed(member.value) && member.normalizedLexical != "" {
			continue
		}
		return newPrecisionDecimalFacetDiagnostic(
			FailureInvalid,
			InvalidPrecisionDecimalEnumerationCode,
			member.Loc(),
			precisionDecimalEnumerationValueSpecRef,
			"invalid precisionDecimal enumeration declaration",
			nil,
			errInvalidPrecisionDecimalEnumeration,
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
	if err := validatePrecisionDecimalLocalPatternState(local.Patterns); err != nil {
		return err
	}
	if err := validatePrecisionDecimalLocalEnumerationState(local.Enumeration); err != nil {
		return err
	}
	if err := validatePrecisionDecimalLocalBoundValueState(local); err != nil {
		return err
	}
	if err := validatePrecisionDecimalBoundState(local.MinInclusive, local.MinExclusive, local.MaxInclusive, local.MaxExclusive); err != nil {
		return err
	}
	return validatePrecisionDecimalLocalWhiteSpaceState(local.WhiteSpace)
}

func validatePrecisionDecimalEffectiveValueFacetState(facets PrecisionDecimalFacets) error {
	if err := validatePrecisionDecimalPatternGroupsState(facets.patterns); err != nil {
		return err
	}
	if err := validatePrecisionDecimalEnumerationState(facets.enumeration); err != nil {
		return err
	}
	if err := validatePrecisionDecimalEffectiveBoundValueState(facets); err != nil {
		return err
	}
	if err := validatePrecisionDecimalBoundState(facets.minInclusive, facets.minExclusive, facets.maxInclusive, facets.maxExclusive); err != nil {
		return err
	}
	return validatePrecisionDecimalWhiteSpaceState(facets.whiteSpace)
}

func validatePrecisionDecimalLocalWhiteSpaceState(facet *PrecisionDecimalWhiteSpaceFacet) error {
	if facet == nil {
		return nil
	}
	if facet.value == "collapse" {
		return nil
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalWhiteSpaceCode,
		facet.Loc(),
		precisionDecimalWhiteSpaceValueSpecRef,
		"completed precisionDecimal whiteSpace is not collapse",
		nil,
		errInvalidPrecisionDecimalWhiteSpace,
	)
}

func precisionDecimalValueIsWellFormed(value precisionDecimalValue) bool {
	switch typed := value.(type) {
	case precisionDecimalFinite:
		if typed.coefficient == nil || typed.scale == nil || typed.coefficient.Sign() < 0 {
			return false
		}
		return typed.sign == precisionDecimalSignPositive || typed.sign == precisionDecimalSignNegative
	case precisionDecimalPositiveInfinity, precisionDecimalNegativeInfinity, precisionDecimalNaN:
		return true
	default:
		return false
	}
}

func precisionDecimalEqualOrIdentical(left, right precisionDecimalValue) bool {
	if !precisionDecimalValueIsWellFormed(left) || !precisionDecimalValueIsWellFormed(right) {
		return false
	}
	_, leftNaN := left.(precisionDecimalNaN)
	_, rightNaN := right.(precisionDecimalNaN)
	if leftNaN || rightNaN {
		return leftNaN && rightNaN
	}
	return comparePrecisionDecimal(left, right) == precisionDecimalOrderEqual
}

func validatePrecisionDecimalLocalFacetMembers(base PrecisionDecimalFacets, local PrecisionDecimalFacetDeclarations) error {
	for _, member := range local.Enumeration {
		input := precisionDecimalFacetInput{
			normalizedLexical: member.normalizedLexical,
			value:             member.value,
		}
		if err := validatePrecisionDecimalFacetInput(input, base, member.Loc()); err != nil {
			return precisionDecimalLocalEnumerationMembershipError(err, member)
		}
	}
	for _, bound := range []struct {
		present bool
		name    string
		lexical string
		value   precisionDecimalValue
		loc     Loc
	}{
		{present: local.MinInclusive != nil, name: "minInclusive", lexical: precisionDecimalLocalMinInclusiveLexical(local.MinInclusive), value: precisionDecimalLocalMinInclusiveValue(local.MinInclusive), loc: precisionDecimalLocalMinInclusiveLoc(local.MinInclusive)},
		{present: local.MinExclusive != nil, name: "minExclusive", lexical: precisionDecimalLocalMinExclusiveLexical(local.MinExclusive), value: precisionDecimalLocalMinExclusiveValue(local.MinExclusive), loc: precisionDecimalLocalMinExclusiveLoc(local.MinExclusive)},
		{present: local.MaxInclusive != nil, name: "maxInclusive", lexical: precisionDecimalLocalMaxInclusiveLexical(local.MaxInclusive), value: precisionDecimalLocalMaxInclusiveValue(local.MaxInclusive), loc: precisionDecimalLocalMaxInclusiveLoc(local.MaxInclusive)},
		{present: local.MaxExclusive != nil, name: "maxExclusive", lexical: precisionDecimalLocalMaxExclusiveLexical(local.MaxExclusive), value: precisionDecimalLocalMaxExclusiveValue(local.MaxExclusive), loc: precisionDecimalLocalMaxExclusiveLoc(local.MaxExclusive)},
	} {
		if !bound.present {
			continue
		}
		membershipException := precisionDecimalBoundMembershipException(base, bound.name, bound.value)
		if precisionDecimalBaseValueSpaceEmpty(base) && !membershipException {
			return newPrecisionDecimalFacetDiagnostic(
				FailureInvalid,
				InvalidPrecisionDecimalBoundCode,
				bound.loc,
				precisionDecimalBoundRestrictionSpecRefForName(bound.name),
				"derived precisionDecimal "+bound.name+" is outside the empty base value space",
				precisionDecimalEffectiveBoundLocations(base),
				fmt.Errorf("%w: base value space is empty", errInvalidPrecisionDecimalBound),
			)
		}
		if membershipException {
			continue
		}
		input := precisionDecimalFacetInput{normalizedLexical: bound.lexical, value: bound.value}
		if err := validatePrecisionDecimalFacetInput(input, base, bound.loc); err != nil {
			return precisionDecimalLocalBoundMembershipError(err, bound.name, bound.loc)
		}
	}
	return nil
}

func precisionDecimalBaseValueSpaceEmpty(base PrecisionDecimalFacets) bool {
	return precisionDecimalValueIsNaN(precisionDecimalLocalMinInclusiveValue(base.minInclusive)) ||
		precisionDecimalValueIsNaN(precisionDecimalLocalMinExclusiveValue(base.minExclusive)) ||
		precisionDecimalValueIsNaN(precisionDecimalLocalMaxInclusiveValue(base.maxInclusive)) ||
		precisionDecimalValueIsNaN(precisionDecimalLocalMaxExclusiveValue(base.maxExclusive))
}

func precisionDecimalValueIsNaN(value precisionDecimalValue) bool {
	_, ok := value.(precisionDecimalNaN)
	return ok
}

func precisionDecimalBoundMembershipException(base PrecisionDecimalFacets, name string, value precisionDecimalValue) bool {
	switch name {
	case "minExclusive":
		if base.minExclusive != nil && precisionDecimalEqualOrIdentical(value, base.minExclusive.value) {
			return true
		}
	case "maxExclusive":
		if base.maxExclusive != nil && precisionDecimalEqualOrIdentical(value, base.maxExclusive.value) {
			return true
		}
	}
	return precisionDecimalFixedNaNBoundRedeclaration(base, name, value)
}

func precisionDecimalFixedNaNBoundRedeclaration(base PrecisionDecimalFacets, name string, value precisionDecimalValue) bool {
	if !precisionDecimalValueIsNaN(value) {
		return false
	}
	var baseValue precisionDecimalValue
	var fixed bool
	switch name {
	case "minInclusive":
		if base.minInclusive == nil {
			return false
		}
		baseValue = base.minInclusive.value
		fixed = base.minInclusive.fixed
	case "minExclusive":
		if base.minExclusive == nil {
			return false
		}
		baseValue = base.minExclusive.value
		fixed = base.minExclusive.fixed
	case "maxInclusive":
		if base.maxInclusive == nil {
			return false
		}
		baseValue = base.maxInclusive.value
		fixed = base.maxInclusive.fixed
	case "maxExclusive":
		if base.maxExclusive == nil {
			return false
		}
		baseValue = base.maxExclusive.value
		fixed = base.maxExclusive.fixed
	default:
		return false
	}
	return fixed && precisionDecimalEqualOrIdentical(value, baseValue)
}

func precisionDecimalEffectiveBoundLocations(base PrecisionDecimalFacets) []Loc {
	locations := make([]Loc, 0, 4)
	for _, location := range []Loc{
		precisionDecimalLocalMinInclusiveLoc(base.minInclusive),
		precisionDecimalLocalMinExclusiveLoc(base.minExclusive),
		precisionDecimalLocalMaxInclusiveLoc(base.maxInclusive),
		precisionDecimalLocalMaxExclusiveLoc(base.maxExclusive),
	} {
		if location.IsZero() {
			continue
		}
		locations = append(locations, location)
	}
	return locations
}

func precisionDecimalMembershipRelatedLocations(err error, primary Loc) []Loc {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return nil
	}
	locations := diagnostic.Related()
	if diagnostic.Loc().IsZero() || diagnostic.Loc() == primary {
		return locations
	}
	for _, location := range locations {
		if location == diagnostic.Loc() {
			return locations
		}
	}
	return append(locations, diagnostic.Loc())
}

func precisionDecimalShouldWrapMembershipError(err error) bool {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return false
	}
	return diagnostic.Class() == FailureInvalid && diagnostic.Code() == PrecisionDecimalFacetValueViolationCode
}

func precisionDecimalLocalEnumerationMembershipError(err error, member PrecisionDecimalEnumerationFacet) error {
	if !precisionDecimalShouldWrapMembershipError(err) {
		return err
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalEnumerationCode,
		member.Loc(),
		precisionDecimalEnumerationRestrictionSpecRef,
		"derived precisionDecimal enumeration member is outside the base value space",
		precisionDecimalMembershipRelatedLocations(err, member.Loc()),
		fmt.Errorf("%w: %w", errInvalidPrecisionDecimalEnumeration, err),
	)
}

func precisionDecimalLocalBoundMembershipError(err error, name string, loc Loc) error {
	if !precisionDecimalShouldWrapMembershipError(err) {
		return err
	}
	return newPrecisionDecimalFacetDiagnostic(
		FailureInvalid,
		InvalidPrecisionDecimalBoundCode,
		loc,
		precisionDecimalBoundRestrictionSpecRefForName(name),
		"derived precisionDecimal "+name+" is outside the base value space",
		precisionDecimalMembershipRelatedLocations(err, loc),
		fmt.Errorf("%w: %w", errInvalidPrecisionDecimalBound, err),
	)
}

func validatePrecisionDecimalLocalBoundValueState(local PrecisionDecimalFacetDeclarations) error {
	for _, bound := range []struct {
		present bool
		value   precisionDecimalValue
		lexical string
		loc     Loc
		spec    string
	}{
		{present: local.MinInclusive != nil, value: precisionDecimalLocalMinInclusiveValue(local.MinInclusive), lexical: precisionDecimalLocalMinInclusiveLexical(local.MinInclusive), loc: precisionDecimalLocalMinInclusiveLoc(local.MinInclusive), spec: precisionDecimalMinInclusiveValueSpecRef},
		{present: local.MinExclusive != nil, value: precisionDecimalLocalMinExclusiveValue(local.MinExclusive), lexical: precisionDecimalLocalMinExclusiveLexical(local.MinExclusive), loc: precisionDecimalLocalMinExclusiveLoc(local.MinExclusive), spec: precisionDecimalMinExclusiveValueSpecRef},
		{present: local.MaxInclusive != nil, value: precisionDecimalLocalMaxInclusiveValue(local.MaxInclusive), lexical: precisionDecimalLocalMaxInclusiveLexical(local.MaxInclusive), loc: precisionDecimalLocalMaxInclusiveLoc(local.MaxInclusive), spec: precisionDecimalMaxInclusiveValueSpecRef},
		{present: local.MaxExclusive != nil, value: precisionDecimalLocalMaxExclusiveValue(local.MaxExclusive), lexical: precisionDecimalLocalMaxExclusiveLexical(local.MaxExclusive), loc: precisionDecimalLocalMaxExclusiveLoc(local.MaxExclusive), spec: precisionDecimalMaxExclusiveValueSpecRef},
	} {
		if !bound.present {
			continue
		}
		if !precisionDecimalValueIsWellFormed(bound.value) || bound.lexical == "" {
			return newPrecisionDecimalFacetDiagnostic(
				FailureInvalid,
				InvalidPrecisionDecimalBoundCode,
				bound.loc,
				bound.spec,
				"completed precisionDecimal ordered bound has no valid value",
				nil,
				errInvalidPrecisionDecimalBound,
			)
		}
	}
	return nil
}

func validatePrecisionDecimalEffectiveBoundValueState(facets PrecisionDecimalFacets) error {
	for _, bound := range []struct {
		value precisionDecimalValue
		loc   Loc
		spec  string
	}{
		{value: precisionDecimalLocalMinInclusiveValue(facets.minInclusive), loc: precisionDecimalLocalMinInclusiveLoc(facets.minInclusive), spec: precisionDecimalMinInclusiveValueSpecRef},
		{value: precisionDecimalLocalMinExclusiveValue(facets.minExclusive), loc: precisionDecimalLocalMinExclusiveLoc(facets.minExclusive), spec: precisionDecimalMinExclusiveValueSpecRef},
		{value: precisionDecimalLocalMaxInclusiveValue(facets.maxInclusive), loc: precisionDecimalLocalMaxInclusiveLoc(facets.maxInclusive), spec: precisionDecimalMaxInclusiveValueSpecRef},
		{value: precisionDecimalLocalMaxExclusiveValue(facets.maxExclusive), loc: precisionDecimalLocalMaxExclusiveLoc(facets.maxExclusive), spec: precisionDecimalMaxExclusiveValueSpecRef},
	} {
		if bound.value == nil {
			continue
		}
		if !precisionDecimalValueIsWellFormed(bound.value) {
			return newPrecisionDecimalFacetDiagnostic(
				FailureInternal,
				InvalidPrecisionDecimalBoundCode,
				bound.loc,
				bound.spec,
				"completed precisionDecimal ordered bound has no valid value",
				nil,
				errInvalidPrecisionDecimalFacetState,
			)
		}
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
	valid := order == precisionDecimalOrderLess || (order == precisionDecimalOrderEqual && minInclusiveKind == maxInclusiveKind)
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

func precisionDecimalLocalMinInclusiveValue(facet *PrecisionDecimalMinInclusiveFacet) precisionDecimalValue {
	if facet == nil {
		return nil
	}
	return facet.value
}

func precisionDecimalLocalMinInclusiveLexical(facet *PrecisionDecimalMinInclusiveFacet) string {
	if facet == nil {
		return ""
	}
	return facet.normalizedLexical
}

func precisionDecimalLocalMinInclusiveLoc(facet *PrecisionDecimalMinInclusiveFacet) Loc {
	if facet == nil {
		return Loc{}
	}
	return facet.Loc()
}

func precisionDecimalLocalMinExclusiveValue(facet *PrecisionDecimalMinExclusiveFacet) precisionDecimalValue {
	if facet == nil {
		return nil
	}
	return facet.value
}

func precisionDecimalLocalMinExclusiveLexical(facet *PrecisionDecimalMinExclusiveFacet) string {
	if facet == nil {
		return ""
	}
	return facet.normalizedLexical
}

func precisionDecimalLocalMinExclusiveLoc(facet *PrecisionDecimalMinExclusiveFacet) Loc {
	if facet == nil {
		return Loc{}
	}
	return facet.Loc()
}

func precisionDecimalLocalMaxInclusiveValue(facet *PrecisionDecimalMaxInclusiveFacet) precisionDecimalValue {
	if facet == nil {
		return nil
	}
	return facet.value
}

func precisionDecimalLocalMaxInclusiveLexical(facet *PrecisionDecimalMaxInclusiveFacet) string {
	if facet == nil {
		return ""
	}
	return facet.normalizedLexical
}

func precisionDecimalLocalMaxInclusiveLoc(facet *PrecisionDecimalMaxInclusiveFacet) Loc {
	if facet == nil {
		return Loc{}
	}
	return facet.Loc()
}

func precisionDecimalLocalMaxExclusiveValue(facet *PrecisionDecimalMaxExclusiveFacet) precisionDecimalValue {
	if facet == nil {
		return nil
	}
	return facet.value
}

func precisionDecimalLocalMaxExclusiveLexical(facet *PrecisionDecimalMaxExclusiveFacet) string {
	if facet == nil {
		return ""
	}
	return facet.normalizedLexical
}

func precisionDecimalLocalMaxExclusiveLoc(facet *PrecisionDecimalMaxExclusiveFacet) Loc {
	if facet == nil {
		return Loc{}
	}
	return facet.Loc()
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
	applyPrecisionDecimalEnumerationFacets(effective, local.Enumeration)
	if err := applyPrecisionDecimalWhiteSpaceFacet(effective, base, local.WhiteSpace, derived); err != nil {
		return err
	}
	return applyPrecisionDecimalBoundFacets(effective, base, local, derived)
}

func applyPrecisionDecimalEnumerationFacets(effective *PrecisionDecimalFacets, local []PrecisionDecimalEnumerationFacet) {
	if local == nil {
		return
	}
	effective.enumeration = clonePrecisionDecimalEnumerationFacetsForEffective(local)
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
	if base.whiteSpace == nil {
		effective.whiteSpace.fixed = true
	}
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
	effective.minInclusive = clonePrecisionDecimalMinInclusiveFacetForEffective(&local)
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
	effective.minExclusive = clonePrecisionDecimalMinExclusiveFacetForEffective(&local)
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
	effective.maxInclusive = clonePrecisionDecimalMaxInclusiveFacetForEffective(&local)
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
	effective.maxExclusive = clonePrecisionDecimalMaxExclusiveFacetForEffective(&local)
	effective.maxInclusive = nil
	if base.maxExclusive != nil && base.maxExclusive.fixed {
		effective.maxExclusive.fixed = true
	}
	return nil
}

func validatePrecisionDecimalBoundRestriction(localValue precisionDecimalValue, localLoc Loc, baseValue precisionDecimalValue, baseLoc Loc, baseFixed bool, localInclusive, baseInclusive, lower bool) error {
	if baseFixed && localInclusive == baseInclusive && !precisionDecimalEqualOrIdentical(localValue, baseValue) {
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

func precisionDecimalBoundRestrictionSpecRefForName(name string) string {
	switch name {
	case "minInclusive":
		return precisionDecimalMinInclusiveRestrictionSpecRef
	case "minExclusive":
		return precisionDecimalMinExclusiveRestrictionSpecRef
	case "maxInclusive":
		return precisionDecimalMaxInclusiveRestrictionSpecRef
	default:
		return precisionDecimalMaxExclusiveRestrictionSpecRef
	}
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
			patternMatched, err := validatePrecisionDecimalPatternMatch(normalizedLexical, pattern)
			if err != nil {
				return err
			}
			if !patternMatched {
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

func validatePrecisionDecimalPatternMatch(normalizedLexical string, pattern PrecisionDecimalPatternFacet) (bool, error) {
	expression, err := parsePrecisionDecimalXMLRegex(pattern.source)
	if err != nil {
		if errors.Is(err, errPrecisionDecimalXMLRegexResourceLimit) {
			return false, newPrecisionDecimalPatternResourceDiagnostic(pattern.Loc(), err)
		}
		return false, newPrecisionDecimalFacetDiagnostic(
			FailureInternal,
			InvalidPrecisionDecimalPatternCode,
			pattern.Loc(),
			precisionDecimalPatternValueSpecRef,
			"completed precisionDecimal pattern is not valid XML Schema syntax",
			nil,
			fmt.Errorf("%w: %w", errInvalidPrecisionDecimalFacetState, err),
		)
	}
	matched, err := expression.match(normalizedLexical)
	if err == nil {
		return matched, nil
	}
	if errors.Is(err, errPrecisionDecimalXMLRegexResourceLimit) || errors.Is(err, errPrecisionDecimalXMLRegexMatchResourceLimit) {
		return false, newPrecisionDecimalPatternResourceDiagnostic(pattern.Loc(), err)
	}
	return false, newPrecisionDecimalFacetDiagnostic(
		FailureInternal,
		InvalidPrecisionDecimalPatternCode,
		pattern.Loc(),
		precisionDecimalPatternValueSpecRef,
		"completed precisionDecimal pattern matching failed",
		nil,
		fmt.Errorf("%w: %w", errInvalidPrecisionDecimalFacetState, err),
	)
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
		precisionDecimalTotalDigitsValidSpecRef,
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
			precisionDecimalMinScaleValidSpecRef,
			"precisionDecimal value is below minScale",
		)
	}
	if facets.maxScale != nil && finite.scale.Cmp(facets.maxScale.value.value) > 0 {
		return precisionDecimalFacetValueViolation(
			valueLoc,
			precisionDecimalRelatedLocation(facets.maxScale.Loc()),
			precisionDecimalMaxScaleValidSpecRef,
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
		if precisionDecimalEqualOrIdentical(value, member.value) {
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
