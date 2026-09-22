# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works in Compatibility/Strict11; Strict10 mismatches. Attribute-free extensions use named empty bases; model-less retains base/locations and inherited `##other`/lax. Particle-plus-use/attribute-only bodies expose local/ref/inline `AttributeUse`; `AttributeReferenceUse` retains QName/`RefLoc`/target/use and admits only global Boolean/integer/decimal plus policy-admitted `precisionDecimal` (Strict10 policy diagnostic). Valid scalar refs fail construction: schema-syntax `FailureUnsupported`/`ErrUnsupported` at `RefLoc`, related target, no partial schema; unresolved/wrong-kind/inaccessible refs remain invalid. `simpleContent` separately allows Boolean/string/integer/decimal bases plus policy `precisionDecimal` and uses. Optional/required effective; prohibited omitted; attribute/simpleContent, value/default/fixed/inheritable semantics unsupported. `xs:any`/`anyAttribute`; `0/0` absent; placements unsupported.
Element refs retain QName/`RefLoc`/target/order/ranges; model-group refs query; nested/local/recursive/broader unsupported. Long-family bounds query; malformed invalid. Global `precisionDecimal` query under Compatibility/Strict11; Strict10 rejects; built-in/named roots validate, inline query-only, consumers reject. Local anonymous Boolean/integer/decimal query-only; consumers reject. Mapped nonzero anonymous string/token/NMTOKEN/`precisionDecimal` and non-string enums unsupported at type/facet `Loc`.
`precisionDecimal` element/particle mappings admit built-in/named-effective locals only in default direct or bounded attribute-free extension choices under Compatibility/Strict11; owners/typed alternatives need defaults, nonprecision query. Inline anonymous, nonzero direct/extension sequences, and non-default choices unsupported; admitting policies omit `0/0`, Strict10 rejects first. Only non-extension defaults validate; extension/anonymous consumers reject.
`GenerateGo` supports global built-in/named Boolean/integer/decimal/string/token/NMTOKEN and inline string/token/NMTOKEN; only non-extension default refs to global Boolean/integer/decimal. Inline long-family/identity facts query; local mappings unsupported. `precisionDecimal` queries under Compatibility/Strict11; `GenerateGo` rejects global, typed-local, inline, anonymous, extension targets.

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
