package goxsd9

const (
	codegenLongMinimum = "-9223372036854775808"
	codegenLongMaximum = "9223372036854775807"
)

func codegenLongKind() codegenBoundedInteger {
	return codegenBoundedInteger{name: "long", minimum: codegenLongMinimum, maximum: codegenLongMaximum}
}

func validateCodegenLongFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, codegenLongKind())
}

func validateCodegenNamedLongReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, codegenLongKind())
}

func codegenLongSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, codegenLongKind())
}
