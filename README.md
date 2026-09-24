# goxsd9

goxsd9 parses/validates/generates Go; unsupported is explicit.

## [Schema parsing](ARCHITECTURE.md#schema-model)

`ParseSchema`: immutable components; caller `ResolvedSource`/`Resolver`; sequential opaque locations; Compatibility default.

XSD 1.0/1.1; `openContent=none` works under Compatibility/Strict11 and mismatches Strict10. Extensions need named empty-content bases; model-less preserves identity/locations. `xs:any`/`anyAttribute` keep facts; admitted `0/0` is absent.
Refs retain QName/RefLoc/target/order; model-group refs query; broader unsupported. Long refs retain bounds; malformed invalid. Global `nonNegativeInteger` refs query; consumers reject. `precisionDecimal` element/type facts query under Compatibility/Strict11; Strict10 rejects before validation; roots validate, inline-element targets query-only.
`precisionDecimal` attributes query-only in Compatibility/Strict11 (built-in/supported named); Strict10 rejects at type `Loc`.
`xs:long`/`xs:unsignedLong` attributes, including named types, are admitted under all policies; bounds `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`; bounds/facets/locations queryable; consumers reject. Each unsupported default/fixed uses `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc`; no `Schema`; only default+fixed declarations use `FailureInvalid`/`XSD3010` (fixed primary, default related, no `Schema`).
Local `nonNegativeInteger` non-0/0 forms reject; 0/0 absent. Global inline Boolean/integer/decimal elements are query-only; string/token/NMTOKEN inline elements generate. Attributes are query-only; inline-attribute consumers excluded; `GenerateGo` rejects every `ComponentKindAttributeDeclaration`.
`precisionDecimal` locals under Compatibility/Strict11 admit default choices or bounded attribute-free extensions; `0/0` absent, Strict10 rejects first. Extensions query; consumers reject; targets GenerateGo-rejected.
Particle-plus-use and attribute-only bodies expose ordered local, referenced, and inline `AttributeUse` facts; global references retain QName/`RefLoc`/`TargetID`/effective use and admit only Boolean/integer/decimal plus policy-gated `precisionDecimal`. Other valid scalar references fail with located `FailureUnsupported`/`ErrUnsupported` and no schema. Bounded scalar `simpleContent` extensions retain base/type/use facts and admit Boolean/string/integer/decimal plus policy-gated `precisionDecimal`; prohibited uses are omitted and consumers remain unsupported.
GenerateGo: global built-in/named-typed `nonNegativeInteger` elements and standalone named simple-type components generate across policies. Built-in/standalone fields use `StrictInteger`; named-typed fields use generated types. Elements require `abstract=false,nillable=false`; either yields `FailureUnsupported`/`GOXSD9029`, nil. Named final/variety/effective-facet gates reject (`FailureUnsupported`/`GOXSD9029`); malformed/stale facts are `FailureInternal`/`GOXSD9030`. Global inline/anonymous `nonNegativeInteger` element/type declarations are query-only and consumer-rejected.

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
