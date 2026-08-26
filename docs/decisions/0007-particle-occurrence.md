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

The table describes the value and mapping boundary for both editions. An
entry that maps to no component is not a public particle with zeroed fields.
Edition-specific `all` restrictions follow the table.

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

Finite comparison is the only comparison used for the `min <= max` rule. An
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
3. The future mapping phase derives `mapsToParticle` from exact `0/0`; it does
   not store an `absent` flag alongside the two bounds.
4. Only a later completed schema phase may copy the range into an immutable
   public component. No public occurrence type is added by this decision.
5. Validator and code-generator plans consume exact bounds on demand; they do
   not cache derived repetition programs in the schema.

The current schema preflight uses this exact private range to validate lexical
occurrence input while explicit occurrence syntax remains unsupported. Existing
default-only choice and sequence construction, `Particle` alternatives, and
the `uint64` `MinOccurs`/`MaxOccurs` accessors remain unchanged. Explicit
occurrence values do not reach public particles.

## Public API migration

The current `MinOccurs() uint64` and `MaxOccurs() uint64` methods are a
default-only compatibility surface, not a lossy representation of future
schema values. The migration sequence is:

1. Keep the exact range private while all public particles remain default-only.
   Do not make an above-`uint64` or unbounded value look like zero, a capped
   integer, or a `uint64` wraparound.
2. Add a documented immutable exact occurrence view before exposing any
   non-default public particle. Its minimum is an owned `StrictInteger`; its
   maximum is a complete tagged finite/unbounded value. Migrate schema,
   validator, and generator callers to that view first.
3. Deprecate the two `uint64` methods during the compatibility window, with
   documentation that they apply only to the still-default-only model.
4. Remove those methods in the next breaking API release before publishing
   non-default occurrence values. There is no lossless compatibility adapter
   for arbitrary integers or `unbounded`; callers must migrate to the exact
   view.

## Consumer policy and diagnostics

Future consumers may materialize a native bound only after exact comparison
with an explicit configured limit. An above-limit finite value, an unbounded
value, or a multiplication that exceeds a resource budget produces an
explicit located unsupported or resource diagnostic with its feature and
specification reference. It never truncates, saturates, uses a sentinel, or
converts through floating point. Repetition validation and repeated Go
emission remain disabled until their consumers have such a policy.

Malformed and negative lexicals are invalid input at their source attribute;
the stable schema-composition diagnostic preserves the underlying lexical or
negative-value cause. A finite ordering error is invalid input at the particle
location. An error-level diagnostic returns no schema.

## Non-goals, risks, and follow-up

Currently, explicit occurrence syntax remains unsupported: no new public API is
exposed, default-only choice and sequence behavior is unchanged, repetition is
not implemented in validation, repeated Go fields are not generated, effective
total ranges are not calculated, and the parser does not support `all` mapping.
The exact value has no fixed resource limit; later phases must set bounded input
and materialization policies.

The main risks are memory proportional to hostile finite lexicals, a breaking
API migration if exact accessors are delayed, and accidentally treating the
semantic `0/0` absence as a public zero-valued component. The range
constructor, ownership tests, and mapping proof guard the latter two; future
resource policy must guard the first.

Public occurrence accessors and retirement of the `uint64` methods belong to the
schema API migration boundary. Schema mapping, including `0/0` absence, belongs
to component construction; bounded materialization and repetition belong to
validation; bounded repeated emission belongs to code generation. These
responsibilities preserve the phase boundaries and edition-specific `all` rules
recorded here.
