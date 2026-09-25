# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

`openContent=none` works Compatibility/Strict11, mismatches Strict10. Scalar simpleContent retains base/type/use `Loc`s, nil particle; bases allow Boolean/string/integer/decimal plus policy-gated `precisionDecimal`. `xs:any` particles queryable; nonzero consumers reject; validated local `0/0` omits before local type mapping after applicable syntax/reference/occurrence and Strict10 precision gates; `anyAttribute` separate.
`Particle-plus-use` (including direct model-group references) and attribute-only bodies expose ordered `AttributeUse`; anonymous local atomics retain identity/use, prohibited omitted. Local/ref types allow Boolean/integer/decimal plus policy-gated `precisionDecimal`; `xs:int` unsupported. Direct local built-in `negativeInteger` rejects only mapped non-`0/0`; exact `0/0` omits before mapping. Named/inline effective forms remain query-only; consumers reject. `form`/`attributeFormDefault` select names; XSD 1.1 `targetNamespace` must match containing target; Strict10 mismatch; chameleon adopts. `attributeGroup`/complexContent-extension unsupported.
Refs retain QName/RefLoc/target/order; unresolved/wrong-kind/ambiguous/inaccessible are invalid with type/base and candidate/target locations. `precisionDecimal` facts query only Compatibility/Strict11; Strict10 gives policy diagnostic/no schema. Homogeneous local NMTOKEN sequences validate exact finite/unbounded/above-uint64 occurrences, but GenerateGo rejects; global inline string/token/NMTOKEN elements generate; attributes query-only.
Global attrs admit `xs:long`/`xs:unsignedLong` all policies; built-in refs expose intrinsic inclusive bounds `[-9223372036854775808,9223372036854775807]`/`[0,18446744073709551615]`, named refs exact narrowed/exclusive facets/provenance/ownership. Element/type facts query-only; built-ins retain intrinsic bounds, named/inline restrictions expose exact narrowed/exclusive facets with provenance/ownership; validation/GenerateGo reject; attr consumers reject.
Local `precisionDecimal` facts admit Compatibility/Strict11 default choices or bounded attr-free extension choices; mapped non-default/nonzero sequences schema-unsupported, non-precision alternatives query-only, admitted extension choices consumer-rejected. Inline/anonymous follows mapping; Strict10 precedes `0/0`; roots/default choices validate, GenerateGo rejects targets. Local `nonNegativeInteger` non-0/0 rejects; validated `0/0` absent. GenerateGo supports global/named `nonNegativeInteger`; inline/anonymous query-only.

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
