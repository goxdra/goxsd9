# goxsd9

goxsd9 targets XSD 1.1/1.0 parsing, XML validation, Go generation; unsupported behavior is explicit.

## Schema parsing

`ParseSchema` exposes immutable components; callers provide `ResolvedSource` and `Resolver`. Calls sequential; locations opaque.

XSD 1.0/1.1 graphs/restrictions supported; limited `precisionDecimal`/string facets/`openAttrs`/extensions. Named global complexes: element-only bodies/`defaultAttributesApply`, XSD 1.1/Compatibility. Direct global named XSD 1.1 `openContent mode="none"` inert/accepted under Compatibility/Strict11; Strict10 located XSD 1.1 mismatch. Non-none/wildcard-bearing `openContent`/`defaultOpenContent`, inline/derivation-local unsupported; malformed=invalid. Named direct complex-type sequence/choice `anyAttribute`: both omitted/`namespace="##any"`/`processContents="strict"`/both explicit forms; effective values=`##any`/`strict`; explicit locations retained, omitted locations=0; `##other`/`lax` supported. Other groups/shapes, local uses/attributes/wildcards, consumers unsupported. Choices/sequences: exact ranges/attribute-wildcards. Compatibility default. `ValidateInstance`/`GenerateGo` cover named direct numeric sequences/choices/refs; global scalar generation.

Named complex types retain effective `abstract` bool via `ComplexTypeDefinition.IsAbstract()`; derivation does not inherit it. Supported concrete validator/generator paths reject abstract use with located unsupported diagnostics; valid abstract declarations remain schema-queryable. Contract: [ARCHITECTURE.md](ARCHITECTURE.md).
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
