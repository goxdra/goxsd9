package goxsd9

// GenerateGo generates deterministic, formatted Go source for the supported
// scalar components and direct scalar choices in schema using packageName.
func GenerateGo(schema Schema, packageName string) ([]byte, error) {
	choicePlan, err := planCodegenDirectChoices(schema, packageName)
	if err != nil {
		return nil, err
	}
	return emitCodegenSourceWithDirectChoices(schema, choicePlan)
}
