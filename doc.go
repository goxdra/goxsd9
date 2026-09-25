// Package goxsd9 provides a supported vertical slice for parsing XML Schema
// documents into immutable schema components and validating scalar XML
// instances.
//
// ParseSchema accepts a caller-created ResolvedSource and a Resolver. The
// current subset discovers mixed XSD 1.0 and XSD 1.1 schema graphs and builds
// supported schema-level components, including simple-type atomic restrictions,
// lists, and unions. Anonymous simple types and resolved built-in, named, and
// anonymous simple-type references are modeled, along with global xs:boolean,
// xs:nonNegativeInteger, and atomic xs:string/xs:token/xs:NMTOKEN declarations
// and their named or anonymous restrictions.
// Queries and walks are deterministic. SimpleTypeDefinition.IsBoolean,
// StringEnumerationFacets, and StringWhiteSpaceFacet report immutable kind
// and implemented scalar facts. ParseSchema uses graph-wide Compatibility;
// ParseSchemaWithPolicy applies one validated policy to the complete graph.
// A successful ParseSchema returns an immutable Schema; Documents, Components,
// Lookup, Find, FindKind, and Walk expose deterministic query views, while
// AttributeDeclaration exposes resolved type and value-constraint facts.
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
// resolved type Loc before conversion; global inline-attribute declarations and
// attribute validation/GenerateGo remain unsupported. Supported local anonymous
// atomic AttributeUse facts are separate. Under admitting policies,
// built-in/named roots validate, while inline anonymous targets remain excluded
// from validation and generation.
// Paths and URLs are never opened by this package. Parsing closes
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
// not global components or Walk entries. Strict10 policy admission precedes 0/0
// omission for both explicitly typed local built-in or named-effective
// `precisionDecimal` forms and inline anonymous
// `<xs:simpleType><xs:restriction base="xs:precisionDecimal">` forms,
// including zero-occurrence cases. Compatibility and Strict11 omit effective
// 0/0 only after the mapped type/reference and occurrence validate as omittable.
// Invalid references/targets, malformed occurrences, and policy mismatches retain
// their diagnostics and return no schema. This Strict10-before-omission rule is
// specific to `precisionDecimal`; ordinary local declared, named, inline, or
// anonymous `nonNegativeInteger` 0/0 forms are likewise absent only after
// validation under every policy.
// Local declared, named, inline, and anonymous integer-derived restrictions are
// admitted at the mapped non-0/0 boundary only when their effective atomic kind
// is integer or negativeInteger through named, forward, imported, included, and
// chameleon chains. A direct local `type="xs:negativeInteger"` is schema-rejected;
// named and inline effective negativeInteger forms are admitted as query facts,
// but ValidateInstance and GenerateGo reject those consumers. Effective int, long,
// unsignedLong, nonNegativeInteger, and
// nonPositiveInteger are valid datatypes but unsupported at this local schema
// boundary: ParseSchema rejects the mapped form with a located
// FailureUnsupported/ErrUnsupported diagnostic at the relevant type or facet
// location and no schema. Ordinary local nonNegativeInteger effective 0/0
// remains absent after occurrence/type validation and policy admission under every policy. The written base
// QName, use-site location, and resolved named ownership remain separate facts.
// The supported anonymous Boolean/integer/
// decimal restriction facet subset remains queryable; mapped non-0/0 non-string
// anonymous enumeration remains explicit unsupported at its facet location with
// no schema.
// Token/NMTOKEN current-state matrix: explicitly typed built-in or supported
// named local particles in direct choices, sequences, and bounded attribute-free
// extensions are modeled and queryable; only non-extension default-occurrence
// homogeneous direct choices made entirely of local token or NMTOKEN
// alternatives validate. Homogeneous direct sequences made entirely of local
// built-in or supported named token or NMTOKEN particles also validate with exact
// occurrences. Anonymous token/NMTOKEN restrictions remain unsupported for
// consumers; local token/NMTOKEN particles and sequences remain
// GenerateGo-unsupported. Direct element references remain
// queryable, but token/NMTOKEN reference consumers remain unsupported.
// Global inline string/token/NMTOKEN elements are the separate generation-eligible
// exception.
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
// Local precisionDecimal forms are distinct. Under Compatibility/Strict11, a
// local element declared with built-in `type="xs:precisionDecimal"` or a named
// type whose effective facets are precisionDecimal is admitted only in
// default-occurrence direct choices and bounded attribute-free extension
// choices. The choice owner and every mapped typed precisionDecimal
// child/alternative require default occurrences; non-precision alternatives
// may retain non-default query-only ranges. An inline anonymous
// `<xs:simpleType><xs:restriction base="xs:precisionDecimal">` restriction is
// schema-unsupported when mapped; mapped nonzero anonymous restrictions remain
// unsupported. Strict10 returns a located
// FeatureDatatypeFacets/FailureUnsupported/ErrUnsupported policy-mismatch
// diagnostic before 0/0 omission for either mapped form, including zero. Under
// Compatibility/Strict11, mapped non-default precisionDecimal choice/alternative
// ranges or non-0/0 direct-sequence precisionDecimal ranges that map to particles
// are schema-unsupported. Only non-extension default-occurrence typed direct
// choices are validation-eligible; extension choices and all anonymous consumers
// are rejected by validation and generation.
// The supported local anonymous model is limited to atomic
// Boolean/integer/decimal restrictions in the direct choice/sequence and bounded
// attribute-free extension shapes above. Local anonymous Boolean/integer/decimal
// restrictions remain queryable but direct validation and generation reject them;
// mapped local anonymous string/token/NMTOKEN/precisionDecimal restrictions remain
// schema-unsupported when nonzero. Global inline-element precisionDecimal remains a query
// target only under Compatibility/Strict11; Strict10 rejects it before validation,
// and every anonymous precisionDecimal target is excluded from validation and
// generation.
// Particle-plus-use bodies (including direct model-group references), attribute-only
// bodies, and extension-only scalar simpleContent bodies expose ordered defensive
// local, referenced, and anonymous-inline AttributeUse facts. Supported local
// anonymous atomic uses retain AnonymousID/NodeID. Local and referenced
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
// other scalar target fails schema construction with located schema-syntax
// FailureUnsupported/ErrUnsupported at RefLoc, relates the target declaration, and
// returns no partial schema; unresolved, wrong-kind, ambiguous, and inaccessible
// references remain invalid, preserving primary ref/type/base Locs and related
// candidate/target locations. A bounded scalar simpleContent extension separately
// admits Boolean/string/integer/decimal bases plus policy-gated precisionDecimal,
// retaining base, type, and ordered-use locations with a nil particle. Local
// value/default/fixed/inheritable semantics and attribute/simpleContent validation
// and generation remain unsupported.
// Element-reference matrix: element-reference particles in local content and
// named groups are queryable immutable facts. Resolution retains QName, RefLoc,
// TargetID, lexical order, and exact occurrences without target-type gating.
// ValidateInstance and GenerateGo consume only supported non-extension
// default-occurrence direct-choice references to built-in or named global
// Boolean, integer, or decimal targets; only those targets are consumer-eligible.
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
// attribute-free extensions over named empty-content bases are queryable as exact
// immutable facts without expanding target members. Direct model-group references
// retain `TargetID`; nested, local, recursive, and broader group-reference shapes
// remain unsupported.
// Default-bounded sequences of supported built-in or named numeric or
// all-Boolean particles are emitted as ordered Go struct fields. Local anonymous
// Boolean/integer/decimal particles remain queryable but validation and generation
// reject them; repeated-field generation and direct-choice repetition remain
// unsupported.
// Bounded attribute-free complexContent/extension over named empty-content
// complex bases, including the supported named `complexContent/restriction` over
// `xs:anyType` representation, retains extension/base identities and locations
// and only inherited bounded, representable wildcard facts (`##other`/`lax`).
// Named complex `Final()`/`FinalLoc()` use the declaring document's
// `finalDefault` when local `final` is absent; explicit local values, including
// an empty value, override it, and effective non-empty controls retain their
// local or default source location. This does not expand validation or
// `GenerateGo` consumer support or change occurrence limits.
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
// sequences, and never adds an anonymous type location. Direct model-group-
// reference bodies with AttributeUse facts hit the AttributeUse consumer gates
// first: the first use's Loc is primary, with declaration/definition and
// AttributeUse locations related. Attribute-free direct and extension
// model-group-reference checks use the group RefLoc as validation and
// generation primary; validation relates the group particle and supplied
// extension context, while generation relates group/component/reference/target
// locations. These gates return located FailureUnsupported/ErrUnsupported
// diagnostics; GenerateGo returns no output.
//
// Global built-in and named xs:nonNegativeInteger roots remain unsupported by
// ValidateInstance under Compatibility, Strict10, and Strict11: the call
// returns FailureUnsupported/XSD4004/ErrUnsupported, and schema-owned bounds
// and facets receive no runtime facet validation.
//
// ValidateInstance supports one complete instance rooted at a global element
// declared as built-in or named xs:boolean/xs:token/xs:NMTOKEN/xs:integer/xs:decimal
// under all policies, or built-in/named xs:precisionDecimal under Compatibility
// or Strict11, or as a named global complex type with one direct
// Boolean-only sequence of local built-in xs:boolean or facet-free named Boolean
// restriction elements, one direct integer/decimal sequence of local built-in or
// named elements, one homogeneous token/NMTOKEN sequence of local built-in or
// supported named token/NMTOKEN elements, or one direct choice
// whose scalar alternatives use default occurrences and contain local built-in or named
// Boolean, token, NMTOKEN, integer, decimal, or precisionDecimal elements, or, in
// non-extension direct choices, default-occurrence references to global Boolean,
// integer, or decimal elements. Local scalar consumers accept
// built-in or named references only: direct choice/sequence checks reject modeled
// anonymous local inline atomic references with located
// FailureUnsupported/ErrUnsupported diagnostics that may include the anonymous
// type location in related facts. Non-model-group-reference extension checks run
// first at the extension boundary, retain complex-content/extension/base/particle
// (and anyAttribute when present) related locations, and do not include the
// anonymous type location; validation also retains declaration/definition owner
// locations and keeps the instance-root primary for sequences, while GenerateGo
// rejects them with the same classification and no output. Direct model-group-
// reference bodies with AttributeUse facts hit the AttributeUse consumer gates
// first: the first use's Loc is primary, with declaration/definition and
// AttributeUse locations related. Attribute-free direct and extension
// model-group-reference checks use the group reference RefLoc as validation and
// generation primary; validation retains the group particle and supplied
// extension context in related facts, and generation retains
// group/component/reference/target related locations.
// Direct local sequences match expanded
// names in lexical declaration order and honor exact finite, unbounded, and
// above-`uint64` outer and child occurrence ranges under Compatibility, Strict10,
// and Strict11. Mixed scalar-family sequences, direct-choice repetition, and excluded particle/target shapes
// remain explicit unsupported behavior. Reference consumers exclude precisionDecimal
// and anonymous targets.
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
// their GenerateGo consumers remain unsupported. Global string values, local
// string particles, lists/unions, broader particles, and other semantics remain
// explicit unsupported behavior.
// Supported global attribute declarations are a separate query-only capability.
// Type admission under
// Compatibility, Strict10, and Strict11 is limited to built-in or supported
// named atomic xs:boolean, xs:integer, xs:decimal, xs:token, xs:negativeInteger,
// xs:language, xs:NCName, xs:anyURI, xs:ID, xs:long, and xs:unsignedLong. Built-in or supported
// named xs:precisionDecimal is admitted for type/value queries only under
// Compatibility or Strict11; Strict10 rejects it at the type Loc with the
// FeatureDatatypeFacets/FailureUnsupported/XSD3030/ErrUnsupported policy
// diagnostic. Declared xs:string, xs:NMTOKEN, xs:int, xs:nonNegativeInteger,
// xs:nonPositiveInteger, narrower built-ins, list/union
// forms remain explicit unsupported behavior. A local named attribute use reports
// FailureUnsupported/UnsupportedSchemaSyntaxCode/ErrUnsupported at its type
// attribute Loc; a local attribute declaration without type reports at its
// declaration Loc; an inline type reports at its simpleType Loc; and a referenced excluded
// global use reports at RefLoc with the target declaration related. Invalid syntax,
// edition/policy mismatches, and
// resolution/reference failures retain their existing diagnostic, specification
// reference, cause, and precedence. Unsupported forms return no Schema.
// Type admission is separate from value-constraint support: only Boolean,
// integer, decimal, token, and precisionDecimal constraints are supported. For
// an admitted type, an individual unsupported default or fixed is
// FailureUnsupported/UnsupportedSchemaSyntaxCode/ErrUnsupported at its value
// Loc; an invalid supported value is FailureInvalid/XSD3036 at its value Loc
// with its lexical/facet cause. Default plus fixed is FailureInvalid/XSD3010
// with fixed Loc primary and default Loc related, and no Schema. Built-in
// xs:long and xs:unsignedLong have intrinsic inclusive bounds
// [-9223372036854775808,9223372036854775807] and [0,18446744073709551615];
// named xs:long/xs:unsignedLong references retain the written QName/type Loc,
// exact effective integer facets/bounds (including narrowed or exclusive bounds),
// facet/variety locations, provenance, and named target identity; built-in
// references have no synthetic ComponentID. Admitted
// global precisionDecimal constraints retain zero or one optional default/fixed
// AttributeValueConstraint; type-only declarations return no value constraint.
// ValueConstraint() copies kind, collapsed lexical spelling, source Loc, and
// exact defensive StrictPrecisionDecimal through PrecisionDecimalValue only
// when present. Attribute validation and generation remain unsupported
// consumers; global inline-attribute declarations are separate, while supported
// local anonymous atomic AttributeUse facts remain queryable.
// GenerateGo matrix: under Compatibility, Strict10, and Strict11, global
// built-in and named-typed nonNegativeInteger element declarations and
// standalone named atomic nonNegativeInteger simple-type components in the
// resolved graph generate.
// Element declarations cover direct, named, forward, included, imported, and
// chameleon forms and must be ordinary: abstract=false and nillable=false. The
// abstract/nillable gate applies only to global element declarations; either
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
// Supported global built-in/named Boolean/integer/decimal and
// string/token/NMTOKEN simple-type components and supported global element
// declarations generate, as do global inline-element string/token/NMTOKEN
// declarations. Non-extension default-occurrence direct-choice references to
// global built-in/named Boolean, integer, or decimal targets are also
// generation-eligible. Global attribute declarations remain query-only,
// inline-attribute consumers remain excluded, and GenerateGo rejects every
// ComponentKindAttributeDeclaration. Mapped non-0/0 local declared,
// named, inline, and anonymous `nonNegativeInteger` forms are rejected during
// schema construction with no schema. Exact local declared, named, inline, and
// anonymous `0/0` forms are admitted then absent
// under every policy. References to global `nonNegativeInteger` remain queryable
// without target gating; direct-choice and sequence consumers reject them with
// located unsupported diagnostics and nil GenerateGo output. Consumer-only
// exclusions for admitted global `nonNegativeInteger` references include
// repetition/non-default occurrences, nested/recursive/broader references,
// anonymous targets, lists/unions, attributes/value constraints, and other
// integer-derived consumers; they are explicit unsupported behavior with located
// diagnostics and no GenerateGo output. Global inline/anonymous
// `nonNegativeInteger` element/type declarations retain schema/query facts; GenerateGo and
// ValidateInstance reject them with their existing diagnostics.
// Global inline-element Boolean/integer/decimal declarations and global
// element/type int/long/unsignedLong/negativeInteger/nonPositiveInteger and
// language/NCName/anyURI/ID declarations
// retain schema/query facts but their validation and generation consumers are
// rejected. This consumer boundary does not widen the global attribute type or
// value-constraint model described above.
// Global built-in, named, and inline precisionDecimal element/type schema/query facts are
// available only under Compatibility/Strict11; Strict10 returns the located
// FeatureDatatypeFacets/FailureUnsupported/ErrUnsupported policy diagnostic
// before validation at the typed reference or type location. Global built-in/named
// roots validate under those policies, while inline precisionDecimal is an
// anonymous target rejected by validation. GenerateGo rejects every global,
// explicitly typed local (including named effective), inline, anonymous, and
// schema-admitted extension precisionDecimal target. Local built-in/named
// Boolean/integer/decimal particles generate only in default-occurrence
// all-Boolean/numeric direct choices and default-bounded direct sequences. Local
// anonymous and token/NMTOKEN consumers, repeated/non-default particles, and
// anonymous targets remain unsupported; numeric integer/decimal mixtures remain
// supported.
package goxsd9
