# 0008: precisionDecimal instance-evidence boundary

Status: accepted

## Decision

`precisionDecimal` is optional, implementation-defined behavior. Its auxiliary
instance rows are evidence for bounded implementation work, not headline
conformance; [0002](0002-precision-decimal.md) remains the semantic boundary.

The current classification is 69 accepted, non-queried instance rows in catalog
order: 50 Saxon plus 19 IBM, with 24 expected valid plus 45 expected invalid.
Each row is paired with a schema row whose expected result is valid. The separate
54 schema rows (21 Saxon and 33 IBM) remain with #83. [#196](https://github.com/goxdra/goxsd9/issues/196)
maintains the mutable full ledger, including exact paths and catalog outcomes;
[#219](https://github.com/goxdra/goxsd9/issues/219) owns the deterministic proof
of selection and ownership metadata.

Ownership is disjoint and remains recorded by the packet issues: [#217](https://github.com/goxdra/goxsd9/issues/217)
owns Saxon rows 1–46 plus IBM row 52 (47); [#218](https://github.com/goxdra/goxsd9/issues/218)
owns Saxon rows 47–50 plus IBM rows 53–54 (6); and [#216](https://github.com/goxdra/goxsd9/issues/216)
owns IBM rows 51 and 55–69 (16).

The two catalog interpretations are:

- Saxon row 27, `pdecimal006.n2.xml`, is source-catalog invalid and
  effective-replay valid.
- IBM row 51, `d3_3_4v14.xml`, is source-catalog and effective-replay valid;
  it has no override.

[0002](0002-precision-decimal.md) is the semantic authority for these outcomes;
[#210](https://github.com/goxdra/goxsd9/issues/210) preserves their full case
evidence and resolution history.

Catalog outcomes are immutable historical provenance. Effective expectations
are derived on demand from the catalog plus a sparse, version-scoped policy:
one explicit row-27 invalid-to-valid override guarded by its source outcome,
and no row-51 override. [#211](https://github.com/goxdra/goxsd9/issues/211)
owns the injected policy and executable runner. It validates the complete plan
before any instance executes and returns no partial report; its detailed
validation and reporting contract remains in that issue.

[#83](https://github.com/goxdra/goxsd9/issues/83) and [#186](https://github.com/goxdra/goxsd9/issues/186)
are schema-only tracks and are not coupled to the instance runner. Local
attributes are distinct from global attribute work in
[#198](https://github.com/goxdra/goxsd9/issues/198).

This decision fixes the evidence boundary; it does not execute auxiliary
instances. Catalog order and the 24-valid/45-invalid source split remain
unchanged, and auxiliary evidence is never promoted to headline conformance.

The canonical catalog sources are [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml),
which references [`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet)
and [`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
Saxon data is under [`saxonData/PDecimal/`](../../testdata/w3c/xsdtests/saxonData/PDecimal/);
IBM data is under [`ibmData/valid/D3_3_4/`](../../testdata/w3c/xsdtests/ibmData/valid/D3_3_4/)
and [`ibmData/instance_invalid/D3_3_4/`](../../testdata/w3c/xsdtests/ibmData/instance_invalid/D3_3_4/).
`instance_invalid` is an instance-fixture directory convention, not a claim that
its paired schema is invalid. The uncataloged Saxon `pdecimal001.n6.xml` is
excluded.

The selected auxiliary rows remain outside headline conformance.
