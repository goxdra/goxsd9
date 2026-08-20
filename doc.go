// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs, builds
// supported schema-level components and simple-type restrictions, and exposes
// deterministic queries and walks. The current two-argument parser retains
// legacy per-document handling: an absent or empty schema/@version defaults to
// XSD 1.1, "1.0" selects the legacy XSD 1.0 path, "1.1" selects the legacy
// XSD 1.1 path, and arbitrary labels are rejected as unsupported. Normatively,
// schema/@version is an inert optional xs:token label; the accepted graph-wide
// Compatibility (default), Strict10, and Strict11 policy is future work and is
// absent from the current parser. Chameleon includes, redefine/override/defaultOpenContent,
// assertions, and unsupported datatype facets return explicit unsupported
// diagnostics. Paths and URLs are never opened by this package. Parsing closes
// the root and every resolved source, but drains and decodes only unseen
// identities; repeated and cyclic identities are closed without decoding.
//
// XML instance validation and Go code generation remain under construction.
package goxsd9
