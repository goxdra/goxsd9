# goxsd9

Parses XSD into immutable components or diagnostics.

## Library

`NewResolvedSource`/`ParseSchema`/`Resolver` resolve XSD 1.0/1.1 graphs.
`ParseSchemaWithPolicy` selects policy.
Admitted typed globals retain authored identity constraints; consumers reject.
Named complex and extension shapes query selected atomic scalars; attribute-free named simpleContent queries selected strings. Exact owner/base limits and exclusions: [contract](doc.go).

`ValidateInstance(schema, sourceID, reader)` checks instances with
built-in/named `xs:short`, `xs:int`, `xs:long`, `xs:unsignedLong`, and bounded
precisionDecimal lists/unions. Compatibility/Strict11 validate ordered
precisionDecimal sequences of typed locals/global refs; `xsi:schemaLocation`
never resolves. `GenerateGo(schema, packageName)` emits `xs:byte`/`xs:long`/`xs:int`/`xs:unsignedLong`:
built-ins: `StrictInteger`; named: generated types.
Selected local precisionDecimal attributes validate; `GenerateGo` rejects uses.
Untyped global elements expose abstract/nillable facts/`Loc`s without an authored type; consumers reject. Named empty/choice/sequence/all/direct-group/grouped/sequence-extension complex owners query facet-free `xs:ID`; supported global-inline grouped/direct-sequence extensions query selected non-ID atomic local uses; consumers reject. `block=""` overrides blockDefault with `Loc`. Compatibility/Strict11 omit false/0 `inheritable` for named empty/all/choice/sequence/group or grouped/direct-sequence-extension locals, selected name-bearing non-ID global-inline grouped/direct-sequence-extension locals, and untyped global attributes (no view; refs reject).
[contract](doc.go), [architecture](ARCHITECTURE.md#schema-model), [occurrences](docs/decisions/0007-particle-occurrence.md).

Empty-base anonymous extensions query ref-only choices plus nested group-ref/element-ref sequences. Bounds/order/`Loc`s/wildcards persist; deeper nesting/attributes reject.
Anonymous empty-base ref-sequences query built-in `xs:token`/`xs:anyURI`; consumers reject.
Named/global-inline wildcard-only restrictions query ##any/##other/positive-set subsets of empty/use-free named bases; consumers reject.
Named-group sequences/nested choices: atomic integer/string/token; consumers reject.
Named-complex choices/sequences query built-in/named `xs:positiveInteger`/`xs:nonPositiveInteger`;
named-complex direct choices/sequences query built-in/named effective `xs:normalizedString` facets/ranges; consumers reject.
`GenerateGo` supports non-extension default-occurrence direct-choice refs to eligible
typed Boolean/integer/decimal globals and ordered default-occurrence direct-sequence
refs to eligible typed integer/decimal globals; other targets reject.
Named choice/sequence and empty-base direct-sequence/grouped extensions query
built-in/named short/int/byte/unsignedLong attributes; consumers reject.
Named empty/direct-choice/direct-sequence owners query facet-free built-in/named
QName local attributes; consumers reject.
Direct named-complex `xs:all` retains ordered built-in/named effective
integer/decimal/Boolean/token/NMTOKEN/negativeInteger/long/int/short/byte/unsignedLong,
built-in string, built-in/named normalizedString/positiveInteger/nonPositiveInteger/nonNegativeInteger, refs with exact bounds; consumers reject.
Compatibility/Strict11: global-element inline complexType admits negative `xs:anyAttribute` with one empty default-bound sequence and no uses; omitted/strict/lax require absent `notQName`, skip admits empty. Named direct choice/sequence complex types admit QName-only negative `notQName` under strict/lax/skip; symbolic `notQName` with negative `notNamespace` rejects lax/skip. Consumers reject. [Contract](doc.go).

## CLI

```sh
go run ./cmd/goxsd9 parse schema.xsd
```

Also: `validate`, `generate`.
`generate`: stdout/`--output`; `--schema-root DIR` bounds resolution
(stdin requires it); `--diagnostics json`: errors.

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
