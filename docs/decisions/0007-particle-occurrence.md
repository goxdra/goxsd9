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
3. Component construction applies the shared exact `0/0` absence rule to
   sequence, choice, and child occurrences after input validation. Invalid or
   unresolved inputs retain their diagnostics; only an unsupported mapped form
   may be omitted. It derives `mapsToParticle` from exact `0/0`; it does not
   store an `absent` flag alongside the two bounds.
4. The completed schema phase copies the range into an immutable public
   occurrence view. Its minimum is an owned `StrictInteger`; its maximum is a
   tagged finite or unbounded value. Queries clone exact finite values at the
   ownership boundary.
5. Validator and code-generator plans consume exact bounds on demand; they do
   not cache derived repetition programs in the schema.

### Current-state matrix

The matrix is authoritative for schema, validation, generation, and reference
behavior after occurrence parsing. It complements normative XSD occurrence and
`all` tables; it does not broaden edition limits.

| Surface | Current behavior |
| --- | --- |
| Query model | Named global complexes expose direct sequence/choice Boolean/integer/decimal/token/NMTOKEN and anonymous Boolean/integer/decimal elements; bounded extensions retain facts/locations. Built-in `xs:long` and supported named effective-long local particles in direct choices, sequences, and bounded attribute-free extensions are queryable under all policies, preserving written QName, named ID, exact bounds/facets, source/type/facet locations, namespace/nillable/block facts, and lexical order; built-ins have no `ComponentID`. Inline long, int, unsignedLong, nonNegativeInteger, nonPositiveInteger, lists/unions, nested/broader forms remain unsupported. Effective precisionDecimal retains its existing policy/0/0 boundary; invalid or unresolved inputs remain errors. Global attributes and inline element facts retain their existing query-only/generation boundaries. |
| Global attribute constraints | Under all policies, global attributes admit built-in or supported named atomic `xs:boolean`, `xs:integer`, `xs:decimal`, `xs:token`, `xs:negativeInteger`, `xs:language`, `xs:NCName`, `xs:anyURI`, and `xs:ID`, plus built-in/named `xs:long` and `xs:unsignedLong` restrictions. Built-in/supported named `xs:precisionDecimal` is query-only in Compatibility/Strict11; Strict10 rejects it at type `Loc` with `FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`. Excluded type refs report `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at use-site `Loc`; local/inline attributes report it at element/`simpleType` `Loc`. Invalid/policy/resolution/reference precedence remains; errors return no `Schema`. Value constraints support Boolean/integer/decimal/token/precisionDecimal. Unsupported default/fixed values use `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc` and return no `Schema`; invalid supported values use `FailureInvalid`/`XSD3036` at value `Loc` with lexical/facet cause. A default+fixed declaration uses `FailureInvalid`/`XSD3010`: fixed `Loc` primary, default `Loc` related, no `Schema`. Built-in `xs:long` and `xs:unsignedLong` have inclusive bounds `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`; named refs retain QName/type `Loc`, exact narrowed/exclusive facets, locations, provenance, and target IDs; built-ins have no synthetic `ComponentID`. Constraints retain at most one default/fixed `AttributeValueConstraint`; type-only declarations have none, and `ValueConstraint()` copies kind, collapsed lexical/source `Loc`, and defensive `StrictPrecisionDecimal` via `PrecisionDecimalValue()`. Attributes are query-only; consumers reject; `GenerateGo` rejects every `ComponentKindAttributeDeclaration`; occurrence N/A. |
| Validation | `ValidateInstance` consumes complexes with local built-in/named Boolean, integer/decimal, or homogeneous token/NMTOKEN sequences, honoring exact ranges and token/NMTOKEN enumeration. Modeled local built-in/named-effective `xs:long` particles remain query-only and return located unsupported diagnostics from validation and generation. `precisionDecimal` roots validate under Compatibility/Strict11. `xs:int`/long-family/`nonNegativeInteger` roots return located unsupported (`FailureUnsupported`/`XSD4004`/`ErrUnsupported`); bounds/facets unvalidated; inline/anonymous consumers reject. Default choices support Boolean/token/NMTOKEN alternatives, integer/decimal mixtures, and local `precisionDecimal`; direct-choice repetition and non-default alternatives reject. Extension, anonymous, mixed-family, nested, recursive, broader, and anonymous-target consumers remain excluded. Homogeneous local NMTOKEN sequences retain exact finite/unbounded/above-`uint64` occurrences under all policies. Only non-extension default-occurrence refs to global built-in/named Boolean/integer/decimal are supported. |
| `precisionDecimal` | Global element/type built-in/named/inline facts query only under Compatibility/Strict11; roots validate, Strict10 rejects before validation, and inline-element consumers reject. Local built-in/named-effective refs require default choices or bounded attribute-free extensions; anonymous/non-default/nonzero-sequence mappings reject. Strict10 precedes `0/0` omission; Compatibility/Strict11 omits it. Only non-extension default choices validate; `GenerateGo` rejects all precisionDecimal targets. |
| Generation | Bounded local Boolean/integer/decimal sequences emit fields; mixed Boolean/numeric, repeated/non-default unsupported. Modeled local built-in/named-effective xs:long particles are queryable but GenerateGo rejects them explicitly. `GenerateGo` supports global built-in/named-typed `nonNegativeInteger` elements and standalone named simple-type components. Only elements require `abstract=false,nillable=false`; either true is `FailureUnsupported`/`GOXSD9029`, nil. Built-in fields/standalone named declarations use `StrictInteger`; named-typed fields use generated types. Canonical built-in facts require integer kind/version, fixed `fractionDigits=0`, no `totalDigits`, `minInclusive=0`, or other bounds. Named restrictions retain bounds/facets; final/atomic-restriction-variety/effective-facet gates are `FailureUnsupported`/`GOXSD9029`, no output; malformed/stale facts are `FailureInternal`/`GOXSD9030`, nil. Global inline/anonymous `nonNegativeInteger` element declarations and `xs:int` element declarations are query-only; consumers reject. Local non-0/0 `nonNegativeInteger` forms have no schema; 0/0 forms are admitted then absent under every policy. Global `nonNegativeInteger` refs query without target gating; direct-choice/sequence consumers reject nil. Only default direct-choice refs to global built-in/named Boolean/integer/decimal are eligible; broader reject. Supported global built-in/named Boolean/integer/decimal/string/token/NMTOKEN simple-type components and supported global element declarations generate; local built-in/supported named token/NMTOKEN particles/sequences remain `GenerateGo`-unsupported; global inline-element generation is limited to string/token/NMTOKEN element declarations; inline-element Boolean/integer/decimal consumers are query-only/rejected. Global attribute declarations remain query-only, inline-attribute consumers remain excluded, and `GenerateGo` rejects every `ComponentKindAttributeDeclaration`. Global/local `precisionDecimal` element/type facts query under Compatibility/Strict11; `GenerateGo` rejects every precisionDecimal target; Strict10 rejects global inline-element precisionDecimal before validation. Other supported scalar/direct-choice forms generate. |
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
