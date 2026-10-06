package goxsd9

import (
	"errors"
	"reflect"
)

type codegenBoundedIntegerKind struct {
	name    string
	minimum string
	maximum string
}

//nolint:gocognit // Exact bounds and enumeration facts share one integrity gate.
func validateCodegenBoundedIntegerFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool, kind codegenBoundedIntegerKind) error {
	namedFacets, named := facets.(schemaIntegerFacetVariant)
	if !builtin && !named {
		return newCodegenBoundedIntegerInternal(loc, context+" has inconsistent named integer facet facts", related, errCodegenSchemaInvariant, version, kind)
	}
	if err := validateCodegenNamedNonNegativeIntegerDigitFacts(loc, context, facets, version, related); err != nil {
		return codegenBoundedIntegerInternalFrom(err, version, kind)
	}
	bounds, err := codegenNonNegativeIntegerBounds(loc, context, facets, version, related)
	if err != nil {
		return codegenBoundedIntegerInternalFrom(err, version, kind)
	}
	if boundsErr := bounds.validate(); boundsErr != nil {
		return newCodegenBoundedIntegerInternal(loc, context+" has invalid integer bounds", related, codegenSchemaInvariantCause(boundsErr), version, kind)
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
		return newCodegenBoundedIntegerInternal(loc, context+" has incomplete effective "+kind.name+" bounds", related, errCodegenSchemaInvariant, version, kind)
	}
	intrinsicMinimum, err := ParseStrictInteger(kind.minimum, Loc{})
	if err != nil {
		return newCodegenBoundedIntegerInternal(loc, "construct "+kind.name+" minimum", related, err, version, kind)
	}
	intrinsicMaximum, err := ParseStrictInteger(kind.maximum, Loc{})
	if err != nil {
		return newCodegenBoundedIntegerInternal(loc, "construct "+kind.name+" maximum", related, err, version, kind)
	}
	if minimum.Compare(intrinsicMinimum) < 0 || maximum.Compare(intrinsicMaximum) > 0 {
		return newCodegenBoundedIntegerInternal(loc, context+" has bounds outside xs:"+kind.name, related, errCodegenSchemaInvariant, version, kind)
	}
	if builtin {
		return validateCodegenBuiltinBoundedIntegerFacts(loc, context, facets, bounds, version, related, kind)
	}
	for _, enumeration := range namedFacets.enumeration.Declarations() {
		value := enumeration.Value()
		if value.Compare(intrinsicMinimum) < 0 || value.Compare(intrinsicMaximum) > 0 {
			return newCodegenBoundedIntegerInternal(loc, context+" has enumeration outside xs:"+kind.name, appendCodegenRelated(related, enumeration.Loc()), errCodegenSchemaInvariant, version, kind)
		}
		if err := bounds.ValidateInteger(value, enumeration.Loc()); err != nil {
			return newCodegenBoundedIntegerInternal(loc, context+" has enumeration outside effective bounds", appendCodegenRelated(related, enumeration.Loc()), codegenSchemaInvariantCause(err), version, kind)
		}
		if err := namedFacets.digits.ValidateInteger(value, enumeration.Loc()); err != nil {
			return newCodegenBoundedIntegerInternal(loc, context+" has enumeration outside effective digit facets", appendCodegenRelated(related, enumeration.Loc()), codegenSchemaInvariantCause(err), version, kind)
		}
	}
	return nil
}

func validateCodegenNamedBoundedIntegerReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc, kind codegenBoundedIntegerKind) error {
	reference, hasReference := declaration.TypeReference()
	definition, hasDefinition := target.SimpleTypeDefinition()
	if hasReference && reference.facts != nil && hasDefinition && definition.facts != nil &&
		reflect.DeepEqual(reference.facts.facets, definition.facts.facets) {
		return nil
	}
	return newCodegenBoundedIntegerInternal(
		component.Loc(), "named global element "+kind.name+" facets differ from its type definition",
		appendCodegenRelated(related, reference.Loc()), errCodegenSchemaInvariant, version, kind,
	)
}

func validateCodegenBuiltinBoundedIntegerFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, bounds IntegerBoundFacets, version XSDVersion, related []Loc, kind codegenBoundedIntegerKind) error {
	digit, ok := facets.(schemaDigitFacetVariant)
	if !ok || digit.value.HasTotalDigits() || digit.decimalBounds.version != "" || digit.decimalBounds.lower != nil || digit.decimalBounds.upper != nil {
		return newCodegenBoundedIntegerInternal(loc, context+" has non-canonical built-in facet facts", related, errCodegenSchemaInvariant, version, kind)
	}
	ordered := bounds.Bounds()
	if len(ordered) != 2 || ordered[0].Kind() != BoundMinInclusive || ordered[0].Value().Canonical() != kind.minimum || ordered[1].Kind() != BoundMaxInclusive || ordered[1].Value().Canonical() != kind.maximum {
		return newCodegenBoundedIntegerInternal(loc, context+" has non-canonical built-in bounds", related, errCodegenSchemaInvariant, version, kind)
	}
	return nil
}

func codegenBoundedIntegerInternalFrom(err error, version XSDVersion, kind codegenBoundedIntegerKind) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return err
	}
	diagnostic.specRef = codegenBoundedIntegerSpecRef(version, kind)
	return diagnostic
}

func newCodegenBoundedIntegerInternal(loc Loc, message string, related []Loc, cause error, version XSDVersion, kind codegenBoundedIntegerKind) Diagnostic {
	var causeDiagnostic Diagnostic
	if errors.As(cause, &causeDiagnostic) {
		related = appendCodegenRelated(related, causeDiagnostic.Loc())
		related = mergeCodegenRelated(related, causeDiagnostic.Related())
	}
	diagnostic := newCodegenInternal(loc, message, related, cause)
	diagnostic.specRef = codegenBoundedIntegerSpecRef(version, kind)
	return diagnostic
}

func codegenBoundedIntegerSpecRef(version XSDVersion, kind codegenBoundedIntegerKind) string {
	if version == XSDVersion10 {
		return "xsd10-datatypes#" + kind.name
	}
	return "xsd11-datatypes#" + kind.name
}
