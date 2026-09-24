# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works Compatibility/Strict11, not Strict10; extensions require named empty-content bases; model-less identities/locations. Supported direct non-`0/0` `xs:any` terms queryable; validation/`GenerateGo` reject; only validated omittable `xs:any` `0/0` disappears. `anyAttribute` retains facts (no particle occurrence); broader/unsupported wildcards retain located diagnostics.
Refs retain QName/RefLoc/target/order; groups query, broader reject. Long refs retain bounds; malformed. `nonNegativeInteger` refs query-only; consumers reject. `precisionDecimal` types query under Compatibility/Strict11; Strict10 first; roots validate, inline targets query-only.
`precisionDecimal` attributes query-only under Compatibility/Strict11; Strict10 rejects at type `Loc`.
`xs:long`/`xs:unsignedLong` attrs queryable under all policies; consumers reject. Unsupported default/fixed: located `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported`; default+fixed `FailureInvalid`/`XSD3010` (fixed primary/default related).
Local `xs:long` particles in direct/bounded attr-free extensions are query-only; consumers reject. Allowlist: direct `xs:integer`; named/inline effective `integer`/`negativeInteger`; built-in/named effective-long; direct `xs:negativeInteger` excluded. Effective-`negativeInteger` locals query-only; consumers reject. Inline-long/out-of-slice/list/union/structural forms return located `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported`; only validated omittable `0/0` disappears; failures retain located diagnostics/causes/no `Schema`. Inline Boolean/integer/decimal query-only; only global inline string/token/NMTOKEN elements generate. Global attrs query-only; local/inline schema-boundary reject; `GenerateGo` rejects attrs.
`precisionDecimal`: Strict10 rejects at typed `Loc` (`FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`). Compatibility/Strict11 require defaults for mapped `precisionDecimal` owners/children/alternatives; non-precision alternatives can be query-only. Default direct choices validate; extension choices query-only/consumer-rejected. Non-`0/0` direct/extension sequences and non-default direct choices reject; inline/anonymous forms reject; valid omittable `0/0` disappears only after validation. `GenerateGo` rejects every target.
`GenerateGo` supports global/named `nonNegativeInteger` elements/standalone types; fields `StrictInteger`/generated. Abstract/nillable/final/variety/facet gates reject (`GOXSD9029`); malformed facts reject (`GOXSD9030`); inline/anonymous query-only.

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
