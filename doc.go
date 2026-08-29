// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating scalar XML
// instances.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs, builds
// supported schema-level components and simple-type restrictions, including
// global xs:boolean declarations and named restrictions with a boolean base,
// and exposes deterministic queries and walks. SimpleTypeDefinition.IsBoolean
// reports that immutable kind fact. ParseSchema uses graph-wide Compatibility;
// ParseSchemaWithPolicy applies one validated policy to the complete graph.
// The unqualified schema/@version is an inert optional xs:token label: absent,
// empty, arbitrary, "1.0", and "1.1" values never select or mismatch a policy.
// Chameleon includes, redefine/override/defaultOpenContent, assertions, and
// Boolean facets and datatype facets outside the supported integer/decimal and
// optional precisionDecimal boundaries return explicit unsupported diagnostics.
// precisionDecimal is available only when explicitly named under Compatibility
// or Strict11; Strict10 reports a located policy diagnostic. Paths and URLs are never opened by this package. Parsing closes
// the root and every resolved source, but drains and decodes only unseen
// identities; repeated and cyclic identities are closed without decoding.
//
// The schema model also exposes one direct ordered sequence of local built-in
// xs:boolean, named boolean-restriction, integer, and decimal scalar elements
// for a named global complex type, including exact immutable occurrence ranges.
// Direct choices likewise model local built-in xs:boolean and named
// boolean-restriction elements. Anonymous types, references, nested and broader
// particles remain unsupported.
// Sequence particles are query-only until validation repetition is implemented.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as built-in or named xs:integer/xs:decimal/xs:precisionDecimal, or
// as a named complex type whose one direct choice contains local built-in or
// named integer/decimal/precisionDecimal elements. Scalar elements contain
// only character data; boolean instance validation, attributes, broader
// particles, and other semantics remain explicit unsupported behavior.
// GenerateGo produces deterministic Go source for the supported scalar schema
// components and direct scalar choices; boolean generation remains unsupported.
package goxsd9
