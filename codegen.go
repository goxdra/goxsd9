package goxsd9

// GenerateGo generates deterministic, formatted Go source for the supported
// scalar components and direct scalar choices or sequences in schema using
// packageName.
func GenerateGo(schema Schema, packageName string) ([]byte, error) {
	directParticlePlan, err := planCodegenDirectParticles(schema, packageName)
	if err != nil {
		return nil, err
	}
	return emitCodegenSourceWithDirectParticles(schema, directParticlePlan)
}
