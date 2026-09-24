# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11 and mismatches Strict10. Extensions need named empty-content bases; model-less preserves identity. `xs:any`/`anyAttribute` keep facts; admitted `0/0` is absent.
Particle-plus-use bodies (including direct model-group refs), attribute-only bodies, and bounded scalar `simpleContent` extensions expose ordered, defensive `AttributeUse` views; anonymous local types retain identity, optional/required uses are effective, and prohibited uses are omitted. Local/ref uses admit only Boolean/integer/decimal, plus policy-gated `precisionDecimal`; `xs:int` and other scalar kinds are unsupported. Explicit `form` or `attributeFormDefault` selects qualified/unqualified names; chameleon includes adopt the including namespace. Validation/GenerateGo reject these consumers with no schema on error.
Refs retain QName/RefLoc/target/order; broader refs unsupported. `precisionDecimal` element/type facts query under Compatibility/Strict11; Strict10 rejects before validation; roots validate, inline targets query-only. Global attributes are query-only; `GenerateGo` rejects them.
`xs:long`/`xs:unsignedLong` global attributes (built-in/named) are separately admitted under all policies with bounds `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`; consumers reject. `precisionDecimal` attributes are query-only in Compatibility/Strict11; Strict10 rejects at type `Loc`.
Local `nonNegativeInteger` non-0/0 rejects; `0/0` absent. GenerateGo supports global/named `nonNegativeInteger` elements and named types; inline/anonymous forms remain query-only.

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
