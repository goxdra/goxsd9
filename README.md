# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, and Go code generation.
It prioritizes a vertical slice and measured conformance without hiding
unsupported behavior.

## Schema parsing

`ParseSchema` exposes immutable schema components. Callers create `ResolvedSource`; `Resolver` supplies sources and policy. Calls are sequential; contexts/locations remain opaque, paths/URLs unopened. Parsing closes streams after unseen identities decode; repeated/cyclic identities close without decoding.

Mixed XSD 1.0/1.1 graphs, declarations, and simple-type restrictions are supported. Chameleon includes, redefine/override/defaultOpenContent, assertions, and broader facets remain unsupported; total/fraction digits are implemented.
`ParseSchema` selects graph-wide `Compatibility` by default; `ParseSchemaWithPolicy` validates
and applies graph-wide `Compatibility`, `Strict10`, or `Strict11` before discovery.
unqualified `schema/@version` is an inert optional `xs:token` label: absent, empty, arbitrary,
`"1.0"`, and `"1.1"` values never select or mismatch a policy. Strict10 routes the complete
graph through XSD 1.0 behavior; Compatibility and Strict11 use XSD 1.1 behavior for the supported subset.
Diagnostics cover invalid/unsupported input; errors omit schema. `ValidateInstance` supports text-only built-in/named integer/decimal globals and named complex globals with one direct local built-in/named integer/decimal choice; attributes and broader particles/semantics unsupported. `GenerateGo` emits deterministic Go for scalar components and direct choices; broader generation staged. [Scalar&nbsp;library&nbsp;quickstart:&nbsp;supported&nbsp;scalar&nbsp;path;&nbsp;product&nbsp;CLI&nbsp;validate&nbsp;is&nbsp;implemented;&nbsp;generate&nbsp;remains&nbsp;future](library_example_test.go).

## Design goals

- Exact value spaces and facets.
- Streaming input through resolvers.
- Immutable deterministic query and walk APIs.
- Located diagnostics without silent gaps.
- No goroutines, locks, or map-order output.
- Measured conformance.

See [ARCHITECTURE.md](ARCHITECTURE.md), [PLAN.md](PLAN.md), and the [CLI decision](docs/decisions/0006-vertical-slice-cli.md).

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

Use `-root PATH`, `-output PATH`, and `-index PATH`; see the [schema bootstrap contract](docs/decisions/0003-schema-bootstrap.md) for digest verification, declared representation conversion, and generated artifacts.

## Project workflow

Work lives in [GitHub Issues](https://github.com/goxdra/goxsd9/issues) and the
[goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1). Agents use worktrees
and workflow skills. See [operations](docs/operations.md) and
[AGENTS.md](AGENTS.md) for workflowctl rules.

## Test data licensing

The `testdata/w3c/xsdtests` submodule is the W3C XML Schema test suite under its own
document license; see its `00COPYRIGHT`. It is not relicensed under Apache-2.0.

## License

Apache-2.0. See [LICENSE](LICENSE).
