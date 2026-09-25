# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

`openContent=none` works in Compatibility/Strict11, mismatches Strict10. Bounded attribute-free extension choices use named empty-content bases or named `complexContent` restrictions over `xs:anyType`; inherited `##other`/`lax` facts remain exact, while model-less forms are query-only and consumers reject.
Syntax, occurrence, element-reference, and policy gates precede mapping; located errors return no `Schema`. Permitted ordinary `0/0` bypasses mapped scalar checks and becomes no particle. Strict10 precisionDecimal rejects before omission; ordinary unsignedLong/long-family `0/0` is absent.
Refs retain QName/RefLoc/target/order; top-level model-group refs query. Global long/unsignedLong element/type facts are query-only under all policies: exact bounds `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`, facets/Locs/provenance; validation/`GenerateGo` reject. Precision attributes query in Compatibility/Strict11; Strict10 rejects at type `Loc`.
Local explicit built-in/supported named-effective `xs:unsignedLong` is queryable in all policies only in direct choices/sequences and permitted bounded extension choices over either base. Retain exact bounds `[0,18446744073709551615]`, facets, QName, type/facet/use-site `Loc`s, IDs, ownership, provenance, and occurrences; `0/0` absent, finite/unbounded/above-`uint64` exact. Mapped nonzero scalar exclusions (effective `int`/`long`/`nonNegativeInteger`/`nonPositiveInteger`, inline/anonymous, list/union, narrower/nested) are schema-syntax unsupported at type/facet/element `Loc`s, no `Schema`. Element/group refs are query-only; admitted `unsignedLong` is consumer-only and `ValidateInstance`/`GenerateGo` reject it.
Direct local `xs:negativeInteger` rejects when mapped nonzero; effective named/inline `negativeInteger` is query-only then consumers return `FailureUnsupported`. Local `nonNegativeInteger` non-`0/0` rejects; `0/0` absent. Inline Boolean/integer/decimal query-only; string/token/NMTOKEN generate. Attributes query-only.
`precisionDecimal` locals admit only default direct choices and bounded attribute-free extension choices in Compatibility/Strict11; built-in/named roots and direct defaults validate, extension choices query-only, and `GenerateGo` rejects all targets. Nonzero sequences, inline/anonymous, and nondefault forms reject; Strict10 first, admitting policies omit `0/0`.
`GenerateGo`: global built-in/named `nonNegativeInteger` elements and standalone named simple types generate; fields use `StrictInteger` or generated types. Element/final/variety/facet gates are `FailureUnsupported`/`GOXSD9029`; malformed/stale facts are `FailureInternal`/`GOXSD9030`.

Abstract/final provenance: [Architecture](ARCHITECTURE.md).

## Product CLI

[Decision 0006](docs/decisions/0006-vertical-slice-cli.md): `parse`, `validate`, `generate`; invalid/usage exit 1/2.

## Design goals

Exact values/facets, streaming, deterministic output; no goroutines/locks/map-order.

## Repository checks

Conformance needs exact version, `-set`, `-case`:
```sh
go tool workflowctl doctor
go tool workflowctl check
```

## Pinned specification corpus

```sh
go tool specs build -id ID
go tool specs search -id ID -query QUERY
go tool specs bootstrap -version VERSION
```
`-root`/`-output`/`-index`; bootstrap.

## Project workflow

[Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
