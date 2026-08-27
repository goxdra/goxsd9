# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, and Go code generation; unsupported behavior is explicit.

## Schema parsing

`ParseSchema` exposes immutable components; callers provide `ResolvedSource` and a `Resolver`. Calls are sequential; source locations stay opaque.

Mixed XSD 1.0/1.1 graphs, restrictions, ordered bounds, and optional `xs:precisionDecimal` work. Named global complex types expose direct ordered integer/decimal scalar sequences with exact immutable occurrence ranges. `Compatibility`/`Strict11` permit precisionDecimal elsewhere; direct sequence precisionDecimal children remain unsupported. `Strict10` reports XSD 1.1 constructs as located unsupported. Malformed input is invalid; `totalDigits`/`fractionDigits` work. Broader facets and composition are unsupported. `ParseSchema` defaults to `Compatibility`; `ParseSchemaWithPolicy` selects policies. Errors return no schema. `ValidateInstance` supports text-only built-in/named integer/decimal/precisionDecimal globals and named complex globals with one direct, non-repeating scalar choice; sequences are query-only. Attributes, broader particles, refs, nested particles, wildcards, inline types, and identity constraints remain unsupported. `GenerateGo` emits deterministic scalar/choice Go. The [direct-choice example](direct_choice_example_test.go) uses [fixtures](examples/direct-choice/); run `go test ./... -run '^Example_directChoice$'` to check invalid `XSD2001` at `examples/direct-choice/invalid.xml:2:19` with `xsd11-datatypes#integer`. [Scalar quickstart](library_example_test.go) is library-only.

## Product CLI

`parse`, `validate`, and `generate` use public APIs; [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) defines the CLI contract.
[`examples/root.xsd`](examples/root.xsd), [`examples/valid.xml`](examples/valid.xml), [`examples/invalid.xml`](examples/invalid.xml)

```console
$ go run ./cmd/goxsd9 parse examples/root.xsd
documents=1 components=2
$ go run ./cmd/goxsd9 validate examples/root.xsd examples/valid.xml
$ go run ./cmd/goxsd9 validate examples/root.xsd examples/invalid.xml
validate stage=validate class=invalid kind=processing source_id=instance/examples/invalid.xml location=1:8 code=XSD2001 related=schema/root.xsd:2:3 spec_ref=xsd11-datatypes#integer invalid xs:integer lexical representation
exit status 1
$ go run ./cmd/goxsd9 generate --package sample examples/root.xsd > generated.go
```

Parse writes stdout; validation is silent on success. Invalid validation exits 1 with a located diagnostic; usage is 2.

## Design goals

Exact value spaces/facets, streaming resolver input, immutable deterministic queries/walks,
located diagnostics, no goroutines/locks or map-order output, and measured conformance.

See [ARCHITECTURE.md](ARCHITECTURE.md) and [PLAN.md](PLAN.md).

## Repository checks

Fresh checkout; inventory metadata only:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance inventory
```

## Pinned specification corpus

Corpus commands:
```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
go tool specs bootstrap -version 1.1
```
Use `-root`, `-output`, and `-index`; `bootstrap` previews without fetching.

## Project workflow

See [GitHub Issues](https://github.com/goxdra/goxsd9/issues), the [goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), and [AGENTS.md](AGENTS.md) for workflow rules.

## Test data licensing

The W3C submodule keeps `00COPYRIGHT`, not Apache-2.0; the repository is Apache-2.0 ([LICENSE](LICENSE)).
