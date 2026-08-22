package goxsd9

// ParseSchema discovers root and referenced schema documents and returns their
// immutable component graph. It uses the Compatibility policy for the entire
// graph. The unqualified schema/@version attribute is an inert optional
// xs:token label; absent, empty, arbitrary, "1.0", and "1.1" values do not
// select a policy or produce a label-only diagnostic.
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
	return discoverSchemaWithPolicy(root, resolver, Compatibility)
}

// ParseSchemaWithPolicy validates an explicit graph-wide language policy
// before discovering the root or calling the resolver. Valid policies
// select one behavior for the entire root/include/import graph, including
// conditional inclusion, grammar validation, component construction, and
// datatype facets. Schema version labels never override the selected policy.
// Invalid policy configuration returns no Schema and a diagnostic at Loc{}
// with the configuration cause preserved. A root with a reader is closed
// exactly once on that preflight failure.
func ParseSchemaWithPolicy(root ResolvedSource, resolver Resolver, policy LanguagePolicy) (Schema, error) {
	cause := validateLanguagePolicy(policy)
	if cause == nil {
		return discoverSchemaWithPolicy(root, resolver, policy)
	}

	return Schema{}, closeDiscoverySourceOnError(
		root,
		Loc{},
		invalidLanguagePolicyDiagnostic(policy, cause),
	)
}
