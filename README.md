# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, and Go code generation; unsupported behavior is explicit.

## Schema parsing

`ParseSchema` exposes immutable components. Callers create `ResolvedSource`; `Resolver` supplies sources. Calls are sequential; contexts and locations stay opaque. Parsing closes unseen streams; repeats/cycles close without decoding.

Mixed XSD 1.0/1.1 graphs, declarations, restrictions, digit facets, and optional `xs:precisionDecimal` are supported; `Strict10` rejects it. Chameleon, redefine, override, defaultOpenContent, assertions, broader facets remain unsupported. `ParseSchema` defaults to `Compatibility`; `schema/@version` is inert. `ValidateInstance` supports text-only built-in/named integer/decimal globals and named complex globals with one direct local choice; attributes and broader particles remain unsupported. `GenerateGo` emits deterministic scalar/choice Go. The [direct-choice example](direct_choice_example_test.go) uses [fixtures](examples/direct-choice/). Run `go test ./... -run '^Example_directChoice$'`; it expects invalid `XSD2001` at `examples/direct-choice/invalid.xml:2:19` with `xsd11-datatypes#integer`. Scope: one direct, non-repeating local integer/decimal choice; refs, nested particles, wildcards, attributes, inline types, identity constraints remain unsupported.

## Product CLI

`parse`, `validate`, and `generate` work through public APIs; [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) defines the CLI contract.
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

Parse writes stdout; validation is silent on success. Invalid validation exits 1 with a located stderr diagnostic; usage is 2. Unsupported behavior is explicit, with no broader conformance claim.

## Design goals

Exact value spaces/facets, streaming resolver input, immutable deterministic queries/walks,
located diagnostics, no goroutines/locks or map-order output, and measured conformance.

See [ARCHITECTURE.md](ARCHITECTURE.md) and [PLAN.md](PLAN.md).

## Repository checks

Fresh checkout; inventory reports metadata only, not tests:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance inventory
```

## Pinned specification corpus

Build/search a verified `.cache` entry:
```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
```
Use `-root`, `-output`, and `-index`; the [schema bootstrap contract](docs/decisions/0003-schema-bootstrap.md) covers digests, conversion, and artifacts.

## Project workflow

Work uses [GitHub Issues](https://github.com/goxdra/goxsd9/issues) and the [goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1); agents use worktrees/workflow skills—see [operations](docs/operations.md) and [AGENTS.md](AGENTS.md) for workflow rules.

## Test data licensing

The W3C submodule keeps `00COPYRIGHT`, not Apache-2.0; the repository is Apache-2.0 ([LICENSE](LICENSE)).
