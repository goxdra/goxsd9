# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; facets/`openAttrs`/extensions; bounded attribute-free extensions; model-less bases; inherited `##other`/lax; XSD 1.1/Compatibility `defaultAttributesApply` globals.
`openContent=none`: Compatibility/Strict11 globals/extensions; Strict10 mismatch.
`xs:any` query facts: `##any` strict|lax|skip; `##other` lax|strict; positive namespaces strict|lax|skip; `0/0` absent. Broader placements unsupported; nonzero wildcard particles consumer-rejected. `anyAttribute`: `##any` strict|lax|skip; `##other` lax|strict|skip; positive namespaces strict.
Refs support local/named-group choice/sequence; model-group refs top-level (nested/local/recursive/broader excluded). Anonymous local Boolean/integer/decimal queryable; non-string enumeration: located `FailureUnsupported`/`ErrUnsupported` at facet `Loc`, no schema. Allowlist `integer`/`negativeInteger` through named/forward/imported/included/chameleon; `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` valid-but-unsupported; malformed `FailureInvalid`.
Local anonymous inline atomics: query-only; direct validation/generation reject. Typed local `precisionDecimal`: Compatibility/Strict11 (Strict10 rejects), default direct choice/mapped-precisionDecimal alternatives; effective `0/0` absent; non-default `precisionDecimal` choice/alternative or non-`0/0` direct-sequence `precisionDecimal` ranges schema-unsupported; non-precision alternatives may retain non-default query-only ranges. Typed local token/NMTOKEN: queryable; default choices validate, generation unsupported. Global inline string/token/NMTOKEN: generation-eligible; global `precisionDecimal`: policy-gated schema/query only.
Ordinary direct choice/sequence target checks use element/particle `Loc`s and may relate anonymous `Loc`; complex-content/model-less extension gates run first (codegen extension primary; validation owner/sequence-instance), without anonymous `Loc`. Direct/extension model-group refs: `RefLoc` primary; validation particle-related, codegen group/component/reference/target-related. `FailureUnsupported`/`ErrUnsupported`; no `GenerateGo` output.

Named complex `abstract` is non-inherited; consumers reject it. [ARCHITECTURE.md](ARCHITECTURE.md).
[Direct-choice example](direct_choice_example_test.go); [scalar quickstart](library_example_test.go).

## Product CLI

CLI APIs; [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) defines contract.

```console
$ go run ./cmd/goxsd9 parse examples/root.xsd
documents=1 components=2
$ go run ./cmd/goxsd9 validate examples/root.xsd examples/valid.xml
$ go run ./cmd/goxsd9 validate examples/root.xsd examples/invalid.xml
error class=invalid location=1:8 code=XSD2001
exit status 1
$ go run ./cmd/goxsd9 generate --package sample examples/root.xsd > generated.go
```

Parse stdout; validation silent; invalid 1; usage 2.

## Design goals

Exact values/facets, streaming input, deterministic queries, located diagnostics,
no goroutines/locks/map-order output, conformance.

## Repository checks

Fresh checkout; bounded schema needs an exact version plus `-set`/`-case`; instances never run:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance inventory
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Pinned specification corpus

Corpus commands:
```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
go tool specs bootstrap -version 1.1
```
Use `-root`/`-output`/`-index`; `bootstrap` previews only.

## Project workflow

See [Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), and [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
