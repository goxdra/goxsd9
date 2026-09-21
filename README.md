# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; bounded attr-free extensions/model-less bases; inherited wildcards; `openContent=none` (Compatibility/Strict11; Strict10 mismatch). `xs:any`/`anyAttribute` retain namespace/process facts; particle `0/0` absent; nonzero wildcard/attribute consumers and broader placements unsupported.
Element refs retain QName/`RefLoc`/target/order/occurrences; model-group refs query separately. Global built-in/named long-family refs query-only: `long` `[-9223372036854775808,9223372036854775807]`, `unsignedLong` `[0,18446744073709551615]`, `negativeInteger≤-1`, `nonNegativeInteger≥0`, `nonPositiveInteger≤0`; malformed refs invalid.
Local anonymous Boolean/integer/decimal query; mapped `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` or non-string enumeration: located `FailureUnsupported`/`ErrUnsupported` at type/facet `Loc`, no schema. `integer`/`negativeInteger` graph bases support named/forward/imported/included/chameleon.
`precisionDecimal` is choice-only: Strict10 rejects typed local forms including `0/0` with located `FeatureDatatypeFacets`/`FailureUnsupported`/`ErrUnsupported`; Compatibility/Strict11 omit `0/0` and allow only non-extension direct choices with default `(1/1)` choice/alternatives. Non-default choices/non-`0/0` direct sequences unsupported. Supported local built-in/named Boolean-only or integer/decimal sequences; only non-extension default-occurrence all-token/all-NMTOKEN choices validate. Mapped anonymous string/token/NMTOKEN/`precisionDecimal` is unsupported; token/NMTOKEN sequence/ref and mixed-choice consumers reject.
GenerateGo supports global built-in/named/inherited/included/imported Boolean/integer/decimal/string/token/NMTOKEN plus global inline string/token/NMTOKEN; only non-extension default-occurrence direct-choice refs to global Boolean/integer/decimal. Global inline long-family and identity-only (`language`/`NCName`/`anyURI`/`ID`) are query-only; consumers reject. Local mapped `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger`/identity-only forms are schema-unsupported. Global `precisionDecimal` is queryable but generation-rejected; diagnostics retain `Loc`/cause; unsupported returns no schema/output.

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
