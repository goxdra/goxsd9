# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

`openContent=none` works under Compatibility/Strict11, mismatches Strict10. Bounded attribute-free extensions use named empty-content bases or named `complexContent` restrictions over `xs:anyType` with representable inherited `##other`/`lax` wildcard facts; model-less query-only with identity/locations; validation/`GenerateGo` reject consumers. `0/0` absent.
Refs retain QName/RefLoc/target/order; model-group refs query. Long bounds remain; malformed invalid. `precisionDecimal` facts query under Compatibility/Strict11; Strict10 rejects first. `nonNegativeInteger` refs query; consumers reject.
Global `xs:long`/`xs:unsignedLong` element/type declarations are query-only under all policies: retain exact bounds `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`, facets, `Loc`s, and graph provenance; validation/`GenerateGo` reject them, separately from refs/attributes.
`precisionDecimal` attributes query-only in Compatibility/Strict11; Strict10 type-Loc rejects.
`xs:long`/`xs:unsignedLong` attributes are admitted under all policies; bounds/facets/locations queryable; consumers reject. Unsupported defaults/fixed use `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported`, no `Schema`; default+fixed uses `FailureInvalid`/`XSD3010` (fixed primary, default related).
Local explicit built-in/supported named-effective `xs:unsignedLong` particles are queryable only in direct choices/sequences and permitted bounded attribute-free extensions under Compatibility/Strict10/Strict11: `0/0` is absent, finite/unbounded/above-`uint64` occurrences stay exact. Inline/anonymous, nested/group/ref, list/union, and narrower shapes are excluded; validation/`GenerateGo` reject admitted particles.
Local `nonNegativeInteger` non-`0/0` rejects; `0/0` absent. Inline Boolean/integer/decimal query-only; inline string/token/NMTOKEN generate. Attributes query-only; `GenerateGo` rejects them.
`precisionDecimal` locals admit default choices/bounded extensions under Compatibility/Strict11; `0/0` absent, Strict10 rejects first; extensions query-only, `GenerateGo` rejects targets.
`GenerateGo`: global built-in/named `nonNegativeInteger` elements and standalone named simple types generate; fields use `StrictInteger` or generated types. Element/final/variety/facet gates are `FailureUnsupported`/`GOXSD9029`; malformed/stale facts are `FailureInternal`/`GOXSD9030`; inline/anonymous forms query-only/rejected.

Named complex `abstract` is non-inherited; `Final()` uses declaring `finalDefault`, locals override;
`FinalLoc()` preserves provenance—[Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

[Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`,
`validate`, `generate`: parse prints, validate silent;
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

[Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
