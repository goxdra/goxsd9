# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, and Go code generation.
It prioritizes a useful vertical slice and measured conformance without hiding
unsupported behavior.

## Schema parsing

`ParseSchema` exposes schema discovery and immutable components. Callers create a
root `ResolvedSource`; `Resolver` supplies referenced sources and resolution policy. Calls are
sequential; contexts and locations stay opaque, and paths/URLs stay unopened.
Parsing closes streams and decodes only unseen identities; repeated and cyclic
identities close without decoding.

Mixed XSD 1.0/1.1 graphs, declarations, and simple-type restrictions are supported. Chameleon includes, redefine/override/defaultOpenContent, assertions, and broader datatype facets remain unsupported; totalDigits/fractionDigits are implemented.
The two-argument parser retains per-document handling: absent or empty `schema/@version` defaults to XSD 1.1, `"1.0"` selects the legacy XSD 1.0 path, `"1.1"` selects the legacy XSD 1.1 path, and arbitrary labels are unsupported.
Normatively, `schema/@version` is an inert optional `xs:token` label.
`ParseSchemaWithPolicy` validates graph-wide `Compatibility`,
`Strict10`, or `Strict11` policy before discovery; propagation and
profile-specific behavior remain future work.
Unsupported behavior and invalid input return located diagnostics; errors return no schema. XML validation and Go generation remain under construction.

## Design goals

- Exact value spaces and facets.
- Streaming input through caller resolvers.
- Immutable, deterministic query and walk APIs.
- Located diagnostics without silent unsupported behavior.
- No goroutines, locks, or map-order output.
- Measured W3C conformance.

See [ARCHITECTURE.md](ARCHITECTURE.md) and [PLAN.md](PLAN.md) for design and phased outcomes.

## Repository checks

Fresh checkout; `go tool conformance inventory` reports metadata only and runs no tests:

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance inventory
```

## Pinned specification corpus

Build and search a verified manifest entry under ignored `.cache`:

```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
```

Use `-root PATH`, `-output PATH`, and `-index PATH` as needed; see the [schema bootstrap contract](docs/decisions/0003-schema-bootstrap.md) for digest verification, declared representation conversion, and generated artifacts.

## Project workflow

Work lives in [GitHub Issues](https://github.com/goxdra/goxsd9/issues) and the
[goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1). Agents use worktrees
and workflow skills. See [scheduled operations](docs/operations.md) and
[AGENTS.md](AGENTS.md) for workflowctl rules.

## Test data licensing

The `testdata/w3c/xsdtests` submodule is the W3C XML Schema test suite under its own
document license; see its `00COPYRIGHT`. It is not relicensed under Apache-2.0.

## License

Apache-2.0. See [LICENSE](LICENSE).
