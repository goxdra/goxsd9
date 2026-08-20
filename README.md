# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, and Go code generation.
It prioritizes a useful vertical slice and measured conformance without hiding
unsupported behavior.

## Schema parsing

`ParseSchema` exposes the supported schema discovery and immutable-component
pipeline. The caller creates the root `ResolvedSource`; a `Resolver` supplies
include/import streams and resolution policy. Calls are sequential, locations
and contexts remain opaque, and paths or URLs are never opened by the package.
Parsing closes every stream; it drains and decodes only unseen identities.
Repeated or cyclic identities are closed without decoding.

The subset supports mixed XSD 1.0/1.1 graphs, schema-level declarations, and
simple-type restrictions. It does not implement chameleon includes,
redefine/override/defaultOpenContent, assertions, or unsupported datatype facets.
Absent version defaults to XSD 1.1; unsupported behavior and invalid input return explicit located diagnostics, and errors return no schema. XML validation and Go code generation remain under construction.

## Design goals

- Exact value spaces and facets, with an optional idiomatic Go datatype profile.
- Streaming input through caller-provided resolvers.
- Immutable components with deterministic query and walk APIs.
- Located diagnostics and no silent acceptance of unsupported behavior.
- No goroutines, locks, or output derived from map iteration.
- Measured conformance against the W3C test suite.

See [ARCHITECTURE.md](ARCHITECTURE.md) for current design and [PLAN.md](PLAN.md) for phased outcomes.

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

Executable work lives in [GitHub Issues](https://github.com/goxdra/goxsd9/issues)
and the [goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1). Agents use
worktrees, atomic claims, and the checked-in `develop`, `backlog`, and `retro`
skills. See [scheduled operations](docs/operations.md); [AGENTS.md](AGENTS.md)
defines commit and PR subjects, which `workflowctl` validates.

## Test data licensing

The `testdata/w3c/xsdtests` submodule is the W3C XML Schema test suite under its own
document license; see its `00COPYRIGHT`. It is not relicensed under Apache-2.0.

## License

Apache-2.0. See [LICENSE](LICENSE).
