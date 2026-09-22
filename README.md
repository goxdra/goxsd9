# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11; mismatches Strict10. Attribute-free extensions remain limited to named empty-content bases; model-less keeps base identity/locations and representable inherited `##other`/lax. Particle-plus-uses and attribute-only bodies expose ordered scalar local/reference/anonymous-inline `AttributeUse` facts; bounded scalar `simpleContent` extensions retain their base/type references and uses without a particle. Optional/required uses are effective, prohibited uses are omitted, and local value/default/fixed/inheritable semantics, attribute consumers, and broader forms remain unsupported. `xs:any`/`anyAttribute` keep facts; `0/0` absent; broader placements unsupported.
Element refs retain QName/`RefLoc`/target/order/occurrences; only top-level direct named model-group refs query; nested/local/recursive/broader refs unsupported. Global long-family refs query-only with exact bounds; malformed refs invalid. Global built-in/named/inline `precisionDecimal` query under Compatibility/Strict11; Strict10 rejects all before validation. Only built-in/named roots validate; global inline query-only; consumers reject.
Local anonymous Boolean/integer/decimal forms are query-only in direct choice/sequence and bounded extensions; validation/`GenerateGo` reject. Mapped nonzero anonymous string/token/NMTOKEN/`precisionDecimal` and non-string enums are unsupported at type/facet `Loc`; no schema.
`precisionDecimal`: Compatibility/Strict11 admits built-in `xs:precisionDecimal` or named-effective local types only in default direct choices/bounded attribute-free extension choices. Owner and each mapped typed child/alternative require default occurrences; nonprecision alternatives may query. Mapped inline anonymous forms unsupported. Compatibility/Strict11 omit `0/0`; Strict10 rejects first, including zero. Non-default choices/nonzero direct/extension sequences unsupported; only non-extension default choices validate; extension/anonymous consumers reject.
GenerateGo supports global built-in/named Boolean/integer/decimal/string/token/NMTOKEN and inline string/token/NMTOKEN; only non-extension default-occurrence refs to global Boolean/integer/decimal. Global inline long-family/identity-only facts query-only; mapped local forms unsupported. Compatibility/Strict11: `precisionDecimal` queryable, but `GenerateGo` rejects every global, explicitly typed local (including named-effective), inline, anonymous, and schema-admitted extension target; no schema/output.

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
