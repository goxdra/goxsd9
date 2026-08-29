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

[#210](https://github.com/goxdra/goxsd9/issues/210) resolved the two apparent
catalog conflicts without changing the pinned catalog or fixtures:

- Saxon row 27, `pdecimal006.n2.xml`, remains source-catalog invalid but is
  effectively valid for XSD 1.1 replay. Its `NaN` value is identical to the
  schema's `NaN` enumeration member under final XSD 1.1
  [`cvc-enumeration-valid`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#cvc-enumeration-valid)
  and [`identity`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#identity).
  This enumeration-membership identity rule does not change general `NaN`
  equality or partial comparison.
- IBM row 51, `d3_3_4v14.xml`, is source-catalog and effectively valid. The
  instance `1.001e3` and enumerated `10.01e2` both have numerical value 1001,
  so numeric equality applies and no replay override exists.

The catalog outcome remains immutable historical provenance. A runner must
derive the effective expectation on demand from the catalog plus a sparse,
version-scoped policy: one explicit row-27 invalid-to-valid override guarded by
the expected source value, and no row-51 override. The guard detects source
drift; it is not a second catalog oracle. The policy must be injected and
opt-in, use the complete case identity rather than a case name alone, validate
the entire replay plan before executing any instance, and reject duplicate or
unknown keys, source-validity drift, a wrong version or origin, and a missing
paired valid schema without returning a partial report.

[#211](https://github.com/goxdra/goxsd9/issues/211) owns this executable runner
and effective-expectation policy. Reports expose source expected, effective
expected, actual, and outcome independently; they distinguish historical-source
and effective-replay mismatches. They never overwrite catalog expectations,
change the 24-valid/45-invalid source split, promote auxiliary rows to headline
conformance, or materialize a duplicate 69-row effective table. [#216](https://github.com/goxdra/goxsd9/issues/216)
and [#217](https://github.com/goxdra/goxsd9/issues/217) retain their row counts
after #210; [#218](https://github.com/goxdra/goxsd9/issues/218) has no affected
catalog contradiction.

[#83](https://github.com/goxdra/goxsd9/issues/83) and [#186](https://github.com/goxdra/goxsd9/issues/186)
are schema-only tracks and are not coupled to the instance runner. Local
attributes are distinct from global attribute work in
[#198](https://github.com/goxdra/goxsd9/issues/198).

This decision records catalog classification and replay policy; it does not
execute auxiliary instances. The runner must preserve catalog order, exact
lexical and value representations, source locations, causes, explicit
unsupported behavior, and deterministic output. Packet detail remains in
[#196](https://github.com/goxdra/goxsd9/issues/196) and [#211](https://github.com/goxdra/goxsd9/issues/211);
execution must not skip, approximate, relabel, or promote auxiliary evidence
into conformance.

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
