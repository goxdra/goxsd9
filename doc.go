// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating supported XML
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
// resolved type Loc before conversion. Global inline-attribute declarations and
// global attribute-declaration validation remain unsupported. Selected local
// precisionDecimal AttributeUse values validate under admitting policies;
// GenerateGo rejects them and global attribute declarations.
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
// outside the global walk; supported precisionDecimal attribute roots and
// sequence targets validate, while their other consumers reject. The model also admits
// built-in/named/inline xs:negativeInteger and built-in or supported
// named-effective xs:long/xs:int/xs:short/xs:byte/xs:unsignedLong/
// xs:nonNegativeInteger particles in
// supported attribute-free extension choices and sequences under every policy.
// Direct-sequence negativeInteger validates beside bounded list/union; long-family and
// nonNegativeInteger remain query-only. Generation rejects all these particles.
// Direct and supported extension
// choices/sequences retain exact finite, unbounded, and above-uint64 occurrences;
// bounded describes the supported derivation/base shape, not occurrence limits.
// Supported local choice/sequence elements may retain a TypeID for a named
// scalar simpleContent complex type, with exact occurrences and no particle on
// the target type.
// That target keeps its ordered attribute uses; string/Boolean/integer/decimal
// bases work under every policy, while precisionDecimal requires Compatibility
// or Strict11. Unsupported bases or attribute forms fail schema construction;
// bounded attribute sequences validate named targets with selected local
// precisionDecimal uses or precisionDecimal simpleContent without uses; a string
// simpleContent base must be built-in. Generation rejects them. The model
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
// resolution. At 0/0, supported inline simple-type forms resolve bases and facets
// before omission; invalid, unresolved, cyclic, wrong-kind, value-constraint,
// and policy failures retain their diagnostics and prevent a Schema. Resolved
// unsupported forms may omit. Direct-all anonymous complex members remain
// located unsupported at every occurrence because their semantic gates are unavailable.
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
// A direct xs:all on a named complex type retains one immutable ordered
// AllParticle member view of local integer/decimal/Boolean declarations,
// built-in token/NMTOKEN declarations, and element references. Member order is lexical
// for queries; matching remains unsupported. XSD 1.0 limits outer and member occurrences; XSD 1.1 permits
// exact general member bounds. Resolved 0/0 terms omit after their gates;
// inline complex members reject even at 0/0. Surviving duplicate names,
// anonymous simple types, excluded member shapes, and other scalar types
// return located diagnostics and no Schema.
// ValidateInstance rejects modeled all
// particles; GenerateGo returns nil output with an unsupported diagnostic.
// In supported direct choices, direct sequences, and bounded attribute-free
// extensions, local declared, named, inline, and anonymous restrictions in the
// integer/negativeInteger branch are admitted at the mapped non-0/0 boundary
// when their effective atomic kind is integer or negativeInteger through named,
// forward, imported, included, and chameleon chains. Those shapes also admit
// explicit built-in and supported named nonNegativeInteger particles. Built-in,
// named-effective, and anonymous-inline negativeInteger forms in those shapes
// remain queryable. Bounded list/union direct sequences validate negativeInteger
// as an atomic sibling; other validation paths and GenerateGo reject it. Built-in
// and named effective-long particles in those shapes remain query-only and
// consumer-rejected.
// Local nonPositiveInteger/positiveInteger and inline/anonymous
// long/int/short/byte/unsignedLong/nonNegativeInteger are excluded when mapped non-0/0: ParseSchema returns a located
// FeatureSchemaSyntax/FailureUnsupported/UnsupportedSchemaSyntaxCode/ErrUnsupported
// diagnostic at the type, facet, or element Loc and no Schema. Nested-particle
// exclusions use the nested-particle Loc. Where occurrence grammar permits it,
// a resolved 0/0 local term omits after applicable gates.
// Inline/anonymous long/int/short/byte/unsignedLong/nonNegativeInteger are mapped schema exclusions at their
// type/simpleType Loc; supported direct choices, direct sequences, and bounded
// attribute-free extensions admit built-in and named-effective
// long/int/short/byte/unsignedLong/nonNegativeInteger as query-only forms.
// The written base QName/base Loc, use-site/type/facet Locs,
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
// identities, graph provenance, and exact occurrences. Outside bounded list/union
// direct sequences, validation rejects negativeInteger and long-family particles;
// GenerateGo rejects them with FailureUnsupported diagnostics.
// The supported anonymous Boolean/integer/
// decimal restriction facet subset remains queryable. Anonymous non-string
// enumeration other than precisionDecimal is unsupported at its facet location.
// PrecisionDecimal enumeration passes that facet gate under Compatibility/Strict11:
// global inline restrictions remain queryable, while mapped nonzero local
// anonymous restrictions fail at the local type location with no schema.
// Token/NMTOKEN current-state matrix: explicitly typed built-in or supported
// named local particles in direct choices, sequences, and bounded attribute-free
// extensions are modeled and queryable. Direct named-complex xs:all also models
// built-in token/NMTOKEN locals; named and inline token/NMTOKEN locals remain excluded there,
// and both consumers reject xs:all. Only non-extension default-occurrence
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
// query-only/consumer-rejected. Bounded list/union sequences also validate
// global anonymous atomic precisionDecimal reference targets; generation rejects them.
// The supported local element anonymous model is limited to atomic
// Boolean/integer/decimal/negativeInteger restrictions in the direct choice/sequence
// and bounded attribute-free extension shapes above. Local anonymous
// Boolean/integer/decimal/negativeInteger restrictions remain queryable; bounded
// list/union direct sequences validate anonymous integer/negativeInteger siblings,
// while other direct validation paths and generation reject them;
// mapped local anonymous string/token/NMTOKEN/precisionDecimal restrictions remain
// schema-unsupported when nonzero. Global inline-element atomic precisionDecimal
// restrictions remain query targets only under Compatibility/Strict11; Strict10
// rejects them before validation. Standalone anonymous atomic precisionDecimal roots
// reject validation; global references to them validate beside bounded list/union
// sequences. Generation rejects both. Bounded inline list/union roots
// validate under admitting policies and remain generation exclusions.
// Global inline complex types expose stable anonymous ComplexTypeID nodes,
// exact ordered sequence/reference particles, and attribute uses without
// entering the global component walk. Named or inline global precisionDecimal
// list/union elements and direct-sequence local/ref links retain immutable
// variety, ordered item/member facts, locations, and identities. Global inline
// anonymous precisionDecimal atomic declarations retain query facts and identities.
// Particle-plus-use bodies, direct model-group references, grouped extensions,
// attribute-only bodies, and scalar simpleContent extensions expose ordered defensive
// local, referenced, and anonymous-inline AttributeUse facts. Supported local
// anonymous atomic uses retain AnonymousID/NodeID. One named complexContent
// extension composes a direct opaque named-group reference, ordered local uses,
// and the supported named empty base; broader attribute-bearing extensions
// remain unsupported. Local and referenced
// global targets admit Boolean/integer/decimal plus policy-gated precisionDecimal.
// Local named/inline lists with atomic precisionDecimal items and two-member
// unions of precisionDecimal then negativeInteger are also admitted; other
// local varieties and explicit xs:int remain unsupported. Strict10 rejects
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
// value/default/fixed/inheritable semantics and attributeGroup expansion remain
// unsupported. Validation supports local precisionDecimal atomic and bounded
// variety uses on direct empty-content roots and direct sequences of global
// element references to inline or named complex targets or named local complex
// targets; grouped extensions remain consumer-unsupported.
// SimpleContent text uses built-in string only
// with selected local precisionDecimal uses, or built-in/named effective
// precisionDecimal. Named effective string remains excluded. Generation rejects these forms.
// Element-reference matrix: element-reference particles in local content and
// named groups are queryable immutable facts. Resolution retains QName, RefLoc,
// TargetID, lexical order, and exact occurrences without target-type gating.
// ValidateInstance consumes supported non-extension default-occurrence
// direct-choice references to built-in or named global Boolean, integer, or
// decimal targets without identity constraints. GenerateGo consumes those
// direct-choice references and also default-occurrence direct-sequence references
// to global built-in or named integer and decimal targets. It retains the
// sequence's lexical order and omits standalone wrappers for referenced elements.
// ValidateInstance also admits bounded direct-sequence references to global
// inline or named complex targets with local precisionDecimal atomic/variety
// uses or precisionDecimal simpleContent without uses, including repeated refs
// with supported child occurrence ranges. Direct-sequence references to
// admitted global precisionDecimal list/union elements, built-in/named
// negativeInteger, or anonymous string/integer/negativeInteger/precisionDecimal
// targets validate under the bounded variety gate; GenerateGo rejects these
// sequences.
// Constrained targets remain queryable; validation rejects at the instance use
// Loc with the first constraint Loc related. GenerateGo rejects target
// classification at the reference Loc; identity-constrained global elements
// reject at the first constraint Loc with no output.
// References to global `nonNegativeInteger` remain queryable without target-type
// gating; direct-choice and sequence consumers reject them with located
// unsupported diagnostics and nil GenerateGo output.
// Outside bounded variety sequences and the selected forms above, anonymous
// targets, substitution, nested, recursive, and broader forms are consumer
// exclusions; query references
// retain their resolved facts. Model-group references are a separate top-level
// direct query boundary with ordered facts and TargetID; nested, local,
// recursive, and broader model-group references remain unsupported.
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
// Boolean/integer/decimal/negativeInteger particles retain query facts;
// generation rejects them. Repeated-field generation and direct-choice
// repetition remain unsupported.
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
// xs:integer/xs:nonNegativeInteger/xs:byte/xs:short/xs:int/xs:decimal
// under all policies, or built-in/named xs:precisionDecimal under Compatibility
// or Strict11, or as a supported local-attribute complex root or bounded
// direct sequence of global complex refs or named local complex targets.
// Named/inline global precisionDecimal lists and bounded unions validate under
// Compatibility/Strict11. Selected local uses admit precisionDecimal atomic,
// precisionDecimal-item list, or precisionDecimal/negativeInteger union types;
// simpleContent text uses built-in string with those uses or built-in/named
// precisionDecimal.
// Other supported named global complex types have one direct
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
// Scalar direct local sequences match expanded names in lexical declaration
// order and honor exact finite, unbounded, and above-`uint64` outer and child
// occurrence ranges under Compatibility, Strict10, and Strict11. Direct
// list/union sequences admit typed local string, integer, negativeInteger, or
// precisionDecimal siblings; anonymous local integer/negativeInteger siblings;
// and global element refs, including supported anonymous string/integer/
// negativeInteger/precisionDecimal targets.
// Outer occurrences must default; child ranges remain exact. Selected
// local precisionDecimal atomic/variety attribute sequences likewise honor
// child ranges and require default outer occurrences; other outer ranges
// return a located unsupported diagnostic. List items split collapsed XML
// whitespace, including an empty list; union members run in declared order
// and retain the selected value semantics. Structure errors precede value
// errors; failures retain instance and related schema locations.
// Outside bounded list/union sequences, mixed scalar-family sequences,
// direct-choice repetition, and excluded particle/target shapes remain unsupported.
// Local long/int/short/byte/unsignedLong/nonNegativeInteger particles are query-only
// to both consumers; negativeInteger also validates as a bounded variety sibling.
// Direct-choice reference consumers exclude precisionDecimal and anonymous targets.
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
// occurrences in direct choices, sequences, and supported bounded extensions.
// They validate beside bounded list/union in direct sequences; other consumer
// paths reject them. Other list/union shapes, broader particles,
// and unimplemented semantics remain explicit unsupported behavior.
// Supported global attribute declarations are a separate query-only capability.
// Type admission under
// Compatibility, Strict10, and Strict11 is limited to built-in or supported
// named atomic xs:boolean, xs:integer, xs:decimal, xs:token, xs:negativeInteger,
// xs:positiveInteger, xs:nonPositiveInteger, xs:language, xs:NCName, xs:anyURI, xs:ID,
// xs:long, xs:int, xs:short, xs:byte, and xs:unsignedLong. Integer-derived
// types retain copied exact bounds. Built-in or
// supported named xs:precisionDecimal is admitted for type/value queries only under
// Compatibility or Strict11; Strict10 rejects it at the type Loc with the
// FeatureDatatypeFacets/FailureUnsupported/XSD3030/ErrUnsupported policy
// diagnostic. Declared xs:string, xs:NMTOKEN, xs:nonNegativeInteger,
// other excluded built-ins, list/union
// forms remain explicit unsupported behavior. Unsupported local attribute types
// without default/fixed report FailureUnsupported/UnsupportedSchemaSyntaxCode/
// ErrUnsupported at type Loc. Local default/fixed reports at value Loc before
// type mapping. A typeless local declaration reports at declaration Loc unless
// default/fixed is present; an inline type without a local value constraint reports
// at simpleType Loc. A referenced excluded global use reports at RefLoc with the
// target declaration related. Invalid syntax, edition/policy mismatches, and
// resolution/reference failures retain their existing diagnostic, specification
// reference, cause, and precedence. Unsupported forms return no Schema.
// Type admission is separate from value-constraint support: only Boolean,
// integer, negativeInteger, positiveInteger, long, int, short, byte, unsignedLong, decimal, token, and precisionDecimal constraints are
// supported. Built-in and supported named negativeInteger, positiveInteger, long, int, short, byte, and unsignedLong default/fixed values
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
// Admitted global precisionDecimal constraints retain zero or one optional default/fixed
// AttributeValueConstraint; type-only declarations return no value constraint.
// ValueConstraint() copies kind, collapsed lexical spelling, source Loc, and
// exact defensive StrictPrecisionDecimal through PrecisionDecimalValue only
// when present. Global attribute-declaration validation and generation remain
// unsupported; global inline-attribute declarations are separate, while supported
// local anonymous atomic AttributeUse facts remain queryable.
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
// global built-in/named Boolean, integer, or decimal targets are generation-
// eligible only when targets have no identity constraints. Default-occurrence
// direct-sequence references to global built-in/named integer or decimal targets
// are also generation-eligible under the same target gates. Any
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
// diagnostics and no GenerateGo output. `nonNegativeInteger` attributes/value
// constraints are schema-admission exclusions with located diagnostics and no
// schema. Global inline/anonymous `nonNegativeInteger` element/type declarations
// retain schema/query facts; GenerateGo and ValidateInstance reject them with
// their existing diagnostics.
// Standalone global inline-element Boolean/integer/decimal declarations and global
// element/type long/unsignedLong/negativeInteger/nonPositiveInteger/positiveInteger and
// language/NCName/anyURI/ID declarations retain schema/query facts but their root
// validation and generation consumers reject.
// Direct built-in and supported named atomic-byte global roots validate;
// global inline byte roots and admitted local/reference byte uses reject validation.
// Local inline byte particles reject schema admission; GenerateGo rejects byte-bearing components.
// Direct built-in and supported named atomic-short global roots validate;
// global inline short roots and admitted local/reference short uses reject validation.
// Local inline short particles reject schema admission; GenerateGo rejects short-bearing components.
// Direct built-in and supported named atomic-int global roots validate;
// global inline int roots and admitted local/reference int uses reject validation.
// Local inline int particles reject schema admission; GenerateGo rejects int-bearing components.
// This consumer boundary does not widen the global attribute type or
// value-constraint model described above.
// Built-in xs:short has inclusive [-32768,32767] bounds without a component ID
// or bound Loc; named restrictions retain exact effective bounds.
// Built-in xs:byte has inclusive [-128,127] bounds without a component ID
// or bound Loc; named restrictions retain exact effective bounds.
// GenerateGo rejects precisionDecimal targets across global, explicitly typed
// local (including named effective), inline, anonymous, and schema-admitted
// extension forms.
// Local built-in/named Boolean/integer/decimal particles generate only in
// default-occurrence all-Boolean/numeric direct choices and default-bounded
// direct sequences.
// All-token direct choices also generate: built-in alternatives use string and
// supported named restrictions use their generated type. Local
// long/int/short/byte/unsignedLong/nonNegativeInteger/negativeInteger, string,
// anonymous, NMTOKEN and mixed-token generation, repeated/non-default
// particles, and anonymous generation targets remain unsupported; numeric integer/decimal
// mixtures remain supported.
package goxsd9
