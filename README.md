# goxsd9

goxsd9 parses XML Schema into immutable components or located errors;
no partial schemas.

## Library

`NewResolvedSource`/`ParseSchema`/`Resolver` resolve mixed XSD 1.0/1.1 graphs.
Resolvers receive URNs/locations.
`ParseSchemaWithPolicy` selects policy.
Identity constraints retain XPath/namespaces/targets; consumers reject.
Named empty/choice/sequence/all/direct-group/grouped/sequence-extension: NCName/`xs:normalizedString`. Language/anyURI/long/token/NMTOKEN/short/int/byte/unsignedLong limits: [contract](doc.go); consumers reject. Named effective short/int/byte/unsignedLong grouped uses require empty/use-free bases. Grouped inherited-ID exception: immediate completed sequence-extension base (default 1/1 sequence, one 0/1 global ref, one optional direct local ID), querying inherited ID and own optional final:xs:simpleDerivationSet/name:xs:NCName; consumers reject. Named zero-use direct-group extensions: default 1/1 sequence, use-free base; consumers reject. Named attribute-free simpleContent: normalizedString/token/NMTOKEN/language/anyURI; consumers reject. Enumerated NCName list/union reject.

`ValidateInstance(schema, sourceID, reader)` checks instances with
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals/global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits `xs:byte`/`xs:long`/`xs:int`/`xs:unsignedLong`:
built-ins: `StrictInteger`; named: generated types.
Selected local precisionDecimal attributes validate; `GenerateGo` rejects uses.
Named empty/choice/sequence/all/direct-group/grouped/sequence-extension query facet-free `xs:ID`; inline grouped/sequence extensions query atomic uses. Consumers reject. Empty `block` overrides named empty complexes/untyped globals. Compatibility/Strict11 omit false/0 `inheritable` on supported named locals, anonymous global-element inline grouped/direct-sequence non-ID locals, and untyped globals; refs reject.
See [contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), [occurrences](docs/decisions/0007-particle-occurrence.md).

Anonymous empty-base extensions query ref-only choices and nested group-ref/element-ref sequences, alone or combined. Bounds/order/`Loc`s/wildcards persist; deeper nesting/attributes reject.
Anonymous empty-base ref-sequence extensions query built-in `xs:token`/`xs:anyURI`; consumers reject.
Named/global-inline wildcard-only restrictions query empty/use-free named bases with ##any/##other/positive-set wildcard subsets; validation/generation reject.
Named-group sequences/nested choices: atomic integer/string/token; consumers reject.
Named-complex choices/sequences query built-in/named `xs:positiveInteger`/`xs:nonPositiveInteger`;
named-complex direct choices/sequences query built-in/named effective `xs:normalizedString` facets/ranges; consumers reject.
`GenerateGo` supports default Boolean/integer/decimal choice refs, ordered
default integer/decimal sequence refs; other targets/occurrences reject.
Named choice/sequence and empty-base direct-sequence/grouped extensions query
built-in/named short/int/byte/unsignedLong attributes; consumers reject.
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
