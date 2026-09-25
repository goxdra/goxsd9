# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works Compatibility/Strict11, mismatches Strict10. ComplexContent extensions need named empty-content bases; model-less keeps identity. Scalar simpleContent retains base/type/use `Loc`s, nil particle; bases allow Boolean/string/integer/decimal plus policy-gated `precisionDecimal`. `xs:any` particles queryable; nonzero consumers reject; validated `0/0` alone is absent. `anyAttribute` facts/consumers separate.
`Particle-plus-use` (including direct model-group references) and attribute-only bodies expose ordered `AttributeUse`; anonymous local atomics retain identity/use, prohibited omitted. Local/ref types allow Boolean/integer/decimal plus policy-gated `precisionDecimal`; `xs:int`/others unsupported. Direct local built-in `negativeInteger` is schema-rejected; named/inline effective forms are query-only; consumers reject. `form`/`attributeFormDefault` select names; XSD 1.1 `targetNamespace` must match containing target, else invalid; Strict10 mismatches; chameleon adopts namespace. Validation/GenerateGo reject consumers; `attributeGroup`/complexContent-extension uses unsupported.
Refs retain QName/RefLoc/target/order; unresolved/wrong-kind/ambiguous/inaccessible are invalid with type/base and candidate/target locations; broader unsupported. `precisionDecimal` facts query only Compatibility/Strict11; Strict10 gives policy diagnostic/no schema. Homogeneous local NMTOKEN sequences validate exact finite/unbounded/above-uint64 occurrences, but GenerateGo rejects; global inline string/token/NMTOKEN elements generate; attributes query-only.
Global attributes admit `xs:long`/`xs:unsignedLong` under all policies with exact bounds `[-9223372036854775808,9223372036854775807]`/`[0,18446744073709551615]`; attribute consumers reject. Global long/unsignedLong built-in/named/inline element/type facts are query-only under all policies with those bounds; validation/GenerateGo reject. `precisionDecimal` attributes query only Compatibility/Strict11; Strict10 rejects at type `Loc`.
Local `precisionDecimal` facts admit Compatibility/Strict11 only default choices or bounded attr-free extension choices; sequences/non-default reject; Strict10 precedes `0/0`; roots/default choices validate, validation rejects extension/inline/anonymous, and GenerateGo rejects all. Local `nonNegativeInteger` non-0/0 rejects; validated `0/0` is absent. GenerateGo supports global/named `nonNegativeInteger` elements/types; inline/anonymous query-only.

Named complex `abstract` is non-inherited; `Final()` applies declaring `finalDefault`, locals override;
`FinalLoc()` preserves provenance—see [Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go).

## CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md): CLI `parse`,
`validate`, `generate`; parse prints, validate silent; invalid 1, usage 2.

## Goals

Exact values/facets, streaming, deterministic queries; no goroutines/locks/map-order output.

## Checks

Fresh checkout; conformance needs exact version/`-set`/`-case`; never run instances:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Corpus

```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query QUERY
go tool specs bootstrap -version VERSION
```
Use `-root`/`-output`/`-index`; bootstrap previews only.

## Project workflow

See [Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
