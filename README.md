# goxsd9

goxsd9 parses/validates/generates Go; unsupported behavior is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works Compatibility/Strict11, not Strict10; named empty-content extension bases. Direct non-`0/0` `xs:any` terms queryable, consumers reject; only validated omittable `0/0` disappears. `anyAttribute` retains facts/no particle occurrence; broader wildcards retain diagnostics.
Refs retain QName/RefLoc/target/order; groups query, broader reject. `long` refs bounded. `nonNegativeInteger` refs query-only. Global `precisionDecimal` queryable in Compatibility/Strict11; built-in/named roots validate; inline/anonymous consumer-excluded; `GenerateGo` rejects all; Strict10 rejects typed/type `Loc` before `0/0`.
`precisionDecimal` attributes query-only under Compatibility/Strict11; Strict10 rejects at type `Loc`.
`xs:long`/`xs:unsignedLong` attrs queryable; consumers reject. Unsupported default/fixed: located `FailureUnsupported`; default+fixed `FailureInvalid`/`XSD3010` (fixed primary/default related).
Local `xs:long`/effective-`negativeInteger`/effective-long: query-only in direct choices/sequences and bounded attr-free extensions under Compatibility/Strict10/Strict11; consumers reject. Direct `xs:integer`; named/inline effective `integer`/`negativeInteger`; built-in/named effective-long; direct `xs:negativeInteger` excluded. Excluded mapped non-`0/0` forms return located `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported`; after syntax/occurrence checks, valid mapped forms—including schema-unsupported—may omit only at `0/0`. Non-`0/0` built-in/named effective-long remains queryable; invalid/unresolved/cyclic/wrong-kind/value-constraint/policy failures retain diagnostics/causes/no `Schema`. Inline Boolean/integer/decimal query-only; global string/token/NMTOKEN generate; attrs query-only/reject local/inline; `GenerateGo` rejects attrs.
`precisionDecimal`: Strict10 rejects at typed `Loc` before `0/0`. Compatibility/Strict11 require default mapped owners/typed-child/alternative occurrences; other alternatives retain non-default ranges; defaults validate. Extension choices query-only; mapped non-`0/0` extension sequences are schema-unsupported. Reject mapped non-`0/0` direct sequences, non-default owners/typed alternatives, and published local mapped inline/anonymous non-`0/0` forms; valid mapped `0/0` omits after validation. `GenerateGo` rejects all.
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
