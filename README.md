# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11 and mismatches Strict10. Extensions need named empty-content bases; model-less preserves identity/locations. `xs:any`/`anyAttribute` keep facts; admitted `0/0` is absent.
Refs retain QName/RefLoc/target/order/occurrences; model-group refs query, broader refs unsupported. Long-family refs retain bounds; malformed refs invalid. Global `nonNegativeInteger` refs query, but direct-choice/sequence consumers reject. `precisionDecimal` facts query under Compatibility/Strict11; Strict10 rejects before validation; roots validate, inline query-only.
`precisionDecimal` attributes query under Compatibility/Strict11; Strict10 rejects at type `Loc`; local/inline reject.
Global `xs:long`/named atomic-long attributes query all policies: built-in `xs:long` intrinsic inclusive bounds `[-9223372036854775808,9223372036854775807]`; named refs retain exact effective facets/bounds, locations, provenance, target IDs; built-ins no ID. One default/fixed: `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc`; both: `FailureInvalid`/`XSD3010`, fixed primary/default related, no `Schema`. Validation/`GenerateGo`: consumer-rejected.
Local `nonNegativeInteger` non-0/0 forms reject; 0/0 absent. Global inline Boolean/integer/decimal are query-only/consumer-rejected; inline string/token/NMTOKEN generate.
`precisionDecimal` locals under Compatibility/Strict11 admit default choices or bounded attribute-free extensions; `0/0` absent, Strict10 rejects first. Extensions queryable; validation/GenerateGo reject; all targets GenerateGo-rejected.
GenerateGo: global built-in/named-typed `nonNegativeInteger` element declarations generate across policies; standalone named atomic `nonNegativeInteger` components generate. Built-in fields and standalone named `nonNegativeInteger` declarations use `StrictInteger`; named-typed fields use generated types. Element declarations require `abstract=false,nillable=false`; either yields `FailureUnsupported`/`GOXSD9029`, nil. Named final/variety/effective-facet gates unsupported (`FailureUnsupported`/`GOXSD9029`, no output); malformed/stale facts internal (`FailureInternal`/`GOXSD9030`, nil). Global inline/anonymous `nonNegativeInteger`: query-only, consumer-rejected.

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
