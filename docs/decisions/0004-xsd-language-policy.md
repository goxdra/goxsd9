# 0004: XSD language-policy selection

Status: accepted

## Decision

XSD language behavior is selected by one explicit, immutable, graph-wide
`LanguagePolicy`/`SchemaLanguagePolicy`. The public concept has exactly these
profiles:

- `Compatibility`, the default;
- `Strict10`; and
- `Strict11`.

The unqualified `xs:schema/@version` attribute is never an edition selector.
It is an optional user label. Its value may be absent, empty, arbitrary, or
look like an XSD edition without changing the selected policy.

The current two-argument `ParseSchema(root, resolver)` behavior is unchanged
by this decision record. A future API may accept the explicit policy and may
retain a one-way `Compatibility` wrapper for the existing call shape. No
future API is prescribed here that would break the current public API without
separate review. This record chooses the contract for the implementation
packets; it does not change current behavior or make a conformance claim.

### Why the schema token cannot select an edition

The pinned XSD 1.0 schema representation declares the unqualified `schema`
`version` attribute as optional `xs:token` in
[`xsd10-structures#element-schema`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#element-schema).
The XSD 1.1 representation makes the same declaration in
[`xsd11-structures#element-schema`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#element-schema).
The normative schema-for-schema mappings are also pinned directly as
[`xsd10-schema-for-schemas`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/XMLSchema.xsd)
and
[`xsd11-schema-for-schemas`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/XMLSchema.xsd).
Both mappings state that `id` and `version` are for user convenience and that
the specifications define no semantics for them. The schema-document rules
are identified separately by
[`xsd10-structures#key-schemaDoc`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#key-schemaDoc)
and
[`xsd11-structures#key-schemaDoc`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#key-schemaDoc);
neither turns the label into a language-edition declaration.

Consequently, treating `"1.0"` or `"1.1"` specially would invent semantics
that the representation does not have and would make a harmless user label
change parsing behavior. Empty and arbitrary token values are no more edition
selectors than those two familiar labels.

The repository evidence is independent of the label. The manifest pins
separate XSD 1.0 and XSD 1.1 structures and datatype specifications, and
separate schema-for-schema artifacts with their representations and edition
metadata in [`specs/manifest.json`](../../specs/manifest.json). The accepted
[manifest-scoped bootstrap decision](0003-schema-bootstrap.md) records those
two entry artifacts and their version-specific prerequisites. The foundations
decision establishes that XSD 1.0 and XSD 1.1 may coexist in one graph
([`0001-foundations.md`](0001-foundations.md)).

### Different kinds of version information

These inputs have different owners and meanings:

| Input | Meaning | Can select the parser language policy? |
| --- | --- | --- |
| XML declaration `<?xml version="1.0"?>` or `<?xml version="1.1"?>` | XML syntax and character-processing declaration for that XML document. It is not the XSD language edition. | No. |
| Unqualified `xs:schema/@version` | Optional `xs:token` user label on one schema document. | No. Labels alone never mismatch. |
| XSD 1.1 `vc:minVersion`/`vc:maxVersion` | Conditional-inclusion bounds compared with the processor capability `V`. They filter marked content; they do not choose a graph policy. | No. The selected policy supplies `V`. |
| Manifest/catalog edition metadata | Repository and conformance-run metadata identifying the pinned XSD edition to exercise. | Yes, before parsing a strict catalog run. |
| Resolver source identity, context, namespace URN, and lexical location | Opaque source and resolution information used by the caller and resolver; identities support repeat/cycle handling. | No. |

The XSD 1.1 conditional-inclusion rule explicitly compares `vc:minVersion`
and `vc:maxVersion` with a processor capability `V`, while the `schema`
`version` token remains a user label. The repository preserves the same
separation: manifest edition metadata selects a strict catalog run, and the
resolver continues to receive opaque contexts, namespace URNs, and lexical
locations without the parser interpreting them.

## Policy contract

The selected policy is created once and is immutable for the complete graph:
the root, includes, imports, cycles, and repeated identities all use the same
policy. No source can replace it with its `schema/@version` label, and a
repeated identity cannot create a second policy state.

| Policy | Graph behavior | Version-sensitive rules and capability |
| --- | --- | --- |
| `Compatibility` | Mixed XSD 1.0/1.1 graphs are allowed. | Admit the union of the supported XSD 1.0 and supported XSD 1.1 lexical forms. Use the documented XSD 1.1-compatible value behavior for the supported subset, including the common value-space rules for `totalDigits` and `fractionDigits`. Evaluate conditional inclusion with compatibility capability `V = 1.1`. This is compatibility behavior, not conformance to either edition. |
| `Strict10` | The whole graph is processed under one exact XSD 1.0 profile. | Use XSD 1.0 lexical, value, facet, and conditional-inclusion rules and capability `V = 1.0`. A document label cannot change the profile. |
| `Strict11` | The whole graph is processed under one exact XSD 1.1 profile. | Use XSD 1.1 lexical, value, facet, and conditional-inclusion rules and capability `V = 1.1`. A document label cannot change the profile. |

For Compatibility, version-sensitive lexical and facet decisions use the
documented XSD 1.1-compatible semantics within the supported subset; the
lexical admission set remains the supported XSD 1.0 union XSD 1.1 union.
The datatype evidence for this policy is the paired pinned decimal and facet
material: [`xsd10-datatypes#decimal-lexical-representation`](https://www.w3.org/TR/2004/REC-xmlschema-2-20041028/#decimal-lexical-representation),
[`xsd11-datatypes#decimal-lexical-representation`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#decimal-lexical-representation),
[`xsd10-datatypes#dt-totalDigits`](https://www.w3.org/TR/2004/REC-xmlschema-2-20041028/#dt-totalDigits),
[`xsd11-datatypes#dt-totalDigits`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#dt-totalDigits),
[`xsd10-datatypes#dt-fractionDigits`](https://www.w3.org/TR/2004/REC-xmlschema-2-20041028/#dt-fractionDigits),
and
[`xsd11-datatypes#dt-fractionDigits`](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/#dt-fractionDigits).
Compatibility uses one documented rule for the common supported facet value
space; it does not choose a rule from the document carrying the facet.

Compatibility is deliberately bounded. If a future XSD 1.0/1.1 difference
has no unambiguous compatibility rule in the supported subset, parsing reports
explicit unsupported behavior at the source construct. It does not infer a
rule from `schema/@version`, silently skip the construct, or claim strict
conformance. The selected graph policy is the one version source of truth;
any per-feature `XSDVersion` or capability value in a future implementation is
derived from that policy and is not an independent selector.

## Examples

The following examples make the label and policy boundaries observable. A
schema label is retained only as input to the schema representation rules; it
never selects a policy and never mismatches a policy by itself.

| Example | Selected policy | Result |
| --- | --- | --- |
| `xs:schema` has no `version` attribute | `Compatibility` by default, or either strict profile when explicitly selected | Process under the selected policy. Absence does not mean XSD 1.0 or XSD 1.1. |
| `version=""` | Any | Accept the empty token label and ignore it for edition selection. No mismatch. |
| `version="release-2026"` | Any | Treat the arbitrary token as a user label. No mismatch. |
| `version="1.0"` | `Strict11` | Use strict XSD 1.1 rules. The label does not downgrade the graph. No mismatch. |
| `version="1.1"` | `Strict10` | Use strict XSD 1.0 rules. The label does not upgrade the graph. No mismatch. |
| Root label `"1.0"`, resolver-supplied import label `"1.1"` | `Compatibility` | Allow the mixed graph and use one compatibility policy for both documents, including cycles and repeated identities. Do not switch policy at the import. |
| A catalog entry selects XSD 1.0 or XSD 1.1 from manifest edition metadata | `Strict10` or `Strict11` respectively | Select the strict policy before parsing. Ignore every schema label in the root and resolver graph; labels cannot override catalog selection. |
| `Strict10` encounters a recognized XSD 1.1-only feature such as `<xs:assert>` that must be processed | `Strict10` | Report an actual strict-profile feature mismatch at the construct’s source `Loc` as explicit unsupported behavior, with the registered feature, stable code, and pinned XSD 1.1 reference. Return no schema. |

The final row is a mismatch because the source invokes behavior outside the
selected strict profile, not because it contains a particular label. A
well-formed graph whose documents merely use different labels is not a strict
mismatch. Conversely, malformed representation input, such as an invalid
`vc:minVersion` decimal, is invalid input rather than a policy mismatch.

## Lifecycle and diagnostics

Future implementation packets preserve these invariants:

1. Validate the policy before discovery and before any resolver call. An
   invalid policy configuration uses the dedicated stable parser-policy code
   `XSD3013`, is classified as `FailureInvalid`, has `Loc{}` because no source
   position exists, and preserves the configuration cause through the returned
   diagnostic.
2. When the root is valid, an invalid policy closes that root exactly once,
   makes zero resolver calls, and returns a zero `Schema`. A close failure is
   preserved with the primary policy diagnostic without causing a second close.
3. Every source handed out by the caller or resolver is closed exactly once on
   every error path, including a source returned alongside a resolver error,
   a repeated or cyclic identity, and queued sources abandoned after a later
   error. No partial schema is returned after any error-level diagnostic.
4. A source policy/feature mismatch carries the offending source `Loc`, a
   stable diagnostic code for the registered feature, that feature identity,
   and its pinned `SpecRef`. Valid behavior unavailable under a selected
   strict profile is `FailureUnsupported`; a malformed schema representation
   is `FailureInvalid`; only resolver failures are `FailureResolution`.
5. Resolver calls remain sequential. The parser passes contexts, namespace
   URNs, and lexical schema locations through unchanged and does not interpret
   paths, open resources, or use source identity as an edition signal.

These rules preserve causes at package boundaries and preserve the diagnostic
classification, primary location, related locations, and specification
reference needed to explain the first failure. A policy error is not turned
into a source error merely to obtain a location.

## Current gap and non-goals

The current implementation demonstrates why this decision must precede code
changes:

- [`schema_build.go`](../../schema_build.go) has the schema-document-version
  helper (`syntaxDocumentVersion`, the `schemaDocumentVersion` gap named by
  this issue) that defaults an absent label to XSD 1.1, maps `"1.0"` and
  `"1.1"` to different behavior, and rejects arbitrary labels. Its
  `schemaTargetNamespace` record stores that version per document and passes it
  into declarations.
- [`schema_phase.go`](../../schema_phase.go) reads the document label during
  root validation and hard-codes `"1.1"` in
  `applySchemaConditionalVersion`, so conditional inclusion is not supplied by
  an independent graph policy.
- [`schema_build.go`](../../schema_build.go) calls
  `reversionDigitFacets` in [`datatype_facets.go`](../../datatype_facets.go)
  to retag inherited digit facets with the child document’s version. This is
  source-specific facet state, not a graph-wide policy.
- [`schema_build_test.go`](../../schema_build_test.go) and
  [`schema_parse_test.go`](../../schema_parse_test.go) assert different
  `DigitFacets().Version()` values for documents labelled 1.0 and 1.1. Those
  tests are evidence of the current gap and must not become the future policy
  contract merely by being retained.

This research packet changes none of those paths, the current `ParseSchema`
behavior, README, ARCHITECTURE, PLAN, source code, or tests. It makes no new
XSD feature, lexical, facet, or conformance claim. The existing public
documentation remains the contract until an implementation packet is accepted
and completed.

## Implementation decomposition

Exactly three bounded implementation packets follow this decision. They share
one policy source of truth and must be delivered in dependency order.

| Packet | Size and dependency | Changed responsibilities | Tests and evidence |
| --- | --- | --- | --- |
| XS: policy value and preflight | XS; blocked until [#113](https://github.com/goxdra/goxsd9/issues/113) is accepted | Define the immutable `LanguagePolicy`/`SchemaLanguagePolicy` values and validation, reserve the parser-policy diagnostic contract, validate before discovery, close a valid root once on invalid configuration, and keep the current two-argument behavior unchanged. | Unit tests for all three profiles, invalid values, `FailureInvalid`, `XSD3013`, zero `Loc`, preserved cause, zero resolver calls, exactly-once root close, and zero schema. Evidence: this decision’s lifecycle invariants. |
| S: graph propagation and capability | S; depends on XS | Carry one selected policy through root, includes, imports, cycles, and repeated identities without putting it in `ResolvedSource`, `Resolver`, context, location, or source identity. Derive conditional-inclusion capability (`V = 1.0` or `V = 1.1`) from the policy, keep calls sequential, and provide any reviewed explicit-policy entrypoint plus one-way Compatibility wrapper. | Mixed-label and mixed-edition resolver graphs; conditional inclusion for each profile; cycle/repeat closure; opaque context/namespace/location assertions; no partial result after errors; labels alone never mismatch. Evidence: `discovery.go`, `schema_phase.go`, `README.md`, and the XSD `key-schemaDoc`/conditional-inclusion rules. |
| M: strict rules and conformance integration | M; depends on S | Apply exact Strict10/Strict11 lexical, value, facet, and registered-feature diagnostics; apply Compatibility’s supported union and common facet rules; remove source-label selection and facet reversion; select strict policy from catalog/manifest edition metadata. | Paired decimal lexical anchors; paired `totalDigits`/`fractionDigits` value-space tests; strict feature mismatch `Loc`/code/feature/`SpecRef`; catalog-selected strict runs; mixed Compatibility graphs; full stream lifecycle and no-schema-after-error evidence. Evidence: `specs/manifest.json`, `0001-foundations.md`, `0003-schema-bootstrap.md`, and the paired XSD specifications. |

No GitHub issues are created by this record. The implementation packets must
not add another edition selector: the immutable graph policy is the single
source of truth, while manifest/catalog metadata is only the authority that
constructs a strict policy for a conformance run.

## Consequences

- Ordinary parsing remains compatibility-oriented and can process mixed graphs
  without treating user labels as language declarations.
- Conformance runs become explicit and reproducible because the catalog selects
  a strict profile from pinned metadata before parsing.
- A strict profile can report a located, feature-linked unsupported diagnostic
  instead of silently interpreting a source label or returning a partial schema.
- Future XSD edition differences require an explicit Compatibility rule or an
  explicit unsupported diagnostic; the parser does not guess from labels.
- The public schema model remains immutable and source identities, contexts,
  namespaces, and locations retain their existing opaque responsibilities.
