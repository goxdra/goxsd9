# 0002: precisionDecimal semantic and representation contract

Status: accepted

## Decision

`precisionDecimal` is an optional, opt-in XSD datatype. The pinned 9 June 2011
artifact’s [§Abstract](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#abstract)
and [§Status](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#status)
identify it as a W3C Working Group Note that
describes an implementation-defined datatype and remains work in progress; it
is not a mandatory XSD 1.1 conformance requirement. [XSD 1.1 Part 2](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/)
§2.5.1 (primitive datatypes; `#dt-primitive`) and [§H.1](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#impl-def)
likewise permit, but do not require,
primitive datatypes outside the standard set. The project therefore keeps
`xsd.datatype.precision-decimal` unsupported until the complete supported
precisionDecimal cluster exists. Unfinished behavior remains an explicit
unsupported result; it is not silently approximated.

The source is pinned as `xsd-precisionDecimal` in
[`specs/manifest.json`](../../specs/manifest.json), including its digest:
[An XSD datatype for IEEE floating-point decimal](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/).
The existing gate is recorded in [`internal/feature/feature.go`](../../internal/feature/feature.go)
and [`feature.go`](../../feature.go).

## Semantic contract

The Note’s [§3.1 value space](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-vs-pD)
has finite decimal values with [numerical value](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-numVal),
[sign](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-sign),
significand, and [integer scale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#vp-pd-precision),
plus `+INF`/`INF`, `-INF`, and `NaN`. For finite values, scale is the exponent
in the significand-times-ten relation. Positive and negative zero retain
different signs but are numerically equal; `NaN` is incomparable with every
value, including itself; positive infinity is above every finite value and
negative infinity, and negative infinity is below every finite value and
positive infinity. The order is therefore partial, not a sortable total order.

The [§3.2 lexical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#pD-lexical-mapping),
its [`pDecimalRep`](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#nt-precDecRep)
grammar, and the [lexical-map function](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecLexmap)
apply collapsed whitespace and admit decimal, decimal-point, scientific, and
special forms (`INF`, `+INF`, `-INF`, `NaN`). The [special-value definition](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#dt-specialvalue)
is part of the same value model. The mapping is exact and does not round. Scale
is fractional-digit count minus the exponent: `3.00` retains
scale 2, while `3.0e2` has numerical value 300 and scale -1. Trailing zeroes
must not be elided before scale is retained. The grammar and value model must
also account for very large signed exponents without assuming a machine-sized
bound.

Applicable facets are exactly:

- fixed `whiteSpace = collapse`;
- value-based `totalDigits`, `minScale`, `maxScale`, `enumeration`,
  `minInclusive`, `minExclusive`, `maxInclusive`, and `maxExclusive`;
- lexical `pattern`; and
- `assertions`, owned by the separate `xsd.assertion` feature.

The Note’s [§3.3 facet declaration](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#sec-f-pD),
[totalDigits](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#rf-totalDigits),
[maxScale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#rf-maxScale),
[minScale](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#rf-minScale),
and [§4 facet rules](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#facets)
exclude `fractionDigits`, `length`, `minLength`, and `maxLength`. Fixed
whitespace is pre-lexical; `pattern` examines the normalized lexical form;
the other listed datatype facets constrain a complete value. Facets are not
evaluated against a partial parse.

The [canonical mapping](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#f-precDecCanmap)
specifies special forms and finite decimal/scientific examples. Its zero and
signed-zero branch is not fully resolved, so the project preserves that
ambiguity as a later project-defined policy and invents no normative zero form.
XSD 1.1’s [`canonical mapping`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#dt-canonical-mapping)
is not required for schema processing. Canonical text is consequently produced
on demand, not used as value identity or stored as a cache.

## Representation and phases

The later value must have one private source of truth: a tagged finite,
`+INF`, `-INF`, or `NaN` value. A finite value contains an arbitrary-precision,
non-negative coefficient, an explicit sign (including signed zero), and an
arbitrary signed scale. Scale cannot be `int` because the lexical exponent is
unbounded. The implementation uses no binary floating point, mutable numeric
internals, raw-lexeme or canonical-string cache, or partially
constructed public value. Any private `big.Int` is owned or copied before
mutation, and coefficient, scale, raw lexeme, and cache state are not exposed.

The existing [`StrictDecimal`](../../datatype.go) is not this representation:
it uses an `int` scale, elides trailing zeroes, and has no special values. Only
its private-copy techniques may be reused. Construction remains sequential and
phase-specific:

1. normalize whitespace and validate grammar;
2. construct a complete finite or special value;
3. apply lexical and value facets, after construction;
4. compare with an explicit partial-order result and canonicalize on demand;
5. integrate with schema and diagnostics.

Comparison must not use `sort.Interface` or manufacture an artificial total
order. Canonicalization must not materialize `10^huge`; later APIs must define
resource limits and return an explicit, cause-preserving error for rejected
input or unrepresentable canonical output.

The Note’s [§5.1 implementation limits](https://www.w3.org/TR/2011/NOTE-xsd-precisionDecimal-20110609/#implementation-limits)
give a 16-totalDigits, maxScale 369, minScale -398 minimum envelope and
recommend a decimal128-like 34-totalDigits, maxScale 6111, minScale -6176
envelope. These numbers are non-mandatory project implementation guidance,
not a current conformance claim or a substitute for documenting resource
limits.

## Bounded follow-up and corpus evidence

Implementation remains bounded into separate packets: the lexical/value core;
facet derivation and validation; partial comparison, canonicalization, and
resource behavior; schema/feature integration; and corpus verification. The
assertion packet belongs to `xsd.assertion`, and exact decimal facet work stays
separate from precisionDecimal value construction. The feature gate changes
only when the supported cluster is complete.

The pinned catalog’s [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml)
references the accepted, undisputed auxiliary groups
[`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet)
and [`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
[`internal/conformance/catalog_test.go`](../../internal/conformance/catalog_test.go)
records 123 accepted, non-queried auxiliary cases: 47 validation and 76 facet
cases, split 71 Saxon and 52 IBM. The groups cover lexical and special values,
signed zero, bounds, enumeration and pattern, totalDigits, scale/derivation,
and invalid facet declarations. [`internal/conformance/catalog.go`](../../internal/conformance/catalog.go)
keeps auxiliary cases out of headline coverage. They are evidence for bounded
testing only; they do not prove mandatory XSD 1.1 precisionDecimal conformance.
