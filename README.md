# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works Compatibility/Strict11, mismatches Strict10. Extensions need named empty-content bases; model-less preserves identity/locations. `xs:any`/`anyAttribute` retain facts; admitted `0/0` absent.
Refs retain QName/RefLoc/target/order; model-group refs query; broader unsupported. Long refs retain bounds; malformed invalid. Global `nonNegativeInteger` refs query; consumers reject. `precisionDecimal` element/type facts query under Compatibility/Strict11; Strict10 rejects before validation; roots validate, inline-element targets query-only.
`precisionDecimal` attributes query-only in Compatibility/Strict11 (built-in/supported named); Strict10 rejects at type `Loc`.
`xs:long`/`xs:unsignedLong` attributes (including named) admitted all policies; bounds/facets/locations queryable; consumers reject. Unsupported defaults/fixed use `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc`; default+fixed uses `FailureInvalid`/`XSD3010` (fixed primary, default related, no `Schema`).
Local built-in/named-effective `xs:long` particles in direct choices/sequences and bounded attribute-free extensions are queryable under Compatibility, Strict10, and Strict11; consumers reject. Mapped non-`0/0` unsupported inline long/other local integer forms reject; effective `0/0` is absent after validation; invalid/unresolved diagnostics remain errors. Global inline elements: Boolean/integer/decimal query-only; string/token/NMTOKEN generation-eligible; attributes query-only; inline-attribute consumers excluded; `GenerateGo` rejects all attribute declarations.
`precisionDecimal` locals under Compatibility/Strict11 admit default choices/bounded attribute-free extensions; `0/0` absent, Strict10 first. Extensions query-only; consumers reject; targets GenerateGo-rejected.
GenerateGo: global/named-typed `nonNegativeInteger` elements and standalone named types generate under all policies; built-in fields use `StrictInteger`, named fields generated types. Element abstract/nillable or named final/variety/facet gates reject (`GOXSD9029`); malformed/stale facts reject (`GOXSD9030`); inline/anonymous forms query-only/consumer-rejected.

Named complex `abstract` is non-inherited; `Final()` uses declaring-document `finalDefault` without
local `final`; explicit empty/non-empty locals override it; `FinalLoc()` preserves
local/default provenance—see [Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`,
`validate`, and `generate` available; parse prints, validate silent;
invalid exits 1, usage exits 2.

## Design goals

Exact values/facets, streaming, deterministic queries, located diagnostics;
no goroutines/locks/map-order output, conformance.

## Repository checks

Fresh checkout; bounded conformance needs exact version, `-set`, `-case`; never run instances:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Pinned specification corpus

```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query QUERY
go tool specs bootstrap -version VERSION
```
Use `-root`/`-output`/`-index`; bootstrap previews only.

## Project workflow

See [Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
