# 0007: Exact particle occurrence ranges

Status: accepted

## Decision

Particle bounds are exact non-negative arbitrary-precision `StrictInteger` values
or distinct `unbounded`, allowed only for a maximum. The value model has no
sentinel, fixed-width conversion, floating point, duplicate flag, or nullable
completed state. A completed range has a finite minimum and finite/unbounded
maximum; a finite maximum requires minimum <= maximum.

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

The table describes value and mapping for both editions. Entries mapping to no
component are not public particles with zeroed fields. Edition-specific `all`
restrictions follow.

| Input or condition | XSD 1.0 | XSD 1.1 |
| --- | --- | --- |
| Both attributes omitted | Effective `1/1`; construct finite `1/1`. | Effective `1/1`; construct finite `1/1`. |
| `minOccurs="0"`, maximum omitted | Effective `0/1`; preserve exact zero and optionality. | Effective `0/1`; preserve exact zero and optionality. |
| Explicit finite `1` | Construct finite `1`; leading `+` and zero padding canonicalize to `1`. | Construct finite `1`; leading `+` and zero padding canonicalize to `1`. |
| Effective `0/0` | Where the representation permits both values, map to no particle; do not publish a zeroed particle. XSD 1.0 `<all>` itself has fixed maximum `1`. | Map to no particle; XSD 1.1 `<all>` permits the `0/0` representation. |
| Arbitrary finite non-negative value, including above `uint64` | Preserve the exact `StrictInteger`; compare numerically without narrowing. | Preserve the exact `StrictInteger`; compare numerically without narrowing. |
| `maxOccurs="unbounded"` | Store the max-only unbounded variant; compare no numeric maximum. | Store the max-only unbounded variant; compare no numeric maximum. |
| Omitted minimum with finite maximum `0` | Effective `1/0`; invalid because minimum exceeds maximum and a completed finite particle cannot have maximum zero. | Effective `1/0`; invalid because minimum exceeds maximum; an actual particle maximum is positive. |
| Finite minimum greater than finite maximum | Invalid `Particle Correct`; retain both located bound inputs in the diagnostic. | Invalid `Particle Correct`; retain both located bound inputs in the diagnostic. |
| Malformed lexical value such as `maybe`, `1.0`, or empty | Invalid `nonNegativeInteger`/`allNNI`; report at the attribute location and preserve the lexical cause. | Invalid `nonNegativeInteger`/`allNNI`; report at the attribute location and preserve the lexical cause. |
| Negative value such as `-1` | Invalid non-negative value; negative zero denotes exact zero and is accepted by the datatype mapping. | Invalid non-negative value; negative zero denotes exact zero and is accepted by the datatype mapping. |
| `unbounded` in `minOccurs` or another attribute | Invalid lexical/value for that attribute; only a maximum may use the keyword. | Invalid lexical/value for that attribute; only a maximum may use the keyword. |

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
3. The particle mapping phase applies the shared exact `0/0` absence rule to
   sequence, choice, and child occurrences. It derives `mapsToParticle` from
   exact `0/0`; it does not store an `absent` flag alongside the two bounds.
4. The completed schema phase copies the range into an immutable public
   occurrence view. Its minimum is an owned `StrictInteger`; its maximum is a
   tagged finite or unbounded value. Queries clone exact finite values at the
   ownership boundary.
5. Validator and code-generator plans consume exact bounds on demand; they do
   not cache derived repetition programs in the schema.

### Current-state matrix

The matrix below is authoritative for current schema, validation, generation,
and reference behavior after exact occurrence parsing. It complements the
normative XSD 1.0/1.1 occurrence and `all` tables above; it does not broaden
their edition limits.

