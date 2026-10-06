package goxsd9

const (
	codegenShortMinimum = "-32768"
	codegenShortMaximum = "32767"
)

func codegenShortKind() codegenBoundedInteger {
	return codegenBoundedInteger{name: "short", minimum: codegenShortMinimum, maximum: codegenShortMaximum}
}

func validateCodegenShortFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, codegenShortKind())
}

func validateCodegenNamedShortReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, codegenShortKind())
}

func newCodegenShortInternal(loc Loc, message string, related []Loc, cause error, version XSDVersion) Diagnostic {
	return newCodegenBoundedIntegerInternal(loc, message, related, cause, version, codegenShortKind())
}

func codegenShortSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, codegenShortKind())
}
