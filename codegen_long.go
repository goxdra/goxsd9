package goxsd9

import (
	"errors"
	"reflect"
)

const (
	codegenLongMinimum = "-9223372036854775808"
	codegenLongMaximum = "9223372036854775807"
	codegenIntMinimum  = "-2147483648"
	codegenIntMaximum  = "2147483647"
)

func validateCodegenLongFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, "long", codegenLongMinimum, codegenLongMaximum)
}

func validateCodegenIntFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, "int", codegenIntMinimum, codegenIntMaximum)
}

//nolint:gocognit // Exact bounded integer facts share one integrity gate.
func validateCodegenBoundedIntegerFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool, name, lower, upper string) error {
	if err := validateCodegenNamedNonNegativeIntegerDigitFacts(loc, context, facets, version, related); err != nil {
		return codegenBoundedIntegerInternalFrom(err, version, name)
	}
	bounds, err := codegenNonNegativeIntegerBounds(loc, context, facets, version, related)
	if err != nil {
		return codegenBoundedIntegerInternalFrom(err, version, name)
	}
	if boundsErr := bounds.validate(); boundsErr != nil {
		return newCodegenBoundedIntegerInternal(loc, context+" has invalid integer bounds", related, codegenSchemaInvariantCause(boundsErr), version, name)
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
		return newCodegenBoundedIntegerInternal(loc, context+" has incomplete effective "+name+" bounds", related, errCodegenSchemaInvariant, version, name)
	}
	intrinsicMinimum, err := ParseStrictInteger(lower, Loc{})
	if err != nil {
		return newCodegenBoundedIntegerInternal(loc, "construct "+name+" minimum", related, err, version, name)
	}
	intrinsicMaximum, err := ParseStrictInteger(upper, Loc{})
	if err != nil {
		return newCodegenBoundedIntegerInternal(loc, "construct "+name+" maximum", related, err, version, name)
	}
	if minimum.Compare(intrinsicMinimum) < 0 || maximum.Compare(intrinsicMaximum) > 0 {
		return newCodegenBoundedIntegerInternal(loc, context+" has bounds outside xs:"+name, related, errCodegenSchemaInvariant, version, name)
	}
	if builtin {
		return validateCodegenBuiltinBoundedIntegerFacts(loc, context, facets, bounds, version, related, name, lower, upper)
	}
	if typed, ok := facets.(schemaIntegerFacetVariant); ok {
		for _, enumeration := range typed.enumeration.Declarations() {
			value := enumeration.Value()
			if value.Compare(intrinsicMinimum) < 0 || value.Compare(intrinsicMaximum) > 0 {
				return newCodegenBoundedIntegerInternal(loc, context+" has enumeration outside xs:"+name, appendCodegenRelated(related, enumeration.Loc()), errCodegenSchemaInvariant, version, name)
			}
			if err := bounds.ValidateInteger(value, enumeration.Loc()); err != nil {
				return newCodegenBoundedIntegerInternal(loc, context+" has enumeration outside effective bounds", appendCodegenRelated(related, enumeration.Loc()), codegenSchemaInvariantCause(err), version, name)
			}
			if err := typed.digits.ValidateInteger(value, enumeration.Loc()); err != nil {
				return newCodegenBoundedIntegerInternal(loc, context+" has enumeration outside effective digit facets", appendCodegenRelated(related, enumeration.Loc()), codegenSchemaInvariantCause(err), version, name)
			}
		}
	}
	return nil
}

func validateCodegenNamedLongReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, "long")
}

func validateCodegenNamedIntReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, "int")
}

func validateCodegenNamedBoundedIntegerReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc, name string) error {
	reference, hasReference := declaration.TypeReference()
	definition, hasDefinition := target.SimpleTypeDefinition()
	if hasReference && reference.facts != nil && hasDefinition && definition.facts != nil &&
		reflect.DeepEqual(reference.facts.facets, definition.facts.facets) {
		return nil
	}
	return newCodegenBoundedIntegerInternal(
		component.Loc(), "named global element "+name+" facets differ from its type definition",
		appendCodegenRelated(related, reference.Loc()), errCodegenSchemaInvariant, version, name,
	)
}

func validateCodegenBuiltinBoundedIntegerFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, bounds IntegerBoundFacets, version XSDVersion, related []Loc, name, lower, upper string) error {
	digit, ok := facets.(schemaDigitFacetVariant)
	if !ok || digit.value.HasTotalDigits() || digit.decimalBounds.version != "" || digit.decimalBounds.lower != nil || digit.decimalBounds.upper != nil {
		return newCodegenBoundedIntegerInternal(loc, context+" has non-canonical built-in facet facts", related, errCodegenSchemaInvariant, version, name)
	}
	ordered := bounds.Bounds()
	if len(ordered) != 2 || ordered[0].Kind() != BoundMinInclusive || ordered[0].Value().Canonical() != lower || ordered[1].Kind() != BoundMaxInclusive || ordered[1].Value().Canonical() != upper {
		return newCodegenBoundedIntegerInternal(loc, context+" has non-canonical built-in bounds", related, errCodegenSchemaInvariant, version, name)
	}
	return nil
}

func codegenBoundedIntegerInternalFrom(err error, version XSDVersion, name string) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return err
	}
	diagnostic.specRef = codegenBoundedIntegerSpecRef(version, name)
	return diagnostic
}

func newCodegenBoundedIntegerInternal(loc Loc, message string, related []Loc, cause error, version XSDVersion, name string) Diagnostic {
	var causeDiagnostic Diagnostic
	if errors.As(cause, &causeDiagnostic) {
		related = appendCodegenRelated(related, causeDiagnostic.Loc())
		related = mergeCodegenRelated(related, causeDiagnostic.Related())
	}
	diagnostic := newCodegenInternal(loc, message, related, cause)
	diagnostic.specRef = codegenBoundedIntegerSpecRef(version, name)
	return diagnostic
}

func codegenLongSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, "long")
}

func codegenIntSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, "int")
}

func codegenBoundedIntegerSpecRef(version XSDVersion, name string) string {
	if version == XSDVersion10 {
		return "xsd10-datatypes#" + name
	}
	return "xsd11-datatypes#" + name
}
