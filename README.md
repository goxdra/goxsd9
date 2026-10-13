# goxsd9

goxsd9 parses XML Schema into immutable components; unsupported features return
located diagnostics without partial output.

## Library

`NewResolvedSource`, `ParseSchema`, and `Resolver` handle includes/imports.
Resolvers receive namespace URNs and lexical locations; library opens no
resources. `ParseSchema` accepts mixed XSD 1.0/1.1 graphs;
`ParseSchemaWithPolicy` selects policy. Queries/walks are deterministic.
Identity constraints retain ordered XPath/namespaces/targets; consumers reject.
Supported named owners query built-in/named NCName/`xs:normalizedString` facets; global
inline NCName enumerations query. Enumerated NCName list/union members reject;
consumers reject.

`ValidateInstance(schema, sourceID, reader)` checks instances with
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals or global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits global `xs:byte`/`xs:long`/`xs:int`:
built-ins use `StrictInteger`; named use generated types.
Selected local precisionDecimal attributes validate; `GenerateGo` rejects uses.
Named empty/choice/sequence and grouped/direct-sequence extension owners query built-in/facet-free `xs:ID` locals (Strict10: one); refs reject; uses ordered; `0/0`/prohibited omit after checks; consumers reject.
Global inline grouped/direct-sequence extensions query ordered non-ID atomic local/ref attributes over empty named bases; consumers reject.
Named empty complexes/untyped global elements admit empty `block` overriding defaults; untyped elements lack views and reject validation/generation. Compatibility/Strict11 omit `inheritable=false/0` on untyped global attributes without typed views; local refs fail schema construction; `ValidateInstance`/`GenerateGo` still reject global attributes. Global-element `@type` rejects named empty complexes; extension-base references work.
See [contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), [occurrences](docs/decisions/0007-particle-occurrence.md).

Direct choices/sequences and bounded extensions expose built-in/named long locals;
extension choices are attribute-free. Valid inline `0/0` omits after checks; nonzero rejects.
Named-complex choices/sequences query built-in/named `xs:positiveInteger`/`xs:nonPositiveInteger`;
named-complex direct choices/sequences query built-in/named effective `xs:normalizedString` facets/ranges; consumers reject.
`GenerateGo` supports default Boolean/integer/decimal choice refs, ordered
default integer/decimal sequence refs; other targets/occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named effective
integer/decimal/Boolean/token/NMTOKEN/negativeInteger/long/int/short/byte/unsignedLong,
built-in string, built-in/named normalizedString/positiveInteger/nonPositiveInteger/nonNegativeInteger, refs with exact bounds; consumers reject.
Compatibility/Strict11 query direct strict `xs:any` QName `notQName`; wildcard consumers reject.

## CLI

`goxsd9` provides `parse`, `validate`, and `generate`:

```sh
go run ./cmd/goxsd9 parse schema.xsd
go run ./cmd/goxsd9 validate schema.xsd instance.xml
go run ./cmd/goxsd9 generate --package generated schema.xsd
```

`parse` summarizes; `validate` succeeds silently; `generate` writes
to stdout or `--output`. `--schema-root DIR` bounds resolution and is
required for stdin. `--diagnostics json` emits machine-readable errors.

## Development

Fresh checkout:

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
```

Links: [plan](PLAN.md), [issues](https://github.com/goxdra/goxsd9/issues),
[roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [rules](AGENTS.md).

## License

Apache-2.0 ([LICENSE](LICENSE)); W3C submodule retains `00COPYRIGHT`.
