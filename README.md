# goxsd9

goxsd9 parses XML Schema into immutable components for queries, instance validation,
and Go generation. Unsupported features return located diagnostics without partial output.

## Library

Use `NewResolvedSource`, `ParseSchema`, and a `Resolver` for
includes/imports. It receives namespace URNs and
lexical schema locations; the library opens no paths or URLs. `ParseSchema`
defaults to Compatibility for mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects graph-wide policy. Queries and walks are
immutable and deterministic. Global `xs:unique`, `xs:key`, and `xs:keyref` retain ordered XPath,
namespaces, and targets;
consumers reject pending identity semantics. Atomic NCName enumerations, including global inline attributes, are query-only; enumerated list items/union members reject.

`ValidateInstance(schema, sourceID, reader)` checks one XML instance, including
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals or global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits global `xs:long`/`xs:int`:
built-in fields use `StrictInteger`, named fields use generated types; global
long/int attributes remain query-only. Named empty/choice/sequence/grouped
owners expose built-in/facet-free `xs:ID` locals (Strict10: one ID);
consumers reject.
See the [package contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), and [decision 0007](docs/decisions/0007-particle-occurrence.md).

Direct choices/sequences and bounded attribute-free extensions expose built-in/named long locals; valid inline `0/0` omits, nonzero rejects.
Direct named-complex choices/sequences expose query-only built-in
`xs:positiveInteger`/`xs:nonPositiveInteger` locals with intrinsic bounds 1/0.
`GenerateGo` supports default Boolean/integer/decimal choice refs and ordered
default integer/decimal sequence refs; other targets or occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named effective
integer/decimal/Boolean/token/NMTOKEN/negativeInteger/long/int/short/byte/unsignedLong, built-in
string, built-in/named nonNegativeInteger, and refs with exact bounds; consumers reject it.
Compatibility/Strict11 expose QName `notQName` on direct strict `xs:any`; consumers reject wildcards.

## CLI

`goxsd9` provides `parse`, `validate`, and `generate`:

```sh
go run ./cmd/goxsd9 parse schema.xsd
go run ./cmd/goxsd9 validate schema.xsd instance.xml
go run ./cmd/goxsd9 generate --package generated schema.xsd
```

`parse` prints a schema summary, successful `validate` is silent, and
`generate` writes Go to standard output unless `--output` names a file. Use
`--schema-root DIR` to set the local resolution boundary; it is required when
reading a schema from standard input. `--diagnostics json` selects
machine-readable errors; input/processing failures exit 1 and usage failures
exit 2.

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
