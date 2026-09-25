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

Matrix records parsed behavior without broadening edition limits.

| Surface | Current behavior |
| --- | --- |
| Query model | Named complexes expose sequence/choice Boolean/integer/decimal/token/NMTOKEN and anonymous Boolean/integer/decimal. Bounded/model-less extensions over named empty-content bases retain locations, identity, wildcards. Local mapped built-in/named-effective `precisionDecimal` is queryable only in default-occurrence choices or bounded attr-free extension choices (Compatibility/Strict11); mapped non-default/nonzero choices/sequences/inline/anonymous are schema-unsupported; non-precision alternatives may remain query-only. Admitted extension choices are queryable but consumers reject. Exact local `0/0` omits before mapping after syntax/occurrence and applicable reference/policy gates; Strict10 `XSD3030` first. Local `negativeInteger` admits integer plus named/inline-effective forms only when mapped non-`0/0`; direct built-in rejects then; named/inline facts query-only, consumers reject. Other mapped integer-derived forms reject. Global long/unsignedLong element/type facts queryable all policies: built-in refs intrinsic inclusive bounds `[-9223372036854775808,9223372036854775807]`/`[0,18446744073709551615]`; named/inline restrictions exact effective narrowed/exclusive facets, locations, provenance, ownership; list/union declarations reject. Built-ins have no normal TypeID/ComponentID; named/anonymous ownership remains. String/token/NMTOKEN elements generate; attributes separate query-only. |
| Attribute uses and scalar `simpleContent` | Particle-plus-use/direct model-group references and attribute-only expose ordered `AttributeUse`; local/ref allow Boolean/integer/decimal plus policy-gated `precisionDecimal`; `xs:int`/other scalars unsupported. Scalar simpleContent retains base/type/use `Loc`s, nil particle; bases allow Boolean/string/integer/decimal plus policy-gated `precisionDecimal`. Refs retain QName/RefLoc/TargetID/use; optional/required effective, prohibited omitted. `form`/`attributeFormDefault` select names; XSD 1.1 local `targetNamespace` must match containing target, missing/mismatch invalid; Strict10 mismatch; chameleon adopts. Local anonymous atomics retain `AnonymousID`/`NodeID`; views copied. Unsupported local named/typeless uses declaration `Loc`, inline `simpleType` `Loc`, global refs `RefLoc` + target. Unresolved/wrong-kind/ambiguous/inaccessible refs invalid with related locations; no schema. Consumers/value/default/fixed/inheritable unsupported; `attributeGroup`/complexContent-extension unsupported. |
| Global attribute constraints | Global attributes admit under all policies: named atomic `xs:boolean`, `xs:integer`, `xs:decimal`, `xs:token`, `xs:negativeInteger`, `xs:language`, `xs:NCName`, `xs:anyURI`, `xs:ID`, `xs:long`, and `xs:unsignedLong`. Built-in/named `xs:precisionDecimal` is query-only in Compatibility/Strict11; Strict10 rejects at type `Loc` with `FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`. Excluded types report `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at type-attribute `Loc`; local named/typeless uses there, inline at `simpleType`, refs at `RefLoc` + target. Precedence applies; errors return no `Schema`. Value constraints support Boolean/integer/decimal/token/precisionDecimal. Unsupported defaults/fixed report `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc`; invalid values report `FailureInvalid`/`XSD3036`; both return no `Schema`. Default+fixed is `FailureInvalid`/`XSD3010`, fixed primary, default related. Built-in long bounds are intrinsic inclusive `[-9223372036854775808,9223372036854775807]`/`[0,18446744073709551615]`; named restrictions retain exact effective narrowed/exclusive facets, type/ref locations, provenance, and ownership. Constraints retain at most one default/fixed; `ValueConstraint()` copies kind, collapsed lexical/source `Loc`, and `StrictPrecisionDecimal`. Attributes query-only; consumers reject; `GenerateGo` rejects every `ComponentKindAttributeDeclaration`; occurrence N/A. |
| Validation | `ValidateInstance` consumes global complexes with local built-in/named Boolean/integer/decimal or homogeneous token/NMTOKEN sequences, honoring ranges/enumeration. Built-in/named `precisionDecimal` roots validate under Compatibility/Strict11; global built-in/named `xs:int`/long-family/`nonNegativeInteger`/`negativeInteger`/`nonPositiveInteger` roots return `FailureUnsupported`/`XSD4004`/`ErrUnsupported`. Choices support Boolean/token/NMTOKEN alternatives, integer/decimal mixtures, and eligible local `precisionDecimal`; repetition/non-default/extension/anonymous/mixed/nested/recursive/broader/anonymous-target forms reject. Local Boolean/integer/decimal sequences retain exact repeated/unbounded/above-`uint64` ranges; homogeneous local NMTOKEN sequences retain exact finite/unbounded/above-`uint64` occurrences. Only non-extension default-occurrence refs to global built-in/named Boolean/integer/decimal are supported. Validated local `0/0` short-circuits before mapping; syntax, occurrence, applicable reference resolution, and policy errors precede, retain diagnostics, and return no schema; no universal type/facet validation. |
| `precisionDecimal` | Global element/type built-in/named/inline facts query only Compatibility/Strict11; built-in/named roots validate, inline rejects; Strict10 `XSD3030` precedes validation. Local mapped built-in/named-effective forms admit direct/default choices or bounded attr-free extension choices. Only mapped precision children/alternatives require default occurrences; non-precision alternatives may remain query-only. Mapped precision non-default choices/sequences and nonzero inline/anonymous forms are `ParseSchema` `FailureUnsupported`/`XSD3003`/no schema; admitted extension choices are queryable but validation/GenerateGo reject. Exact `0/0` terms omit even unsupported nonzero forms; direct default choices validate, GenerateGo rejects every target. |
| Generation | Bounded local Boolean/integer/decimal sequences emit fields; mixed/repeated/non-default unsupported. `GenerateGo` supports global/named `nonNegativeInteger` elements and standalone types when `abstract=false,nillable=false`; either true gives `FailureUnsupported`/`GOXSD9029`, nil. Built-in fields use `StrictInteger`; named fields are generated. Canonical built-ins require integer kind/version, `fractionDigits=0`, `minInclusive=0`, no `totalDigits`/other bounds; named facets remain, effective gates reject `GOXSD9029`, nil, malformed/stale facts fail `GOXSD9030`, nil. Local non-`0/0` forms have no schema; `0/0` is absent. `nonNegativeInteger` refs query; direct-choice/sequence/inline/anonymous consumers reject. Supported global Boolean/integer/decimal/string/token/NMTOKEN types/elements generate; local token/NMTOKEN particles/sequences remain `GenerateGo`-unsupported. Global inline elements generate only string/token/NMTOKEN; inline Boolean/integer/decimal, int, long, unsignedLong, negativeInteger, nonPositiveInteger, and precisionDecimal consumers are query-only/rejected. Global long/unsignedLong/negativeInteger/nonPositiveInteger facts remain query-only; validation returns `XSD4004`, generation `GOXSD9029`; attributes remain query-only and `GenerateGo` rejects every `ComponentKindAttributeDeclaration`. Strict10 rejects global inline precisionDecimal before validation. |
| References | Element-reference particles are immutable facts; resolution retains QName, `RefLoc`, `TargetID`, order, and exact occurrences. Global `nonNegativeInteger` refs remain queryable; direct-choice/sequence consumers reject with located diagnostics/nil output. Only non-extension default-occurrence direct-choice refs to global built-in/named Boolean/integer/decimal targets are eligible. Sequence, anonymous-target, repetition, nested, recursive, broader, and mixed references are consumer exclusions. Model-group refs are a top-level direct query boundary: supported named-complex and bounded attr-free-extension refs retain ordered facts/`TargetID` without expansion; nested/local/recursive/broader refs remain unsupported. |
| Diagnostics and extensions | Ordinary direct checks use element/particle locations and may relate anonymous locations. Complex-content/model-less extension gates run first; codegen uses extension-primary facts, validation owner or sequence-instance primary, and never adds an anonymous location. Direct+`AttributeUse`: first-use `Loc` + related `AttributeUse`/definition; attribute-free direct/extension: group `RefLoc`; validation: group/extension; generation: group/component/reference/target. Direct `xs:any` facts retain ordered namespace/process values: nonzero particles remain queryable, wildcard consumers reject, broader forms unsupported. `anyAttribute` facts are separate and consumers unsupported. Validated local `0/0` omission short-circuits before local declared/named/inline type mapping; syntax (name/block/nillable and type/inline policy checks), occurrence parsing, applicable element-ref QName/resolution, and policy gates precede it; no universal type/facet validation is implied. Choice refs/duplicates precede zero owner/term; sequence-owner `0/0` skips children; child refs precede child zero. Applicable reference/target, malformed-occurrence, and policy errors take precedence, return no schema; gates return no `GenerateGo` output. |

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
