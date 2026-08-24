package goxsd9

import (
	"errors"
	"fmt"
)

const (
	// InvalidEnumerationCode identifies an invalid enumeration declaration.
	InvalidEnumerationCode = "XSD2032"
	// InvalidEnumerationRestrictionCode identifies an invalid derived
	// enumeration declaration.
	InvalidEnumerationRestrictionCode = "XSD2033"
	// EnumerationValueViolationCode identifies a value outside an enumeration.
	EnumerationValueViolationCode = "XSD2034"
)

var (
	errInvalidEnumerationValue       = errors.New("invalid enumeration value")
	errInvalidEnumerationRestriction = errors.New("invalid enumeration restriction")
	errEnumerationValueViolation     = errors.New("enumeration value violation")
	errInvalidEnumerationState       = errors.New("invalid completed enumeration state")
	errInvalidEnumerationVersion     = errors.New("incompatible XSD version policy for enumeration")
)

const (
	diagnosticEnumerationEffectiveVersionCode = "GOXSD9036"
	diagnosticEnumerationEffectiveStateCode   = "GOXSD9037"
)

type enumerationRule uint8

const (
	enumerationDefinitionRule enumerationRule = iota + 1
	enumerationValueRule
	enumerationRestrictionRule
)

// IntegerEnumerationFacet is one immutable integer enumeration declaration.
type IntegerEnumerationFacet struct {
	value   StrictInteger
	loc     Loc
	version XSDVersion
}

// IntegerEnumerationValue is an alternate name for an integer enumeration
// declaration.
type IntegerEnumerationValue = IntegerEnumerationFacet

// Value returns the exact integer enumeration value.
func (facet IntegerEnumerationFacet) Value() StrictInteger {
	return cloneStrictInteger(facet.value)
}

// Loc returns the source location of the enumeration declaration.
func (facet IntegerEnumerationFacet) Loc() Loc {
	return facet.loc
}

// Version reports the XSD version policy used for the declaration.
func (facet IntegerEnumerationFacet) Version() XSDVersion {
	return facet.version
}

// DecimalEnumerationFacet is one immutable decimal enumeration declaration.
type DecimalEnumerationFacet struct {
	value   StrictDecimal
	loc     Loc
	version XSDVersion
}

// DecimalEnumerationValue is an alternate name for a decimal enumeration
// declaration.
type DecimalEnumerationValue = DecimalEnumerationFacet

// Value returns the exact decimal enumeration value.
func (facet DecimalEnumerationFacet) Value() StrictDecimal {
	return cloneStrictDecimal(facet.value)
}

// Loc returns the source location of the enumeration declaration.
func (facet DecimalEnumerationFacet) Loc() Loc {
	return facet.loc
}

// Version reports the XSD version policy used for the declaration.
func (facet DecimalEnumerationFacet) Version() XSDVersion {
	return facet.version
}

// ParseIntegerEnumerationFacet parses one integer enumeration declaration.
func ParseIntegerEnumerationFacet(lexical string, loc Loc, versions ...XSDVersion) (IntegerEnumerationFacet, error) {
	version, err := selectEnumerationVersion(versions)
	if err != nil {
		return IntegerEnumerationFacet{}, invalidEnumerationVersionDiagnostic(loc, err)
	}
	value, err := ParseStrictInteger(lexical, loc)
	if err != nil {
		return IntegerEnumerationFacet{}, invalidEnumerationLexicalDiagnostic(
			"integer",
			version,
			loc,
			err,
		)
	}
	return newIntegerEnumerationFacet(value, loc, version), nil
}

// ParseIntegerEnumerationFacetFor parses one integer enumeration declaration
// with an explicit XSD version.
func ParseIntegerEnumerationFacetFor(version XSDVersion, lexical string, loc Loc) (IntegerEnumerationFacet, error) {
	return ParseIntegerEnumerationFacet(lexical, loc, version)
}

// ParseDecimalEnumerationFacet parses one decimal enumeration declaration.
func ParseDecimalEnumerationFacet(lexical string, loc Loc, versions ...XSDVersion) (DecimalEnumerationFacet, error) {
	version, err := selectEnumerationVersion(versions)
	if err != nil {
		return DecimalEnumerationFacet{}, invalidEnumerationVersionDiagnostic(loc, err)
	}
	value, err := ParseStrictDecimal(lexical, loc, version)
	if err != nil {
		return DecimalEnumerationFacet{}, invalidEnumerationLexicalDiagnostic(
			"decimal",
			version,
			loc,
			err,
		)
	}
	return newDecimalEnumerationFacet(value, loc, version), nil
}

