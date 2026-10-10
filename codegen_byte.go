package goxsd9

const (
	codegenByteMinimum = "-128"
	codegenByteMaximum = "127"
)

func codegenByteKind() codegenBoundedIntegerKind {
	return codegenBoundedIntegerKind{name: "byte", minimum: codegenByteMinimum, maximum: codegenByteMaximum}
}

func validateCodegenByteFacts(loc Loc, context string, facets schemaSimpleTypeFacetVariant, version XSDVersion, related []Loc, builtin bool) error {
	return validateCodegenBoundedIntegerFacts(loc, context, facets, version, related, builtin, codegenByteKind())
}

func validateCodegenNamedByteReferenceFacts(component Component, declaration ElementDeclaration, target Component, version XSDVersion, related []Loc) error {
	return validateCodegenNamedBoundedIntegerReferenceFacts(component, declaration, target, version, related, codegenByteKind())
}

func newCodegenByteInternal(loc Loc, message string, related []Loc, cause error, version XSDVersion) Diagnostic {
	return newCodegenBoundedIntegerInternal(loc, message, related, cause, version, codegenByteKind())
}

func codegenByteSpecRef(version XSDVersion) string {
	return codegenBoundedIntegerSpecRef(version, codegenByteKind())
}