| Surface | Current behavior |
| --- | --- |
| Query model | Named global complexes expose direct sequence/choice built-in/named Boolean/integer/decimal/token/NMTOKEN and anonymous Boolean/integer/decimal elements. Bounded extensions over named empty-content bases retain facts, locations, particles, and inherited wildcard behavior; model-less retain identity/locations. Local built-in or named-effective `precisionDecimal` types are queryable only in admitted default-occurrence direct or bounded attribute-free extension choices under Compatibility/Strict11; owners and mapped alternatives require default occurrences. Mapped inline anonymous `precisionDecimal` and nonzero precisionDecimal sequences are schema-unsupported. For `precisionDecimal`, Compatibility/Strict11 omit effective `0/0`; Strict10 rejects before omission. Anonymous facts retain IDs, QName ownership, facets, locations, and occurrences without `ComponentID`/global-walk ownership. Local declared/named/inline/anonymous integer kinds accept only `integer`/`negativeInteger` through named/forward/imported/included/chameleon chains; `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` reject non-0/0 mappings with located `FailureUnsupported`/`ErrUnsupported` and no schema. Local declared/named/inline/anonymous `nonNegativeInteger` `0/0` forms are admitted then absent under every policy. Global long-family refs retain exact bounds; malformed refs are invalid. Global inline string/token/NMTOKEN, Boolean/integer/decimal, long-family, identity, and `nonNegativeInteger` retain query facts; references to global `nonNegativeInteger` remain queryable without target-type gating, while direct-choice and sequence consumers are unsupported with located diagnostics and nil output; global inline/anonymous `nonNegativeInteger` forms are query-only and consumer-rejected; built-in/named `nonNegativeInteger` follows the Generation row; inline `precisionDecimal` is query-only under Compatibility/Strict11. |
| Global attribute constraints | Compatibility/Strict11 admits global built-in/named-effective precisionDecimal attributes with one default/fixed `AttributeValueConstraint` for query. `AttributeDeclaration.ValueConstraint()` copies kind, collapsed `Lexical`, source `Loc`, and exact defensive `StrictPrecisionDecimal` via `PrecisionDecimalValue()`; Strict10 rejects at type `Loc` before conversion. Inline/local attributes and validation/`GenerateGo` remain unsupported; invalid values return no schema with the existing diagnostic. |
| Validation | `ValidateInstance` consumes named global complexes with local built-in/named Boolean-only or integer/decimal sequences, honoring exact finite/unbounded/above-`uint64` ranges under Compatibility/Strict10/Strict11; homogeneous local built-in/named token sequences honor occurrences and validate token enumeration. Built-in/named `precisionDecimal` roots validate under Compatibility/Strict11. Built-in/named `nonNegativeInteger` roots unsupported under all policies: returns located `FailureUnsupported`/`XSD4004`/`ErrUnsupported`; schema-owned bounds/facets unvalidated. Global inline/anonymous `nonNegativeInteger` retains schema/query facts; consumer rejects it (`FailureUnsupported`/`XSD4004`/`ErrUnsupported`). Default choices validate Boolean/token/NMTOKEN alternatives and integer/decimal mixtures; local built-in/named-effective `precisionDecimal` eligible only there. Extensions and anonymous consumers, NMTOKEN sequences, mixed-family choices/sequences, choice repetition, and non-default/repeated refs unsupported. Default direct-choice refs to built-in/named Boolean/integer/decimal are supported; sequence/repetition/non-default/nested/recursive/broader/mixed/anonymous-target refs are excluded. |
| `precisionDecimal` | Global built-in/named/inline schema/query admission and built-in/named-root validation are Compatibility/Strict11-only; Strict10 rejects before validation with a policy diagnostic. Global inline is queryable, but anonymous targets are rejected by consumers. Local built-in/named-effective refs require default-occurrence direct choices or bounded attribute-free extension choices; mapped inline anonymous restrictions, non-default choices/alternatives, and nonzero direct sequences reject. Strict10 precedes `precisionDecimal` `0/0` omission; Compatibility/Strict11 omits it. Only non-extension default choices validate; `GenerateGo` rejects every global, local, inline, anonymous, and extension target. |
| Generation | Bounded local Boolean/integer/decimal sequences emit fields; mixed Boolean/numeric and repeated/non-default forms are unsupported. `GenerateGo` supports global built-in/named-typed `nonNegativeInteger` element declarations and standalone named atomic `nonNegativeInteger` simple-type components. Only element declarations use ordinary (`abstract=false,nillable=false`); either is unsupported (`FailureUnsupported`/`GOXSD9029`, nil). Built-in element fields and standalone named `nonNegativeInteger` declarations use `StrictInteger`; named-typed element fields use generated named types. Canonical built-in facts require integer kind/version, fixed `fractionDigits=0`, no `totalDigits`, `minInclusive=0`, and no other bounds. Named restrictions retain schema bounds/facets; named final, atomic-restriction-variety, and effective-facet gates reject unsupported forms (`FailureUnsupported`/`GOXSD9029`, no output), while malformed/stale facts fail closed (`FailureInternal`/`GOXSD9030`, nil). Global inline/anonymous `nonNegativeInteger` forms are query-only; consumers reject them. Local declared, named, inline, and anonymous non-0/0 `nonNegativeInteger` forms have no schema; ordinary local 0/0 forms are admitted then absent under every policy. References to global `nonNegativeInteger` remain queryable without target-type gating; direct-choice/sequence consumers are unsupported with nil output. Only default direct-choice refs to global built-in/named Boolean/integer/decimal are eligible; broader refs reject. Global built-in/named Boolean/integer/decimal/string/token/NMTOKEN components generate, and global inline generation is limited to string/token/NMTOKEN; inline Boolean/integer/decimal consumers remain query-only and rejected. Global/local `precisionDecimal` facts query under Compatibility/Strict11, but `GenerateGo` rejects every target; Strict10 rejects global inline by policy. Other supported scalar/direct-choice forms generate. |
| References | Element-reference particles in local content and named groups are queryable immutable facts. Resolution retains the expanded QName, `RefLoc`, `TargetID`, lexical order, and exact occurrences without target-type gating. References to global `nonNegativeInteger` remain queryable; direct-choice and sequence consumers reject them with located unsupported diagnostics and nil output. Only non-extension default-occurrence direct-choice references to built-in or named global Boolean/integer/decimal targets are consumer-eligible. Sequence, anonymous-target, repetition, nested, recursive, broader, and mixed element-reference forms are consumer exclusions; query model references retain their resolved facts. Model-group references are a separate top-level direct query boundary: supported named-complex and bounded attribute-free-extension references retain ordered facts and `TargetID` without expansion, while nested/local/recursive/broader model-group references remain unsupported. |
| Diagnostics and extensions | Ordinary direct checks use element/particle locations and may relate anonymous locations. Complex-content/model-less extension gates run first; codegen uses extension-primary facts, validation uses owner or sequence-instance primary, and no validation diagnostic adds an anonymous location. Direct and extension model-group references use group `RefLoc` primary, relate the group particle for validation, and retain group/component/reference/target locations for generation. Supported `xs:any` and `anyAttribute` forms retain ordered namespace/process query facts; effective `0/0` is a particle-occurrence rule, while broader forms and wildcard consumers remain unsupported; unsupported gates return no `GenerateGo` output. |

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

The matrix is the current-state inventory. Other constraints: omitted direct
`anyAttribute` defaults to `##any`/`strict`; `##any`/`##other` are supported, while
positive namespace enumerations require strict processing. `##local` and a
target-namespace marker without a target are absent; effective values are sorted,
unique, copied, and retain normalized lexical/source locations. Local/inline
complex/list/union and remaining local/inline value/default/fixed/attribute forms,
unsupported Boolean facets, nested/broader particles/groups, `all` mapping, and
broader wildcard/attribute forms remain unsupported. Global attribute value
constraints are query-only; validation/generation consumers do not consume them.
Exact occurrences have no fixed resource limit; later phases must bound
input/materialization.

Risks are hostile-lexical memory use, delayed exact-accessor API breakage, and
leaking semantic `0/0` as a public zero component. Range-constructor, ownership,
and mapping tests guard the latter two; future resource policy must guard the first.

Exact occurrence accessors and temporary `uint64` methods belong to the schema
API. Mapping, including `0/0` absence, belongs to construction; bounded
materialization/repetition to validation; bounded emission to code generation.
These responsibilities preserve the phase and edition-specific `all` rules.