// ParseDecimalEnumerationFacetFor parses one decimal enumeration declaration
// with an explicit XSD version.
func ParseDecimalEnumerationFacetFor(version XSDVersion, lexical string, loc Loc) (DecimalEnumerationFacet, error) {
	return ParseDecimalEnumerationFacet(lexical, loc, version)
}

// NewIntegerEnumerationFacet constructs one integer enumeration declaration
// from a completed exact value.
func NewIntegerEnumerationFacet(value StrictInteger, loc Loc, versions ...XSDVersion) (IntegerEnumerationFacet, error) {
	version, err := selectEnumerationVersion(versions)
	if err != nil {
		return IntegerEnumerationFacet{}, invalidEnumerationVersionDiagnostic(loc, err)
	}
	return newIntegerEnumerationFacet(value, loc, version), nil
}

// NewDecimalEnumerationFacet constructs one decimal enumeration declaration
// from a completed exact value.
func NewDecimalEnumerationFacet(value StrictDecimal, loc Loc, versions ...XSDVersion) (DecimalEnumerationFacet, error) {
	version, err := selectEnumerationVersion(versions)
	if err != nil {
		return DecimalEnumerationFacet{}, invalidEnumerationVersionDiagnostic(loc, err)
	}
	return newDecimalEnumerationFacet(value, loc, version), nil
}

// IntegerEnumerationFacetDeclarations contains the ordered local integer
// enumeration declarations. A nil Values slice means the facet was omitted.
type IntegerEnumerationFacetDeclarations struct {
	Values []IntegerEnumerationFacet
}

// NewIntegerEnumerationFacetDeclarations makes an owned copy of local
// integer enumeration declarations.
func NewIntegerEnumerationFacetDeclarations(values []IntegerEnumerationFacet) IntegerEnumerationFacetDeclarations {
	return IntegerEnumerationFacetDeclarations{Values: cloneIntegerEnumerationFacets(values)}
}

// DecimalEnumerationFacetDeclarations contains the ordered local decimal
// enumeration declarations. A nil Values slice means the facet was omitted.
type DecimalEnumerationFacetDeclarations struct {
	Values []DecimalEnumerationFacet
}

// NewDecimalEnumerationFacetDeclarations makes an owned copy of local decimal
// enumeration declarations.
func NewDecimalEnumerationFacetDeclarations(values []DecimalEnumerationFacet) DecimalEnumerationFacetDeclarations {
	return DecimalEnumerationFacetDeclarations{Values: cloneDecimalEnumerationFacets(values)}
}

// IntegerEnumerationFacets is an immutable effective integer enumeration set.
type IntegerEnumerationFacets struct {
	version XSDVersion
	values  []IntegerEnumerationFacet
}

// NewIntegerEnumerationFacets constructs effective integer enumeration facets
// from ordered local declarations.
func NewIntegerEnumerationFacets(values []IntegerEnumerationFacet, versions ...XSDVersion) (IntegerEnumerationFacets, error) {
	local := NewIntegerEnumerationFacetDeclarations(values)
	version, err := resolveIntegerEnumerationVersion(versions, local.Values)
	if err != nil {
		return IntegerEnumerationFacets{}, invalidEnumerationVersionDiagnostic(integerEnumerationVersionLoc(local.Values), err)
	}
	base := IntegerEnumerationFacets{version: version}
	return completeIntegerEnumerationFacets(base, local, false)
}

// NewIntegerEnumerationFacetsFromDeclarations constructs effective integer
// enumeration facets from local declarations.
func NewIntegerEnumerationFacetsFromDeclarations(local IntegerEnumerationFacetDeclarations, versions ...XSDVersion) (IntegerEnumerationFacets, error) {
	return NewIntegerEnumerationFacets(local.Values, versions...)
}

