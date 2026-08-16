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

See [ARCHITECTURE.md](ARCHITECTURE.md) for current design and [PLAN.md](PLAN.md) for phased outcomes.

## Repository checks

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
```

## Pinned specification corpus

Build one manifest entry into verified, navigable artifacts under ignored `.cache`:

```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
```

`specs build` reads `specs/manifest.json`, verifies the raw HTTPS response against
its SHA-256 digest, converts the declared representation, and writes Markdown/XSD
and a compact index. Use `-root`, `-output`, or `-index PATH` for alternate locations.

## Project workflow

Executable work lives in [GitHub Issues](https://github.com/goxdra/goxsd9/issues)
and the [goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1). Agents use
worktrees, atomic claims, and the checked-in `develop`, `backlog`, and `retro`
skills. Develop must launch from the canonical primary on clean `main` equal to
fresh `origin/main`, with recursive pins ready; `doctor` gates it and
`base-sync` recovers it. See [scheduled operations](docs/operations.md).
Commit subjects and PR titles follow [AGENTS.md](AGENTS.md); `workflowctl`
validates them. `sync` is GitHub Project-only; `base-sync` fast-forwards clean
canonical `main` without reset, rebase, stash, or discard. After squash merge,
`pr finish` converges base and removes proof-backed claims; use `go tool
workflowctl pr recover PR` for idempotent recovery or `claim prune ISSUE` only
with merged proof.

## Test data licensing

The `testdata/w3c/xsdtests` submodule is the W3C XML Schema test suite. It is
distributed under its own W3C document license; see the submodule's
`00COPYRIGHT`. It is not relicensed under this repository's Apache-2.0 license.

## License

Apache-2.0. See [LICENSE](LICENSE).
