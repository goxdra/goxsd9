# goxsd9

goxsd9 parses XML Schema into an immutable component model. Query it, validate
supported instances, or generate Go. Unsupported features return located
diagnostics rather than partial schemas or output.

## Library

Create a root with `NewResolvedSource`, then call `ParseSchema` with a caller
supplied `Resolver` for includes and imports. The resolver receives namespace
URNs and lexical schema locations; the library does not open paths or URLs.
`ParseSchema` uses the Compatibility policy for mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects a graph-wide language policy. Schema queries
and walks return immutable, deterministic views.
Supported global elements retain ordered `xs:unique`, `xs:key`, and `xs:keyref` facts
with XPath, namespaces, and resolved keyref targets. Validation and generation reject
them until identity semantics are implemented.

`ValidateInstance(schema, sourceID, reader)` checks one XML instance, including
global built-in and supported named `xs:short` values and bounded
precisionDecimal lists/unions;
`GenerateGo(schema, packageName)` returns Go source. A component can be
queryable even when one or both consumers reject it.
Grouped extensions retain refs/attributes over named empty bases; `0/0` omits.
See the [package contract](doc.go), [architecture](ARCHITECTURE.md#schema-model),
and [decision 0007](docs/decisions/0007-particle-occurrence.md) for behavior and exact occurrences.

Direct choices, sequences, and bounded attribute-free extensions retain local built-in
`xs:long` and supported named effective-long particles as query-only immutable facts.
`GenerateGo` supports default Boolean/integer/decimal choice refs and ordered
default integer/decimal sequence refs; other reference targets or occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named integer, decimal,
Boolean locals, built-in `xs:token`/`xs:NMTOKEN` locals, and element refs with exact bounds.
Validation/generation reject it.

## CLI

The `goxsd9` command provides `parse`, `validate`, and `generate`:

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
the [roadmap](https://github.com/orgs/goxdra/projects/1), and
[operations](docs/operations.md) cover ongoing work. See [AGENTS.md](AGENTS.md)
for repository rules.

## License

Apache-2.0 ([LICENSE](LICENSE)); the W3C submodule retains `00COPYRIGHT`.
