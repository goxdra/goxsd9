# goxsd9

goxsd9 parses XML Schema into immutable components for queries, instance validation,
and Go generation. Unsupported features return located diagnostics and no partial output.

## Library

Create a root with `NewResolvedSource` and call `ParseSchema` with a
caller-supplied `Resolver` for includes/imports. It receives namespace URNs and
lexical schema locations; the library opens no paths or URLs. `ParseSchema`
defaults to Compatibility for mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects graph-wide policy. Queries and walks are
immutable and deterministic. Global elements retain ordered `xs:unique`,
`xs:key`, and `xs:keyref` facts with XPath, namespaces, and resolved targets;
validation and generation reject them pending identity semantics.

`ValidateInstance(schema, sourceID, reader)` checks one XML instance, including
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals or global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits global `xs:long`:
built-in fields use `StrictInteger`, named fields use generated types; global
long attributes remain query-only. Grouped extensions retain refs/attributes
over named empty bases; valid `0/0` omits particles, and prohibited uses may
leave no effective uses.
See the [package contract](doc.go), [architecture](ARCHITECTURE.md#schema-model),
and [decision 0007](docs/decisions/0007-particle-occurrence.md) for details.

Direct choices, sequences, and bounded attribute-free extensions admit
built-in/named long locals for queries; valid inline `0/0` omits, nonzero rejects.
`GenerateGo` supports default Boolean/integer/decimal choice refs and ordered
default integer/decimal sequence refs; other targets or occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named integer/decimal/
Boolean, built-in string, effective token/NMTOKEN, negativeInteger/
nonNegativeInteger, and refs with exact bounds; consumers reject it.

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

The [plan](PLAN.md) gives project phases. [Issues](https://github.com/goxdra/goxsd9/issues),
the [roadmap](https://github.com/orgs/goxdra/projects/1), and [operations](docs/operations.md)
cover ongoing work. See [AGENTS.md](AGENTS.md) for repository rules.

## License

Apache-2.0 ([LICENSE](LICENSE)); the W3C submodule retains `00COPYRIGHT`.
