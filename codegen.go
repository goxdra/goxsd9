package goxsd9

// GenerateGo generates deterministic, formatted Go source for the supported
// scalar components and direct scalar choices or sequences in schema using
// packageName. Ordinary direct-choice/direct-sequence target checks reject
// modeled anonymous local inline types with located FailureUnsupported/ErrUnsupported
// diagnostics at element/particle locations; they may relate the anonymous type
// location and produce no output. Non-model-group-reference complex-content and
// model-less extension gates reject at the extension boundary with that extension
// location primary and related complex-content/extension/base/particle (and
// anyAttribute, when present) locations. Direct model-group-reference bodies
// with AttributeUse facts hit the AttributeUse consumer gate first: the first
// use's Loc is primary, with definition and AttributeUse locations related.
// When no earlier AttributeUse gate applies, direct and extension model-group
// references are classified by group RefLoc, with group/component/reference/
// target related locations.
func GenerateGo(schema Schema, packageName string) ([]byte, error) {
	directParticlePlan, err := planCodegenDirectParticles(schema, packageName)
	if err != nil {
		return nil, err
	}
	return emitCodegenSourceWithDirectParticles(schema, directParticlePlan)
}
