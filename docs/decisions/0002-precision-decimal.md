# 0002: precisionDecimal semantic and representation contract

Status: accepted

## Decision

`precisionDecimal` is optional. The pinned 9 June 2011 [Note](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/)
calls it implementation-defined work in progress, not mandatory XSD 1.1. [XSD 1.1 Part 2](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/)
§2.5.1 (`#dt-primitive`) and [§H.1](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#impl-def) permit, but do not require,
primitive datatypes outside the standard set. The project boundary is opt-in; validator/code generation remain separate.

The `xsd-precisionDecimal` source and its digest are pinned in [`specs/manifest.json`](../../specs/manifest.json).
The boundary has no precisionDecimal unsupported gate.

## Semantic contract

The Note’s [§3.1 value space](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-vs-pD) has finite
decimals with [numerical value](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-numVal), sign,
significand, and arbitrary integer scale, plus `+INF`/`INF`, `-INF`, and `NaN`. Signed zeros are distinct but
numerically equal; `NaN` is incomparable, including itself. `+INF` is above finite values and `-INF`; `-INF`
is below finite values and `+INF`: this is a partial, not total, order.

General equality/comparison remains: `NaN` is unordered and not equal to itself. For final XSD 1.1
enumeration membership, [`cvc-enumeration-valid`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#cvc-enumeration-valid)
accepts values `equal or identical` to a member; [`identity`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#identity)
therefore accepts a `NaN` member by value identity. Signed zero and finite lexical variants use numeric equality. This
does not change global equality/`Compare` or mandate the optional datatype.

The [§3.2 lexical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#pD-lexical-mapping),
[`pDecimalRep`](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#nt-precDecRep) grammar, and
[lexical-map function](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecLexmap) apply collapsed
whitespace and admit decimal, decimal-point, scientific, and special forms (`INF`, `+INF`, `-INF`, `NaN`).
The mapping is exact and does not round. Scale is fractional-digit count minus exponent: `3.00`
retains scale 2, while `3.0e2` has numerical value 300 and scale -1. Retain trailing zeroes; very large signed
exponents must not acquire a machine-sized bound.

Facets are fixed `whiteSpace = collapse`; value-based `totalDigits`, `minScale`, `maxScale`,
`enumeration`, `minInclusive`, `minExclusive`, `maxInclusive`, and `maxExclusive`; lexical `pattern`; and
`assertions`, under `xsd.assertion`. The Note’s [§3.3 facet declaration](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-f-pD)
and [§4 facet rules](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#facets)
exclude `fractionDigits`, `length`, `minLength`, and `maxLength`. Fixed whitespace is pre-lexical; `pattern` examines
normalized lexical form; other facets constrain a complete value, never a partial parse.

The [canonical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecCanmap) has no
resolved zero branch in the pinned Note. The project chooses non-normative, sign-preserving spellings: every positive
zero projects to `0.0E0`, every negative zero to `-0.0E0`, regardless of retained scale. Examples: `3.00 -> 3.00`,
`3.00e2 -> 300`, `3.0e2 -> 3.0E2`, `1e-6 -> 0.000001`, `1e-7 -> 1E-7`, `+INF -> INF`, `-INF -> -INF`, and
`NaN -> NaN`; thus `+INF` canonicalizes to `INF`. Canonical text is on-demand output only: never value identity,
facet input, or round-trip serialization of retained zero scale. XSD 1.1’s [`canonical mapping`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#dt-canonical-mapping)
is not required for schema processing; the datatype remains optional.

## Representation and phases

The private representation’s source of truth is a tagged finite, `+INF`, `-INF`, or `NaN` value. A finite
value contains an arbitrary-precision, non-negative coefficient, explicit sign (including signed zero), and arbitrary
signed scale; scale cannot be `int` because the lexical exponent is unbounded. `StrictDecimal` instead has an `int`
scale, elides trailing zeroes, lacks special values; reuse only copy techniques. No binary floating
point, mutable numeric internals, raw-lexeme or cached canonical strings, or partially constructed public value are
allowed. Private `big.Int` values are owned or copied before mutation; coefficient, scale, raw lexeme, and cache state
are not exposed.

The private, on-demand `canonicalPrecisionDecimal` canonicalizer accepts a finite, non-negative ASCII-byte budget `B`
for the exact final canonical form (ASCII characters and bytes coincide). With exact planned length `L`, output is
returned iff `L <= B`; `L = B+1` is rejected, and `B=0` rejects every valid value. Compute `L`, or safely cap it at
`B+1`, from the arbitrary-precision representation before allocating/materializing output. No binary floating point,
native-width scale, `10^huge` construction, padding expansion before the check, cached string, partial output,
truncation, or value mutation is permitted.

For valid `L > B`, the canonicalizer returns no string and leaves the value unchanged. It reports a located
`FailureInvalid` diagnostic preserving the exported `ErrPrecisionDecimalCanonicalOutputLimit` as its cause and the caller’s
`Loc`. Public/schema APIs expose the boundary; private representation stays hidden. It is a resource/invalid-request
result, not lexical invalidity, unsupported behavior, or internal failure.

Canonicalization remains separate from comparison and optional schema policy boundary:

| Case | Result | Classification |
| --- | --- | --- |
| `L < B` or exact `L = B` | Complete output; exact limit accepted. | Accepted output |
| One-over, `L = B+1` | Reject with `ErrPrecisionDecimalCanonicalOutputLimit`; no output or mutation. | Resource/invalid request |
| Invalid lexical `1e+` or `1e-` | Reject under existing `XSD2010` / [`xsd-precisionDecimal#f-precDecLexmap`](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecLexmap) semantics. | Invalid lexical input |
| Compare with `NaN` | Unordered; no error or artificial total order. | Partial comparison |

Construction is sequential: (1) normalize and validate grammar; (2) construct complete finite or special value; (3) apply
lexical/value facets; (4) compare with explicit partial order; (5) canonicalize on demand under bounded-output contract;
(6) integrate schema and diagnostics.
Comparison must not use `sort.Interface` or manufacture a total order. The Note’s [§5.1 implementation limits](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#implementation-limits)
give a 16-totalDigits, maxScale 369, minScale -398 minimum envelope and recommend a decimal128-like 34-totalDigits,
maxScale 6111, minScale -6176 envelope. These non-mandatory numbers are guidance, not a conformance claim or
substitute for the per-call resource contract.

## Bounded follow-up and corpus evidence

The completed optional boundary covers values/applicable facets, partial comparison, bounded canonical
output, and immutable schema facts. Assertions and remaining precisionDecimal-specific facet work stay separate;
integer/decimal ordered-bound parsing, effective schema facts, and scalar validation are integrated.

The pinned [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml) references auxiliary groups
[`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet) and
[`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
[`catalog_test.go`](../../internal/conformance/catalog_test.go) records 123 accepted, non-queried auxiliary cases
(47 validation, 76 facet; 71 Saxon, 52 IBM) covering lexical/special values, signed zero, bounds, enumeration,
pattern, totalDigits, scale/derivation, and invalid facet declarations; [`catalog.go`](../../internal/conformance/catalog.go)
excludes them from headline coverage. This is bounded-testing evidence, not mandatory precisionDecimal conformance.

Replay decisions in [issue #210](https://github.com/goxdra/goxsd9/issues/210):

- Saxon `pdecimal006.n2.xml`: source catalog expected invalid; effective replay valid because its `NaN` matches the
  schema member by value identity under final XSD 1.1’s equal-or-identical rule. `pdecimal006.v1.xml` has no `NaN`;
  independent post-fix confirmation is `Simple/simple022.v02.xml`. Preserve the pinned historical catalog.
- IBM `d3_3_4v14.xml`: source and effective valid because instance `1.001e3` matches enum `10.01e2`, both numeric
  1001; it does not match `+1000.00`. No fixture/catalog correction is required.

The pinned W3C submodule remains unchanged; the 69-row source inventory is immutable: 50 Saxon, 19 IBM; source
expectations are 24 valid and 45 invalid. Source catalog outcomes remain historical provenance, distinct from effective
replay. Existing [#211](https://github.com/goxdra/goxsd9/issues/211) owns a generic runner with an injected sparse
effective-expectation policy and must keep source expected, effective expected, and actual distinct. This record
specifies no code design.
