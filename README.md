# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema` returns immutable components from `ResolvedSource`/`Resolver`; Compatibility is default; locations are opaque.

`openContent=none` works in Compatibility/Strict11, not Strict10. Bounded attribute-free extensions use named empty-content bases or `xs:anyType` restrictions, retaining `##other`/`lax` and model-less identity. Scalar simpleContent extensions retain base/type/use `Loc`s and nil particles; restrictions, attribute-bearing complexContent/attributeGroup extensions are unsupported. Scalar simpleContent is query-only; consumers reject.
`AttributeUse` retains ordered locations and reference/use facts. Boolean/integer/decimal and policy-gated precisionDecimal are admitted; simpleContent also admits string; both exclude unsignedLong. Forms and matching XSD 1.1 `targetNamespace` apply. Attribute/simpleContent consumers reject; value/default/fixed/inheritable remain unsupported.
Applicable syntax, occurrence, reference, and policy gates precede mapping. Direct/supported extension choices/sequences omit effective `0/0` local public particles only after them; named/inline non-reference mapping is not universal for `0/0`. Graph-wide declaration/facet failures surface; sequence owners skip children; choices resolve refs first; groups resolve/check before owner/child omission; child refs resolve first. Strict10 `precisionDecimal` and direct reference checks precede omission; long-family/unsignedLong `0/0` absent.
Element/model-group refs retain QName/RefLoc/TargetID/order; eligible direct-choice refs consumable; model-group/excluded refs consumer-rejected. Nonzero `xs:any` queryable; wildcard consumers reject; broader unsupported; `0/0` absent. Global long/unsignedLong retain bounds/facets/locations/ownership; consumers reject.
All policies admit local built-in/named-effective `xs:unsignedLong` in direct choices/sequences and supported attribute-free extension choices/sequences: exact-finite/`unbounded`/above-`uint64` occurrences; bounds/facets/IDs/ownership/provenance/locations retained; query-only/consumer-rejected; excluded mapped nonzero forms: located `FailureUnsupported`, no schema.
Direct `xs:negativeInteger` rejects mapped nonzero; named/anonymous-inline forms are query-only. Compatibility/Strict11 admit direct precisionDecimal sequences, default choices, and bounded extension choices. Inline complexes expose anonymous IDs, refs, and uses; bounded list/union links and global inline restrictions are query-only. Non-default precisionDecimal choices and local inline precisionDecimal remain unsupported; Strict10 rejects before omission. Consumers reject unsupported shapes.

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
