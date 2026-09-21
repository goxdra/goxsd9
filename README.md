# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; facets/`openAttrs`/extensions; bounded attribute-free extensions; model-less bases; inherited `##other`/lax; XSD 1.1/Compatibility `defaultAttributesApply`.
`openContent=none`: Compatibility/Strict11 globals/extensions; Strict10 mismatch.
`xs:any`: `##any` strict|lax|skip; `##other` lax|strict; positive namespaces strict|lax|skip; `0/0` absent; broader placements/nonzero wildcard consumers unsupported. `anyAttribute`: `##any`/`##other` strict|lax|skip; positive namespaces strict.
Refs support local/named-group choice/sequence; nested/local/recursive/broader group refs excluded. Anonymous Boolean/integer/decimal queryable; non-string enumeration: located `FailureUnsupported`/`ErrUnsupported`, no schema. Local anonymous integer-derived mapped particles allow `integer`/`negativeInteger` through named/forward/imported/included/chameleon; excluded `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` yield located `FailureUnsupported`/`ErrUnsupported` at type/facet `Loc`, no schema. Global built-in/named long-family refs queryable; malformed: `FailureInvalid`.
Mapped anonymous locals: string/token/NMTOKEN/precisionDecimal schema-unsupported (`0/0` omitted); Boolean/integer/decimal consumer-rejected. Typed local token/NMTOKEN queryable; all-token/all-NMTOKEN choices validate; mixed-family choices, token/NMTOKEN sequences and reference consumers, local generation unsupported. Typed local precisionDecimal: Compatibility/Strict11 (Strict10 rejects); choice/mapped-alternatives default-occurrence (1/1); <all> unsupported; only non-extension direct choices validate; non-default choice/alt or non-0/0 direct-sequence ranges schema-unsupported; non-precision query-only. Global inline string/token/NMTOKEN generate; global inline precisionDecimal Compatibility/Strict11 schema/query support (Strict10 rejects); anonymous-target consumers reject; element-reference exclusions.
Direct choice/sequence checks use element/particle Locs; anonymous Loc may relate. Extension gates first; model-group refs retain RefLoc and related locations. FailureUnsupported/ErrUnsupported; no GenerateGo output.

Named complex `abstract` non-inherited; consumers reject it. [ARCHITECTURE.md](ARCHITECTURE.md).
[Direct-choice example](direct_choice_example_test.go); [scalar quickstart](library_example_test.go).

## Product CLI

CLI APIs; [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) sets contract.

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

Fresh checkout; bounded schema needs exact version plus `-set`/`-case`; instances never run:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance inventory
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Pinned specification corpus

Commands:
```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
go tool specs bootstrap -version 1.1
```
Use `-root`/`-output`/`-index`; `bootstrap` previews only.

## Project workflow

See [Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
