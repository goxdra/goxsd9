# 0008: precisionDecimal instance-evidence boundary

Status: accepted

## Decision

`precisionDecimal` is optional, implementation-defined behavior. Its auxiliary
instance rows are evidence for bounded implementation work, not headline
conformance; [0002](0002-precision-decimal.md) remains the semantic boundary.

The pinned auxiliary selection contains 69 non-queried instance rows in catalog
order: 50 Saxon and 19 IBM, with 24 source-catalog valid and 45 invalid. Each
instance row has a paired schema row expected valid. The separate 54 schema
rows (21 Saxon and 33 IBM) are schema evidence, not instance outcomes.
Selection and expectations come from catalog order, paired schema outcomes,
and the sparse policy below, independent of work allocation.

The two catalog interpretations are:

- Saxon row 27, `pdecimal006.n2.xml`, is source-catalog invalid and
  effective-replay valid.
- IBM row 51, `d3_3_4v14.xml`, is source-catalog and effective-replay valid;
  it has no override.

Catalog outcomes are immutable historical provenance. Effective expectations
are derived on demand from the catalog plus a sparse, version-scoped policy:
one explicit row-27 invalid-to-valid override guarded by its source outcome,
and no row-51 override. The injected policy and executable runner validate the
complete selection before any instance executes and return no partial report. Schema-only checks
remain separate from this instance runner. Successful auxiliary execution does
not promote a result to headline conformance.

The canonical catalog sources are [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml),
which references [`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet)
and [`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
Saxon data is under [`saxonData/PDecimal/`](../../testdata/w3c/xsdtests/saxonData/PDecimal/);
IBM data is under [`ibmData/valid/D3_3_4/`](../../testdata/w3c/xsdtests/ibmData/valid/D3_3_4/)
and [`ibmData/instance_invalid/D3_3_4/`](../../testdata/w3c/xsdtests/ibmData/instance_invalid/D3_3_4/).
`instance_invalid` is an instance-fixture directory convention, not a claim that
its paired schema is invalid. The uncataloged Saxon `pdecimal001.n6.xml` is
excluded.
