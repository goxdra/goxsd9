# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller-provided `ResolvedSource`/`Resolver`; sequential calls, opaque locations; Compatibility default.

XSD 1.0/1.1 graphs, restrictions, limited facets, `openAttrs`, and bounded extensions are modeled. Complexes include element-only/model-group references, bounded attribute-free extensions, model-less extensions over completed named empty-content bases, and bounded inherited `##other`/lax wildcards. XSD 1.1 `defaultAttributesApply`, abstract/final controls, and policy mismatches retain located facts/diagnostics.
`openContent=none`: globals/bounded extensions in Compatibility/Strict11; Strict10 mismatch; other unsupported; malformed invalid.
`xs:any` supports `##any`/strict|lax|skip, `##other`/lax|strict, and positive namespaces (`##local`, `##targetNamespace`, URI lists) with strict/lax/explicit-skip processing; broader placements unsupported; consumers reject nonzero wildcards. Named-global sequence/choice owners: `anyAttribute`, `##any`/strict; `##any`/lax|skip (optional/explicit namespace; skip), `##other`/lax|strict, `##other`/skip, positive namespaces (`##local`, `##targetNamespace`, URI lists) strict; locations retained; validation/generation unsupported.
Particle-plus-uses and attribute-only bodies expose immutable ordered local/ref `AttributeUse` views with lexical locations and resolved type/reference identities; one bounded scalar-base `simpleContent` extension retains its base and local/ref uses without a particle. Optional/required uses are effective; prohibited uses are omitted. Attribute-bearing validation/generation, value constraints, inheritable attributes, broader wildcards/groups/derivation remain unsupported.
Only top-level direct model-group refs and named-global groups direct choice/sequence refs queryable; nested/other groups unsupported. Typed built-in/supported named
`xs:token`/`xs:NMTOKEN` particles modeled; default-occurrence all-token/NMTOKEN choices validate; local token/NMTOKEN sequences unsupported; globals/generation unchanged.
Model-group references and token/NMTOKEN facts are queryable within their documented boundaries; validation/generation remain explicit about unsupported shapes. `abstract` applies to named complexes; non-inherited; consumers reject use with located unsupported diagnostics. [ARCHITECTURE.md](ARCHITECTURE.md).
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
