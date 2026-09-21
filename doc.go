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
// Global anonymous inline precisionDecimal is available under Compatibility or
// Strict11; Strict10 reports a located policy diagnostic. Paths and URLs are never opened by this package. Parsing closes
// the root and every resolved source, but drains and decodes only unseen
// identities; repeated and cyclic identities are closed without decoding.
//
// The schema model exposes one direct ordered sequence and direct choices of local
// built-in xs:boolean, named boolean-restriction, integer, decimal, and explicitly
// typed built-in or supported named xs:token/xs:NMTOKEN particles for named global
// complex types. It also exposes local inline anonymous atomic Boolean, integer, and
// decimal restrictions in direct choices/sequences and bounded attribute-free
// extensions under Compatibility, Strict10, and Strict11. Their immutable
// TypeReference/AnonymousType views retain SimpleTypeID ownership through
// AnonymousID/NodeID, base QName context, effective facets, source locations, and
// exact particle occurrences; anonymous definitions have zero ComponentID and are
// not global components or Walk entries. Effective 0/0 maps to absence before
// type-specific gating. Local anonymous integer-derived restrictions are admitted
// by effective atomic kind: effective integer or negativeInteger remains accepted
// through named, forward, imported, included, and chameleon chains; effective
// long, unsignedLong, nonNegativeInteger, and nonPositiveInteger are valid but
// unsupported at this local boundary: ParseSchema returns a located
// FailureUnsupported/ErrUnsupported diagnostic at the relevant type or facet
// location and no schema. The written base QName, use-site location, and resolved
// named ownership remain separate facts. The supported anonymous Boolean/integer/
// decimal restriction facet subset remains queryable; non-string anonymous
// enumeration remains explicit unsupported at its facet location with no schema.
// Token/NMTOKEN current-state matrix: explicitly typed built-in or supported
// named local particles in direct choices, sequences, and bounded attribute-free
// extensions are modeled and queryable; default-occurrence direct choices made
// entirely of local token or NMTOKEN alternatives validate. Local token/NMTOKEN
// sequences, anonymous token/NMTOKEN restrictions, and generation remain
// unsupported. Direct element references remain queryable, but token/NMTOKEN
// reference consumers remain unsupported. Global inline string/token/NMTOKEN
// elements are the separate generation-eligible exception.
// Direct xs:any terms with effective ##any/strict, ##any/lax (including an
// omitted namespace with processContents="lax"), ##any/skip with explicit
// processContents="skip", ##other/lax, and ##other/strict forms, plus positive
// namespace constraints (##local, ##targetNamespace, and URI lists) with strict,
// lax, or explicit skip processing, are exposed as immutable WildcardParticle values. Positive constraints retain
// immutable effective namespace values in sorted order, their lexical form, and
// source location; explicit constraint-attribute locations are retained in
// lexical order with element and reference terms. Other wildcard constraints
// and broader wildcard placements remain unsupported. Nonzero wildcard-bearing
// particles remain unsupported to validation and generation consumers.
// Named global direct sequence and choice complex types also expose immutable
// anyAttribute facts for effective ##any and ##other constraints, plus positive
// ##local, ##targetNamespace, and URI namespace enumerations with strict
// processing (omitted processContents defaults to strict). NamespaceConstraint
// resolves markers against the owner's effective schema namespace and returns
// copied, sorted, deduplicated effective values. Normalized lexical forms and
// exact anyAttribute, namespace, and processContents source locations are
// retained, with omitted locations zero. Attribute-wildcard validation and
// generation remain unsupported.
// Effective 0/0 sequence, choice, child, and wildcard ranges map
// to absence. Non-0/0 integer/decimal choice and
// alternative ranges are queryable, but direct-choice repetition is not
// implemented. Direct choices made entirely of local Boolean elements use
// built-in xs:boolean or named Boolean restrictions; mixed Boolean/numeric
// choices remain unsupported.
// Explicitly typed XSD 1.1 precisionDecimal is supported in direct choices only when the choice
// and each mapped precisionDecimal alternative use default occurrences;
// non-default precisionDecimal choice or alternative ranges and non-0/0
// direct-sequence precisionDecimal ranges that map to particles are
// schema-unsupported; effective 0/0 maps to absence before type-specific
// gating, and non-precision alternatives may retain non-default query-only
// ranges. The supported local anonymous model is limited to atomic
// Boolean/integer/decimal restrictions in the direct choice/sequence and bounded
// attribute-free extension shapes above. The local anonymous boundary is restriction
// particles, not all inline forms: local inline complex/list/union and
// local anonymous string/token/NMTOKEN/precisionDecimal restrictions, local
// value/default/fixed/attribute constraints, nested, anonymous-reference, and
// broader forms remain unsupported. Local anonymous inline atomic types are
// query-only; direct validation and generation reject them. Global inline
// precisionDecimal is Compatibility/Strict11 policy-gated schema/query support
// only (Strict10 rejects), not a validation, generation, or direct-reference
// target.
// Element-reference matrix: element-reference particles in local content and
// named groups are queryable immutable facts. ValidateInstance and GenerateGo
// consume only supported default-occurrence direct-choice references to global
// Boolean, integer, or decimal targets; only those targets are consumer-eligible.
// Sequence references, anonymous targets, repetition, nested or recursive
// references, and broader element-reference shapes remain unsupported.
// Model-group reference particles are limited to the supported top-level direct
// `ModelGroupReferenceParticle` form. Named global model groups expose direct
// element-reference choices or sequences without expansion.
// Top-level direct model-group references on named complex types and bounded
// attribute-free extensions over named empty-content bases are queryable as exact
// immutable facts without expanding target members. Direct model-group references
// retain `TargetID`; nested, local, recursive, and broader group-reference shapes
// remain unsupported.
// Default-bounded numeric or all-Boolean sequences are emitted as ordered Go
// struct fields; repeated-field generation and direct-choice repetition remain
// unsupported.
// Bounded attribute-free complexContent/extension over named empty-content
// complex bases, including the supported named `complexContent/restriction` over
// `xs:anyType` representation, retains extension/base identities and locations
// and only inherited bounded, representable wildcard facts (`##other`/`lax`).
// An extension with a present direct choice or sequence particle retains its exact
// occurrence. A model-less extension retains its named base identity and locations
// with a nil optional particle, no occurrence, and no synthetic content. For
// Ordinary direct-choice/direct-sequence target checks use element/particle
// locations and may include the anonymous type location in related facts.
// Non-model-group-reference complex-content/model-less extension checks run first
// at the extension boundary: code generation uses the extension location as
// primary with related complex-content/extension/base/particle facts (and
// anyAttribute when present); validation retains declaration/definition owner
// locations, uses the extension boundary for choices and the instance root for
// sequences, and never adds an anonymous type location. Direct and extension
// model-group-reference checks use the group RefLoc as validation and generation
// primary; validation relates the group particle, while generation relates
// group/component/reference/target locations. These gates return located
// FailureUnsupported/ErrUnsupported diagnostics; GenerateGo returns no output.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as built-in or named xs:boolean/xs:token/xs:NMTOKEN/xs:integer/xs:decimal/
// xs:precisionDecimal, or as a named global complex type with one direct
// Boolean-only sequence of local built-in xs:boolean or facet-free named Boolean
// restriction elements, one direct integer/decimal sequence, or one direct choice
// whose scalar alternatives use default occurrences and contain local built-in or named
// Boolean, token, NMTOKEN, integer, decimal, or precisionDecimal elements, or default-occurrence references
// to global Boolean, integer, or decimal elements. Local scalar consumers accept
// built-in or named references only: direct choice/sequence checks reject modeled
// anonymous local inline atomic references with located
// FailureUnsupported/ErrUnsupported diagnostics that may include the anonymous
// type location in related facts. Non-model-group-reference extension checks run
// first at the extension boundary, retain complex-content/extension/base/particle
// (and anyAttribute when present) related locations, and do not include the
// anonymous type location; validation also retains declaration/definition owner
// locations and keeps the instance-root primary for sequences, while GenerateGo
// rejects them with the same classification and no output. Direct and extension
// model-group-reference checks use the group reference RefLoc as validation and
// generation primary; validation retains the group particle location in related
// facts, and generation retains group/component/reference/target related locations.
// Direct local sequences match expanded
// names in lexical declaration order and honor exact finite, unbounded, and
// above-`uint64` outer and child occurrence ranges under Compatibility, Strict10,
// and Strict11. Mixed Boolean/numeric sequences, direct-choice repetition, and excluded particle/target shapes
// remain explicit unsupported behavior. Reference alternatives exclude precisionDecimal
// targets.
// Mixed local Boolean/numeric, token/non-token, or NMTOKEN/non-NMTOKEN choices are unsupported. Nonzero
// wildcard-bearing particles are explicit unsupported
// behavior in both consumers; absent 0/0 wildcard terms do not enter those
// gates. Scalar elements contain only character data. Global and supported
// local-choice token values
// collapse XML whitespace before effective enumeration comparison without
// changing retained schema facts. Global NMTOKEN values also collapse XML
// whitespace and enforce the repository XML NameChar policy. Global string
// values, local string particles, token/NMTOKEN sequence particles, lists/unions,
// attributes, broader particles, and other semantics remain explicit unsupported
// behavior.
// GenerateGo produces deterministic Go source for built-in, named, or inherited
// global Boolean/integer/decimal scalar components, explicitly supported global
// inline string/token/NMTOKEN elements, default-occurrence all-Boolean or numeric
// direct choices, and default-bounded numeric or all-Boolean local sequences.
// Global inline numeric, Boolean, and precisionDecimal forms remain query-only or
// consumer-rejected; boolean facets, mixed Boolean/numeric sequences, mixed direct
// choices, and local string/token/NMTOKEN particles remain unsupported.
package goxsd9
