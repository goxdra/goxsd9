package goxsd9

import (
	"errors"
	"fmt"
)

const (
	// InvalidStringWhiteSpaceCode identifies an invalid string whiteSpace
	// declaration.
	InvalidStringWhiteSpaceCode = "XSD2035"
	// InvalidStringWhiteSpaceRestrictionCode identifies an invalid derived
	// string whiteSpace declaration.
	InvalidStringWhiteSpaceRestrictionCode = "XSD2036"
)

const (
	stringWhiteSpaceXSD10SpecRef = "xsd10-datatypes#rf-whiteSpace"
	stringWhiteSpaceXSD11SpecRef = "xsd11-datatypes#rf-whiteSpace"
)

const diagnosticStringWhiteSpaceStateCode = "GOXSD9042"

var (
	errInvalidStringWhiteSpaceValue       = errors.New("invalid string whiteSpace value")
	errInvalidStringWhiteSpaceRestriction = errors.New("invalid string whiteSpace restriction")
	errInvalidStringWhiteSpaceState       = errors.New("invalid completed string whiteSpace state")
	errDuplicateStringWhiteSpaceFacet     = errors.New("duplicate string whiteSpace facet")
	errInvalidStringWhiteSpaceVersion     = errors.New("incompatible XSD version policy for string whiteSpace")
)

// StringWhiteSpaceFacet is one immutable effective string whiteSpace
// declaration.
type StringWhiteSpaceFacet struct {
	value string
	loc   Loc
	fixed bool
}

// Value returns the effective whiteSpace value.
func (facet StringWhiteSpaceFacet) Value() string {
	return facet.value
}

// Loc returns the source location of the declaration that supplied the
// effective value. Built-in xs:string and xs:token have zero locations.
func (facet StringWhiteSpaceFacet) Loc() Loc {
	return facet.loc
}

// Fixed reports whether the effective whiteSpace value is fixed for derived
// restrictions.
func (facet StringWhiteSpaceFacet) Fixed() bool {
	return facet.fixed
}

// WithFixed returns a copy with the requested fixed property.
func (facet StringWhiteSpaceFacet) WithFixed(fixed bool) StringWhiteSpaceFacet {
	facet.fixed = fixed
	return facet
}

// ParseStringWhiteSpaceFacet parses one string whiteSpace declaration.
func ParseStringWhiteSpaceFacet(lexical string, loc Loc, versions ...XSDVersion) (StringWhiteSpaceFacet, error) {
	version, err := selectXSDVersion(versions)
	if err != nil {
		return StringWhiteSpaceFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			"invalid XSD version policy for string whiteSpace",
			fmt.Errorf("%w: %w", errInvalidStringWhiteSpaceVersion, err),
		)
	}
	return parseStringWhiteSpaceFacetFor(version, lexical, loc, false)
}

// ParseStringWhiteSpaceFacetFor parses one string whiteSpace declaration
// under an explicit XSD version.
func ParseStringWhiteSpaceFacetFor(version XSDVersion, lexical string, loc Loc) (StringWhiteSpaceFacet, error) {
	return parseStringWhiteSpaceFacetFor(version, lexical, loc, false)
}

// ParseStringWhiteSpaceFacetWithFixed parses one string whiteSpace
// declaration, including its fixed property.
func ParseStringWhiteSpaceFacetWithFixed(lexical string, loc Loc, fixed bool, versions ...XSDVersion) (StringWhiteSpaceFacet, error) {
	version, err := selectXSDVersion(versions)
	if err != nil {
		return StringWhiteSpaceFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			"invalid XSD version policy for string whiteSpace",
			fmt.Errorf("%w: %w", errInvalidStringWhiteSpaceVersion, err),
		)
	}
	return parseStringWhiteSpaceFacetFor(version, lexical, loc, fixed)
}

// ParseStringWhiteSpaceFacetForWithFixed parses one string whiteSpace
// declaration with an explicit XSD version and fixed property.
func ParseStringWhiteSpaceFacetForWithFixed(version XSDVersion, lexical string, loc Loc, fixed bool) (StringWhiteSpaceFacet, error) {
	return parseStringWhiteSpaceFacetFor(version, lexical, loc, fixed)
}

func parseStringWhiteSpaceFacetFor(version XSDVersion, lexical string, loc Loc, fixed bool) (StringWhiteSpaceFacet, error) {
	if version != XSDVersion10 && version != XSDVersion11 {
		return StringWhiteSpaceFacet{}, newDiagnostic(
			FailureInvalid,
			InvalidXSDVersionCode,
			loc,
			"invalid XSD version policy for string whiteSpace",
			fmt.Errorf("%w: %q", errInvalidStringWhiteSpaceVersion, version),
		)
	}
	value := collapseXMLWhitespace(lexical)
	if _, ok := stringWhiteSpaceRestrictionRank(value); !ok {
		return StringWhiteSpaceFacet{}, newStringWhiteSpaceDiagnostic(
			FailureInvalid,
			InvalidStringWhiteSpaceCode,
			loc,
			stringWhiteSpaceSpecRef(version),
			"invalid string whiteSpace facet value",
			nil,
			fmt.Errorf("%w: %q", errInvalidStringWhiteSpaceValue, value),
		)
	}
	return StringWhiteSpaceFacet{value: value, loc: loc, fixed: fixed}, nil
}

