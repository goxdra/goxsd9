package goxsd9

// ParseSchema discovers root and referenced schema documents and returns their
// immutable component graph. The current parser supports the implemented
// mixed XSD 1.0 and XSD 1.1 schema subset. The current two-argument parser
// retains legacy per-document handling: an absent or empty schema/@version
// defaults to XSD 1.1, "1.0" selects the legacy XSD 1.0 path, "1.1" selects
// the legacy XSD 1.1 path, and arbitrary labels are rejected as unsupported.
// Normatively, schema/@version is an inert optional xs:token label. Use
// ParseSchemaWithPolicy to validate an explicit graph-wide policy; policy
// propagation and profile-specific behavior remain future work.
//
// root must be created with NewResolvedSource. The caller supplies that root;
// resolver supplies referenced sources. Parsing drains and closes root and
// every source returned by resolver, including sources for repeated or cyclic
// identities. Resolver calls are sequential and receive schema locations and
// contexts unchanged; the package never opens paths or URLs.
// A nil resolver is accepted when no reference needs resolution and produces
// a resolution diagnostic when a reference does. On every error, the returned
// Schema is zero.
func ParseSchema(root ResolvedSource, resolver Resolver) (Schema, error) {
	return discoverSchema(root, resolver)
}

// ParseSchemaWithPolicy validates an explicit graph-wide language policy
// before discovering the root or calling the resolver. Valid policies
// currently enter the same parser pipeline as ParseSchema; policy propagation
// and profile-specific behavior are not implemented in this boundary.
// Invalid policy configuration returns no Schema and a diagnostic at Loc{}
// with the configuration cause preserved. A root with a reader is closed
// exactly once on that preflight failure.
func ParseSchemaWithPolicy(root ResolvedSource, resolver Resolver, policy LanguagePolicy) (Schema, error) {
	cause := validateLanguagePolicy(policy)
	if cause == nil {
		return discoverSchema(root, resolver)
	}

	return Schema{}, closeDiscoverySourceOnError(
		root,
		Loc{},
		invalidLanguagePolicyDiagnostic(policy, cause),
	)
}
