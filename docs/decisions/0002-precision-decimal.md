# 0002: precisionDecimal semantic and representation contract

Status: accepted

## Decision

`precisionDecimal` is an optional, opt-in XSD datatype. The pinned 9 June 2011
artifact’s [§Abstract](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#abstract)
and [§Status](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#status)
identify it as a W3C Working Group Note describing an implementation-defined
datatype and work in progress; it is not a mandatory XSD 1.1 conformance
requirement. [XSD 1.1 Part 2](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/)
§2.5.1 (primitive datatypes; `#dt-primitive`) and [§H.1](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#impl-def)
permit, but do not require, primitive datatypes outside the standard set.
Opt-in boundary.

[`Decision 0007`](0007-particle-occurrence.md) governs placement/consumers:

- Schema/query: Compatibility/Strict11 admits global built-in/named/inline `precisionDecimal`; Strict10 rejects by policy. Element/particle mappings admit local built-in/named-effective forms only in default direct or bounded attribute-free extension choices with default owners/typed alternatives; inline anonymous, non-default, and nonzero direct/extension sequences reject. Strict10 rejects before `0/0`; other policies omit zero. `AttributeUse` admits Boolean/integer/decimal plus policy-gated `precisionDecimal`; scalar `simpleContent` admits Boolean/string/integer/decimal plus the same. Strict10 rejects those precisionDecimal facts; refs retain use/provenance, not copied types.
- Validation: Compatibility/Strict11 built-in/named roots validate. Only non-extension default element/particle choices with built-in/named-effective local `precisionDecimal` validate; inline, anonymous, extension, attribute-bearing, and scalar `simpleContent` consumers reject.
- Generation: Compatibility/Strict11 facts remain queryable; `GenerateGo` rejects global, typed local, inline, anonymous, extension, attribute-bearing, and scalar `simpleContent` precisionDecimal targets.

## Semantic contract

The Note’s [§3.1 value space](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-vs-pD)
has finite decimal values with [numerical value](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-numVal),
[sign](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-sign), significand, and
[integer scale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-precision), plus `+INF`/`INF`,
`-INF`, and `NaN`. Signed zeros are distinct but numerically equal; `NaN` is incomparable, including with itself.
+INF is above finite values and -INF; -INF is below finite values and +INF. This is a partial order.

Final XSD 1.1 [`cvc-enumeration-valid`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#cvc-enumeration-valid)
uses `equal or identical`; [`identity`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#identity) lets a `NaN`
enumeration member accept `NaN`. General comparison leaves `NaN` unordered and unequal to itself; signed zero and
finite lexical variants use numeric equality.

The [§3.2 lexical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#pD-lexical-mapping),
[`pDecimalRep`](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#nt-precDecRep), and
[lexical-map function](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecLexmap) apply
collapsed whitespace to decimal, decimal-point, scientific, and special forms; the [special-value definition](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#dt-specialvalue)
remains part of the value model. Mapping is exact; scale is fractional-digit count minus exponent (`3.00` scale 2,
`3.0e2` value 300, scale -1). Retain trailing zeroes; signed exponents remain unbounded.

Applicable facets are exactly:

- fixed `whiteSpace = collapse`;
- value-based `totalDigits`, `minScale`, `maxScale`, `enumeration`,
  `minInclusive`, `minExclusive`, `maxInclusive`, and `maxExclusive`;
- lexical `pattern`; and
- `assertions`, owned by the separate `xsd.assertion` feature.

The Note’s [§3.3 facet declaration](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-f-pD),
[totalDigits](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#rf-totalDigits), [maxScale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#rf-maxScale),
[minScale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#rf-minScale), and [§4 facet rules](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#facets)
exclude `fractionDigits`, `length`, `minLength`, and `maxLength`. Fixed whitespace is pre-lexical; `pattern`
examines normalized lexical form; other facets constrain a complete value, never a partial parse.

The [canonical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecCanmap) has no
resolved zero branch in the pinned Note. The project chooses these non-normative, sign-preserving spellings:
every positive zero, regardless of retained scale, projects to `0.0E0`; every negative zero, regardless of
retained scale, projects to `-0.0E0`. Note-compatible examples are `3.00 -> 3.00`, `3.00e2 -> 300`,
`3.0e2 -> 3.0E2`, `1e-6 -> 0.000001`, `1e-7 -> 1E-7`, `+INF -> INF`, `-INF -> -INF`, and `NaN -> NaN`;
thus `+INF` canonicalizes to `INF`. Canonical text is an on-demand output projection only: never value identity,
facet input, or round-trip serialization of retained zero scale. XSD 1.1’s [`canonical mapping`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#dt-canonical-mapping)
is not required for schema processing, and this policy does not make the optional datatype mandatory.

## Representation and phases

The value representation has one private source of truth: a tagged finite, `+INF`, `-INF`, or `NaN` value. A
finite value contains an arbitrary-precision, non-negative coefficient, explicit sign (including signed zero),
and arbitrary signed scale; scale cannot be `int` because the lexical exponent is unbounded. `StrictDecimal` differs:
it has an `int` scale, elides trailing zeroes, and lacks special values; only its copy techniques may be reused.
The representation exposes no binary floating point, mutable numeric internals,
raw lexemes, cached canonical strings, or partial public values; private `big.Int`
values are owned or copied before mutation, and coefficient, scale, and cache state
remain private.

The private, on-demand `canonicalPrecisionDecimal` canonicalizer accepts a finite, non-negative ASCII-byte budget
`B` for the exact final canonical lexical form; this grammar is ASCII, so characters and bytes coincide. Let `L` be the
exact planned length: complete output is returned iff `L <= B`, while the one-over-limit case `L = B+1` is
rejected. `B=0` rejects every valid value because every canonical lexical form is non-empty. Exact planned length
must be computed, or safely capped at `B+1` while preserving the accept/reject distinction, from the
arbitrary-precision representation before allocating or materializing output. No binary floating point, native-width scale,
`10^huge` construction, padding expansion before the check, cached canonical string, partial output, truncation,
or value mutation is permitted.

For valid `L > B`, `canonicalPrecisionDecimal` returns no string, leaves the value
unchanged, and reports located `FailureInvalid` with the exported
`ErrPrecisionDecimalCanonicalOutputLimit` cause and caller `Loc`. Public/schema
APIs expose this boundary. It is a resource/invalid-request result, not lexical
invalidity, unsupported behavior, or internal failure.

Canonicalization remains separate from comparison and the optional schema
policy boundary. Boundary contract:

| Case | Result | Classification |
| --- | --- | --- |
| `L < B` or exact `L = B` | Complete output; exact limit accepted. | Accepted output |
| One-over, `L = B+1` | Reject with `ErrPrecisionDecimalCanonicalOutputLimit`; no output or mutation. | Resource/invalid request |
| Invalid lexical `1e+` or `1e-` | Reject under existing `XSD2010` / [`xsd-precisionDecimal#f-precDecLexmap`](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecLexmap) semantics. | Invalid lexical input |
| Compare with `NaN` | Unordered result; no error or artificial total order. | Partial comparison |

Construction remains sequential and phase-specific:

1. normalize whitespace and validate grammar;
2. construct a complete finite or special value;
3. apply lexical and value facets, after construction;
4. compare with an explicit partial-order result;
5. canonicalize only on demand under the bounded-output contract;
6. integrate with schema and diagnostics.

Comparison must not use `sort.Interface` or manufacture a total order. The Note’s [§5.1 implementation limits](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#implementation-limits)
give a 16-totalDigits, maxScale 369, minScale -398 minimum envelope and recommend a decimal128-like 34-totalDigits,
maxScale 6111, minScale -6176 envelope. These non-mandatory numbers are implementation guidance, not a
conformance claim or a substitute for the per-call resource contract.

## Bounded follow-up and corpus evidence

The boundary covers values/facets, partial comparison, bounded canonical output,
and schema facts; assertions/remaining facets stay separate,
while bound parsing, effective facts, and scalar validation integrate.

Pinned [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml) references auxiliary groups
[`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet)
and [`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
[#210](https://github.com/goxdra/goxsd9/issues/210) owns resolved auxiliary outcomes; [#196](https://github.com/goxdra/goxsd9/issues/196)
and [#211](https://github.com/goxdra/goxsd9/issues/211) own the source ledger and executable effective-expectation policy; the pinned
catalog remains provenance, and auxiliary results stay outside headline conformance.
