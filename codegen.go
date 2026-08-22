package goxsd9

// GenerateGo generates deterministic, formatted Go source for the supported
// scalar components in schema using packageName.
func GenerateGo(schema Schema, packageName string) ([]byte, error) {
	names, err := newCodegenNaming(codegenNamingInput{
		packageName: packageName,
		schema:      schema,
		importAliases: []codegenImportAliasRequest{{
			identity: codegenRuntimeImportPath,
			alias:    "runtime",
		}},
	})
	if err != nil {
		return nil, err
	}
	return emitCodegenSource(schema, names)
}
