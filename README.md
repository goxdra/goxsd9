# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, and measured Go code generation; unsupported behavior is explicit.

## Schema parsing

`ParseSchema` exposes immutable components. Callers create `ResolvedSource`; `Resolver` supplies sources/policy. Calls are sequential; contexts/locations stay opaque, paths/URLs unopened. Parsing closes unseen streams; repeated/cyclic identities close without decoding.

Mixed XSD 1.0/1.1 graphs, declarations, restrictions, digit facets, and `xs:precisionDecimal` are supported; precisionDecimal is optional under `Compatibility`/`Strict11`, and `Strict10` rejects it. Chameleon/redefine/override/defaultOpenContent/assertions and broader facets remain unsupported. `ParseSchema` defaults to `Compatibility`; `ParseSchemaWithPolicy` selects a policy. `schema/@version` is inert. Errors omit schema. `ValidateInstance` supports text-only built-in/named integer/decimal globals and named complex globals with one direct local integer/decimal choice; precisionDecimal instance validation, attributes, particles, and semantics remain unsupported. `GenerateGo` emits deterministic scalar/choice Go; broader generation is staged. [Scalar library quickstart](library_example_test.go) is library-only.

## Product CLI

From repository root, product `parse`/`validate` are implemented; the [library quickstart](library_example_test.go) is separate, and product `generate` is future. [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) defines paths, source IDs, limits, diagnostics, and statuses.
[`examples/root.xsd`](examples/root.xsd), [`examples/valid.xml`](examples/valid.xml), [`examples/invalid.xml`](examples/invalid.xml)

```console
$ go run ./cmd/goxsd9 parse examples/root.xsd
documents=1 components=2
$ go run ./cmd/goxsd9 validate examples/root.xsd examples/valid.xml
$ go run ./cmd/goxsd9 validate examples/root.xsd examples/invalid.xml
validate stage=validate class=invalid kind=processing source_id=instance/examples/invalid.xml location=1:8 code=XSD2001 related=schema/root.xsd:2:3 spec_ref=xsd11-datatypes#integer invalid xs:integer lexical representation
exit status 1
```

Parse prints summary to stdout; valid validation is silent and exits 0. Invalid exits 1 with empty stdout and a located stderr diagnostic; `go run` adds `exit status 1`. Usage is 2; unsupported behavior is explicit, with no broader conformance claim.

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
