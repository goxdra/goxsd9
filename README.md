# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema` returns immutable components from `ResolvedSource`/`Resolver`; Compatibility is default; locations are opaque.

`openContent=none` works in Compatibility/Strict11, not Strict10. Bounded attribute-free extensions use named empty-content bases or restrictions over `xs:anyType`, retaining `##other`/`lax` and model-less identity. Scalar simpleContent extensions retain base/type/use `Loc`s and a nil particle; restrictions, attribute-bearing, and `attributeGroup` extensions are unsupported.
`AttributeUse` facts in particle-plus-use, model-group, attribute-only, and simpleContent bodies preserve order, locations, ownership, QName/RefLoc/TargetID, and effective use. Forms select names; matching XSD 1.1 `targetNamespace`/chameleon adoption apply; prohibited uses are omitted. Consumers reject value/default/fixed/inheritable semantics; excluded references preserve locations and return no schema.
Named-type/facet, syntax, occurrence, reference, and policy gates precede mapping. Direct and supported extension choices/sequences omit effective `0/0` local particles only after those gates; sequence owners skip children; choices resolve refs before omission; named groups resolve/check before owner/child omission; child refs resolve first. Strict10 precisionDecimal and direct reference checks precede omission; long-family/unsignedLong `0/0` is absent.
Element/model-group references are query-only facts retaining QName/RefLoc/TargetID/order without expansion. Nonzero `xs:any` is queryable; wildcard consumers reject; broader forms unsupported; `0/0` absent. Global long/unsignedLong facts retain exact bounds/facets/locations/ownership; consumers reject.
Under every policy, local built-in/supported named-effective `xs:unsignedLong` is queryable only in direct/supported attribute-free extension choices/sequences. Preserve exact inclusive/exclusive bounds, integer enumeration/digit facets, locations, IDs, ownership/provenance, and finite, unbounded, or above-`uint64` occurrences. Mapped nonzero exclusions return located `FailureUnsupported`; no schema. Inline/anonymous forms are a separate mapped nonzero schema exclusion; admitted built-in/named-effective forms are query-only and consumer-rejected.
Direct built-in `xs:negativeInteger` rejects mapped nonzero; named/anonymous-inline forms are query-only. `precisionDecimal` admits only default direct/bounded extension choices in Compatibility/Strict11; Strict10 first; nondefault/nonzero/inline forms reject and `0/0` is absent.

## CLI

CLI `parse`, `validate`, `generate`; parse prints, validate silent; invalid 1, usage 2.

## Goals

Exact values/facets and deterministic queries; no goroutines/locks/map-order output.

## Checks

Fresh checkout; conformance needs exact version/`-set`/`-case`; never run instances:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Corpus

```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query QUERY
go tool specs bootstrap -version VERSION
```
Use `-root`/`-output`/`-index`; bootstrap previews only.

## Project workflow

[Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
