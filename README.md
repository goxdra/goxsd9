# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11 and mismatches Strict10. Extensions need named empty-content bases; model-less preserves identity/locations. `xs:any`/`anyAttribute` keep facts; admitted `0/0` is absent. Strict10 rejects `precisionDecimal` before omission.
Refs retain QName/RefLoc/target/order/occurrences without target gating; model-group refs query, broader refs unsupported. Long-family refs retain bounds; malformed refs invalid. Global `nonNegativeInteger` references remain queryable; direct-choice/sequence consumers reject them with located unsupported diagnostics/no output. `precisionDecimal` facts query under Compatibility/Strict11; Strict10 rejects before validation; roots validate, inline query-only.
`precisionDecimal` attributes are query-only under Compatibility/Strict11; Strict10 rejects at type `Loc`; local/inline consumers are unsupported.
Admitted supported anonymous forms are query-only; mapped non-0/0 local declared/inline/anonymous `nonNegativeInteger` forms are rejected at schema construction (no schema), while exact local 0/0 forms are omitted under every policy. Other unsupported forms report type/facet `Loc`.
`precisionDecimal` locals: Compatibility/Strict11 admits built-in/named-effective types only in default choices/bounded attribute-free extensions with default occurrences; other local consumers reject. Compatibility/Strict11 omits `0/0`; Strict10 rejects before omission.
GenerateGo-only: global built-in/named `nonNegativeInteger` elements generate under all policies only for ordinary (`abstract=false`, `nillable=false`); either flag is unsupported (`FailureUnsupported`/`GOXSD9029`, nil). Built-in/named fields use `StrictInteger`; stale facts fail closed (`FailureInternal`/`GOXSD9030`, nil). Global inline/anonymous forms are query-only; global references remain queryable without target gating, while consumers are unsupported/no output.

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
