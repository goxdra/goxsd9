# goxsd9

goxsd9 parses/validates/generates Go; unsupported explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works Compatibility/Strict11, not Strict10; named empty-content extension bases. Direct non-`0/0` `xs:any` terms queryable, consumers reject; only validated omittable `0/0` disappears. `anyAttribute` retains facts/no particle occurrence; broader wildcards retain diagnostics.
Refs retain QName/RefLoc/target/order; groups query, broader reject. `long` refs bounded. `nonNegativeInteger` refs query-only. `precisionDecimal` types query in Compatibility/Strict11; Strict10 rejects; roots validate, inline targets query-only.
`precisionDecimal` attributes query-only under Compatibility/Strict11; Strict10 rejects at type `Loc`.
`xs:long`/`xs:unsignedLong` attrs queryable; consumers reject. Unsupported default/fixed: located `FailureUnsupported`; default+fixed `FailureInvalid`/`XSD3010` (fixed primary/default related).
Local `xs:long`/effective-`negativeInteger`/effective-long particles in direct choices/sequences and bounded attr-free extensions are query-only under Compatibility/Strict10/Strict11; consumers reject. Allowlist: direct `xs:integer`; named/inline effective `integer`/`negativeInteger`; built-in/named effective-long; direct `xs:negativeInteger` excluded. Published non-`0/0` inline-long/out-of-slice/list/union/structural forms return located `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported`; after syntax/occurrence/input validation, only exact `0/0` mapped forms may omit; failures retain diagnostics/causes/no `Schema`. Inline Boolean/integer/decimal query-only; global string/token/NMTOKEN generate. Global attrs query-only; local/inline reject; `GenerateGo` rejects attrs.
`precisionDecimal`: syntax/occurrence/input validation first; malformed occurrence `FailureInvalid` at own `Loc`; Strict10 rejects typed `Loc` (`FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`) before `0/0`. Compatibility/Strict11 require default occurrences for mapped owners and typed `precisionDecimal` children/alternatives; non-precision alternatives may retain non-default query-only ranges. Default choices validate; extensions query-only. Reject mapped non-`0/0` direct/extension sequences, non-default mapped owners or typed `precisionDecimal` alternatives, and inline/anonymous forms; valid mapped `0/0` omits after validation. Anonymous excluded; `GenerateGo` rejects all.
`GenerateGo` supports global/named `nonNegativeInteger` elements/types; fields `StrictInteger`/generated. Abstract/nillable/final/variety/facet gates reject (`GOXSD9029`); malformed facts reject (`GOXSD9030`); inline/anonymous query-only.

Named complex `abstract`/finality preserve provenance; see [Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`/`validate`/`generate`
available; parse prints, validate silent;
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