// RestrictIntegerEnumerationFacets inherits an omitted enumeration from base
// and validates a present local enumeration as a value-space restriction.
func RestrictIntegerEnumerationFacets(base IntegerEnumerationFacets, local IntegerEnumerationFacetDeclarations) (IntegerEnumerationFacets, error) {
	if err := base.validate(); err != nil {
		return IntegerEnumerationFacets{}, err
	}
	return completeIntegerEnumerationFacets(base, NewIntegerEnumerationFacetDeclarations(local.Values), true)
}

// ConstructIntegerEnumerationFacets is the phase-oriented name for
// RestrictIntegerEnumerationFacets.
func ConstructIntegerEnumerationFacets(base IntegerEnumerationFacets, local IntegerEnumerationFacetDeclarations) (IntegerEnumerationFacets, error) {
	return RestrictIntegerEnumerationFacets(base, local)
}

// Version reports the explicit XSD version policy of the effective set.
func (facets IntegerEnumerationFacets) Version() XSDVersion {
	return facets.version
}

// HasEnumeration reports whether the effective set contains an enumeration
// facet. An omitted facet is distinct from a present declaration set.
func (facets IntegerEnumerationFacets) HasEnumeration() bool {
	return facets.values != nil
}

// Len reports the number of ordered effective declarations.
func (facets IntegerEnumerationFacets) Len() int {
	return len(facets.values)
}

// Values returns owned exact integer values in declaration order.
func (facets IntegerEnumerationFacets) Values() []StrictInteger {
	if facets.values == nil {
		return nil
	}
	values := make([]StrictInteger, len(facets.values))
	for index := range facets.values {
		values[index] = facets.values[index].Value()
	}
	return values
}

// Locations returns effective declaration locations in declaration order.
func (facets IntegerEnumerationFacets) Locations() []Loc {
	if facets.values == nil {
		return nil
	}
	locations := make([]Loc, len(facets.values))
	for index := range facets.values {
		locations[index] = facets.values[index].Loc()
	}
	return locations
}

// Declarations returns owned effective declarations in declaration order.
func (facets IntegerEnumerationFacets) Declarations() []IntegerEnumerationFacet {
	return cloneIntegerEnumerationFacets(facets.values)
}

// ValidateInteger validates an exact integer value against the effective
// enumeration. valueLoc is the primary location for a value violation.
func (facets IntegerEnumerationFacets) ValidateInteger(value StrictInteger, valueLoc Loc) error {
	if err := facets.validate(); err != nil {
		return err
	}
	if facets.values == nil {
		return nil
	}
	for index := range facets.values {
		if value.Equal(facets.values[index].value) {
			return nil
		}
	}
	return enumerationValueViolationDiagnostic(valueLoc, facets.Locations(), facets.version, "integer")
}

// ValidateIntegerEnumeration validates an exact integer against effective
// enumeration facets.
func ValidateIntegerEnumeration(value StrictInteger, facets IntegerEnumerationFacets, valueLoc Loc) error {
	return facets.ValidateInteger(value, valueLoc)
}

// ValidateIntegerEnumerationFacets validates an exact integer against
// effective enumeration facets.
func ValidateIntegerEnumerationFacets(value StrictInteger, facets IntegerEnumerationFacets, valueLoc Loc) error {
	return facets.ValidateInteger(value, valueLoc)
}

// DecimalEnumerationFacets is an immutable effective decimal enumeration set.
type DecimalEnumerationFacets struct {
	version XSDVersion
	values  []DecimalEnumerationFacet
}

// NewDecimalEnumerationFacets constructs effective decimal enumeration facets
// from ordered local declarations.
func NewDecimalEnumerationFacets(values []DecimalEnumerationFacet, versions ...XSDVersion) (DecimalEnumerationFacets, error) {
	local := NewDecimalEnumerationFacetDeclarations(values)
	version, err := resolveDecimalEnumerationVersion(versions, local.Values)
	if err != nil {
		return DecimalEnumerationFacets{}, invalidEnumerationVersionDiagnostic(decimalEnumerationVersionLoc(local.Values), err)
	}
	return completeDecimalEnumerationFacets(DecimalEnumerationFacets{version: version}, local, false)
}

// NewDecimalEnumerationFacetsFromDeclarations constructs effective decimal
// enumeration facets from local declarations.
func NewDecimalEnumerationFacetsFromDeclarations(local DecimalEnumerationFacetDeclarations, versions ...XSDVersion) (DecimalEnumerationFacets, error) {
	return NewDecimalEnumerationFacets(local.Values, versions...)
}

