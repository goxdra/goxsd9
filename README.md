# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11 and mismatches Strict10. Attribute-free extensions only over named empty-content bases; model-less keeps base identity/locations and representable inherited `##other`/lax. `xs:any`/`anyAttribute` keep facts; `0/0` absent; broader placements unsupported.
Element refs retain QName/`RefLoc`/target/order/occurrences; only top-level direct named model-group refs query; nested/local/recursive/broader refs unsupported. Global long-family refs are query-only with exact bounds; malformed refs invalid. Global built-in/named/inline `precisionDecimal` query under Compatibility/Strict11; Strict10 rejects all before validation. Only built-in/named roots validate; global inline is query-only and consumers reject.
Local anonymous Boolean/integer/decimal forms are query-only in direct choice/sequence and bounded extensions; validation/`GenerateGo` reject. Mapped nonzero anonymous string/token/NMTOKEN/`precisionDecimal` and non-string enums are unsupported at type/facet `Loc`; no schema.
`precisionDecimal`: Compatibility/Strict11 admits built-in `xs:precisionDecimal` or named-effective local types only in default direct choices/bounded attribute-free extensions. The owner and each mapped typed child/alternative require default occurrences; nonprecision alternatives may query. Mapped inline anonymous forms are unsupported. Compatibility/Strict11 omit `0/0`; Strict10 rejects first, including zero. Non-default choices/nonzero direct sequences are unsupported; only non-extension default choices validate; extension/anonymous consumers reject.
GenerateGo supports global built-in/named Boolean/integer/decimal/string/token/NMTOKEN and inline string/token/NMTOKEN; only non-extension default refs to global Boolean/integer/decimal. Global inline long-family/identity-only facts are query-only; mapped local forms unsupported. Compatibility/Strict11: `precisionDecimal` is queryable, but `GenerateGo` rejects every global, explicitly typed local (including named-effective), inline, anonymous, and schema-admitted extension target; no schema/output.

Named complex `abstract` is non-inherited; see [ARCHITECTURE.md](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`,
`validate`, and `generate` are available; parse prints, validate is silent,
invalid exits 1, and usage exits 2.

## Design goals

Exact values/facets, streaming input, deterministic queries, located diagnostics,
no goroutines/locks/map-order output, conformance.

## Repository checks

Fresh checkout; bounded conformance needs exact version, `-set`, and `-case`; never run instances:
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
