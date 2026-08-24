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
permit, but do not require, primitive datatypes outside the standard set. The
project implements this datatype as an explicit opt-in library/schema boundary
and keeps it optional.

The source is pinned as `xsd-precisionDecimal` in [`specs/manifest.json`](../../specs/manifest.json),
including its digest: [An XSD datatype for IEEE floating-point decimal](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/).
The completed optional precisionDecimal library/schema boundary has no
precisionDecimal unsupported gate; validator and code-generation support remain
separate.

## Semantic contract

The Note’s [§3.1 value space](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-vs-pD)
has finite decimal values with [numerical value](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-numVal),
[sign](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-sign), significand, and
[integer scale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-precision), plus `+INF`/`INF`,
`-INF`, and `NaN`. Signed zeros are distinct but numerically equal; `NaN` is incomparable, including with itself.
+INF is above finite values and -INF; -INF is below finite values and +INF. This is a partial, not total, order.

The [§3.2 lexical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#pD-lexical-mapping),
its [`pDecimalRep`](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#nt-precDecRep) grammar, and
the [lexical-map function](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecLexmap) apply
collapsed whitespace and admit decimal, decimal-point, scientific, and special forms (`INF`, `+INF`, `-INF`,
`NaN`). The [special-value definition](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#dt-specialvalue)
is part of the value model. Mapping is exact and does not round. Scale is fractional-digit count minus exponent:
`3.00` retains scale 2, while `3.0e2` has numerical value 300 and scale -1. Retain trailing zeroes; very large
signed exponents must not acquire a machine-sized bound.

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
examines normalized lexical form; other listed facets constrain a complete value, never a partial parse.

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
The representation uses no binary floating point, mutable
numeric internals, raw-lexeme or cached canonical strings, or partially constructed public value. Any private
`big.Int` is owned or copied before mutation, and coefficient, scale, raw lexeme, and cache state are not exposed.

The private, on-demand `canonicalPrecisionDecimal` canonicalizer accepts a finite, non-negative ASCII-byte budget
`B` for the exact final canonical lexical form; this grammar is ASCII, so characters and bytes coincide. Let `L` be the
exact planned length: complete output is returned iff `L <= B`, while the one-over-limit case `L = B+1` is
rejected. `B=0` rejects every valid value because every canonical lexical form is non-empty. Exact planned length
must be computed, or safely capped at `B+1` while preserving the accept/reject distinction, from the
arbitrary-precision representation before allocating or materializing output. No binary floating point, native-width scale,
`10^huge` construction, padding expansion before the check, cached canonical string, partial output, truncation,
or value mutation is permitted.

For a valid value with `L > B`, the private `canonicalPrecisionDecimal` canonicalizer returns no string and leaves
the value unchanged. It reports a located `FailureInvalid` diagnostic preserving the exported
`ErrPrecisionDecimalCanonicalOutputLimit` sentinel as its cause and the caller's `Loc`. Public and schema APIs expose
this completed boundary without exposing the private representation.
It is a resource/invalid-request result, not lexical invalidity, unsupported behavior, or internal failure.

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
maxScale 6111, minScale -6176 envelope. These non-mandatory numbers are implementation guidance, not a current
conformance claim or a substitute for the per-call resource contract.

## Bounded follow-up and corpus evidence

The completed optional precisionDecimal boundary covers exact precisionDecimal
library values and applicable facets, partial comparison, bounded canonical
output, and immutable schema facts. Assertions and exact-decimal facet work
remain separate.

Pinned catalog’s [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml) references the accepted,
undisputed auxiliary groups [`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet)
and [`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
[`internal/conformance/catalog_test.go`](../../internal/conformance/catalog_test.go) records 123 accepted,
non-queried auxiliary cases: 47 validation and 76 facet cases, split 71 Saxon and 52 IBM. The groups cover
lexical and special values, signed zero, bounds, enumeration and pattern, totalDigits, scale/derivation, and
invalid facet declarations. [`internal/conformance/catalog.go`](../../internal/conformance/catalog.go) keeps
auxiliary cases out of headline coverage. They are evidence for bounded testing only; they do not prove mandatory
XSD 1.1 precisionDecimal conformance.
