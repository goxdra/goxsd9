# goxsd9

goxsd9 targets XSD parsing, validation, Go generation; unsupported is explicit.

## Schema parsing

`ParseSchema` exposes immutable components; callers provide `ResolvedSource`/`Resolver`. Calls sequential; locations opaque. Compatibility default.

XSD 1.0/1.1; limited facets/`openAttrs`/extensions. Named complexes: element-only, direct global model-group refs, bounded attribute-free extensions; `defaultAttributesApply`: named globals (XSD 1.1/Compatibility); no schema-level `defaultAttributes`. [`Final()`/`FinalLoc()` and extension-base rules](ARCHITECTURE.md).
Named XSD 1.1 `openContent mode="none"` inert under Compatibility/Strict11; Strict10 mismatches. Other open-content/inline/derivation unsupported; malformed=invalid.
Named direct sequence/choice default-form `xs:any`: immutable `WildcardParticle`
(`##any`/`strict`), exact ranges; nonzero unsupported, `0/0` absent. Named direct sequence/choice support `anyAttribute`:
omitted/canonical `##any`/strict; explicit `##other`/lax; locations retained; omitted=0. Direct model-group refs are schema-queryable only; ValidateInstance and GenerateGo reject them. Other groups/shapes, local uses/attributes,
broader `xs:any` unsupported. Consumers support sequences/choices/element refs; default local all-Boolean choices; scalar generation separate.

`abstract` applies to named complexes; non-inherited; consumers reject use with located unsupported diagnostics. Contract: [ARCHITECTURE.md](ARCHITECTURE.md).
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

Parse stdout; validation silent on success. Invalid validation exits 1 with located diagnostic; usage 2.

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
