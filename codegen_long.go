package goxsd9

import (
	"errors"
	"reflect"
)

const (
	codegenLongMinimum = "-9223372036854775808"
	codegenLongMaximum = "9223372036854775807"
)

//nolint:gocognit // Exact long bounds and enumeration facts share one integrity gate.
func validateCodegenLongFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	if err := validateCodegenNamedNonNegativeIntegerDigitFacts(loc, context, facets, version, related); err != nil {
		return codegenLongInternalFrom(err, version)
	}
	bounds, err := codegenNonNegativeIntegerBounds(loc, context, facets, version, related)
	if err != nil {
		return codegenLongInternalFrom(err, version)
	}
	if boundsErr := bounds.validate(); boundsErr != nil {
		return newCodegenLongInternal(loc, context+" has invalid integer bounds", related, codegenSchemaInvariantCause(boundsErr), version)
	}
	minimum, hasMinimum := bounds.MinInclusive()
	if !hasMinimum {
		minimum, hasMinimum = bounds.MinExclusive()
	}
	maximum, hasMaximum := bounds.MaxInclusive()
	if !hasMaximum {
		maximum, hasMaximum = bounds.MaxExclusive()
	}
	if !hasMinimum || !hasMaximum {
		return newCodegenLongInternal(loc, context+" has incomplete effective long bounds", related, errCodegenSchemaInvariant, version)
	}
	longMinimum, err := ParseStrictInteger(codegenLongMinimum, Loc{})
	if err != nil {
		return newCodegenLongInternal(loc, "construct long minimum", related, err, version)
	}
	longMaximum, err := ParseStrictInteger(codegenLongMaximum, Loc{})
	if err != nil {
		return newCodegenLongInternal(loc, "construct long maximum", related, err, version)
	}
	if minimum.Compare(longMinimum) < 0 || maximum.Compare(longMaximum) > 0 {
		return newCodegenLongInternal(loc, context+" has bounds outside xs:long", related, errCodegenSchemaInvariant, version)
	}
	if builtin {
		return validateCodegenBuiltinLongFacts(loc, context, facets, bounds, version, related)
	}
	if typed, ok := facets.(schemaIntegerFacetVariant); ok {
		for _, enumeration := range typed.enumeration.Declarations() {
			value := enumeration.Value()
			if value.Compare(longMinimum) < 0 || value.Compare(longMaximum) > 0 {
				return newCodegenLongInternal(loc, context+" has enumeration outside xs:long", appendCodegenRelated(related, enumeration.Loc()), errCodegenSchemaInvariant, version)
			}
			if err := bounds.ValidateInteger(value, enumeration.Loc()); err != nil {
				return newCodegenLongInternal(loc, context+" has enumeration outside effective bounds", appendCodegenRelated(related, enumeration.Loc()), codegenSchemaInvariantCause(err), version)
			}
		}
	}
	return nil
}

func validateCodegenNamedLongReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	reference, hasReference := declaration.TypeReference()
	definition, hasDefinition := target.SimpleTypeDefinition()
	if hasReference && reference.facts != nil && hasDefinition && definition.facts != nil &&
		reflect.DeepEqual(reference.facts.facets, definition.facts.facets) {
		return nil
	}
	return newCodegenLongInternal(
		component.Loc(), "named global element long facets differ from its type definition",
		appendCodegenRelated(related, reference.Loc()), errCodegenSchemaInvariant, version,
	)
}

func validateCodegenBuiltinLongFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, bounds IntegerBoundFacets, version XSDVersion, related []Loc) error {
	digit, ok := facets.(schemaDigitFacetVariant)
	if !ok || digit.value.HasTotalDigits() || digit.decimalBounds.version != "" || digit.decimalBounds.lower != nil || digit.decimalBounds.upper != nil {
		return newCodegenLongInternal(loc, context+" has non-canonical built-in facet facts", related, errCodegenSchemaInvariant, version)
	}
	ordered := bounds.Bounds()
	if len(ordered) != 2 || ordered[0].Kind() != BoundMinInclusive || ordered[0].Value().Canonical() != codegenLongMinimum || ordered[1].Kind() != BoundMaxInclusive || ordered[1].Value().Canonical() != codegenLongMaximum {
		return newCodegenLongInternal(loc, context+" has non-canonical built-in bounds", related, errCodegenSchemaInvariant, version)
	}
	return nil
}

func codegenLongInternalFrom(err error, version XSDVersion) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return err
	}
	diagnostic.specRef = codegenLongSpecRef(version)
	return diagnostic
}

func newCodegenLongInternal(loc Loc, message string, related []Loc, cause error, version XSDVersion) Diagnostic {
	diagnostic := newCodegenInternal(loc, message, related, cause)
	diagnostic.specRef = codegenLongSpecRef(version)
	return diagnostic
}

func codegenLongSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-datatypes#long"
	}
	return "xsd11-datatypes#long"
}
