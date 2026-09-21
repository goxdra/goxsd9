package goxsd9

// GenerateGo generates deterministic, formatted Go source for the supported
// scalar components and direct scalar choices or sequences in schema using
// packageName. Non-model-group-reference direct-choice, direct-sequence, and
// model-less complex-content extensions are rejected as located
// FailureUnsupported/ErrUnsupported diagnostics at the extension boundary with
// the extension location as primary and related complex-content/extension/base/
// particle (and anyAttribute, when present) locations; they produce no output.
// Model-group-reference extension particles are classified first as group
// references: the group RefLoc is primary, with group/component/reference/target
// related locations.
func GenerateGo(schema Schema, packageName string) ([]byte, error) {
	directParticlePlan, err := planCodegenDirectParticles(schema, packageName)
	if err != nil {
		return nil, err
	}
	return emitCodegenSourceWithDirectParticles(schema, directParticlePlan)
}
