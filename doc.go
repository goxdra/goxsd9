// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating scalar XML
// instances.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs, builds
// supported schema-level components and simple-type restrictions, and exposes
// deterministic queries and walks. ParseSchema uses graph-wide Compatibility;
// ParseSchemaWithPolicy applies one validated policy to the complete graph.
// The unqualified schema/@version is an inert optional xs:token label: absent,
// empty, arbitrary, "1.0", and "1.1" values never select or mismatch a policy.
// Chameleon includes, redefine/override/defaultOpenContent,
// assertions, and unsupported datatype facets return explicit unsupported
// diagnostics. Paths and URLs are never opened by this package. Parsing closes
// the root and every resolved source, but drains and decodes only unseen
// identities; repeated and cyclic identities are closed without decoding.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as built-in or named xs:integer or xs:decimal. The scalar element
// must contain only character data; attributes and child elements are
// explicit unsupported behavior. GenerateGo produces deterministic Go source
// for the supported scalar schema components and direct scalar choices.
package goxsd9
