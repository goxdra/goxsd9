# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works in Compatibility/Strict11; Strict10 mismatches. Attribute-free extensions use named empty bases; model-less retains base/locations and inherited `##other`/lax. Particle-plus-use/attribute-only bodies expose ordered local/ref/inline `AttributeUse` facts for Boolean/integer/decimal plus `precisionDecimal` under Compatibility/Strict11 (Strict10 policy diagnostic); refs retain QName/`RefLoc`/target/use. Scalar `simpleContent` has Boolean/string/integer/decimal bases plus policy-gated `precisionDecimal` with ordered uses. Optional/required effective; prohibited omitted; attribute/simpleContent, value/default/fixed/inheritable semantics unsupported. `xs:any`/`anyAttribute`; `0/0` absent; placements unsupported.
Element refs keep QName/`RefLoc`/target/order/occurrences; top-level direct model-group refs query; nested/local/recursive/broader unsupported. Global long-family refs keep bounds; malformed invalid. Global `precisionDecimal` query under Compatibility/Strict11; Strict10 rejects. Built-in/named roots validate; inline query-only; consumers reject. Local anonymous Boolean/integer/decimal query-only in direct choices/sequences and bounded extensions; consumers reject. Mapped nonzero anonymous string/token/NMTOKEN/`precisionDecimal` and non-string enums unsupported at type/facet `Loc`.
`precisionDecimal` element/particle mappings: Compatibility/Strict11 admits built-in/named-effective locals only in default direct or bounded attribute-free extension choices; owners and typed alternatives need defaults, nonprecision query. Inline anonymous mappings and nonzero direct/extension sequences unsupported; Compatibility/Strict11 omits `0/0`, Strict10 rejects mapped forms first, including zero. Only non-extension defaults validate; extension/anonymous consumers reject.
GenerateGo supports global built-in/named Boolean/integer/decimal/string/token/NMTOKEN and inline string/token/NMTOKEN; only non-extension default-occurrence refs to global Boolean/integer/decimal. Global inline long-family/identity facts query; local mappings unsupported. `precisionDecimal` queries under Compatibility/Strict11, but `GenerateGo` rejects global, typed local, inline, anonymous, and extension targets.

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
Use `-root`/`-output`/`-index`; bootstrap previews.

## Project workflow

See [Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
