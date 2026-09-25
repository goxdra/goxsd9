# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11 and mismatches Strict10. ComplexContent extensions need named empty-content bases; model-less keeps identity. Scalar simpleContent extensions separately retain base/type/use locations and nil particle; bases allow Boolean/string/integer/decimal plus policy-gated `precisionDecimal`. `xs:any`/`anyAttribute` keep facts; admitted `0/0` is absent.
Particle-plus-use (including group refs) and attribute-only bodies expose ordered defensive `AttributeUse` views; local anonymous atomics retain identity, optional/required are effective, and prohibited omitted. Local/ref uses allow only Boolean/integer/decimal plus policy-gated `precisionDecimal`; `xs:int`/others unsupported. `form`/`attributeFormDefault` select qualified/unqualified names; XSD 1.1 local `targetNamespace` selects a namespace but must match a containing target namespace; missing/mismatch is invalid, Strict10 edition-mismatches. Chameleon includes adopt the including namespace. Validation/GenerateGo reject these consumers.
Refs retain QName/RefLoc/target/order; unresolved/wrong-kind/ambiguous/inaccessible refs are invalid, preserving primary ref/type/base and related candidate/target locations; broader unsupported. `precisionDecimal` facts query only under Compatibility/Strict11; Strict10 gives a policy diagnostic/no schema; under those policies built-in/named roots validate and inline targets are query-only. Global inline string/token/NMTOKEN element declarations generate; attributes remain query-only and GenerateGo-rejected.
`xs:long`/`xs:unsignedLong` global attributes (built-in/named) are separately admitted under all policies with bounds `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`; consumers reject. `precisionDecimal` attributes are query-only in Compatibility/Strict11; Strict10 rejects at type `Loc`.
Local `nonNegativeInteger` non-0/0 rejects; `0/0` absent. GenerateGo supports global/named `nonNegativeInteger` elements and named types; inline/anonymous forms remain query-only.

Named complex `abstract` is non-inherited; `Final()` uses declaring-document `finalDefault` without
local `final`; explicit empty/non-empty locals override it; `FinalLoc()` preserves
local/default provenance—see [Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`,
`validate`, and `generate` available; parse prints, validate silent;
invalid exits 1, usage exits 2.

## Design goals

Exact values/facets, streaming, deterministic queries, located diagnostics;
no goroutines/locks/map-order output, conformance.

## Repository checks

Fresh checkout; bounded conformance needs exact version, `-set`, `-case`; never run instances:
```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
go tool conformance schema -version 1.0 -set SET -case CASE
```

## Pinned specification corpus

```sh
go tool specs build -id xsd11-structures
go tool specs search -id xsd11-structures -query QUERY
go tool specs bootstrap -version VERSION
```
Use `-root`/`-output`/`-index`; bootstrap previews only.

## Project workflow

See [Issues](https://github.com/goxdra/goxsd9/issues), [Roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [AGENTS.md](AGENTS.md).

## Test data licensing

W3C submodule keeps `00COPYRIGHT`; Apache-2.0 ([LICENSE](LICENSE)).
