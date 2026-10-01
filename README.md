# goxsd9

goxsd9 parses XML Schema documents into an immutable component model. You can
query the model, validate supported XML instances, or generate Go for supported
schema components. Unsupported features return located diagnostics rather than
partial schemas or output.

## Library

Create a root with `NewResolvedSource`, then call `ParseSchema` with a caller
supplied `Resolver` for includes and imports. The resolver receives namespace
URNs and lexical schema locations; the library does not open paths or URLs.
`ParseSchema` uses the Compatibility policy for mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects a graph-wide language policy. Schema queries
and walks return immutable, deterministic views.
Supported global elements retain ordered `xs:unique`, `xs:key`, and `xs:keyref`
facts, including raw XPath expressions, namespace context, and resolved keyref
targets. Instance validation and Go generation reject these declarations until
identity semantics are implemented.

`ValidateInstance(schema, sourceID, reader)` checks one XML instance;
`GenerateGo(schema, packageName)` returns Go source. A component can be
queryable even when one or both consumers reject it.
Grouped complex-content extensions resolve one opaque group reference and
ordered local attribute uses over a supported named empty base. Validated
`0/0` omits the particle, and prohibited uses may leave no effective uses.
See the [package contract](doc.go) for public behavior and current limits, the
[architecture](ARCHITECTURE.md#schema-model) for admission and consumer
boundaries, and [decision 0007](docs/decisions/0007-particle-occurrence.md)
for exact particle occurrences and `0/0` omission.

Direct choices/sequences/bounded attribute-free extensions expose local `xs:long`
and supported named effective-long particles as query-only facts.
Direct strict XSD 1.1 `xs:any` exposes explicit `notQName`.

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
