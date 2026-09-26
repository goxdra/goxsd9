# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema` returns immutable components from `ResolvedSource`/`Resolver`; Compatibility is default; locations are opaque.

`openContent=none` works in Compatibility/Strict11, not Strict10. Bounded attribute-free extensions use named empty-content bases or `xs:anyType` restrictions, retaining `##other`/`lax` and model-less identity. Scalar simpleContent extensions retain base/type/use `Loc`s and nil particles; restrictions and attribute-bearing complexContent/attributeGroup extensions are unsupported. Consumers reject scalar simpleContent.
`AttributeUse` in particle/model-group/attribute-only/simpleContent bodies retains order, locations, ownership, QName/RefLoc/TargetID, and use. It admits Boolean/integer/decimal plus policy-gated precisionDecimal; simpleContent also admits string. Local `int`/`unsignedLong` are schema-unsupported in both. Forms and matching XSD 1.1 `targetNamespace` select names; chameleon adoption applies; prohibited uses vanish. Consumers reject attributes/simpleContent; values/default/fixed/inheritable remain unsupported; excluded refs return no schema with locations.
Syntax, occurrence, reference, and policy gates precede mapping and effective `0/0` omission. Named/inline mapping is not universally skipped at `0/0`; graph-wide declaration/facet failures surface. Sequence owners skip children; choices resolve refs first; groups resolve/check before omission; child refs resolve first. Strict10 `precisionDecimal` checks precede omission.
Element/model-group refs retain QName/RefLoc/TargetID/order; eligible direct-choice refs consumable; model-group/excluded refs consumer-rejected. Nonzero `xs:any` queryable; wildcard consumers reject; broader forms unsupported; `0/0` absent. Global long/int/unsignedLong retain bounds/facets/locations; consumers reject.
Direct and supported bounded attribute-free extension choices/sequences admit local built-in/named-effective `xs:int` and `xs:unsignedLong` under all policies. Exact occurrences, bounds, facets, IDs, locations, ownership, and graph provenance survive; consumers reject. Built-in `int` has zero component ID and intrinsic bounds without source `Loc`; inline forms remain unsupported.
Direct `xs:negativeInteger` rejects mapped nonzero; named/anonymous-inline query-only. `precisionDecimal`: Compatibility/Strict11 admit default direct/bounded attribute-free extension choices; extension choices are query-only/consumer-rejected. Mapped non-default choices, nonzero sequences, and mapped nonzero inline/anonymous forms are schema-unsupported; Strict10 precedes `0/0` omission.

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
