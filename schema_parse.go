package goxsd9

// ParseSchema discovers root and referenced schema documents and returns their
// immutable component graph. The current parser supports the implemented
// mixed XSD 1.0 and XSD 1.1 schema subset; a missing schema version defaults
// to XSD 1.1.
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
