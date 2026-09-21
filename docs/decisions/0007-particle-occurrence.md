# 0007: Exact particle occurrence ranges

Status: accepted

## Decision

Particle occurrence bounds are exact, finite, non-negative integer values, or
the distinct `unbounded` variant allowed only for a maximum. The value model
owns arbitrary-precision `StrictInteger` values. It has no sentinel, fixed-
width conversion, floating-point value, duplicate unbounded flag, or nullable
completed state. A completed range always has a finite minimum and a finite or
unbounded maximum; a finite maximum is accepted only when minimum is less than
or equal to maximum.

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

Only finite comparison is used for the `min <= max` rule. An
unbounded maximum satisfies the range boundary without comparing a numeric
sentinel. The `0/0` mapping is applied after effective defaults and before a
public component is allocated.

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

The preflight applies exact lexical ranges. Named global complexes expose direct sequence/choice local built-in or named Boolean/integer/decimal/token/NMTOKEN elements and local anonymous atomic Boolean/integer/decimal restrictions; bounded attribute-free extensions expose the same facts. Anonymous facts are immutable query facts (`SimpleTypeID`/`NodeID`, no `ComponentID`/global-walk ownership, written base QName plus resolved named ownership, facets/locations/exact occurrences); `0/0` is absent and non-default ranges are query-only. Effective local anonymous integer kind accepts exactly `integer`/`negativeInteger` through named/forward/imported/included/chameleon chains; `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` are valid but unsupported, with a located `FailureUnsupported`/`ErrUnsupported` at the type/facet `Loc` and no schema. Supported anonymous Boolean/integer/decimal facets remain queryable, but non-string anonymous enumeration is likewise explicit unsupported at its facet location. Direct choice/sequence checks may relate anonymous locations.

Ordinary direct choice/sequence consumer checks use element/particle locations and may relate anonymous type locations. Complex-content/model-less extension gates reject first at the boundary: codegen uses extension-primary and complex-content/extension/base/particle (optional `anyAttribute`) related locations; validation uses owner or sequence-instance primary, never an anonymous location; generation returns no output. Direct and extension model-group refs use group `RefLoc` primary; validation relates the group particle, while generation relates group/component/reference/target. `xs:any` supported forms map ordered wildcard facts; broader forms/consumers are unsupported. Named groups expose ordered refs/ranges; top-level refs retain target IDs.

Exact occurrences remain in choice facts. `ValidateInstance` supports named global homogeneous Boolean/numeric sequences with exact ranges; direct-choice repetition is unsupported. Bounded attribute-free extensions over named empty bases retain identities/locations, particles, and inherited `##other`/lax facts; model-less extensions retain no particle/occurrence/content. Local token/NMTOKEN facts remain; default all-token/NMTOKEN choices validate, but their sequences, mixed families, and generation remain unsupported.

Default-bounded direct integer/decimal or all-Boolean sequence children emit ordered fields; mixed and repeated-field generation remain unsupported. XSD 1.1 default-occurrence choices may use `precisionDecimal` only with default choice/alternatives; non-precision alternatives may retain non-default query ranges. Other `precisionDecimal` particle ranges are unsupported. The local anonymous boundary is restriction particles, not all inline forms: local complex/list/union/string/token/NMTOKEN/precisionDecimal and value/default/fixed/attribute/nested/reference/broader forms remain outside the model. Global anonymous string/token/NMTOKEN remain supported; global anonymous `precisionDecimal` is Compatibility/Strict11-only and Strict10-rejected. Explicit typed local token/NMTOKEN remain queryable; exclusions are consumer-only.

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

