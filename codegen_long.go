package goxsd9

const (
	codegenLongMinimum = "-9223372036854775808"
	codegenLongMaximum = "9223372036854775807"
	codegenIntMinimum  = "-2147483648"
	codegenIntMaximum  = "2147483647"
)

func codegenLongKind() codegenBoundedIntegerKind {
	return codegenBoundedIntegerKind{name: "long", minimum: codegenLongMinimum, maximum: codegenLongMaximum}
}

func codegenIntKind() codegenBoundedIntegerKind {
	return codegenBoundedIntegerKind{name: "int", minimum: codegenIntMinimum, maximum: codegenIntMaximum}
}

func validateCodegenLongFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, codegenLongKind())
}

func validateCodegenIntFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, codegenIntKind())
}

func validateCodegenNamedLongReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, codegenLongKind())
}

func validateCodegenNamedIntReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, codegenIntKind())
}

func codegenLongSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, codegenLongKind())
}

func codegenIntSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, codegenIntKind())
}
