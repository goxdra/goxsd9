// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating scalar XML
// instances.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs and builds
// supported schema-level components, including simple-type atomic restrictions,
// lists, and unions. Anonymous simple types and resolved built-in, named, and
// anonymous simple-type references are modeled, along with global xs:boolean
// and atomic xs:string/xs:token/xs:NMTOKEN declarations and their named or anonymous
// restrictions.
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
// including exact immutable occurrence ranges. Direct xs:any terms with omitted
// or canonical explicit-default namespace="##any" and/or processContents="strict"
// spellings are also exposed as immutable WildcardParticle values with effective
// namespace ##any and strict processing; explicit constraint-attribute locations
// are retained, in lexical order with element and reference terms. Non-default
// wildcard constraints and broader wildcard placements remain unsupported.
// Effective 0/0 sequence, choice, child, and wildcard ranges map
// to absence. Non-0/0 integer/decimal choice and
// alternative ranges are queryable, but direct-choice repetition is not
// implemented. Direct choices made entirely of local Boolean elements use
// built-in xs:boolean or named Boolean restrictions; mixed Boolean/numeric
// choices remain unsupported.
// XSD 1.1 precisionDecimal is supported in direct choices only when the choice
// and each mapped precisionDecimal alternative use default occurrences;
// non-default precisionDecimal choice or alternative ranges and non-0/0
// direct-sequence precisionDecimal ranges that map to particles are
// schema-unsupported. Anonymous, nested, and broader particles remain
// unsupported; local string/token/NMTOKEN particles remain unsupported. Anonymous simple-type
// models and resolved built-in, named, and anonymous simple-type references are
// modeled. Direct element references are queryable immutable particles;
// validation and code generation support default-occurrence direct choices
// made entirely of references to global integer/decimal scalar elements, while
// other reference particles beyond the supported top-level direct
// `ModelGroupReferenceParticle` form, repetition, and broader shapes remain
// explicitly unsupported.
// Named global model groups expose direct choices or sequences of global
// element-reference particles as immutable query facts with exact ranges;
// validation and code generation do not expand them.
// Top-level direct model-group references on named complex types and bounded
// attribute-free extensions over named empty-content bases are queryable as exact
// immutable particles retaining `TargetID` without expanding target members;
// `ValidateInstance` and `GenerateGo` reject them. Nested, local, recursive, and
// broader group-reference shapes remain unsupported.
// Default-bounded direct integer and decimal sequences are emitted as ordered Go
// struct fields; repeated-field generation and direct-choice repetition remain
// unsupported.
// Bounded attribute-free complexContent/extension over named empty-content
// complex bases is modeled with extension/base identities and locations,
// inherited bounded wildcard facts, and exact direct choice/sequence occurrences;
// validation and code generation reject extension types as unsupported.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as built-in or named xs:boolean/xs:token/xs:NMTOKEN/xs:integer/xs:decimal/
// xs:precisionDecimal, or as a named global complex type with one direct local
// one direct Boolean-only or integer/decimal sequence or one direct choice whose scalar alternatives use
// default occurrences and contain local built-in or named
// Boolean, integer, decimal, or precisionDecimal elements, or default-occurrence references
// to global integer/decimal elements. Direct local sequences match expanded
// names in lexical declaration order and honor exact finite, unbounded, and
// above-`uint64` outer and child occurrence ranges under Compatibility, Strict10,
// and Strict11. Mixed Boolean/numeric sequences, direct-choice repetition, and excluded particle/target shapes
// remain explicit unsupported behavior. Reference alternatives exclude boolean
// and precisionDecimal targets. Mixed local Boolean/numeric choices are
// unsupported. Nonzero wildcard-bearing particles are explicit unsupported
// behavior in both consumers; absent 0/0 wildcard terms do not enter those
// gates. Scalar elements contain only character data. Global token values
// collapse XML whitespace before effective enumeration comparison without
// changing retained schema facts. Global NMTOKEN values also collapse XML
// whitespace and enforce the repository XML NameChar policy. Global string
// values, local string/token/NMTOKEN particles, lists/unions,
// attributes, broader particles, and other semantics remain explicit unsupported
// behavior.
// GenerateGo produces deterministic Go source for global boolean/integer/decimal/
// atomic string/token/NMTOKEN scalar components, default-occurrence all-Boolean or
// numeric direct choices, and default-bounded direct integer/decimal sequences;
// boolean facets, local Boolean direct sequences, mixed direct choices, and local
// string/token/NMTOKEN particles remain unsupported.
package goxsd9