// RestrictDecimalEnumerationFacets inherits an omitted enumeration from base
// and validates a present local enumeration as a value-space restriction.
func RestrictDecimalEnumerationFacets(base DecimalEnumerationFacets, local DecimalEnumerationFacetDeclarations) (DecimalEnumerationFacets, error) {
	if err := base.validate(); err != nil {
		return DecimalEnumerationFacets{}, err
	}
	return completeDecimalEnumerationFacets(base, NewDecimalEnumerationFacetDeclarations(local.Values), true)
}

// ConstructDecimalEnumerationFacets is the phase-oriented name for
// RestrictDecimalEnumerationFacets.
func ConstructDecimalEnumerationFacets(base DecimalEnumerationFacets, local DecimalEnumerationFacetDeclarations) (DecimalEnumerationFacets, error) {
	return RestrictDecimalEnumerationFacets(base, local)
}

// Version reports the explicit XSD version policy of the effective set.
func (facets DecimalEnumerationFacets) Version() XSDVersion {
	return facets.version
}

// HasEnumeration reports whether the effective set contains an enumeration
// facet. An omitted facet is distinct from a present declaration set.
func (facets DecimalEnumerationFacets) HasEnumeration() bool {
	return facets.values != nil
}

// Len reports the number of ordered effective declarations.
func (facets DecimalEnumerationFacets) Len() int {
	return len(facets.values)
}

// Values returns owned exact decimal values in declaration order.
func (facets DecimalEnumerationFacets) Values() []StrictDecimal {
	if facets.values == nil {
		return nil
	}
	values := make([]StrictDecimal, len(facets.values))
	for index := range facets.values {
		values[index] = facets.values[index].Value()
	}
	return values
}

// Locations returns effective declaration locations in declaration order.
func (facets DecimalEnumerationFacets) Locations() []Loc {
	if facets.values == nil {
		return nil
	}
	locations := make([]Loc, len(facets.values))
	for index := range facets.values {
		locations[index] = facets.values[index].Loc()
	}
	return locations
}

// Declarations returns owned effective declarations in declaration order.
func (facets DecimalEnumerationFacets) Declarations() []DecimalEnumerationFacet {
	return cloneDecimalEnumerationFacets(facets.values)
}

// ValidateDecimal validates an exact decimal value against the effective
// enumeration. valueLoc is the primary location for a value violation.
func (facets DecimalEnumerationFacets) ValidateDecimal(value StrictDecimal, valueLoc Loc) error {
	if err := facets.validate(); err != nil {
		return err
	}
	if facets.values == nil {
		return nil
	}
	for index := range facets.values {
		if value.Equal(facets.values[index].value) {
			return nil
		}
	}
	return enumerationValueViolationDiagnostic(valueLoc, facets.Locations(), facets.version, "decimal")
}

// ValidateDecimalEnumeration validates an exact decimal against effective
// enumeration facets.
func ValidateDecimalEnumeration(value StrictDecimal, facets DecimalEnumerationFacets, valueLoc Loc) error {
	return facets.ValidateDecimal(value, valueLoc)
}

// ValidateDecimalEnumerationFacets validates an exact decimal against
// effective enumeration facets.
func ValidateDecimalEnumerationFacets(value StrictDecimal, facets DecimalEnumerationFacets, valueLoc Loc) error {
	return facets.ValidateDecimal(value, valueLoc)
}

func newIntegerEnumerationFacet(value StrictInteger, loc Loc, version XSDVersion) IntegerEnumerationFacet {
	return IntegerEnumerationFacet{value: cloneStrictInteger(value), loc: loc, version: version}
}

func newDecimalEnumerationFacet(value StrictDecimal, loc Loc, version XSDVersion) DecimalEnumerationFacet {
	return DecimalEnumerationFacet{value: cloneStrictDecimal(value), loc: loc, version: version}
}

func cloneIntegerEnumerationFacet(facet IntegerEnumerationFacet) IntegerEnumerationFacet {
	facet.value = cloneStrictInteger(facet.value)
	return facet
}

func cloneIntegerEnumerationFacets(values []IntegerEnumerationFacet) []IntegerEnumerationFacet {
	if values == nil {
		return nil
	}
	owned := make([]IntegerEnumerationFacet, len(values))
	for index := range values {
		owned[index] = cloneIntegerEnumerationFacet(values[index])
	}
	return owned
}

