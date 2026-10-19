package goxsd9

// GenerateGo generates deterministic, formatted Go source for the supported
// scalar components and direct scalar choices or sequences in schema using
// packageName. Ordinary direct-choice/direct-sequence target checks reject
// modeled anonymous local inline types with located FailureUnsupported/ErrUnsupported
// diagnostics at element/particle locations; they may relate the anonymous type
// location and produce no output. A named complex type with effective
// AttributeUse facts rejects at the first use Loc before extension or
// group-reference checks, including direct-sequence extensions; definition
// and use locations are related. Otherwise, non-model-group-reference complex-content and
// model-less extensions reject at the extension Loc, relating complex-content,
// extension, base, particle, and anyAttribute locations when present. Direct
// and extension model-group references without effective uses reject at group
// RefLoc, with group/component/reference/target locations related.
// Global inline complex-content extensions and restrictions reject at their element Loc before
// named-base planning, including when their sequence or uses are omitted.
func GenerateGo(schema Schema, packageName string) ([]byte, error) {
	directParticlePlan, err := planCodegenDirectParticles(schema, packageName)
	if err != nil {
		return nil, err
	}
	return emitCodegenSourceWithDirectParticles(schema, directParticlePlan)
}
