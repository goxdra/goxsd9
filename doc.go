// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating scalar XML
// instances.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs and builds
// supported schema-level components, including simple-type atomic restrictions,
// lists, and unions. Anonymous simple types and resolved built-in, named, and
// anonymous simple-type references are modeled, along with global xs:boolean,
// xs:nonNegativeInteger, xs:positiveInteger, and atomic
// xs:string/xs:normalizedString/xs:token/xs:NMTOKEN declarations
// and their named or anonymous restrictions.
// Queries and walks are deterministic. SimpleTypeDefinition.IsBoolean,
// StringEnumerationFacets, and StringWhiteSpaceFacet report immutable kind
// and implemented scalar facts. ParseSchema uses graph-wide Compatibility;
// ParseSchemaWithPolicy applies one validated policy to the complete graph.
// xs:normalizedString has distinct built-in identity and replace whiteSpace.
// Restrictions, list items, and union members may reference it. Direct,
// named-typed, and inline atomic-restriction global elements are queryable;
// list/union-typed global elements reject. Mapped nonzero local typed particles
// and global attributes reject at schema admission; applicable 0/0 local forms
// omit after reference, facet, and policy gates. Element-ref particles to
// admitted global elements query; ValidateInstance and GenerateGo reject them.
// xs:QName references are distinct context-sensitive atomics in facet-free
// restrictions, lists, unions, and direct/named/inline global elements.
// Mapped nonzero QName-bearing typed local particles reject, including mixed
// precisionDecimal unions; applicable 0/0 forms omit after
// reference, facet, and policy gates. All QName facets reject at schema admission.
// Individual default/fixed values on QName-typed global declarations are
// unsupported; simultaneous values or either on an element ref are invalid at
// the conflicting or forbidden attribute.
// ValidateInstance rejects admitted global elements and refs with located
// diagnostics; GenerateGo rejects them with located diagnostics and nil output.
// A successful ParseSchema returns an immutable Schema; Documents, Components,
// Lookup, Find, FindKind, and Walk expose deterministic query views, while
// AttributeDeclaration exposes resolved type and value-constraint facts.
// Supported global elements expose ordered immutable unique, key, and keyref
// facts with lexical XPath and namespace context. Keyrefs resolve to nested
// identity IDs before publication. Identity-constrained elements are query-only;
// ValidateInstance and GenerateGo return located unsupported diagnostics;
// XPath evaluation, local constraints, and XSD 1.1 ref reuse remain unsupported.
// ValidateInstance and GenerateGo are separate consumers of their supported
// schema projections and do not expand the query model.
// The unqualified schema/@version is an inert optional xs:token label: absent,
// empty, arbitrary, "1.0", and "1.1" values never select or mismatch a policy.
// Chameleon includes adopt the including target namespace and repair
// unqualified direct element-reference QNames in supported particles. In XSD
// 1.1, a local attribute targetNamespace selects its explicit namespace only
// when a containing targetNamespace exists and matches; missing or mismatched
// values are invalid, while Strict10 reports an edition mismatch.
// Redefine/override/defaultOpenContent, assertions, and Boolean facets and
// datatype facets outside the supported string enumeration/whiteSpace, integer/decimal,
// and optional precisionDecimal boundaries return explicit unsupported diagnostics.
// Global built-in, named, and inline precisionDecimal element/type facts are
// available only under Compatibility or Strict11; Strict10 rejects each before
// validation with a located FeatureDatatypeFacets/FailureUnsupported/ErrUnsupported
// policy diagnostic at the typed reference or type location. Global attributes
// whose resolved type is built-in xs:precisionDecimal or a named type with
// effective precisionDecimal facets retain zero or one optional default/fixed
// AttributeValueConstraint for query only under Compatibility or Strict11;
// type-only declarations return no value constraint. AttributeDeclaration
// ValueConstraint() exposes its kind, collapsed lexical spelling, source Loc,
// and exact defensive StrictPrecisionDecimal through PrecisionDecimalValue
// only when a default or fixed value is present. Strict10 rejects at the
// resolved type Loc before conversion; inline global attribute default/fixed and
// attribute validation/GenerateGo remain unsupported. Supported local anonymous
// atomic AttributeUse facts are separate. Under admitting policies,
// built-in/named roots validate, while inline anonymous targets remain excluded
// from validation and generation.
// Paths and URLs are never opened by this package. Parsing closes
// the root and every resolved source, but drains and decodes only unseen
// identities; repeated and cyclic identities are closed without decoding.
//
// The schema model exposes one direct ordered sequence and direct choices of local
// built-in xs:boolean, named boolean-restriction, integer, decimal, built-in/named/inline
// xs:negativeInteger, explicitly typed built-in xs:long, supported named
// effective-long, built-in or supported named xs:int/xs:short/xs:byte/
// xs:unsignedLong/xs:nonNegativeInteger, and explicitly typed built-in or
// supported named xs:token/xs:NMTOKEN particles for named global and global
// inline complex types, and atomic xs:string particles for named global
// complex types, global element inline complex types, and supported bounded
// attribute-free extensions. Inline complexes retain anonymous IDs and query facts
// outside the global walk; their consumers reject. The model also admits
// built-in/named/inline xs:negativeInteger and built-in or supported
// named-effective xs:long/xs:int/xs:short/xs:byte/xs:unsignedLong/
// xs:nonNegativeInteger particles in
// supported attribute-free extension choices and sequences under every policy; they
// remain query-only and consumer-rejected. Direct and supported extension
// choices/sequences retain exact finite, unbounded, and above-uint64 occurrences;
// bounded describes the supported derivation/base shape, not occurrence limits.
// Supported local choice/sequence elements may retain a TypeID for a named
// scalar simpleContent complex type, with exact occurrences and no particle on
// the target type.
// That target keeps its ordered attribute uses; string/Boolean/integer/decimal
// bases work under every policy, while precisionDecimal requires Compatibility
// or Strict11. Unsupported bases or attribute forms fail schema construction;
// validation and generation reject these local particle targets. The model
// exposes local inline anonymous atomic
// Boolean, integer, decimal, and
// negativeInteger restrictions
// in direct choices/sequences and bounded attribute-free extensions under Compatibility,
// Strict10, and Strict11. Their immutable
// TypeReference/AnonymousType views retain SimpleTypeID ownership through
// AnonymousID/NodeID, base QName context, effective facets, source locations, and
// exact particle occurrences; anonymous definitions have zero ComponentID and are
// not global components or Walk entries. Applicable syntax, occurrence,
// element-reference, and policy gates run before local public-particle mapping;
// graph-wide declaration/facet failures still surface, and located gate errors
// preserve their causes/Locs and return no Schema. Ordinary local named/inline
// type mapping is not universal for non-reference 0/0 terms. For every affected
// local owner or term, syntax and exact occurrences precede inline semantic
// resolution. At 0/0, unsupported inline syntax waits for its base and supported
// facets to resolve; invalid, unresolved, cyclic, wrong-kind, value-constraint,
// and policy failures retain their diagnostics and prevent a Schema. Resolved
// unsupported forms may omit.
// Direct sequences resolve children before owner omission; direct choices resolve
// child refs without duplicate checks before child omission; named groups
// resolve/check duplicate refs before owner/child omission; child refs resolve
// before child omission. Mapped non-0/0 unsupported scalar forms return located
// schema-syntax diagnostics. Ordinary 0/0 forms are admitted only after the
// applicable gates, then the local public particle is absent; this is not a
// universal named-type/facet or validation bypass. Strict10 policy admission precedes omission for explicitly typed local
// built-in or named-effective and inline anonymous `precisionDecimal` forms,
// including zero-occurrence cases. Ordinary `int`/`short`/`unsignedLong` and other long-family 0/0
// forms use the admission-then-absence rule under every policy.
// Local declared, named, inline, and anonymous restrictions in the
// integer/negativeInteger branch are admitted at the mapped non-0/0 boundary
// when their effective atomic kind is integer or negativeInteger through named,
// forward, imported, included, and chameleon chains. Explicit built-in and
// supported named nonNegativeInteger particles are also admitted. Direct built-in,
// named-effective, and anonymous-inline negativeInteger forms are admitted as query
// facts, but ValidateInstance and GenerateGo reject those consumers. Built-in
// and named effective-long particles remain query-only and consumer-rejected.
// Local nonPositiveInteger/positiveInteger and inline/anonymous
// long/int/short/byte/unsignedLong/nonNegativeInteger are excluded when mapped non-0/0: ParseSchema returns a located
// FeatureSchemaSyntax/FailureUnsupported/UnsupportedSchemaSyntaxCode/ErrUnsupported
// diagnostic at the type, facet, or element Loc and no Schema. Nested-particle
// exclusions use the nested-particle Loc. Ordinary 0/0 is admitted after
// applicable gates and remains absent.
// Inline/anonymous long/int/short/byte/unsignedLong/nonNegativeInteger are mapped schema exclusions at their
// type/simpleType Loc; built-in and named-effective long/int/short/byte/unsignedLong/nonNegativeInteger are admitted
// query-only forms. The written base QName/base Loc, use-site/type/facet Locs,
// named ID versus built-in zero identity, ownership, and resolved facts remain
// separate. Built-in int retains intrinsic inclusive bounds
// [-2147483648,2147483647] without synthetic bound locations or component IDs;
// built-in short retains [-32768,32767] with no source bound Loc or component ID;
// built-in byte retains [-128,127] with no source bound Loc or component ID;
// built-in unsignedLong retains intrinsic inclusive bounds
// [0,18446744073709551615]; built-in nonNegativeInteger retains intrinsic
// minInclusive=0 without a source Loc; built-in positiveInteger retains
// minInclusive=1 without a source Loc or component ID; built-in negativeInteger retains
// maxInclusive=-1 at its type Loc with no component ID; named-effective particles retain exact narrowed,
// inclusive/exclusive bounds, integer enumeration/digit facets, source locations,
// identities, graph provenance, and exact occurrences; validation and GenerateGo return
// FailureUnsupported diagnostics.
// The supported anonymous Boolean/integer/
// decimal restriction facet subset remains queryable. Anonymous non-string
// enumeration other than precisionDecimal is unsupported at its facet location.
// PrecisionDecimal enumeration passes that facet gate under Compatibility/Strict11:
// global inline restrictions remain queryable, while mapped nonzero local
// anonymous restrictions fail at the local type location with no schema.
// Token/NMTOKEN current-state matrix: explicitly typed built-in or supported
// named local particles in direct choices, sequences, and bounded attribute-free
// extensions are modeled and queryable; only non-extension default-occurrence
// homogeneous direct choices made entirely of local token or NMTOKEN
// alternatives validate. Homogeneous direct sequences made entirely of local
// built-in or supported named token or NMTOKEN particles also validate with exact
// occurrences. Anonymous token/NMTOKEN restrictions remain unsupported for
// consumers; local token/NMTOKEN sequences and NMTOKEN choices remain
// GenerateGo-unsupported. Default-occurrence all-token direct choices generate.
// Direct element references remain
// queryable, but token/NMTOKEN reference consumers remain unsupported.
// Global inline string/token/NMTOKEN elements are the separate generation-eligible
// exception.
// Direct xs:any terms with effective ##any/strict, ##any/lax (including an
// omitted namespace with processContents="lax"), ##any/skip with explicit
// processContents="skip", and ##other/strict, ##other/lax, and ##other/skip forms,
// plus positive namespace constraints (##local, ##targetNamespace, and URI lists) with strict,
// lax, or explicit skip processing, are exposed as immutable WildcardParticle values.
// Compatibility and Strict11 also expose direct notNamespace lists with omitted or explicit
// strict, lax, or skip processing; Strict10 reports a located edition mismatch. Positive sets contain
// included namespaces; negative sets contain excluded namespaces. Both retain sorted,
// unique owner-relative values, normalized lexical forms, and exact attribute locations
// in lexical order with element and reference terms. notQName,
// wildcard algebra, and broader placements remain unsupported. Nonzero wildcard-bearing
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
// After those gates, effective 0/0 sequence, choice, child, and wildcard ranges
// map to absence. Non-0/0 integer/decimal choice and alternative ranges are
// queryable; direct-choice repetition validation remains unsupported. Direct
// choices made entirely of local Boolean elements use
// built-in xs:boolean or named Boolean restrictions; mixed Boolean/numeric
// choices remain unsupported.
// Local precisionDecimal forms are distinct. Under Compatibility/Strict11, a
// local element declared with built-in `type="xs:precisionDecimal"` or a named
// type whose effective facets are precisionDecimal is admitted in direct
// sequences with exact occurrences, default-occurrence direct choices, and
// bounded attribute-free extension choices. In choices, the owner and mapped
// precisionDecimal alternatives require default occurrences; non-precision alternatives
// may retain non-default query-only ranges. An inline anonymous
// `<xs:simpleType><xs:restriction base="xs:precisionDecimal">` restriction is
// schema-unsupported when mapped; mapped nonzero anonymous restrictions remain
// unsupported. Strict10 returns a located
// FeatureDatatypeFacets/FailureUnsupported/ErrUnsupported policy-mismatch
// diagnostic before 0/0 omission for either mapped form, including zero. Under
// Compatibility/Strict11, mapped non-default precisionDecimal choice/alternative
// ranges are schema-unsupported. Direct sequences remain query-only when their
// consumers cannot model them. Only non-extension default-occurrence typed direct
// choices are validation-eligible; precisionDecimal extension choices remain
// query-only/consumer-rejected, and all anonymous consumers are rejected by
// validation and generation.
// The supported local element anonymous model is limited to atomic
// Boolean/integer/decimal/negativeInteger restrictions in the direct choice/sequence
// and bounded attribute-free extension shapes above. Local anonymous
// Boolean/integer/decimal/negativeInteger restrictions remain queryable but direct
// validation and generation reject them;
// mapped local anonymous string/token/NMTOKEN/precisionDecimal restrictions remain
// schema-unsupported when nonzero. Global inline-element precisionDecimal remains a query
// target only under Compatibility/Strict11; Strict10 rejects it before validation,
// and every anonymous precisionDecimal target is excluded from validation and
// generation.
// Global inline complex types expose stable anonymous ComplexTypeID nodes,
// exact ordered sequence/reference particles, and attribute uses without
// entering the global component walk. Direct non-extension precisionDecimal
// list/union sequence links without QName and anonymous global precisionDecimal
// restrictions are query-only.
// Particle-plus-use bodies, direct model-group references, grouped extensions,
// attribute-only bodies, and scalar simpleContent extensions expose ordered defensive
// local, referenced, and anonymous-inline AttributeUse facts. Supported local
// anonymous atomic uses retain AnonymousID/NodeID. One named complexContent
// extension composes a direct opaque named-group reference, ordered local uses,
// and the supported named empty base; broader attribute-bearing extensions
// remain unsupported. Local and referenced
// global targets admit only Boolean/integer/decimal plus policy-gated precisionDecimal;
// explicit xs:int and other scalar kinds are unsupported, and Strict10 rejects
// precisionDecimal by policy. AttributeReferenceUse retains QName, RefLoc, TargetID,
// and effective use. Explicit form or attributeFormDefault selects qualified or
// unqualified local names; XSD 1.1 local targetNamespace selects a namespace only
// when it matches the containing targetNamespace; missing/mismatched values are
// invalid, and Strict10 reports an edition mismatch. Chameleon includes adopt
// the including target namespace.
// Anonymous local types retain AnonymousID/NodeID ownership, and returned views are
// copied. Optional/required uses are effective; prohibited uses are omitted. A valid
// referenced excluded global scalar target fails schema construction with located
// schema-syntax FailureUnsupported/ErrUnsupported at RefLoc, relates its target declaration, and
// returns no partial schema; unresolved, wrong-kind, ambiguous, and inaccessible
// references remain invalid, preserving primary ref/type/base Locs and related
// candidate/target locations. SimpleContent restrictions remain unsupported; a
// bounded scalar simpleContent extension separately
// admits Boolean/string/integer/decimal bases plus policy-gated precisionDecimal,
// retaining base, type, and ordered-use locations with a nil particle. Local
// value/default/fixed/inheritable semantics, attributeGroup expansion, and
// attribute/simpleContent validation and generation remain unsupported.
// Element-reference matrix: element-reference particles in local content and
// named groups are queryable immutable facts. Resolution retains QName, RefLoc,
// TargetID, lexical order, and exact occurrences without target-type gating.
// ValidateInstance and GenerateGo consume only supported non-extension
// default-occurrence direct-choice references to built-in or named global
// Boolean, integer, or decimal targets without identity constraints; constrained
// targets remain queryable but validation rejects at the instance use Loc with
// the first constraint Loc related. GenerateGo rejects at that constraint Loc
// with no output.
// References to global `nonNegativeInteger` remain queryable without target-type
// gating; direct-choice and sequence consumers reject them with located
// unsupported diagnostics and nil GenerateGo output.
// Sequence, anonymous-target, repetition, nested, recursive, and broader
// element-reference forms are consumer exclusions; query references retain their
// resolved facts. Model-group references are a separate top-level direct query
// boundary with ordered facts and TargetID; nested, local, recursive, and broader
// model-group references remain unsupported.
// Model-group reference particles are limited to the supported top-level direct
// `ModelGroupReferenceParticle` form. Named global model groups expose direct
// element-reference choices or sequences without expansion.
// Top-level direct model-group references on named complex types and bounded
// attribute-free extensions over named empty-content bases or named
// complexContent/restriction over built-in xs:anyType with representable
// ##other/lax wildcards are queryable as exact immutable facts without expanding
// target members. Direct model-group references retain `RefLoc`/`TargetID`; their
// consumer gates use the group RefLoc and reject them. Grouped extensions add
// one opaque direct group reference and ordered local AttributeUse facts over a
// supported named empty base. They resolve the group before the uses and base;
// effective 0/0 omits the group, and prohibited uses may leave no effective uses.
// Consumers reject at the first effective use Loc, then a present group RefLoc,
// or the extension Loc when both are absent.
// Nested, local, recursive, and broader group-reference shapes remain unsupported.
// Default-bounded sequences of supported built-in/named integer/decimal or
// all-Boolean particles are emitted as ordered Go struct fields. Local anonymous
// Boolean/integer/decimal/negativeInteger particles remain queryable but validation and generation
// reject them; repeated-field generation and direct-choice repetition remain
// unsupported.
// Bounded complexContent/extension over named empty-content complex bases,
// including the supported named `complexContent/restriction` over `xs:anyType`,
// retains extension/base identities, locations, and inherited representable
// `##other`/`lax` wildcard facts. The grouped form also retains ordered
// AttributeUse facts and never expands the referenced group's members.
// Named complex `Final()`/`FinalLoc()` use the declaring document's
// `finalDefault` when local `final` is absent; explicit local values, including
// an empty value, override it, and effective non-empty controls retain their
// local or default source location. This does not expand validation or
// `GenerateGo` consumer support or change occurrence limits.
// An extension with a present direct choice or sequence particle retains its exact
// occurrence. A model-less extension retains its named base identity and locations
// with a nil optional particle, no occurrence, and no synthetic content.
// Ordinary direct-choice/direct-sequence target checks use element/particle
// locations and may include the anonymous type location in related facts.
// Extension checks without an effective AttributeUse or group-reference particle
// use the extension boundary: code generation uses the extension location as
// primary with related complex-content/extension/base/particle facts (and
// anyAttribute when present); validation retains declaration/definition owner
// locations, uses the extension boundary for choices and the instance root for
// sequences, and never adds an anonymous type location. Direct and grouped
// model-group-reference bodies with AttributeUse facts hit the consumer gates
// first: the first use's Loc is primary, with declaration/definition and
// AttributeUse locations related. Direct and extension model-group-reference
// checks with no effective use and a present group use RefLoc as validation and
// generation primary; validation relates the group particle and supplied
// extension context, while generation relates group/component/reference/target
// locations. These gates return located FailureUnsupported/ErrUnsupported
// diagnostics; GenerateGo returns no output.
//
// Global built-in and named xs:nonNegativeInteger roots validate under
// Compatibility, Strict10, and Strict11 through the exact integer scalar plan.
// Runtime parsing accepts signed-zero lexical forms and compares them as zero.
// It preserves arbitrary precision, schema-owned bounds, enumeration, and their
// located diagnostic causes.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as direct xs:string or a named/anonymous restriction with effective
// xs:string atomic kind, or built-in/named xs:boolean/xs:token/xs:NMTOKEN/
// xs:integer/xs:nonNegativeInteger/xs:decimal
// under all policies, or built-in/named xs:precisionDecimal under Compatibility
// or Strict11, or as a named global complex type with one direct
// Boolean-only sequence of local built-in xs:boolean or facet-free named Boolean
// restriction elements, one direct integer/decimal sequence of local built-in or
// named elements, one homogeneous token/NMTOKEN sequence of local built-in or
// supported named token/NMTOKEN elements, or one direct choice
// whose scalar alternatives use default occurrences and contain local built-in or named
// Boolean, token, NMTOKEN, integer, decimal, or precisionDecimal elements, or, in
// non-extension direct choices, default-occurrence references to global Boolean,
// integer, or decimal elements without identity constraints. Constrained roots
// and referenced targets reject validation at the instance use Loc, relating
// the first constraint Loc.
// Direct local sequences match expanded
// names in lexical declaration order and honor exact finite, unbounded, and
// above-`uint64` outer and child occurrence ranges under Compatibility, Strict10,
// and Strict11. Mixed scalar-family sequences, direct-choice repetition, and excluded particle/target shapes
// remain explicit unsupported behavior. Local long/int/short/byte/unsignedLong/nonNegativeInteger/negativeInteger particles are
// query-only and remain explicit unsupported behavior in both consumers.
// Reference consumers exclude precisionDecimal and anonymous targets.
// Mixed local Boolean/numeric, token/non-token, or NMTOKEN/non-NMTOKEN choices or sequences are unsupported. Nonzero
// wildcard-bearing particles are explicit unsupported
// behavior in both consumers; absent 0/0 wildcard terms do not enter those
// gates. Scalar elements contain only character data. Global token values and
// supported local token/NMTOKEN choices and token/NMTOKEN sequences
// collapse XML whitespace before effective enumeration comparison without
// changing retained schema facts. Global NMTOKEN values and homogeneous local
// sequences made entirely of built-in or supported named NMTOKEN particles
// collapse XML whitespace and enforce the repository XML NameChar policy.
// Those sequences validate with exact occurrences and NMTOKEN value-space rules;
// their GenerateGo consumers remain unsupported. Global roots with effective
// xs:string atomic kind normalize instance text with effective whiteSpace,
// then compare enumeration values interpreted by each declaration's base type;
// violations retain text and related schema locations. Local atomic string
// particles retain their written
// QName, resolved named identity, immutable type/facet facts, locations, and exact
// occurrences in direct choices, sequences, and supported bounded extensions, but
// remain unsupported to consumers. Lists/unions,
// broader particles, and other semantics remain explicit unsupported behavior.
// Supported global attribute declarations are query-only. Compatibility,
// Strict10, and Strict11 admit built-in or supported named atomic xs:boolean,
// xs:integer, xs:decimal, xs:token, xs:negativeInteger, xs:language, xs:NCName,
// xs:anyURI, xs:ID, xs:long, xs:int, xs:short, xs:byte, and xs:unsignedLong;
// xs:short and xs:byte retain copied exact bounds. One inline simpleType may
// instead declare restriction, list, or union. Named and nested anonymous
// item/member references retain lexical order. Their atomic leaves must have
// supported attribute kinds; xs:string is admitted only below the top type.
// InlineSimpleType() and TypeReference() expose immutable anonymous identities,
// source/variety/facet locations, copied facets, ordered references, and the
// effective final/FinalLoc, including schema finalDefault. Built-in or supported
// named xs:precisionDecimal is admitted for type/value queries only under
// Compatibility or Strict11; inline precisionDecimal leaves are type-only under
// those policies. Strict10 rejects built-in/named precisionDecimal at the type
// Loc with the FeatureDatatypeFacets/FailureUnsupported/XSD3030/ErrUnsupported
// policy diagnostic; inline leaves are also policy-gated at their source Loc.
// A type attribute plus an inline child is invalid. Unresolved, invisible,
// ambiguous, wrong-kind, and cyclic member references report located errors
// without a Schema. Declared xs:string, xs:NMTOKEN, xs:nonNegativeInteger,
// xs:nonPositiveInteger, xs:positiveInteger, other excluded built-ins, and
// named list/union attribute types remain unsupported. Unsupported local attribute types
// without default/fixed report FailureUnsupported/UnsupportedSchemaSyntaxCode/
// ErrUnsupported at type Loc. Local default/fixed reports at value Loc before
// type mapping. A typeless local declaration reports at declaration Loc unless
// default/fixed is present; an inline type without a local value constraint reports
// at simpleType Loc. A referenced excluded global use reports at RefLoc with the
// target declaration related. Invalid syntax, edition/policy mismatches, and
// resolution/reference failures retain their existing diagnostic, specification
// reference, cause, and precedence. Unsupported forms return no Schema.
// Type admission is separate from value-constraint support: only Boolean,
// integer, negativeInteger, long, int, short, unsignedLong, decimal, token, and precisionDecimal constraints are
// supported. Built-in and supported named negativeInteger, long, int, short, and unsignedLong default/fixed values
// use exact IntegerValue and effective integer facets under all three policies. For
// an admitted type, an individual unsupported default or fixed is
// FailureUnsupported/UnsupportedSchemaSyntaxCode/ErrUnsupported at its value
// Loc; an invalid supported value is FailureInvalid/XSD3036 at its value Loc
// with its lexical/facet cause. Default plus fixed is FailureInvalid/XSD3010
// with fixed Loc primary and default Loc related, and no Schema. Built-in
// xs:long, xs:int, and xs:unsignedLong have intrinsic inclusive bounds
// [-9223372036854775808,9223372036854775807], [-2147483648,2147483647],
// and [0,18446744073709551615]; named references retain the written QName/type Loc,
// exact effective integer facets/bounds (including narrowed or exclusive bounds),
// facet/variety locations, provenance, ownership, and named target identity; built-in
// references have no synthetic ComponentID. TypeReference().IntegerBounds()
// returns copied effective bounds for both kinds of integer reference.
// Admitted unsignedLong values require digits-only spelling under Strict10; Compatibility
// and Strict11 permit an optional sign, including negative zero. Constraints
// retain the collapsed lexical spelling and exact value without narrowing.
// Inline global restriction/list/union facts remain queryable without
// default/fixed; either value constraint is unsupported at its source Loc.
// Attribute validation and GenerateGo remain unsupported; supported local
// anonymous atomic AttributeUse facts are separately queryable.
// GenerateGo matrix: under Compatibility, Strict10, and Strict11, global
// built-in and named-typed nonNegativeInteger element declarations and
// standalone named atomic nonNegativeInteger simple-type components in the
// resolved graph generate only when element declarations have no identity
// constraints; standalone simple-type components have no element facts.
// Element declarations cover direct, named, forward, included, imported, and
// chameleon forms and must be ordinary: abstract=false, nillable=false, and no
// identity constraints. The abstract/nillable gate applies only to global
// element declarations; either
// flag true is unsupported by GenerateGo with FailureUnsupported/GOXSD9029 and
// nil output. Built-in nonNegativeInteger element fields and standalone named
// nonNegativeInteger declarations use StrictInteger; named-typed element fields
// use the generated named type.
// Built-in canonical facts require integer kind/version,
// fractionDigits exactly 0 and fixed, no totalDigits, and exactly minInclusive=0
// with no other bounds. Named restrictions may retain schema-owned bounds/facets;
// malformed/stale built-in or named facts fail closed as FailureInternal/GOXSD9030
// with nil output. Named final, atomic-restriction-variety, and effective-facet
// gates reject unsupported forms with FailureUnsupported/GOXSD9029 and no output.
// Supported global built-in/named Boolean/integer/decimal and effective
// xs:string-atomic/token/NMTOKEN simple-type components and global element
// declarations generate, as do global inline-element string/token/NMTOKEN
// declarations. Standalone named normalizedString restrictions reject.
// Non-extension default-occurrence direct-choice references to
// global built-in/named Boolean, integer, or decimal targets are also
// generation-eligible only when targets have no identity constraints. Any
// identity-constrained global element, including a reference target, makes
// GenerateGo return FailureUnsupported/GOXSD9029 at its first constraint Loc
// with no output. Global attribute declarations remain query-only,
// inline-attribute consumers remain excluded, and GenerateGo rejects every
// ComponentKindAttributeDeclaration. GenerateGo also rejects retained local
// `nonNegativeInteger` particles.
// References to global `nonNegativeInteger` remain queryable
// without target gating; direct-choice and sequence consumers reject them with
// located unsupported diagnostics and nil GenerateGo output. Consumer-only
// exclusions for admitted global `nonNegativeInteger` references include
// repetition/non-default occurrences, nested/recursive/broader references,
// and anonymous targets; they are explicit unsupported behavior with located
// diagnostics and no GenerateGo output. Admitted lists/unions and other
// integer-derived declarations retain query facts while their consumers remain
// unsupported. `nonNegativeInteger` attributes/value constraints are
// schema-admission exclusions with located diagnostics and no schema. Global inline/anonymous
// `nonNegativeInteger` element/type declarations retain schema/query facts; GenerateGo and
// ValidateInstance reject them with their existing diagnostics.
// Global inline-element Boolean/integer/decimal declarations and global
// element/type int/long/short/byte/unsignedLong/negativeInteger/nonPositiveInteger/positiveInteger and
// language/NCName/anyURI/ID declarations
// retain schema/query facts but their validation and generation consumers are
// rejected. This consumer boundary does not widen the global attribute type or
// value-constraint model described above.
// Built-in xs:short has inclusive [-32768,32767] bounds without a component ID
// or bound Loc; named restrictions retain exact effective bounds.
// Built-in xs:byte has inclusive [-128,127] bounds without a component ID
// or bound Loc; named restrictions retain exact effective bounds.
// Global built-in, named, and inline precisionDecimal element/type schema/query facts are
// available only under Compatibility/Strict11; Strict10 returns the located
// FeatureDatatypeFacets/FailureUnsupported/ErrUnsupported policy diagnostic
// before validation at the typed reference or type location. Global built-in/named
// roots validate under those policies, while inline precisionDecimal is an
// anonymous target rejected by validation. GenerateGo rejects every global,
// explicitly typed local (including named effective), inline, anonymous, and
// schema-admitted extension precisionDecimal target. Local built-in/named
// Boolean/integer/decimal particles generate only in default-occurrence
// all-Boolean/numeric direct choices and default-bounded direct sequences.
// All-token direct choices also generate: built-in alternatives use string and
// supported named restrictions use their generated type. Local
// long/int/short/byte/unsignedLong/nonNegativeInteger/negativeInteger, string,
// anonymous, NMTOKEN and mixed-token consumers, repeated/non-default
// particles, and anonymous targets remain unsupported; numeric integer/decimal
// mixtures remain supported.
package goxsd9
