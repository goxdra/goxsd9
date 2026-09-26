# 0007: Exact particle occurrence ranges

Status: accepted

## Decision

Particle bounds are exact non-negative arbitrary-precision `StrictInteger` values
or max-only `unbounded`. The model has no sentinel, fixed-width conversion,
floating point, duplicate flag, or nullable state. A completed range has finite
minimum and finite/unbounded maximum; finite maximum requires minimum <= maximum.

The XSD 1.0 definitions are [`xsd10-structures#Particle_details`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#Particle_details),
[`xsd10-structures#p-min_occurs`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#p-min_occurs),
[`xsd10-structures#p-max_occurs`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#p-max_occurs),
[`xsd10-structures#coss-particle`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#coss-particle),
and [`xsd10-structures#cParticles`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#cParticles).
The XSD 1.1 definitions are [`xsd11-structures#Particle_details`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#Particle_details),
[`xsd11-structures#p-min_occurs`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#p-min_occurs),
[`xsd11-structures#p-max_occurs`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#p-max_occurs),
[`xsd11-structures#coss-particle`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#coss-particle),
and [`xsd11-structures#cParticles`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#cParticles).
The finite value space and lexical rules come from
[`xsd10-datatypes#nonNegativeInteger`](https://www.w3.org/TR/2004/REC-xmlschema-2-20041028/#nonNegativeInteger)
and [`xsd11-datatypes#nonNegativeInteger`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#nonNegativeInteger).

## Normative occurrence table

Both editions share table; no-component entries are not zeroed public particles;
edition-specific `all` restrictions follow.

| Input or condition | XSD 1.0 | XSD 1.1 |
| --- | --- | --- |
| Both attributes omitted | Effective `1/1`; construct finite `1/1`. | Same as XSD 1.0. |
| `minOccurs="0"`, maximum omitted | Effective `0/1`; preserve exact zero and optionality. | Same as XSD 1.0. |
| Explicit finite `1` | Construct finite `1`; leading `+` and zero padding canonicalize to `1`. | Same as XSD 1.0. |
| Effective `0/0` | Where the representation permits both values, map to no particle; do not publish a zeroed particle. XSD 1.0 `<all>` itself has fixed maximum `1`. | Map to no particle; XSD 1.1 `<all>` permits the `0/0` representation. |
| Arbitrary finite non-negative value, including above `uint64` | Preserve the exact `StrictInteger`; compare numerically without narrowing. | Same as XSD 1.0. |
| `maxOccurs="unbounded"` | Store the max-only unbounded variant; compare no numeric maximum. | Same as XSD 1.0. |
| Omitted minimum with finite maximum `0` | Effective `1/0`; invalid because minimum exceeds maximum and a completed finite particle cannot have maximum zero. | Effective `1/0`; invalid because minimum exceeds maximum; an actual particle maximum is positive. |
| Finite minimum greater than finite maximum | Invalid `Particle Correct`; retain both located bound inputs in the diagnostic. | Same as XSD 1.0. |
| Malformed lexical value such as `maybe`, `1.0`, or empty | Invalid `nonNegativeInteger`/`allNNI`; report at the attribute location and preserve the lexical cause. | Same as XSD 1.0. |
| Negative value such as `-1` | Invalid non-negative value; negative zero denotes exact zero and is accepted by the datatype mapping. | Same as XSD 1.0. |
| `unbounded` in `minOccurs` or another attribute | Invalid lexical/value for that attribute; only a maximum may use the keyword. | Same as XSD 1.0. |

Finite comparison enforces `min <= max`; unbounded maxima bypass numeric
sentinels. Apply `0/0` after effective defaults and before public allocation.

### Edition-specific `all` restrictions

| Edition | Applicable restrictions |
| --- | --- |
| XSD 1.0 | XSD 1.0 all members are element particles with minOccurs 0 or 1 and fixed maxOccurs 1; the current parser validates these restrictions and leaves explicit occurrence syntax unsupported. |
| XSD 1.1 | An `all` model group has `minOccurs` and `maxOccurs` each in `0/1`. It has the permitted model-group-definition/content-type placements, and an `all` term may also occur as a `1/1` particle inside an `all` group. Its member terms that are model groups must themselves be `all`; a group-reference member is fixed at `1/1`. Element and wildcard members use the exact general occurrence model. The XML representation permits element, wildcard, and group children. |

These are constraints on future component construction, not a claim that the
current parser supports all particles or their repetition semantics.

## Representation and phase boundaries

The private kernel in `particle_occurrence.go` is the first durable phase
boundary:

1. Syntax collection keeps lexical presence and source locations only long
   enough to apply the omitted-value default and detect duplicate attributes.
2. Lexical conversion uses `ParseStrictInteger`, rejects negative values, and
   constructs a tagged finite or max-only unbounded bound. The range
   constructor owns copies and rejects an unbounded minimum or finite
   `min > max`.
3. After applicable syntax, occurrence, reference, and policy gates, mapping
   applies exact `0/0` absence to sequence, choice, and child occurrences;
   `mapsToParticle` derives from bounds, not an `absent` flag.
   Graph-wide declaration/facet failures and policy errors prevent construction;
   non-reference named/inline type mapping is not universal before local
   public-particle omission. Local public particle alone is omitted.
4. The completed schema phase copies the range into an immutable public
   occurrence view. Its minimum is an owned `StrictInteger`; its maximum is a
   tagged finite or unbounded value. Queries clone exact finite values at the
   ownership boundary.
5. Validator and code-generator plans consume exact bounds on demand; they do
   not cache derived repetition programs in the schema.

### Current-state matrix

Matrix is authoritative for all surfaces after occurrence parsing; it complements
normative tables without broadening edition limits.

Bounded attribute-free extensions describe derivation/base shape, not occurrence
limits; direct and supported extension choices/sequences retain exact finite,
`unbounded`, and above-`uint64` values.

| Surface | Current behavior |
| --- | --- |
| Query model | Complexes expose sequence/choice Boolean/integer/decimal/token/NMTOKEN and anonymous Boolean/integer/decimal/negativeInteger. Bounded attribute-free complexContent extensions use named empty-content bases or named complexContent restrictions over built-in `xs:anyType`, retaining facts/locations and representable wildcards; model-less extensions retain base identity. All policies admit `integer`, named-effective or anonymous-inline `negativeInteger`, and explicitly typed built-in/supported named-effective `unsignedLong` only in direct/permitted extension choices/sequences. Retain written base QName/base `Loc`, exact inclusive/exclusive bounds, integer enumeration/digit facets, type/facet/use-site `Loc`s, named/built-in IDs, ownership/provenance, and exact occurrences; owner/child `0/0` is absent after applicable gates. Mapped non-`0/0` exclusions (direct built-in `negativeInteger`; effective `int`/`long`/`nonNegativeInteger`/`nonPositiveInteger`; list/union; narrower built-in atomic kinds) return `FeatureSchemaSyntax`/`FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at type/facet/element `Loc`; mapped non-`0/0` local inline/anonymous `unsignedLong` forms return located schema-syntax `FailureUnsupported` at type/`simpleType` `Loc`; no `Schema`; nested particles use the nested-particle `Loc`. Inline/anonymous `unsignedLong` is a separate scalar boundary; admitted local built-in/named-effective `unsignedLong` is query-only and consumers reject. Element-reference and top-level model-group refs are separate queryable boundaries after applicable resolution/visibility/wrong-kind/ambiguity/occurrence/policy gates; named-group paths alone duplicate-check; retain QName/RefLoc/TargetID/order/occurrences without target expansion. Nested/local/recursive/broader reference/group forms unsupported or consumer-excluded. Global long/unsignedLong/int/identity query-only; `nonNegativeInteger` `GenerateGo`-only; validation unsupported; refs query, direct consumers reject. Only global inline string/token/NMTOKEN elements generate; other inline/local-attribute consumers excluded. |
| Attribute uses and scalar `simpleContent` | Particle-plus-use/direct model-group and attribute-only bodies expose ordered local, referenced, and anonymous-inline `AttributeUse` facts. Local uses retain scoped name/type/declaration `Loc`s and named type identity or anonymous `AnonymousID`/`NodeID` ownership; references retain QName/RefLoc/TargetID/use. Optional/required are effective and prohibited uses are omitted. Forms select names; XSD 1.1 local `targetNamespace` must match the containing target, missing/mismatch is invalid, Strict10 is an edition mismatch, and chameleon includes adopt. Particle scalar widening does not widen AttributeUse: it remains Boolean/integer/decimal plus policy-gated `precisionDecimal`; unsignedLong remains schema-unsupported. Scalar simpleContent separately supports string/Boolean/integer/decimal plus policy-gated `precisionDecimal`, retains base/type/use `Loc`s and nil particle, and keeps unsignedLong unsupported. Restrictions and attribute-bearing/`attributeGroup` extensions are unsupported. Value/default/fixed/inheritable semantics and attribute consumers remain unsupported; excluded references preserve use-site/target locations and return no partial schema. |
| Global attribute constraints | Under all policies, global attributes admit built-in or supported named atomic `xs:boolean`, `xs:integer`, `xs:decimal`, `xs:token`, `xs:negativeInteger`, `xs:language`, `xs:NCName`, `xs:anyURI`, and `xs:ID`, plus built-in/named `xs:long` and `xs:unsignedLong` restrictions. Built-in/supported named `xs:precisionDecimal` is query-only in Compatibility/Strict11; Strict10 rejects at type `Loc` with `FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`. Excluded type refs report `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at use-site `Loc`; local/inline attributes report it at element/`simpleType` `Loc`. Invalid/policy/resolution/reference precedence remains; errors return no `Schema`. Value constraints support Boolean/integer/decimal/token/precisionDecimal. Unsupported default/fixed values use `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc`; invalid supported values use `FailureInvalid`/`XSD3036` at value `Loc` with lexical/facet cause; default+fixed uses `FailureInvalid`/`XSD3010` with fixed primary/default related. Built-in long bounds are intrinsic inclusive `[-9223372036854775808,9223372036854775807]`/`[0,18446744073709551615]`; named refs retain exact narrowed/exclusive facets, locations, provenance, ownership, and target IDs. Attributes are query-only; consumers reject; `GenerateGo` rejects every `ComponentKindAttributeDeclaration`; occurrence N/A. |
| Validation | `ValidateInstance` consumes complexes with local built-in/named Boolean, integer/decimal, or homogeneous token/NMTOKEN sequences, honoring exact ranges/enumeration. `precisionDecimal` roots validate under Compatibility/Strict11. `xs:int`/long-family/`nonNegativeInteger` roots return located unsupported (`FailureUnsupported`/`XSD4004`/`ErrUnsupported`); bounds/facets remain unvalidated. Default choices support Boolean/token/NMTOKEN alternatives, integer/decimal mixtures, and local `precisionDecimal`; repetition/non-default alternatives reject. Extension, anonymous, mixed-family, nested, recursive, broader, and anonymous-target consumers remain excluded. Homogeneous local NMTOKEN sequences retain exact finite/unbounded/above-`uint64` occurrences under all policies. Only non-extension default-occurrence refs to global built-in/named Boolean/integer/decimal are supported. |
| `precisionDecimal` | Built-in/named roots validate under Compatibility/Strict11; inline/anonymous roots query-only/consumer-rejected. Strict10 rejects before validation/`0/0`. Local built-in/named-effective targets validate in non-extension default choices under Compatibility/Strict11; direct sequences and bounded extension choices are query-only. Local inline/anonymous targets and non-default choices reject. Admitting policies omit permitted `0/0`. `GenerateGo` rejects every target. |
| Generation | Global/named Boolean, integer, decimal, string, token, NMTOKEN, and `nonNegativeInteger` components/elements generate, as do inline global string/token/NMTOKEN elements. Elements require `abstract=false,nillable=false`; otherwise `FailureUnsupported`/`GOXSD9029`, nil. Built-in fields use `StrictInteger`; named fields use generated types. Canonical built-ins require integer kind/version, `fractionDigits=0`, `minInclusive=0`, and no other bounds; named facets remain, but final/variety/effective-facet gates reject (`GOXSD9029`, nil), while malformed/stale facts fail (`GOXSD9030`, nil). Local nonzero `nonNegativeInteger` has no schema; `0/0` is absent; refs query but direct consumers reject. Only default direct-choice Boolean/integer/decimal refs are eligible. Local generation excludes unsignedLong, precisionDecimal, token/NMTOKEN, anonymous, repeated, and non-default forms. Global long/unsignedLong and all attributes remain query-only; validation/`GenerateGo` reject. |
| References | Element-reference particles in local content/named groups are queryable immutable facts. Resolution retains QName, `RefLoc`, `TargetID`, lexical order, and exact occurrences without target gating. Global `nonNegativeInteger` refs remain queryable; direct-choice/sequence consumers reject them with located unsupported diagnostics and nil output. Only non-extension default-occurrence direct-choice refs to global built-in/named Boolean/integer/decimal targets are eligible. Sequence, anonymous-target, repetition, nested, recursive, broader, and mixed forms are consumer exclusions; query references retain facts. Model-group refs are a separate top-level direct query boundary: supported named-complex and bounded attribute-free-extension refs retain ordered facts/`TargetID` without expansion; nested/local/recursive/broader refs remain unsupported. |
| Diagnostics and extensions | Applicable syntax/occurrence/element-reference/policy gates precede mapping; named-group paths alone duplicate-check. Extension/model-less gates first; codegen extension-primary; validation owner/sequence-primary; omit anonymous locations. Direct-sequence `0/0` owners omit before child resolution; direct choices no duplicate-check and resolve child refs before child omission; named groups resolve/check duplicate refs before owner/child `0/0` omission. Zero owners/children absent; direct-sequence zero owners skip child mapping. Direct checks use element/particle `Loc`s plus anonymous related facts; scalar mapping exclusions use type/facet/element `Loc`s and nested exclusions use nested-particle `Loc`. Model-group refs use group `RefLoc` primary; validation relates particle; generation retains group/component/reference/target locations. `xs:any` facts retain ordered namespace/process values: nonzero facts remain queryable, wildcard consumers reject, broader forms unsupported; `anyAttribute` is separate and consumers reject. Reference/malformed-occurrence/policy/unsupported diagnostics return no partial `Schema`/`GenerateGo`. |

## Public API migration

`Particle` exposes `Occurrences()`, `MinOccurs() uint64`, and
`MaxOccurs() uint64`. `Occurrences()` is the exact public occurrence view. The
two `uint64` methods remain only as a default-only compatibility surface, not a
representation of arbitrary schema values. Every concrete particle returns
`1` from both methods only for an exact default `1/1` range and returns `0` for
a non-default range; callers that need an exact value must use the occurrence
view. The migration boundary is:

1. Keep exact values in `particleOccurrenceRange` through syntax, resolution,
   and completed facts. Do not make an above-`uint64` or unbounded value look
   like a capped integer or a `uint64` wraparound.
2. Expose non-default sequence particles only through the documented exact
   view. `Particles()` returns the owned ordered copy of completed `Particle`
   facts; `Elements()` remains a separate element-only collection.
3. Keep the deprecated `uint64` methods during the compatibility window for
   existing default-only callers. There is no lossless compatibility adapter
   for arbitrary integers or `unbounded`; callers must migrate to the exact
   view before relying on non-default particles.

## Consumer policy and diagnostics

Consumers that materialize a native bound do so only after exact comparison
with an explicit configured limit. For those consumers, an above-limit finite
value, an unbounded value, or a multiplication that exceeds a resource budget
produces an explicit located unsupported or resource diagnostic with its feature
and specification reference. The direct scalar sequence validator consumes exact
outer and child ranges on demand, including unbounded and above-`uint64` values,
without narrowing. No consumer truncates, saturates, uses a sentinel, or converts
through floating point. Direct-choice repetition validation and non-default
repeated-field emission remain disabled until their consumers have such a policy.

Malformed and negative lexicals are invalid input at their source attribute;
the stable schema-composition diagnostic preserves the underlying lexical or
negative-value cause and carries the edition-specific
`xsd10-datatypes#nonNegativeInteger` or
`xsd11-datatypes#nonNegativeInteger` reference. `unbounded` in `minOccurs` is
invalid at the source attribute and carries the corresponding
`xsd10-structures#p-min_occurs` or `xsd11-structures#p-min_occurs` reference.
A finite ordering error is invalid input at the particle location, retains
explicit bound locations, and carries the corresponding
`xsd10-structures#coss-particle` or `xsd11-structures#coss-particle` reference.
Duplicate XML attributes remain syntax errors with the existing `XSD3001`
behavior. An error-level diagnostic returns no schema.

## Non-goals, risks, and follow-up

Global attributes are outside particle-occurrence materialization; local attributes/attribute
groups and validation/`GenerateGo` consumers remain separate boundaries. Omitted direct
`anyAttribute` is `##any`/`strict`; `##any`/`##other` are supported; positive namespace
lists require strict. `##local` and target-namespace markers without a target are absent;
effective values are sorted, unique copies with normalized lexical/source locations.
Local/inline complex/list/union, Boolean facets, nested/broader particles/groups, `all`
mapping, and broader wildcard/attribute forms remain unsupported.
Exact occurrences have no fixed resource limit; later phases must bound
input/materialization.

Risks are hostile-lexical memory use, delayed exact-accessor API breakage, and
leaking semantic `0/0` as a public zero component. Range-constructor, ownership,
and mapping tests guard the latter two; future resource policy must guard the first.

Exact occurrence accessors and temporary `uint64` methods belong to the schema
API. Mapping, including `0/0` absence, belongs to construction; bounded
materialization/repetition to validation; bounded emission to code generation.
These responsibilities preserve the phase and edition-specific `all` rules.
