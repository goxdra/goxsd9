# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema` returns immutable components from `ResolvedSource`/`Resolver`; Compatibility is the default and locations are opaque.

`openContent=none` works in Compatibility/Strict11 and mismatches Strict10. Bounded attribute-free complexContent extensions use named empty-content bases or named complexContent restrictions over built-in `xs:anyType`, retaining `##other`/`lax` and model-less identity. Scalar simpleContent extensions retain base/type/use `Loc`s and a nil particle; restrictions, attribute-bearing, and `attributeGroup` extensions are unsupported.
Particle-plus-use, direct model-group, attribute-only, and simpleContent bodies expose ordered local, referenced, and anonymous-inline `AttributeUse` facts with locations, identity/ownership, QName/RefLoc/TargetID, and effective use. Forms select names; matching XSD 1.1 `targetNamespace` and chameleon adoption apply; prohibited uses are omitted. Value/default/fixed/inheritable semantics and consumers reject; excluded references preserve locations and return no schema.
Syntax, occurrence, reference, and policy gates precede mapping. After them, `0/0` maps absent: sequence owners skip children; choices resolve refs before child omission without duplicate checks; named groups resolve/check before owner/child omission; child refs resolve before omission. Strict10 precisionDecimal comes first; long-family/unsignedLong `0/0` is absent.
Element/model-group references are separate query-only facts retaining QName/RefLoc/TargetID/order without expansion. Nonzero `xs:any` is queryable, wildcard consumers reject, broader forms are unsupported, and `0/0` is absent. Global long/unsignedLong facts are query-only with exact bounds/facets/locations/ownership; consumers reject.
Local built-in/supported named-effective `xs:unsignedLong` is queryable under every policy only in direct or extension choices/sequences. Retain exact bounds/facets/locations/IDs/ownership/provenance and unbounded/above-`uint64` occurrences. Mapped nonzero exclusions return `FailureUnsupported` at type/facet/element or nested-particle `Loc`; no schema. Consumers reject it.
Direct built-in `xs:negativeInteger` rejects mapped nonzero; named/anonymous-inline forms are query-only. Local inline scalars remain bounded. `precisionDecimal` admits only default direct/bounded extension choices in Compatibility/Strict11; Strict10 first; nondefault/nonzero/inline forms reject and `0/0` is absent.

## CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md): CLI `parse`,
`validate`, `generate`; parse prints, validate silent; invalid 1, usage 2.

## Goals

Exact values/facets, streaming, deterministic queries; no goroutines/locks/map-order output.

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
