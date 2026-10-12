# goxsd9

goxsd9 parses XML Schema into immutable components. Unsupported features return
located diagnostics without partial output.

## Library

Use `NewResolvedSource`, `ParseSchema`, and `Resolver` for includes/imports.
Resolvers receive namespace URNs and lexical locations; the library opens no
resources. `ParseSchema` accepts mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects graph policy. Queries and walks are deterministic.
Identity constraints retain ordered XPath,
namespaces, and targets; consumers reject. Atomic NCName enumerations,
including inline attributes, are query-only; enumerated list/union members reject.

`ValidateInstance(schema, sourceID, reader)` checks instances with
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals or global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits global `xs:byte`/`xs:long`/`xs:int`:
built-in fields use `StrictInteger`; named fields use generated types.
Selected local precisionDecimal attributes validate; `GenerateGo` rejects attribute uses.
Named empty/choice/sequence and grouped/direct-sequence extensions query built-in/facet-free `xs:ID` locals (Strict10: one); refs/uses stay ordered over empty bases, and `0/0`/prohibited omit after checks; consumers reject extensions.
Global inline grouped and direct-sequence extensions expose ordered non-ID atomic local/ref attributes over named empty bases; consumers reject.
Named empty types accept effective-empty `block` across policies; empty/whitespace overrides `blockDefault="#all"` with authored `Loc`; global-element `@type` and `GenerateGo` reject.
See [contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), [occurrences](docs/decisions/0007-particle-occurrence.md).

Direct choices/sequences and bounded extensions expose built-in/named long locals;
extension choices are attribute-free. Valid inline `0/0` omits after checks; nonzero rejects.
Named-complex choices/sequences query built-in/named `xs:positiveInteger`/`xs:nonPositiveInteger`;
named-complex direct choices/sequences query built-in/named effective `xs:normalizedString` facets/ranges; consumers reject.
`GenerateGo` supports default Boolean/integer/decimal choice refs, ordered
default integer/decimal sequence refs; other targets/occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named effective
integer/decimal/Boolean/token/NMTOKEN/negativeInteger/long/int/short/byte/unsignedLong,
built-in string, built-in/named positiveInteger/nonPositiveInteger/nonNegativeInteger, refs with exact bounds; consumers reject.
Compatibility/Strict11 expose QName `notQName` on direct strict `xs:any`; consumers reject wildcards.

## CLI

`goxsd9` provides `parse`, `validate`, and `generate`:

```sh
go run ./cmd/goxsd9 parse schema.xsd
go run ./cmd/goxsd9 validate schema.xsd instance.xml
go run ./cmd/goxsd9 generate --package generated schema.xsd
```

`parse` summarizes schemas; successful `validate` is silent; `generate` writes
Go to stdout or `--output`. `--schema-root DIR` bounds resolution and is
required for stdin. `--diagnostics json` emits machine-readable errors.

## Development

Run checks from a fresh checkout:

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
```

[Plan](PLAN.md), [issues](https://github.com/goxdra/goxsd9/issues), [roadmap](https://github.com/orgs/goxdra/projects/1), and
[operations](docs/operations.md) cover work; [AGENTS.md](AGENTS.md) gives repository rules.

## License

Apache-2.0 ([LICENSE](LICENSE)); the W3C submodule retains `00COPYRIGHT`.