func cloneDecimalEnumerationFacet(facet DecimalEnumerationFacet) DecimalEnumerationFacet {
	facet.value = cloneStrictDecimal(facet.value)
	return facet
}

func cloneDecimalEnumerationFacets(values []DecimalEnumerationFacet) []DecimalEnumerationFacet {
	if values == nil {
		return nil
	}
	owned := make([]DecimalEnumerationFacet, len(values))
	for index := range values {
		owned[index] = cloneDecimalEnumerationFacet(values[index])
	}
	return owned
}

func cloneStrictDecimal(value StrictDecimal) StrictDecimal {
	return StrictDecimal{
		coefficient: value.coefficientCopy(),
		scale:       value.scale,
		negative:    value.negative,
	}
}

func selectEnumerationVersion(versions []XSDVersion) (XSDVersion, error) {
	return selectXSDVersion(versions)
}

func resolveIntegerEnumerationVersion(versions []XSDVersion, values []IntegerEnumerationFacet) (XSDVersion, error) {
	if len(versions) != 0 {
		return selectEnumerationVersion(versions)
	}
	if len(values) == 0 {
		return XSDVersion11, nil
	}
	version := values[0].Version()
	if version != XSDVersion10 && version != XSDVersion11 {
		return "", fmt.Errorf("%w: %q", errInvalidEnumerationVersion, version)
	}
	for index := 1; index < len(values); index++ {
		if values[index].Version() != version {
			return "", fmt.Errorf("%w: declarations use %q and %q", errInvalidEnumerationVersion, version, values[index].Version())
		}
	}
	return version, nil
}

func resolveDecimalEnumerationVersion(versions []XSDVersion, values []DecimalEnumerationFacet) (XSDVersion, error) {
	if len(versions) != 0 {
		return selectEnumerationVersion(versions)
	}
	if len(values) == 0 {
		return XSDVersion11, nil
	}
	version := values[0].Version()
	if version != XSDVersion10 && version != XSDVersion11 {
		return "", fmt.Errorf("%w: %q", errInvalidEnumerationVersion, version)
	}
	for index := 1; index < len(values); index++ {
		if values[index].Version() != version {
			return "", fmt.Errorf("%w: declarations use %q and %q", errInvalidEnumerationVersion, version, values[index].Version())
		}
	}
	return version, nil
}

func completeIntegerEnumerationFacets(base IntegerEnumerationFacets, local IntegerEnumerationFacetDeclarations, derived bool) (IntegerEnumerationFacets, error) {
	if err := base.validateForConstruction(); err != nil {
		return IntegerEnumerationFacets{}, err
	}
	if err := validateIntegerEnumerationDeclarations(local, base.version); err != nil {
		return IntegerEnumerationFacets{}, err
	}

	effective := IntegerEnumerationFacets{
		version: base.version,
		values:  cloneIntegerEnumerationFacets(base.values),
	}
	if local.Values == nil {
		return effective, nil
	}
	if derived && base.values != nil {
		for index := range local.Values {
			if integerEnumerationContains(base.values, local.Values[index].value) {
				continue
			}
			return IntegerEnumerationFacets{}, enumerationRestrictionDiagnostic(
				local.Values[index].Loc(),
				integerEnumerationLocations(base.values),
				base.version,
				"integer",
			)
		}
	}
	effective.values = cloneIntegerEnumerationFacets(local.Values)
	return effective, nil
}

func completeDecimalEnumerationFacets(base DecimalEnumerationFacets, local DecimalEnumerationFacetDeclarations, derived bool) (DecimalEnumerationFacets, error) {
	if err := base.validateForConstruction(); err != nil {
		return DecimalEnumerationFacets{}, err
	}
	if err := validateDecimalEnumerationDeclarations(local, base.version); err != nil {
		return DecimalEnumerationFacets{}, err
	}

	effective := DecimalEnumerationFacets{
		version: base.version,
		values:  cloneDecimalEnumerationFacets(base.values),
	}
	if local.Values == nil {
		return effective, nil
	}
	if derived && base.values != nil {
		for index := range local.Values {
			if decimalEnumerationContains(base.values, local.Values[index].value) {
				continue
			}
			return DecimalEnumerationFacets{}, enumerationRestrictionDiagnostic(
				local.Values[index].Loc(),
				decimalEnumerationLocations(base.values),
				base.version,
				"decimal",
			)
		}
	}
	effective.values = cloneDecimalEnumerationFacets(local.Values)
	return effective, nil
}

