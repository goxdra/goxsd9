// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating scalar XML
// instances.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs and builds
// supported schema-level components, including simple-type atomic restrictions,
// lists, and unions. Anonymous simple types and resolved built-in, named, and
// anonymous simple-type references are modeled, along with global xs:boolean
// and atomic xs:string declarations and their named or anonymous restrictions.
// Queries and walks are deterministic. SimpleTypeDefinition.IsBoolean,
// StringEnumerationFacets, and StringWhiteSpaceFacet report immutable kind
// and implemented scalar facts. ParseSchema uses graph-wide Compatibility;
// ParseSchemaWithPolicy applies one validated policy to the complete graph.
// The unqualified schema/@version is an inert optional xs:token label: absent,
// empty, arbitrary, "1.0", and "1.1" values never select or mismatch a policy.
// Chameleon includes adopt the including target namespace and repair
// unqualified direct element-reference QNames in supported particles.
// Redefine/override/defaultOpenContent, assertions, and Boolean facets and
// datatype facets outside the supported string enumeration/whiteSpace, integer/decimal,
// and optional precisionDecimal boundaries return explicit unsupported diagnostics.
// precisionDecimal is available only when explicitly named under Compatibility
// or Strict11; Strict10 reports a located policy diagnostic. Paths and URLs are never opened by this package. Parsing closes
// the root and every resolved source, but drains and decodes only unseen
// identities; repeated and cyclic identities are closed without decoding.
//
// The schema model also exposes one direct ordered sequence of local built-in
// xs:boolean, named boolean-restriction, integer, and decimal scalar elements
// for a named global complex type, and direct choices of those scalar elements,
// including exact immutable occurrence ranges. Effective 0/0 sequence, choice,
// and child ranges map to absence. Non-0/0 integer/decimal choice and
// alternative ranges are queryable, but repetition is not implemented.
// XSD 1.1 precisionDecimal is supported in direct choices only when the choice
// and each mapped precisionDecimal alternative use default occurrences;
// non-default precisionDecimal choice or alternative ranges and non-0/0
// direct-sequence precisionDecimal ranges that map to particles are
// schema-unsupported. Anonymous, nested, and broader particles remain
// unsupported; local string particles remain unsupported. Anonymous simple-type
// models and resolved built-in, named, and anonymous simple-type references are
// modeled. Direct element references are queryable immutable particles;
// validation supports default-occurrence direct choices made entirely of
// references to global integer/decimal scalar elements, while other reference
// particles and code generation remain explicitly unsupported.
// Default-bounded direct integer and decimal sequences are emitted as ordered Go
// struct fields; sequence validation and non-default occurrences remain unsupported.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as built-in or named xs:boolean/xs:integer/xs:decimal/
// xs:precisionDecimal, or as a named complex type whose one direct choice and
// its scalar alternatives use default occurrences and contain local built-in or
// named integer/decimal/precisionDecimal elements, or default-occurrence
// references to global integer/decimal elements. Reference alternatives exclude
// boolean and precisionDecimal targets. Scalar elements contain only character
// data; string globals, local boolean/string particles, attributes, broader
// particles, and other semantics remain explicit unsupported behavior.
// GenerateGo produces deterministic Go source for global boolean/integer/decimal
// scalar components, direct scalar choices, and default-bounded direct integer/
// decimal sequences; string, boolean facets, and local boolean/string particles
// remain unsupported.
package goxsd9
