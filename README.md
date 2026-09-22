# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11, mismatches Strict10. Attribute-free extensions require named empty-content bases; model-less keeps base identity/locations, inherited `##other`/lax. `xs:any`/`anyAttribute` retain facts; `0/0` absent; broader placements unsupported.
Element refs retain QName/`RefLoc`/target/order/occurrences; only top-level direct named model-group refs query; nested/local/recursive/broader unsupported. Global element/simple-type long-family refs query exact bounds; malformed invalid; global `xs:attribute` `long`/`unsignedLong`/`nonPositiveInteger` remain unsupported. Compatibility/Strict10/Strict11: explicit built-in/supported-named `nonNegativeInteger` global `xs:attribute` types are schema/query-only: exact `minInclusive=0` (named restrictions narrow), written QName, declaration/type `Loc`s, order/provenance retained; built-in no `ComponentID`, named target ID retained. Default/fixed: `FailureUnsupported`/`ErrUnsupported` at constraint `Loc`/no schema; local/inline attrs, validation/`GenerateGo` excluded. Global built-in/named and inline-element `precisionDecimal` facts query under Compatibility/Strict11; Strict10 rejects; only built-in/named roots validate.
Local anonymous Boolean/integer/decimal are query-only in direct choices/sequences/bounded extensions; validation/`GenerateGo` reject. Mapped nonzero anonymous string/token/NMTOKEN/`precisionDecimal` and non-string enums unsupported at type/facet `Loc`; no schema.
`precisionDecimal`: Compatibility/Strict11 admits built-in/named-effective local types only in default direct/bounded extension choices; owner/typed children/alternatives require defaults; nonprecision alternatives query. Inline anonymous unsupported. Compatibility/Strict11 omit `0/0`; Strict10 rejects before omission. Non-default choices/nonzero direct/extension sequences unsupported; default non-extension choices validate; extension/anonymous consumers reject.
GenerateGo supports global built-in/named Boolean/integer/decimal/string/token/NMTOKEN and inline string/token/NMTOKEN; only non-extension default direct-choice refs to global Boolean/integer/decimal. Global inline-element long-family/identity-only query-only; local forms unsupported. Compatibility/Strict11: `precisionDecimal` queryable; `GenerateGo` rejects listed targets; no output.

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