func validateIntegerEnumerationDeclarations(local IntegerEnumerationFacetDeclarations, version XSDVersion) error {
	if local.Values == nil {
		return nil
	}
	if len(local.Values) == 0 {
		return invalidEnumerationDeclarationDiagnostic(Loc{}, version, "enumeration declaration has no values")
	}
	for index := range local.Values {
		if local.Values[index].Version() == version {
			continue
		}
		return invalidEnumerationVersionDiagnostic(local.Values[index].Loc(), fmt.Errorf("%w: declaration uses %q, want %q", errInvalidEnumerationVersion, local.Values[index].Version(), version))
	}
	return nil
}

func validateDecimalEnumerationDeclarations(local DecimalEnumerationFacetDeclarations, version XSDVersion) error {
	if local.Values == nil {
		return nil
	}
	if len(local.Values) == 0 {
		return invalidEnumerationDeclarationDiagnostic(Loc{}, version, "enumeration declaration has no values")
	}
	for index := range local.Values {
		if local.Values[index].Version() == version {
			continue
		}
		return invalidEnumerationVersionDiagnostic(local.Values[index].Loc(), fmt.Errorf("%w: declaration uses %q, want %q", errInvalidEnumerationVersion, local.Values[index].Version(), version))
	}
	return nil
}

func (facets IntegerEnumerationFacets) validateForConstruction() error {
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newEnumerationDiagnostic(
			FailureInternal,
			diagnosticEnumerationEffectiveVersionCode,
			Loc{},
			"",
			"completed integer enumeration facets have an unknown XSD version",
			nil,
			errInvalidEnumerationState,
		)
	}
	if facets.values == nil {
		return nil
	}
	if len(facets.values) == 0 {
		return newEnumerationDiagnostic(
			FailureInternal,
			diagnosticEnumerationEffectiveStateCode,
			Loc{},
			enumerationSpecRef(facets.version, enumerationDefinitionRule),
			"completed integer enumeration facets contain no declarations",
			nil,
			errInvalidEnumerationState,
		)
	}
	for index := range facets.values {
		if facets.values[index].Version() == facets.version {
			continue
		}
		return newEnumerationDiagnostic(
			FailureInternal,
			diagnosticEnumerationEffectiveStateCode,
			facets.values[index].Loc(),
			enumerationSpecRef(facets.version, enumerationDefinitionRule),
			"completed integer enumeration declaration uses an incompatible XSD version",
			nil,
			errInvalidEnumerationState,
		)
	}
	return nil
}

func (facets IntegerEnumerationFacets) validate() error {
	return facets.validateForConstruction()
}

func (facets DecimalEnumerationFacets) validateForConstruction() error {
	if facets.version != XSDVersion10 && facets.version != XSDVersion11 {
		return newEnumerationDiagnostic(
			FailureInternal,
			diagnosticEnumerationEffectiveVersionCode,
			Loc{},
			"",
			"completed decimal enumeration facets have an unknown XSD version",
			nil,
			errInvalidEnumerationState,
		)
	}
	if facets.values == nil {
		return nil
	}
	if len(facets.values) == 0 {
		return newEnumerationDiagnostic(
			FailureInternal,
			diagnosticEnumerationEffectiveStateCode,
			Loc{},
			enumerationSpecRef(facets.version, enumerationDefinitionRule),
			"completed decimal enumeration facets contain no declarations",
			nil,
			errInvalidEnumerationState,
		)
	}
	for index := range facets.values {
		if facets.values[index].Version() == facets.version {
			continue
		}
		return newEnumerationDiagnostic(
			FailureInternal,
			diagnosticEnumerationEffectiveStateCode,
			facets.values[index].Loc(),
			enumerationSpecRef(facets.version, enumerationDefinitionRule),
			"completed decimal enumeration declaration uses an incompatible XSD version",
			nil,
			errInvalidEnumerationState,
		)
	}
	return nil
}

func (facets DecimalEnumerationFacets) validate() error {
	return facets.validateForConstruction()
}

