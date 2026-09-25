# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable schema; caller `ResolvedSource`/`Resolver`; opaque sequential locations; Compatibility default.

`openContent=none` works in Compatibility/Strict11, mismatches Strict10. Bounded attribute-free extension choices/sequences use named empty-content bases or named `complexContent` restrictions over `xs:anyType`; inherited `##other`/`lax` facts remain exact, while model-less forms query-only; consumers reject.
Syntax/occurrence/reference/policy gates precede mapping; located errors return no `Schema`. Permitted `0/0` bypasses mapped scalar checks and becomes no particle. Strict10 precisionDecimal rejects first; ordinary unsignedLong/long-family `0/0` is absent.
Resolved element-reference particles and top-level direct model-group refs are queryable only after resolution/visibility/wrong-kind/ambiguity/duplicate/occurrence/policy gates, without target-type gating/expansion; retain QName/RefLoc/TargetID/order. Nested/local/recursive/broader refs/groups unsupported/consumer-excluded. Global long/unsignedLong element/type facts query-only under all policies: built-in intrinsic bounds are `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`; named-effective exact narrowed/exclusive bounds/facets (Tight max 7), written base QName/base `Loc`, use-site/type/facet `Loc`s, named ID/built-in zero identity, ownership/provenance; validation/`GenerateGo` reject.
Local built-in/supported named-effective `xs:unsignedLong` queryable under all policies only in direct choices/sequences or bounded attribute-free extensions over either base. Retain exact intrinsic/effective bounds/facets, QName/base/type/facet/use-site `Loc`s, named ID/built-in-zero identity, ownership/provenance/occurrences; `0/0` absent, finite/unbounded/above-`uint64` exact. Scalar exclusions (effective `int`/`long`/`nonNegativeInteger`/`nonPositiveInteger`; inline/anonymous `unsignedLong`/unsupported; list/union; narrower) return `FailureUnsupported` at type/facet/element `Loc`; nested-particle exclusions return `FailureUnsupported` at nested-particle `Loc`; no `Schema`. `unsignedLong` consumer-only; `ValidateInstance`/`GenerateGo` reject.
Direct local `xs:negativeInteger` rejects mapped-nonzero; effective named/inline `negativeInteger` query-only, consumers return `FailureUnsupported`. Local `nonNegativeInteger` non-`0/0` rejects; `0/0` absent. Inline Boolean/integer/decimal query-only; only global inline string/token/NMTOKEN elements generate. Local inline string/token/NMTOKEN and attributes schema-unsupported/no `Schema`; supported global attributes query-only.
`precisionDecimal` locals admit only default direct choices/bounded attribute-free extension choices in Compatibility/Strict11; built-in/named roots/direct defaults validate, extension choices query-only; `GenerateGo` rejects all. Nonzero sequences/inline/anonymous/nondefault forms reject; Strict10 first, admitting policies omit `0/0`.
`GenerateGo`: global/named `nonNegativeInteger` elements and standalone named types generate; fields use `StrictInteger`/generated types. Element/final/variety/facet gates are `FailureUnsupported`/`GOXSD9029`; malformed/stale facts are `FailureInternal`/`GOXSD9030`.

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
