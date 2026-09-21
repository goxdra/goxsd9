# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; facets/`openAttrs`/extensions; bounded attribute-free extensions/model-less bases; inherited `##other`/lax; `defaultAttributesApply`; `openContent=none` (Compatibility/Strict11, Strict10 mismatch). `xs:any`/`anyAttribute`: namespace/process forms; `0/0` absent; broader/nonzero unsupported.
Refs: local/named-group choice/sequence; nested/local/recursive/broader refs excluded. Anonymous Boolean/integer/decimal queryable. Mapped non-0/0 local anonymous non-string enumeration and excluded integer-derived particles (`long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger`) return located `FailureUnsupported`/`ErrUnsupported` at facet/type `Loc`, no schema; `integer`/`negativeInteger` remain allowed through named/forward/imported/included/chameleon.
Effective `0/0` omits anonymous particles after policy admission. Explicit local `precisionDecimal` admission is first: Strict10 returns located FeatureDatatypeFacets `FailureUnsupported`/`ErrUnsupported` policy mismatch, including zero; Compatibility/Strict11 omit. Mapped anonymous string/token/NMTOKEN/precisionDecimal unsupported; typed token/NMTOKEN non-extension default-occurrence homogeneous choices validate, with Boolean/numeric/token/non-token/NMTOKEN mixes, sequences/refs, and generation unsupported; integer/decimal mixtures supported. Mapped non-default precisionDecimal choice/alternative or non-0/0 direct-sequence range is schema-syntax-unsupported; default (1/1), `<all>` unsupported; non-precision query-only.
Generation matrix: global built-in/named/inherited/included/imported Boolean/integer/decimal/string/token/NMTOKEN scalar components and global inline string/token/NMTOKEN components generate. Global precisionDecimal (built-in/named/inline) is queryable but `GenerateGo` rejects it; global inline Boolean/integer/decimal are query-only. Local built-in/named Boolean/integer/decimal generate in non-extension default-occurrence all-Boolean/numeric choices/default-bounded sequences; local anonymous/token/NMTOKEN, repeated/non-default, anonymous targets excluded.
Diagnostics preserve `Loc`/cause; extension gates first; refs retain `RefLoc`; unsupported returns no schema/`GenerateGo` output.

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
