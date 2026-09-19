# 0003: Manifest-scoped schema bootstrap boundary

Status: accepted

## Decision

Bootstrap is a reproducible corpus-materialization boundary, not a claim that
the current parser can consume the complete W3C schema-for-schemas documents.
For one requested XSD version, [`Manifest.BootstrapPlan`](../../internal/specs/bootstrap.go)
selects only bootstrap artifacts tagged with that version, requires exactly one
entry artifact, requires every dependency to be another selected bootstrap
artifact, and orders the graph dependency-first with manifest order as the
tie-breaker. [`GenerateBootstrap`](../../internal/specs/bootstrap.go) then
fetches and converts those entries sequentially. A failed materialization
returns no partial document list and retains the existing corpus error and
cause.

This boundary is accepted because it makes the pinned prerequisite graph
explicit and testable while leaving syntax, discovery, declaration, and
immutable-schema phases unchanged. It does not parse or silently approximate
unfinished schema-for-schemas behavior.

## Normative and artifact evidence

The XSD 1.0 Part 1 [schema document definition](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#key-schemaDoc),
[schema-for-schemas appendix](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#normative-schemaSchema),
and [schema component XML mappings](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#components)
establish the XML vocabulary and global declaration representations. XSD 1.1
retains the [schema document model](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#key-schemaDoc)
and [normative schema for schema documents](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#normative-schemaSchema),
but its [composition model](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#composition)
also covers conditional inclusion, `override`, and the changed composition
rules; its schema vocabulary includes [`defaultOpenContent`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#element-defaultOpenContent)
and assertion-related constructs. These differences are requirements to track,
not current parser-support claims.

The pinned manifest is the artifact evidence:

| Requirement | Pinned artifact and representation | Implementation boundary |
| --- | --- | --- |
| XSD 1.0 Part 1 schema document | `xsd10-schema-for-schemas`, `xml`, `entry: true` | Selected for version `1.0`; its manifest prerequisites are `xsd10-datatypes-schema` and `xml-schema`. |
| XSD 1.1 Part 1 schema document | `xsd11-schema-for-schemas`, `xml`, `entry: true` | Selected for version `1.1`; its manifest prerequisites are `xsd11-datatypes-schema` and `xml-schema`. |
| XSD 1.0 Part 2 schema for datatype declarations | `xsd10-datatypes-schema`, `html-cdata-pre-xsd10-datatypes` | After the raw SHA-256 check, remove only the exact wrapper, require the pinned DTD/declaration envelope, and move the one declaration through `?>` before the unchanged DTD; complete converted-document XML validation follows. |
| XSD 1.1 Part 2 schema for datatype declarations | `xsd11-datatypes-schema`, `xml` | Consumed as the verified XML representation. |
| XML namespace declarations used by both graphs | `xml-schema`, `xml`, `xsdVersions: ["1.0", "1.1"]` | The manifest alias `http://www.w3.org/2001/xml.xsd` points to the pinned `xml-schema` artifact at [`https://www.w3.org/2001/xml.xsd`](https://www.w3.org/2001/xml.xsd). The lexical location remains unchanged for the resolver. |

The Part 2 specifications describe the datatype vocabulary and their schema
documents in the [XSD 1.0 datatype appendix](https://www.w3.org/TR/2004/REC-xmlschema-2-20041028/datatypes.html#schema)
and [XSD 1.1 datatype appendix](https://www.w3.org/TR/2012/REC-xmlschema11-2-20120405/datatypes.html#schema).
The IDs, URLs, SHA-256 digests, versions, dependencies, representations, and
alias above are pinned in [`specs/manifest.json`](../../specs/manifest.json);
the plan tests assert the real graph and order from that file.

## Kernel boundary

The syntax, discovery, declaration, location, ordering, and immutable-component
invariants are canonical in [ARCHITECTURE.md](../../ARCHITECTURE.md). This
decision does not duplicate those implementation details or add syntax-phase
support. [`BootstrapPlan`](../../internal/specs/bootstrap.go) operates on
manifest records and verified corpus representations; it does not mutate or
backpatch completed schema components. [`GenerateBootstrap`](../../internal/specs/bootstrap.go)
does not claim complete parsing of either schema-for-schemas document.

## Regeneration boundary and deferred breadth

The reproducible path is:

1. [`Fetch`](../../internal/specs/corpus.go) obtains the raw response and
   verifies its pinned SHA-256 digest.
2. [`convert`](../../internal/specs/corpus.go) applies exactly the manifest
   representation conversion: verified `xml`, ordinary `html-cdata-pre`, or
   the strict `html-cdata-pre-xsd10-datatypes` wrapper removal and declaration
   relocation before complete XML validation.
3. [`BootstrapPlan`](../../internal/specs/bootstrap.go) materializes the
   selected bootstrap-artifact subgraph in deterministic dependency-first
   order; [`GenerateBootstrap`](../../internal/specs/bootstrap.go) performs
   the calls sequentially.
4. [`internal/specs/bootstrap_test.go`](../../internal/specs/bootstrap_test.go)
   regenerates both version fixtures twice, checks every generated data slice
   byte-for-byte, and exercises XML plus the XSD 1.0 datatype representation.
   Ordinary `html-cdata-pre` conversion is covered by
   [`internal/specs/corpus_test.go`](../../internal/specs/corpus_test.go).

The implementation and focused tests support manifest selection, prerequisite
closure, cycle/missing/out-of-version diagnostics, copied plan entries, raw
digest and representation handling through the existing generator, no-partial-
output failure behavior, and byte-identical repeated fixture generation.

Explicit prerequisites are the two version-specific datatype artifacts and
the shared `xml-schema` artifact listed by each entry in the manifest. The
datatype prerequisite is a manifest planning dependency; it is not an XML
include/import edge. Outside this boundary are parsing the full pinned
documents, QName/reference resolution, derivation, substitution, assertions,
override/default-open-content semantics, complete particles and content
models, and full XSD 1.0/1.1 conformance. The implementation does not silently
accept those behaviors as supported.