Currently, the boundary supports named global complexes with direct sequence/choice
local built-in or named Boolean/integer/decimal/token/NMTOKEN and local anonymous
atomic Boolean/integer/decimal restrictions; named groups/direct element refs;
and top-level model-group refs or bounded attribute-free XSD 1.0/1.1 extensions.
The anonymous model, effective integer allowlist, facet subset, and consumer
location rules are above. Direct refs retain target IDs; extensions retain exact
ranges/base identities/locations; model-less extensions retain no
particle/occurrence/content. Wildcards and `0/0` follow above.
`ValidateInstance`/`GenerateGo` reject anonymous inline atomic refs with located
`FailureUnsupported`/`ErrUnsupported`; `GenerateGo` returns no output.
For instance validation, named global complex homogeneous Boolean/numeric sequences
match expanded names in lexical declaration order and honor exact finite, unbounded, and above-`uint64`
outer and child ranges under `Compatibility`, `Strict10`, and `Strict11`; direct-choice validation
remains limited to default occurrences; excluded particle and target shapes remain unsupported.
Direct choices may also include XSD 1.1 `precisionDecimal` elements only when
the choice and each mapped `precisionDecimal` alternative use default
occurrences. Non-precision alternatives may retain non-default ranges for
queries. Non-`0/0` direct-sequence `precisionDecimal` ranges that map to a
particle remain unsupported even under XSD 1.1 and Compatibility. An effective
`0/0` sequence, choice, or child maps to absence before type-specific support
gating. Supported direct-choice occurrence attributes and non-`0/0`
alternative ranges are parsed and queryable, but direct-choice repetition is not
implemented in validation, and effective total ranges are not calculated. Non-default
direct sequence occurrences are not generated as repeated fields. Non-default
`precisionDecimal` choice and alternative ranges that map to a particle are
schema-unsupported. The supported anonymous Boolean/integer/decimal restriction
facet subset remains queryable, but non-string anonymous enumeration remains
explicit unsupported with its existing diagnostic and no partial schema. Boolean
facet forms outside that subset and local inline complex, list/union, local
anonymous string/token/NMTOKEN/precisionDecimal restrictions, local value/default/fixed/
attribute constraints, nested, anonymous-reference, or broader particles, including nested
choices and `all`; nested, local, recursive, or broader group shapes and broader
wildcard/attribute remain unsupported. Direct named-complex/bounded-extension
group refs remain supported facts; anonymous simple-type models and resolved
built-in, named, and supported anonymous simple-type references are modeled as
immutable schema-query facts, not consumer targets. Named direct sequence/choice types expose direct
`anyAttribute`: omitted attributes default to `##any`/`strict`;
`##any`/`##other` supported. Positive namespace enumerations
(`##local`, `##targetNamespace`, URI lists) allow only strict `processContents`
(omitted/explicit). Markers use the owner's effective schema namespace after
graph composition: `##local` is absent; no-target `##targetNamespace` is absent.
Values are sorted, unique, copied. `anyAttribute` location, normalized lexical form, and
namespace/processContents locations remain; omitted locations are zero.
Anonymous local restriction references remain query-only; direct choice/sequence
diagnostics may include their location. The extension location rules above apply
before target inspection. Direct
element-reference particles support local choice/sequence children and global
named-group direct choices/sequences; direct model-group refs are top-level only
for named complexes or bounded attribute-free extensions, retain target IDs
without expansion, and exclude nested refs. Validation consumes named global
homogeneous Boolean/numeric sequences and default-occurrence built-in/named
scalar choices or global Boolean/integer/decimal refs; anonymous inline atomic
refs remain query-only and direct checks reject them with located
`FailureUnsupported`/`ErrUnsupported`, possibly including the anonymous
location. The extension rules above apply before target inspection; generation
returns no output. Global text-only Boolean validation works under all policies;
Boolean scalar generation works. Direct choices support default local Boolean or
all-token/NMTOKEN; token/NMTOKEN sequences, mixed families, and local generation
remain unsupported. All-Boolean choices and default-bounded sequences generate;
the parser does not map `all`. The exact value has no fixed resource limit;
later phases must set bounded input and materialization policies.

The main risks are memory proportional to hostile finite lexicals, a breaking
API migration if exact accessors are delayed, and accidentally treating the
semantic `0/0` absence as a public zero-valued component. The range
constructor, ownership tests, and mapping proof guard the latter two; future
resource policy must guard the first.

The exact occurrence accessors and the temporary `uint64` compatibility methods
belong to the schema API boundary. Schema mapping, including `0/0` absence,
belongs to component construction; bounded materialization and repetition
belong to validation; bounded direct-particle emission belongs to code generation.
These responsibilities preserve the phase boundaries and edition-specific
`all` rules recorded here.
