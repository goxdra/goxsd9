package goxsd9

const (
	codegenUnsignedLongMinimum = "0"
	codegenUnsignedLongMaximum = "18446744073709551615"
)

func codegenUnsignedLongKind() codegenBoundedIntegerKind {
	return codegenBoundedIntegerKind{name: "unsignedLong", minimum: codegenUnsignedLongMinimum, maximum: codegenUnsignedLongMaximum}
}

func validateCodegenUnsignedLongFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, codegenUnsignedLongKind())
}

func validateCodegenNamedUnsignedLongReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, codegenUnsignedLongKind())
}
