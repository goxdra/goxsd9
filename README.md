# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, Go generation; unsupported behavior is explicit.

## Schema parsing

`ParseSchema` exposes immutable components; callers provide `ResolvedSource` and `Resolver`. Calls sequential; locations opaque.

XSD 1.0/1.1 graphs/restrictions; limited `precisionDecimal`; string-enumeration/whiteSpace facts. Bounded `openAttrs`: empty content and `##other`/`lax` wildcard facts; validation/generation reject them. Bounded attribute-free `complexContent`/`extension` over named empty-content complex bases retain extension/base identities/locations, inherited bounded wildcard facts, and exact direct choice/sequence occurrences; validation and generation reject extension types as unsupported. Direct choices/sequences expose attribute-free `anyAttribute` across policies; defaults `##any`/`strict`; `##other`/`lax` retained. Other wildcard/attribute forms and local strings unsupported; ranges exact, `0/0` omitted. `Strict10` reports 1.1; malformed input invalid; digit facets work. `ParseSchema` defaults Compatibility; policy selects; errors return no schema. `ValidateInstance` supports default scalar choices/references; `GenerateGo` emits deterministic scalar/default-choice/bounded integer/decimal sequences; strings, Boolean facets, and local particles unsupported.
[Direct-choice example](direct_choice_example_test.go); run `go test ./... -run '^Example_directChoice$'`. [Scalar quickstart](library_example_test.go).

## Product CLI

`parse`, `validate`, and `generate` use public APIs; [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) defines CLI contract.
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

Parse writes stdout; validation is silent on success. Invalid validation exits 1 with a located diagnostic; usage 2.

## Design goals

Exact value spaces/facets, streaming resolver input, immutable deterministic queries/walks,
located diagnostics, no goroutines/locks or map-order output, and measured conformance.

See [ARCHITECTURE.md](ARCHITECTURE.md) and [PLAN.md](PLAN.md).

## Repository checks

Fresh checkout; inventory remains metadata-only. Bounded schema requires exact `-version 1.0` or `-version 1.1` plus `-set` or `-case`; instances never run:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance inventory
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Pinned specification corpus

Corpus commands:
```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query "content model"
go tool specs bootstrap -version 1.1
```
Use `-root`, `-output`, `-index`; `bootstrap` previews without fetching.

## Project workflow

See [GitHub Issues](https://github.com/goxdra/goxsd9/issues), [goxsd9 Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), and [AGENTS.md](AGENTS.md) for workflow rules.

## Test data licensing

W3C submodule keeps `00COPYRIGHT`, not Apache-2.0; repository is Apache-2.0 ([LICENSE](LICENSE)).
