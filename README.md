# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11, mismatches Strict10. Attribute-free extensions require named empty-content bases; model-less preserves identity/locations and inherited `##other`/lax. `xs:any`/`anyAttribute` keep facts; `0/0` absent; broader unsupported.
Refs retain QName/RefLoc/target/order/occurrences; only top-level named model-group refs query; nested/local/recursive/broader unsupported. Long-family refs query exact bounds; malformed refs invalid. precisionDecimal element/type facts query under Compatibility/Strict11; Strict10 rejects before validation; built-in/named roots validate, inline query-only; consumers reject.
Built-in/named-effective precisionDecimal attributes query-only: default/fixed AttributeValueConstraint retains kind, collapsed lexical spelling, source Loc, and exact defensive StrictPrecisionDecimal via PrecisionDecimalValue() under Compatibility/Strict11. Strict10 rejects at type Loc; inline/local attributes and validation/GenerateGo unsupported.
Local anonymous Boolean/integer/decimal forms query-only; validation/GenerateGo reject. Mapped nonzero anonymous string/token/NMTOKEN/precisionDecimal and non-string enums unsupported at type/facet Loc; no schema.
precisionDecimal locals: Compatibility/Strict11 admits built-in or named-effective types only in default choices/bounded attribute-free extension choices; owners/mapped typed alternatives require default occurrences. Inline, non-default/nonzero sequences, and extension/anonymous consumers reject. Compatibility/Strict11 omits 0/0; Strict10 rejects first.
GenerateGo-only: global built-in/named `nonNegativeInteger` elements generate in Compatibility/Strict10/Strict11; named forms pass final/variety/effective-facet gates; nonempty final controls unsupported with no `GenerateGo` output. Fields use `StrictInteger`; named fields retain named types. Non-0/0 local forms fail schema construction with no schema; global inline/anonymous forms retain query facts; `GenerateGo`/`ValidateInstance` reject them. Other consumers unsupported/no output; scalar/precisionDecimal exclusions remain.

Named complex `abstract` is non-inherited; `Final()` uses declaring-document `finalDefault` if
no local `final`; explicit empty/non-empty locals override it; `FinalLoc()` preserves
local/default provenance—see [Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`,
`validate`, and `generate` available; parse prints, validate silent;
invalid exits 1, usage exits 2.

## Design goals

Exact values/facets, streaming input, deterministic queries, located diagnostics;
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
