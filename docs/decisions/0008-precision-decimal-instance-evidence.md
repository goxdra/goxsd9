# 0008: precisionDecimal instance-evidence classification

Status: accepted

## Decision

`precisionDecimal` is optional, implementation-defined behavior. Its auxiliary
instance rows are evidence for bounded implementation work, not headline
conformance. This decision records the accepted classification of issue #196
without turning the corpus into a mandatory XSD 1.1 claim; the semantic boundary
remains [0002](0002-precision-decimal.md).

The exact baseline is 69 accepted, non-queried instance rows in catalog order:
50 Saxon rows and 19 IBM rows, with 24 expected valid and 45 expected invalid
instances. Every instance row is paired with a schema row whose expected result
is valid. The separate 54 schema rows remain schema-only work in #83 (21 Saxon
and 33 IBM), even when one of their schemas is paired with an instance here.

Each row has one owner. The grouped ledger below uses repository-relative paths;
`V` and `I` are the catalog's expected instance results.

| Owner | Catalog rows and path evidence | Count |
| --- | --- | ---: |
| #217 | Saxon rows 1–46 under `testdata/w3c/xsdtests/saxonData/PDecimal/`: `pdecimal001` (`v1`, `v2`, `n1`–`n5`), `pdecimal002` (`v1`, `n1`–`n3`), `pdecimal003` (`v1`, `n1`–`n4`), `pdecimal004` (`v1`, `n1`–`n3`), `pdecimal005` (`v1`, `n1`–`n3`), `pdecimal006` (`v1`, `n1`, `n2`), `pdecimal007` (`v1`, `n1`), `pdecimal008` (`v1`, `n1`–`n5`), `pdecimal010` (`v1`, `n1`–`n5`), and `pdecimal016` (`v1`, `n1`–`n4`), each paired with its `.xsd`; IBM row 52 is `testdata/w3c/xsdtests/ibmData/valid/D3_3_4/d3_3_4v15.xsd` with `d3_3_4v15.xml`. | 47 (12 V, 35 I) |
| #218 | Saxon rows 47–50 under `testdata/w3c/xsdtests/saxonData/PDecimal/`: `pdecimal019` (`v1`, `n1`) and `pdecimal020` (`v1`, `n1`), paired with their schemas; IBM rows 53–54 are `testdata/w3c/xsdtests/ibmData/valid/D3_3_4/d3_3_4v16.xsd`/`.xml` and `d3_3_4v17.xsd`/`.xml`. | 6 (4 V, 2 I) |
| #216 | IBM row 51 is `testdata/w3c/xsdtests/ibmData/valid/D3_3_4/d3_3_4v14.xsd`/`.xml`; rows 55–61 are the same directory's `d3_3_4v18` through `d3_3_4v24` schema/instance pairs; rows 62–68 use `testdata/w3c/xsdtests/ibmData/instance_invalid/D3_3_4/d3_3_4ii01.xsd` with `d3_3_4ii01.xml` and `d3_3_4ii01a.xml` through `d3_3_4ii01f.xml`; row 69 is the `d3_3_4ii02.xsd`/`d3_3_4ii02.xml` pair. | 16 (8 V, 8 I) |

Thus #217 owns Saxon rows 1–46 plus IBM row 52 (47 rows), #218 owns Saxon
rows 47–50 plus IBM rows 53–54 (6 rows), and #216 owns IBM rows 51 and 55–69
(16 rows). The ownership is disjoint and totals the baseline.

## Prerequisites and contracts

`#210` is a `needs-human` clarification for the Saxon `pdecimal006.n2` `NaN`
enumeration conflict and the IBM `d3_3_4v14i` `1.001e3`/enumeration conflict.
The catalog evidence is retained until that clarification is resolved: the
Saxon row is catalog-invalid despite its `NaN` enumeration, while the IBM row
is catalog-valid although `1.001e3` maps to 1001 and the schema enumeration
contains `+1000.00`.

The remaining contracts are deliberately narrow:

| Issue | Contract |
| --- | --- |
| #211 | Generic catalog runner. |
| #212 | Exact occurrences. |
| #213 | Simple-type varieties. |
| #214 | Atomic precisionDecimal validator. |
| #215 | Complex-content bridge. |

All implementation packets #211–#218 are XS/S/M and non-executable while native
blockers are open. #211 is independent of the schema-only
evidence in #83 and #186, and no packet may silently convert catalog evidence
into a conformance requirement.

## Boundaries and evidence handling

`#83` and `#186` own schema-only evidence and must not be coupled to the
instance runner. Local attributes in #217 are distinct from global attribute
work in #198. The runner and its consumers must preserve catalog order,
exact lexical and value representations, source locations, and explicit
unsupported diagnostics; they must not skip, approximate, or relabel an
unresolved case.

The pinned catalog sources are [`extra-suite.xml`](../../testdata/w3c/xsdtests/extra-suite.xml),
which references [`saxonMeta/PDecimal.testSet`](../../testdata/w3c/xsdtests/saxonMeta/PDecimal.testSet)
and [`ibmMeta/precisionDecimal.testSet`](../../testdata/w3c/xsdtests/ibmMeta/precisionDecimal.testSet).
The Saxon data is under
[`saxonData/PDecimal/`](../../testdata/w3c/xsdtests/saxonData/PDecimal/); IBM
valid data is under
[`ibmData/valid/D3_3_4/`](../../testdata/w3c/xsdtests/ibmData/valid/D3_3_4/).
The `ibmData/instance_invalid/D3_3_4/` directory is an instance-fixture
convention, not a claim that its paired schema is invalid. The uncataloged
Saxon `pdecimal001.n6.xml` is excluded.

This classification freezes historical catalog outcomes and ownership. Future
execution may add results, but must retain the 69-row baseline, its deterministic
order, and the explicit #210 gate.
