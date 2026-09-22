# goxsd9

goxsd9 parses/validates/generates Go; unsupported remains explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential, opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11; mismatches Strict10. Attribute-free extensions only over named empty-content bases; model-less keeps base identity/locations and representable inherited `##other`/lax. `xs:any`/`anyAttribute` keep facts; `0/0` absent; broader placements unsupported.
Element refs retain QName/RefLoc/target/order/occurrences; only top-level direct named model-group refs query; nested/local/recursive/broader unsupported. Global long-family refs query with exact bounds; malformed refs invalid. Global precisionDecimal element/type facts query under Compatibility/Strict11; Strict10 rejects before validation; built-in/named roots validate, inline remains query-only but consumers reject.
Global built-in/named-effective precisionDecimal attributes are query-only: a default/fixed AttributeValueConstraint retains kind, collapsed lexical spelling, source Loc, and exact defensive StrictPrecisionDecimal via PrecisionDecimalValue() under Compatibility/Strict11. Strict10 rejects at type Loc before conversion; inline/local attributes and validation/GenerateGo remain unsupported.
Local anonymous Boolean/integer/decimal forms are query-only; validation/GenerateGo reject. Mapped nonzero anonymous string/token/NMTOKEN/precisionDecimal and non-string enums are unsupported at type/facet Loc; no schema.
precisionDecimal locals: Compatibility/Strict11 admits built-in or named-effective types only in default choices/bounded attribute-free extension choices; owners and mapped typed alternatives require default occurrences. Inline forms, non-default/nonzero sequences, and extension/anonymous consumers reject. Compatibility/Strict11 omits 0/0; Strict10 rejects first.
GenerateGo-only: global built-in/named atomic nonNegativeInteger elements (`StrictInteger`) generate across the resolved graph under Compatibility/Strict10/Strict11 (direct/named/forward/included/imported/chameleon); ValidateInstance (FailureUnsupported/XSD4004/ErrUnsupported) and local/inline/choice/sequence/repetition/non-default/list/union/attribute/value-constraint/other integer-derived consumers remain unsupported with no output. Scalar support and precisionDecimal exclusions remain as documented.

Named complex `abstract` is non-inherited; `Final()` uses declaring-document `finalDefault` if
no local `final`; explicit empty/non-empty locals override it; `FinalLoc()` preserves
local/default provenance—see [Architecture](ARCHITECTURE.md).
[Examples](direct_choice_example_test.go), [quickstart](library_example_test.go).

## Product CLI

See [Decision 0006](docs/decisions/0006-vertical-slice-cli.md). `parse`,
`validate`, and `generate` available; parse prints, validate silent;
invalid exits 1, usage exits 2.

## Design goals

Exact values/facets, streaming input, deterministic queries, located diagnostics;
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
