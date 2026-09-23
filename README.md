# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11, mismatches Strict10. Attribute-free extensions require named empty-content bases; model-less preserves identity/locations and inherited `##other`/lax. `xs:any`/`anyAttribute` keep facts; `0/0` absent; broader unsupported.
Refs retain QName/RefLoc/target/order/occurrences; only top-level named model-group refs query; nested/local/recursive/broader unsupported. Long-family refs query exact bounds; malformed refs invalid. precisionDecimal element/type facts query under Compatibility/Strict11; Strict10 rejects before validation; built-in/named roots validate, inline query-only; consumers reject.
Built-in/named-effective precisionDecimal attributes query-only: default/fixed AttributeValueConstraint retains kind, collapsed lexical spelling, source Loc, and exact defensive StrictPrecisionDecimal via PrecisionDecimalValue() under Compatibility/Strict11. Strict10 rejects at type Loc; inline/local attributes and validation/GenerateGo unsupported.
Local anonymous Boolean/integer/decimal forms query-only; validation/GenerateGo reject. Mapped nonzero anonymous string/token/NMTOKEN/precisionDecimal and non-string enums unsupported at type/facet Loc; no schema.
precisionDecimal locals: Compatibility/Strict11 admits built-in or named-effective types only in default choices/bounded attribute-free extension choices; owners/mapped typed alternatives require default occurrences. Inline, non-default/nonzero sequences, extension/anonymous consumers reject. Compatibility/Strict11 omits 0/0; Strict10 rejects first.
GenerateGo-only: global built-in/named `nonNegativeInteger` elements generate in Compatibility/Strict10/Strict11 only for ordinary (`abstract=false`, `nillable=false`); either flag true is unsupported (`FailureUnsupported`/`GOXSD9029`, nil). Built-in element fields use `StrictInteger`; named `nonNegativeInteger` types are backed by `StrictInteger`; named-typed elements use the generated named type. Canonical/named facts gate generation; malformed/stale facts fail closed (`FailureInternal`/`GOXSD9030`, nil). Local non-0/0: no schema; global inline/anonymous: query-only/consumer-rejected; consumers unsupported/no output.

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