func defaultStringWhiteSpaceFacet() *StringWhiteSpaceFacet {
	return &StringWhiteSpaceFacet{value: "preserve"}
}

func defaultTokenWhiteSpaceFacet() *StringWhiteSpaceFacet {
	return &StringWhiteSpaceFacet{value: "collapse", fixed: true}
}

func cloneStringWhiteSpaceFacet(facet *StringWhiteSpaceFacet) *StringWhiteSpaceFacet {
	if facet == nil {
		return nil
	}
	owned := *facet
	return &owned
}

func restrictStringWhiteSpaceFacet(base, local *StringWhiteSpaceFacet, version XSDVersion) (StringWhiteSpaceFacet, error) {
	if base == nil {
		base = defaultStringWhiteSpaceFacet()
	}
	if err := validateStringWhiteSpaceFacetState(*base, version); err != nil {
		return StringWhiteSpaceFacet{}, err
	}
	if local == nil {
		return *cloneStringWhiteSpaceFacet(base), nil
	}
	if err := validateStringWhiteSpaceFacetState(*local, version); err != nil {
		return StringWhiteSpaceFacet{}, err
	}
	if base.fixed && local.value != base.value {
		return StringWhiteSpaceFacet{}, stringWhiteSpaceRestrictionDiagnostic(
			local.loc,
			base.loc,
			version,
			"derived string whiteSpace changes a fixed base facet",
		)
	}
	baseRank, _ := stringWhiteSpaceRestrictionRank(base.value)
	localRank, _ := stringWhiteSpaceRestrictionRank(local.value)
	if localRank < baseRank {
		return StringWhiteSpaceFacet{}, stringWhiteSpaceRestrictionDiagnostic(
			local.loc,
			base.loc,
			version,
			"derived string whiteSpace is less restrictive than its base",
		)
	}
	effective := *cloneStringWhiteSpaceFacet(local)
	if base.fixed {
		effective.fixed = true
	}
	return effective, nil
}

func validateStringWhiteSpaceFacetInputs(inputs []schemaFacetInput, version XSDVersion) error {
	for _, input := range inputs {
		if input.kind != schemaFacetWhiteSpace {
			continue
		}
		if _, err := ParseStringWhiteSpaceFacetFor(version, input.lexical, schemaFacetValueLocation(input)); err != nil {
			return err
		}
	}
	return nil
}

func validateStringWhiteSpaceFacetState(facet StringWhiteSpaceFacet, version XSDVersion) error {
	if _, ok := stringWhiteSpaceRestrictionRank(facet.value); ok {
		return nil
	}
	return newStringWhiteSpaceDiagnostic(
		FailureInternal,
		diagnosticStringWhiteSpaceStateCode,
		facet.loc,
		stringWhiteSpaceSpecRef(version),
		"completed string whiteSpace facet has an invalid value",
		nil,
		errInvalidStringWhiteSpaceState,
	)
}

func stringWhiteSpaceRestrictionRank(value string) (int, bool) {
	switch value {
	case "preserve":
		return 0, true
	case "replace":
		return 1, true
	case "collapse":
		return 2, true
	default:
		return 0, false
	}
}

func stringWhiteSpaceSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return stringWhiteSpaceXSD10SpecRef
	}
	if version == XSDVersion11 {
		return stringWhiteSpaceXSD11SpecRef
	}
	return ""
}

func newStringWhiteSpaceDiagnostic(class FailureClass, code string, loc Loc, specRef, message string, related []Loc, cause error) Diagnostic {
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

func stringWhiteSpaceRestrictionDiagnostic(loc, baseLoc Loc, version XSDVersion, message string) Diagnostic {
	related := make([]Loc, 0, 1)
	if !baseLoc.IsZero() && baseLoc != loc {
		related = append(related, baseLoc)
	}
	return newStringWhiteSpaceDiagnostic(
		FailureInvalid,
		InvalidStringWhiteSpaceRestrictionCode,
		loc,
		stringWhiteSpaceSpecRef(version),
		message,
		related,
		fmt.Errorf("%w: %s", errInvalidStringWhiteSpaceRestriction, message),
	)
}

func duplicateStringWhiteSpaceFacetDiagnostic(loc Loc, version XSDVersion) Diagnostic {
	return newStringWhiteSpaceDiagnostic(
		FailureInvalid,
		InvalidStringWhiteSpaceCode,
		loc,
		stringWhiteSpaceSpecRef(version),
		"simple type restriction whiteSpace facet must be unique",
		nil,
		errDuplicateStringWhiteSpaceFacet,
	)
}

func stringWhiteSpaceFacetSyntaxValueLocation(element *syntaxElement) Loc {
	if element == nil {
		return Loc{}
	}
	attributes := syntaxAttributesByLocal(element, "value")
	if len(attributes) > 0 {
		return attributes[0].loc
	}
	return element.loc
}
