# goxsd9

goxsd9 is a Go implementation of an XSD 1.1 and XSD 1.0 parser, XML instance
validator, and Go code generator. The project prioritizes a useful vertical
slice, then expands measured conformance without hiding unsupported behavior.

The repository is currently in bootstrap. Public parsing and validation APIs
are not ready for use.

## Design goals

- Strict XSD semantics by default, including exact value spaces and facets.
- An optional idiomatic Go datatype profile and caller-defined datatype
  implementations.
- Streaming schema input through caller-provided resolvers.
- Immutable completed schema components with deterministic query and walk APIs.
- Source locations on every user-facing diagnostic.
- No goroutines, locks, or output derived from map iteration.
- No silent acceptance of unimplemented specification behavior.
- Continuously measured conformance against the W3C test suite.

See [ARCHITECTURE.md](ARCHITECTURE.md) for current design and [PLAN.md](PLAN.md)
for phased outcomes.

## Repository checks

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
```

There is no Makefile. Repeated work belongs in `workflowctl` or a focused Go
tool.

## Project workflow

Executable work lives in [GitHub Issues](https://github.com/goxdra/goxsd9/issues)
and the [goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1). Agents use
worktrees and atomic issue-branch claims. The checked-in `develop`, `backlog`,
and `retro` skills define the autonomous lifecycle. See
[scheduled operations](docs/operations.md) for Paseo prompts and timing.
Commit subjects and PR titles follow the convention in [AGENTS.md](AGENTS.md);
`workflowctl` validates branch commits before both PR creation and merge, along
with the requested and final GitHub PR titles.

## Test data licensing

The `testdata/w3c/xsdtests` submodule is the W3C XML Schema test suite. It is
distributed under its own W3C document license; see the submodule's
`00COPYRIGHT`. It is not relicensed under this repository's Apache-2.0 license.

## License

Apache-2.0. See [LICENSE](LICENSE).
