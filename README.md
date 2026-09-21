# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; facets/`openAttrs`/extensions; bounded attribute-free extensions; model-less bases; inherited `##other`/lax; XSD 1.1/Compatibility `defaultAttributesApply`.
`openContent=none`: Compatibility/Strict11 globals/extensions; Strict10 mismatch.
`xs:any`: `##any` strict|lax|skip; `##other` lax|strict; positive namespaces strict|lax|skip; `0/0` absent; broader placements/nonzero wildcard consumers unsupported. `anyAttribute`: `##any`/`##other` strict|lax|skip; positive namespaces strict.
Refs support local/named-group choice/sequence; top-level model-group refs exclude nested/local/recursive/broader. Anonymous Boolean/integer/decimal queryable; non-string enumeration: located `FailureUnsupported`/`ErrUnsupported` at facet `Loc`, no schema. Integer allowlist: `integer`/`negativeInteger` through named/forward/imported/included/chameleon; valid-but-unsupported `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` yield `FailureUnsupported`/`ErrUnsupported` at relevant type/facet `Loc`, no schema; malformed input: `FailureInvalid`.
Local anonymous Boolean/integer/decimal queryable; mapped anonymous token/NMTOKEN/precisionDecimal schema-unsupported (`0/0` omitted before type gating); anonymous locals consumer-rejected. Typed local precisionDecimal: Compatibility/Strict11 (Strict10 rejects); default-occurrence direct `<choice>`/mapped alternatives; unsupported `<all>` mapping; only non-extension choices validate. Non-default precisionDecimal choice/alternative or non-0/0 direct-sequence precisionDecimal ranges schema-unsupported; non-precision alternatives retain non-default query-only ranges. Token/NMTOKEN queryable; default all-token/all-NMTOKEN choices validate; mixed families/token/NMTOKEN sequences unsupported. Global inline string/token/NMTOKEN generate; inline precisionDecimal queryable in Compatibility/Strict11 (Strict10 rejects); anonymous refs consumer-rejected.
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
