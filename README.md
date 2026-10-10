# goxsd9

goxsd9 parses XML Schema into immutable components for queries, instance validation,
and Go generation. Unsupported features return located diagnostics without partial output.

## Library

Use `NewResolvedSource`, `ParseSchema`, and a `Resolver` for
includes/imports. It receives namespace URNs and
lexical schema locations; the library opens no paths or URLs. `ParseSchema`
defaults to Compatibility for mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects graph-wide policy. Queries and walks are
immutable and deterministic. Identity constraints retain ordered XPath,
namespaces, and targets; consumers reject. Atomic NCName enumerations,
including inline attributes, are query-only; enumerated list/union members reject.

`ValidateInstance(schema, sourceID, reader)` checks an instance, including
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals or global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits global `xs:byte`/`xs:long`/`xs:int`:
built-in fields use `StrictInteger`, named fields use generated types; their
attributes remain query-only. Named empty/choice/sequence/grouped owners
expose built-in/facet-free `xs:ID` locals (Strict10: one ID); consumers reject.
Grouped extensions retain refs/attributes over named empty bases; valid `0/0`
omits particles and prohibited uses may leave no effective uses.
See the [package contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), and [decision 0007](docs/decisions/0007-particle-occurrence.md).

Direct choices/sequences and bounded attribute-free extensions expose built-in/named long locals; valid inline `0/0` omits, nonzero rejects.
Direct named-complex choices/sequences expose query-only built-in/named
`xs:positiveInteger` and `xs:nonPositiveInteger` with exact bounds.
`GenerateGo` supports default Boolean/integer/decimal choice refs and ordered
default integer/decimal sequence refs; other targets or occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named effective
integer/decimal/Boolean/token/NMTOKEN/negativeInteger/long/int/short/byte/unsignedLong,
built-in string/positiveInteger/nonPositiveInteger, built-in/named nonNegativeInteger, and refs with exact bounds; consumers reject it.
Compatibility/Strict11 expose QName `notQName` on direct strict `xs:any`; consumers reject wildcards.

## CLI

`goxsd9` provides `parse`, `validate`, and `generate`:

```sh
go run ./cmd/goxsd9 parse schema.xsd
go run ./cmd/goxsd9 validate schema.xsd instance.xml
go run ./cmd/goxsd9 generate --package generated schema.xsd
```

`parse` summarizes schemas; successful `validate` is silent; `generate` writes
Go to standard output or `--output`. `--schema-root DIR` sets the local
resolution boundary and is required for standard input. `--diagnostics json`
selects machine-readable errors; processing failures exit 1, usage errors 2.

## Development

Run the repository checks from a fresh checkout:

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
```

[Plan](PLAN.md), [issues](https://github.com/goxdra/goxsd9/issues), [roadmap](https://github.com/orgs/goxdra/projects/1), and
[operations](docs/operations.md) cover work; [AGENTS.md](AGENTS.md) gives repository rules.

## License

Apache-2.0 ([LICENSE](LICENSE)); the W3C submodule retains `00COPYRIGHT`.
