# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works Compatibility/Strict11, not Strict10; extensions require named empty-content bases; model-less identity/locations retained. `xs:any`/`anyAttribute` retain facts; effective `0/0` absent.
Refs retain QName/RefLoc/target/order; model-group query, broader reject. Long refs retain bounds; malformed invalid. `nonNegativeInteger` refs query-only; consumers reject. `precisionDecimal` types query under Compatibility/Strict11; Strict10 first; roots validate, inline targets query-only.
`precisionDecimal` attributes query-only under Compatibility/Strict11; Strict10 rejects at type `Loc`.
`xs:long`/`xs:unsignedLong` attributes queryable under all policies; consumers reject. Unsupported default/fixed use located `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported`; default+fixed `FailureInvalid`/`XSD3010` (fixed primary/default related).
Local built-in/named-effective `xs:long` particles in direct choices/sequences and bounded attr-free extensions queryable under all policies; consumers reject. Integer allowlist: direct `xs:integer`; named/inline effective `integer`/`negativeInteger` (not direct `xs:negativeInteger`). Valid non-`0/0` mapped exclusions—inline `long`, `int`, `unsignedLong`, `nonNegativeInteger`, `nonPositiveInteger`, list/union—return `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at `Loc`; nested/broader shapes are structural exclusions. Only successfully validated omittable mapped `0/0` is absent; invalid, unresolved, cyclic, wrong-kind, value-constraint, and policy errors retain diagnostics/causes/locations; no `Schema`. Inline Boolean/integer/decimal query-only; string/token/NMTOKEN generate; attributes query-only; `GenerateGo` rejects attributes.
`precisionDecimal`: Strict10 rejects at typed `Loc` (`FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`); Compatibility/Strict11 validate only default-occurrence direct choices, requiring defaults for owner and every typed child/alternative. Bounded extensions are query-only; consumers reject. Non-`0/0` inline/anonymous, non-default choices/alternatives, and non-`0/0` direct sequences are unsupported; validated `0/0` is absent. `GenerateGo` rejects all targets.
`GenerateGo` supports global/named `nonNegativeInteger` elements and standalone types; fields use `StrictInteger`/generated types. Abstract/nillable/final/variety/facet gates reject (`GOXSD9029`); malformed facts reject (`GOXSD9030`); inline/anonymous query-only.

Named complex `abstract`/finality preserve provenance; see [Architecture](ARCHITECTURE.md).
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
