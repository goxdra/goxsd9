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

The current schema preflight uses this exact private range to validate lexical
occurrence input. A named global complex type with one direct sequence of local
built-in boolean/token/NMTOKEN or named boolean/token/NMTOKEN restriction,
integer, or decimal scalar elements, or one direct choice of those scalar
elements, maps the completed
range and ordered children into the public schema. Direct `xs:any` `##any`/strict|lax|skip, `##other`/lax|strict, positive namespaces (`##local`, `##targetNamespace`, URI lists)/strict|lax|skip (skip explicit) map to `WildcardParticle` with exact locations/ranges, lexical order; broader/other constraints and consumers unsupported. A supported named
complex type's particle-plus-uses and attribute-only bodies expose immutable ordered local/ref `AttributeUse` views with lexical locations and resolved type/reference identities. One bounded scalar-base `simpleContent` extension retains its base/ref and local/ref uses without a particle; optional/required uses are effective and prohibited declarations omitted. Attribute-bearing validation/generation, value constraints, inheritable attributes, groups, wildcards, and broader derivation remain unsupported.

A supported named
model group with one direct choice or sequence of global element-reference
particles exposes ordered children with exact ranges; its sequence uses
grammar-default 1/1; compositor occurrence attrs are unsupported. Named complex type or bounded attribute-free extension may expose a direct model-group reference with target ID/exact
range; members are not copied. `0/0` group or child maps to absence. Sequence/choice/child mapping maps `0/0` to absence before gating.
The exact representation is retained for choice facts. `ValidateInstance`
supports named global complex types with homogeneous Boolean/numeric sequences,
matching expanded names in lexical declaration order and honoring exact finite,
unbounded, and above-`uint64` outer and child ranges under `Compatibility`,
`Strict10`, and `Strict11`. Direct-choice repetition is unsupported. The
same exact occurrence representation covers bounded attribute-free `complexContent`/`extension`
over named empty-content bases. Extensions retain extension/base identities/locations and only bounded/representable inherited `##other`/lax wildcard facts. Extensions with present direct choice/sequence particles retain exact occurrences; model-less extensions retain those identities/locations but no particle or occurrence or synthetic content. Validation and code generation reject extensions. Local token/NMTOKEN facts remain; default-occurrence all-token/NMTOKEN choices validate; token/NMTOKEN sequences, mixed token-family choices, and local token/NMTOKEN generation remain unsupported.
Default-bounded direct integer/decimal or all-Boolean sequence children are emitted
as ordered Go struct fields; mixed Boolean/numeric sequences and repeated-field
generation remain unsupported. XSD 1.1
default-occurrence direct choices may use `precisionDecimal` only when the
choice and each mapped `precisionDecimal` alternative use default occurrences;
non-precision alternatives may retain non-default ranges for queries. Non-`0/0`
`precisionDecimal` choice or alternative ranges that map to a particle are
schema-unsupported, as are non-`0/0` direct-sequence `precisionDecimal` ranges
that map to a particle.

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

Currently, the occurrence boundary supports one named global complex type
with one direct sequence or direct choice of local built-in boolean/token/NMTOKEN
or named boolean/token/NMTOKEN restrictions, integer, or decimal scalar elements,
or one global named model group with one direct choice or sequence of global element-reference particles, or a top-level direct model-group reference for named complex types or bounded attribute-free extensions over named empty-content bases, in XSD 1.0 and 1.1. Direct model-group references retain exact ranges and target IDs. For bounded attribute-free extensions, exact occurrences apply with a present direct choice, sequence, or group-reference particle. Model-less extensions retain extension/base identities and locations but no particle or occurrence or synthetic content; validation and code generation reject them. Wildcard terms follow rules above. Supported forms retain exact ranges; `0/0` maps to absence.
Sequence validation honors exact finite, unbounded, and above-`uint64` ranges for its documented homogeneous scalar shapes; direct-choice validation remains limited to default occurrences.
XSD 1.1 direct choices may include `precisionDecimal` only with default choice
and mapped-alternative occurrences; non-precision alternatives retain non-default
ranges for queries. Non-default `precisionDecimal` ranges that map to particles
remain unsupported. Effective `0/0` maps to absence before gating. Non-default
direct-choice ranges are queryable, but repetition validation and repeated-field
generation remain unsupported. Boolean facets, anonymous/nested/broader particles,
groups, and broader wildcard/attribute shapes remain unsupported. Direct
named-complex/bounded-extension group refs retain facts; anonymous simple-type models
and resolved built-in/named/anonymous references are modeled. Named direct sequence/choice types expose local/ref `AttributeUse` views with lexical locations and resolved type/reference identities in particle-plus-uses and attribute-only bodies. One bounded `simpleContent` extension retains its scalar base/ref and local/ref uses without a particle; optional/required uses are returned and prohibited declarations omitted. Validation/code-generation consumers reject attribute-bearing types. Named direct sequence/choice types expose direct
`anyAttribute`: omitted attributes default to `##any`/`strict`; omitted or
explicit `##any`/`lax` and omitted or explicit `##any`/`skip` with explicit
`processContents` are supported under all editions/policies. Canonical
explicit strict values remain supported.
`##other`/`lax` and `##other`/`strict` (omitted/explicit `processContents`), plus explicit `##other`/`skip` are supported across editions/policies. The `anyAttribute` element and explicit
`namespace`/`processContents` locations are retained; omitted defaults are
zero. Validation and code-generation consumers remain unsupported. Direct
element-reference particles are supported in the schema model for local choice
and sequence children and for global named-group direct choices or sequences; direct model-group references are
supported only as the top-level particle of a named complex type or bounded attribute-free extension;
they retain target IDs without expanding group members; nested group references remain unsupported. Validator consumption covers named global complex homogeneous Boolean/numeric sequences and default-occurrence scalar choices or references to global Boolean/integer/decimal elements; generation supports the latter, other direct references remain unsupported. Global text-only Boolean validation works under Compatibility, Strict10, and Strict11; Boolean scalar generation works. Direct choices support default-occurrence local Boolean (including named restrictions) or all-token/NMTOKEN alternatives; token/NMTOKEN sequences and mixed token-family choices remain unsupported. Generation supports default-occurrence all-Boolean choices and default-bounded all-Boolean sequences; local token/NMTOKEN generation remains unsupported;
the parser does not support `all` mapping. The exact value has no fixed
resource limit; later phases must set bounded input and materialization
policies.

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
