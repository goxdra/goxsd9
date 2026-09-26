# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema` returns immutable components from `ResolvedSource`/`Resolver`; Compatibility is default; locations are opaque.

`openContent=none` works in Compatibility/Strict11, not Strict10. Bounded attribute-free extensions use named empty-content bases or `xs:anyType` restrictions, retaining `##other`/`lax` and model-less identity. Scalar simpleContent extensions retain base/type/use `Loc`s and nil particles; restrictions and broader attribute-bearing complexContent/attributeGroup extensions are unsupported. Scalar simpleContent is query-only; consumers reject.
`AttributeUse` facts retain order, locations, names, references, ownership, and effective use. Local uses support Boolean/integer/decimal and policy-gated precisionDecimal; simpleContent also supports string. Forms and chameleon namespaces apply; prohibited uses disappear. Consumers reject attribute/simpleContent facts; value constraints and unsignedLong remain unsupported.
Named complexContent extensions compose one opaque group reference, ordered local uses, and a supported named empty base. Resolution follows group, uses, base. Validated `0/0` removes the particle; consumers reject.
Syntax, occurrence, reference, and policy checks precede `0/0` omission. Choices resolve refs first, groups validate targets first, and sequences skip children. Graph-wide declaration/facet failures surface; named/inline non-reference mapping is not universal at `0/0`. Strict10 `precisionDecimal` rejects before omission.
Element/model-group refs retain QName/RefLoc/TargetID/order; eligible direct-choice refs consumable; model-group/excluded refs consumer-rejected. Nonzero `xs:any` queryable; wildcard consumers reject; broader unsupported; `0/0` absent. Global long/unsignedLong retain bounds/facets/locations/ownership; consumers reject.
All policies admit local built-in/named-effective `xs:unsignedLong` in direct choices/sequences and supported attribute-free extension choices/sequences: exact-finite/`unbounded`/above-`uint64` occurrences; bounds/facets/IDs/ownership/provenance/locations retained; query-only/consumer-rejected; excluded mapped nonzero forms: located `FailureUnsupported`, no schema.
Direct `xs:negativeInteger` rejects mapped nonzero; named/anonymous-inline query-only. `precisionDecimal`: Compatibility/Strict11 admit default direct/bounded attribute-free extension choices; extension choices are consumer-only. Mapped non-default choices, nonzero sequences, and mapped nonzero inline/anonymous forms are schema-unsupported; Strict10 precedes `0/0` omission.

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