func integerEnumerationContains(values []IntegerEnumerationFacet, candidate StrictInteger) bool {
	for index := range values {
		if candidate.Equal(values[index].value) {
			return true
		}
	}
	return false
}

func decimalEnumerationContains(values []DecimalEnumerationFacet, candidate StrictDecimal) bool {
	for index := range values {
		if candidate.Equal(values[index].value) {
			return true
		}
	}
	return false
}

func integerEnumerationLocations(values []IntegerEnumerationFacet) []Loc {
	locations := make([]Loc, 0, len(values))
	for index := range values {
		if values[index].Loc().IsZero() {
			continue
		}
		locations = append(locations, values[index].Loc())
	}
	return locations
}

func decimalEnumerationLocations(values []DecimalEnumerationFacet) []Loc {
	locations := make([]Loc, 0, len(values))
	for index := range values {
		if values[index].Loc().IsZero() {
			continue
		}
		locations = append(locations, values[index].Loc())
	}
	return locations
}

func integerEnumerationVersionLoc(values []IntegerEnumerationFacet) Loc {
	if len(values) == 0 {
		return Loc{}
	}
	return values[0].Loc()
}

func decimalEnumerationVersionLoc(values []DecimalEnumerationFacet) Loc {
	if len(values) == 0 {
		return Loc{}
	}
	return values[0].Loc()
}

func invalidEnumerationVersionDiagnostic(loc Loc, cause error) Diagnostic {
	return newDiagnostic(FailureInvalid, InvalidXSDVersionCode, loc, "invalid XSD version policy for enumeration", cause)
}

func invalidEnumerationLexicalDiagnostic(datatype string, version XSDVersion, loc Loc, cause error) Diagnostic {
	return newEnumerationDiagnostic(
		FailureInvalid,
		InvalidEnumerationCode,
		loc,
		enumerationSpecRef(version, enumerationDefinitionRule),
		"invalid "+datatype+" enumeration facet value",
		nil,
		fmt.Errorf("%w: %w", errInvalidEnumerationValue, cause),
	)
}

func invalidEnumerationDeclarationDiagnostic(loc Loc, version XSDVersion, message string) Diagnostic {
	return newEnumerationDiagnostic(
		FailureInvalid,
		InvalidEnumerationCode,
		loc,
		enumerationSpecRef(version, enumerationDefinitionRule),
		message,
		nil,
		errInvalidEnumerationValue,
	)
}

func enumerationRestrictionDiagnostic(loc Loc, related []Loc, version XSDVersion, datatype string) Diagnostic {
	return newEnumerationDiagnostic(
		FailureInvalid,
		InvalidEnumerationRestrictionCode,
		loc,
		enumerationSpecRef(version, enumerationRestrictionRule),
		"local "+datatype+" enumeration value is not in the base enumeration",
		related,
		fmt.Errorf("%w: local value is outside the base enumeration", errInvalidEnumerationRestriction),
	)
}

func enumerationValueViolationDiagnostic(loc Loc, related []Loc, version XSDVersion, datatype string) Diagnostic {
	return newEnumerationDiagnostic(
		FailureInvalid,
		EnumerationValueViolationCode,
		loc,
		enumerationSpecRef(version, enumerationValueRule),
		"value is not in the "+datatype+" enumeration",
		related,
		fmt.Errorf("%w: candidate is outside the effective enumeration", errEnumerationValueViolation),
	)
}

func enumerationSpecRef(version XSDVersion, rule enumerationRule) string {
	if version == XSDVersion10 {
		return enumerationSpecRefForPrefix("xsd10", rule)
	}
	if version == XSDVersion11 {
		return enumerationSpecRefForPrefix("xsd11", rule)
	}
	return ""
}

func enumerationSpecRefForPrefix(prefix string, rule enumerationRule) string {
	switch rule {
	case enumerationDefinitionRule:
		return prefix + "-datatypes#rf-enumeration"
	case enumerationValueRule:
		return prefix + "-datatypes#cvc-enumeration-valid"
	case enumerationRestrictionRule:
		return prefix + "-datatypes#enumeration-valid-restriction"
	default:
		return prefix + "-datatypes#rf-enumeration"
	}
}

func newEnumerationDiagnostic(class FailureClass, code string, loc Loc, specRef, message string, related []Loc, cause error) Diagnostic {
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
