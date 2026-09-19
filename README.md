# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller-provided `ResolvedSource`/`Resolver`; sequential calls, opaque locations; Compatibility default.

XSD 1.0/1.1 graphs model restrictions, facets, `openAttrs`, bounded extensions, abstract/final, and located diagnostics. `defaultAttributesApply`: named globals only under XSD 1.1/Compatibility, without schema-level `defaultAttributes`. `openContent=none`: direct globals/bounded extensions in Compatibility/Strict11; located Strict10 mismatch. Element/model-group refs, model-less extensions, inherited `##other`/lax retain facts.
Named global complex direct sequence/choice types alone expose `anyAttribute`: omitted `##any`/strict; `##any` strict/lax/skip; `##other` strict/lax/skip; positive namespaces strict-only. `xs:any` separately: `##any` strict/lax/skip; `##other` strict/lax; positive namespaces strict/lax/skip; processContents/locations/effective namespaces retained; broader placements/nonzero consumers unsupported.
Particle-plus-uses/attribute-only expose ordered scalar local/ref/anonymous-inline `AttributeUse` facts; only bounded scalar `simpleContent` extensions retain base/type refs and ordered local/ref/anonymous-inline uses. Broader forms/attribute consumers unsupported; optional/required effective; prohibited omitted.
Named groups expose direct global-element-reference choice/sequence; top-level group refs retain queryable facts/ranges/identities. Nested/local/recursive/broader shapes/consumers unsupported. Token/NMTOKEN queryable; default all-token/NMTOKEN choices validate; sequences unsupported. [ARCHITECTURE.md](ARCHITECTURE.md).
[Direct-choice example](direct_choice_example_test.go); run `go test ./... -run '^Example_directChoice$'`. [Scalar quickstart](library_example_test.go).

## Product CLI

`parse`, `validate`, and `generate` use APIs; [Decision 0006](docs/decisions/0006-vertical-slice-cli.md) defines CLI contract.
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

Parse stdout; validation silent on success. Invalid exits 1 with located diagnostic; usage 2.

## Design goals

Exact value spaces/facets, streaming resolver input, immutable deterministic queries/walks,
located diagnostics, no goroutines/locks/map-order output, measured conformance.

See [ARCHITECTURE.md](ARCHITECTURE.md) and [PLAN.md](PLAN.md).

## Repository checks

Fresh checkout; inventory metadata-only. Bounded schema requires exact `-version 1.0` or `-version 1.1` plus `-set` or `-case`; instances never run:
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
Use `-root`/`-output`/`-index`; `bootstrap` previews without fetching.

## Project workflow

See [GitHub Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), and [AGENTS.md](AGENTS.md) for workflow rules.

## Test data licensing

W3C submodule keeps `00COPYRIGHT`, not Apache-2.0; repository is Apache-2.0 ([LICENSE](LICENSE)).
