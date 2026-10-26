# goxsd9

goxsd9 parses XML Schema into immutable components or located errors;
no partial schemas.

## Library

`NewResolvedSource`/`ParseSchema`/`Resolver` resolve mixed XSD 1.0/1.1 graphs.
Resolvers receive URNs/locations.
`ParseSchemaWithPolicy` selects policy; queries/walks deterministic.
Identity constraints retain XPath/namespaces/targets; consumers reject.
Named empty/choice/sequence/all/grouped/sequence-extension: NCName/`xs:normalizedString`. Empty/choice/sequence/all; named direct-sequence/grouped extensions: built-in/named facet-free `xs:language`/`xs:anyURI` and effective `xs:long`; named direct-group: built-in `xs:language`/`xs:anyURI` only. Attribute-use grouped extensions: empty bases. Named zero-use direct-group extensions: default 1/1 sequence, one 0/1 global-element ref; consumers reject. Language/anyURI/long: query-only. Named empty/choice/sequence/direct-sequence/grouped extensions: effective built-in/named `xs:token`/`xs:NMTOKEN`; consumers reject. Empty/direct-all: effective built-in/named `xs:short`/`xs:int`/`xs:byte`/`xs:unsignedLong` attributes; named grouped extensions: built-ins. Named attribute-free simpleContent: built-in/named `xs:normalizedString`/`xs:token`/`xs:NMTOKEN`/facet-free `xs:language`/`xs:anyURI`; consumers reject. Grouped `xs:simpleDerivationSet` `final` query-only; enumerated NCName list/union reject.

`ValidateInstance(schema, sourceID, reader)` checks instances with
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals/global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits `xs:byte`/`xs:long`/`xs:int`/`xs:unsignedLong`:
built-ins: `StrictInteger`; named: generated types.
Selected local precisionDecimal attributes validate; `GenerateGo` rejects uses.
Named empty/choice/sequence/all/grouped/sequence-extension query facet-free `xs:ID`; inline grouped/sequence extensions query atomic uses. Consumers reject. Empty `block` overrides named empty complexes/untyped globals. Compatibility/Strict11 omit false/0 `inheritable` on supported named locals, anonymous global-element inline grouped/direct-sequence non-ID locals, and untyped globals; refs reject.
See [contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), [occurrences](docs/decisions/0007-particle-occurrence.md).

Anonymous empty-base extensions query ref-only choices/sequences; bounds, order, and provenance persist; deeper forms reject.
Named/global-inline wildcard-only restrictions query empty/use-free named bases with ##any/##other/positive-set wildcard subsets; validation/generation reject.
Named use-free, wildcard-free restrictions query copied optional global-element and group refs over a completed extension; IDs, bounds, and `Loc`s persist. Consumers reject.
Named-complex choices/sequences query built-in/named `xs:positiveInteger`/`xs:nonPositiveInteger`;
named-complex direct choices/sequences query built-in/named effective `xs:normalizedString` facets/ranges; consumers reject.
`GenerateGo` supports default Boolean/integer/decimal choice refs, ordered
default integer/decimal sequence refs; other targets/occurrences reject.
Direct named-complex `xs:all` retains ordered built-in/named effective
integer/decimal/Boolean/token/NMTOKEN/negativeInteger/long/int/short/byte/unsignedLong,
built-in string, built-in/named normalizedString/positiveInteger/nonPositiveInteger/nonNegativeInteger, refs with exact bounds; consumers reject.
Compatibility/Strict11 query strict `xs:any` `notQName`; consumers reject.

## CLI

`goxsd9` commands: `parse`, `validate`, `generate`:

```sh
go run ./cmd/goxsd9 parse schema.xsd
go run ./cmd/goxsd9 validate schema.xsd instance.xml
go run ./cmd/goxsd9 generate --package generated schema.xsd
```

`parse` summarizes; `validate` succeeds; `generate` writes
stdout/`--output`. `--schema-root DIR` bounds resolution; stdin requires
it. `--diagnostics json` emits errors.

## Development

```sh
git submodule update --init --recursive
go tool workflowctl doctor
go tool workflowctl check
```

[plan](PLAN.md), [issues](https://github.com/goxdra/goxsd9/issues),
[roadmap](https://github.com/orgs/goxdra/projects/1), [operations](docs/operations.md), [rules](AGENTS.md).

## License

Apache-2.0 ([LICENSE](LICENSE)); W3C submodule retains `00COPYRIGHT`.
