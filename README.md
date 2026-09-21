# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; bounded attr-free extensions/model-less bases; inherited wildcards; `openContent=none` (Compatibility/Strict11; Strict10 mismatch). `xs:any`/`anyAttribute` retain namespace/process facts; particle `0/0` absent; nonzero wildcard/attribute consumers/broader placements unsupported.
Element refs retain QName/`RefLoc`/target/order/occurrences; model-group refs query. Global built-in/named long-family refs are query-only with exact integer bounds; malformed refs invalid. Global built-in/named/inline `precisionDecimal` schema/query/validation admission: Compatibility/Strict11 only; Strict10 rejects them before validation with located `FeatureDatatypeFacets`/`FailureUnsupported`/`ErrUnsupported`; built-in/named roots validate, inline anonymous consumers reject.
Local anonymous Boolean/integer/negativeInteger/decimal query. Mapped non-`0/0` local `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` or non-string enumeration is unsupported at type/facet `Loc`; no schema.
`precisionDecimal` admission is policy-first: Strict10 typed forms, even `0/0`, return located `FeatureDatatypeFacets`/`FailureUnsupported`/`ErrUnsupported`; Compatibility/Strict11 omit `0/0`. Mapped precision remains in default direct/bounded extensions; mapped non-default choices or non-`0/0` direct sequences unsupported. Nonprecision non-defaults query-only. Only non-extension defaults validate; extension/anonymous consumers reject. Mapped non-`0/0` anonymous string/token/NMTOKEN/`precisionDecimal` unsupported; token/NMTOKEN sequence/ref/mixed consumers reject.
GenerateGo supports global built-in/named Boolean/integer/decimal/string/token/NMTOKEN and inline string/token/NMTOKEN; only non-extension default refs to global Boolean/integer/decimal. Inline long-family/identity-only (`language`/`NCName`/`anyURI`/`ID`) facts query-only; consumers reject. Mapped non-`0/0` local `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger`/identity-only forms have no schema. Compatibility/Strict11 only: `precisionDecimal` queryable; `GenerateGo` rejects global/anonymous; unsupported yields no schema/output.

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
