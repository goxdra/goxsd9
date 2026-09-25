# Architecture

## Boundaries

goxsd9 parses schema, exposes immutable queries/walks, validates XML, and generates
Go; validation/generation are schema-model leaves.

## Deterministic phase pipeline

```mermaid
flowchart LR
  A["Root byte stream"] --> B["XML and XSD syntax"]
  B --> C["Document discovery queue"]
  C --> D["Component declarations and identities"]
  D --> E["Reference and derivation ordering"]
  E --> F["Schema constraints and facets"]
  F --> G["Immutable Schema"]
  G --> H["Instance validator"]
  G --> I["Go code generator"]
```

Phases consume results; immutable components never backpatch. Identities intern
before discovery; repeats/cycles close, acyclic dependencies use stable topological
order, and ordered slices define walks/output.

## Input and resolution

Entrypoint: `ParseSchema(root ResolvedSource, resolver Resolver)`. Resolvers supply
references/policy; streams close; identities decode once; repeats/cycles close
without decoding.

```go
type Resolver interface {
    Resolve(
        ctx context.Context,
        namespaceURN string,
        schemaLocation string,
    ) (ResolvedSource, error)
}
```

Sources carry opaque identity, reader-closer, child context; resolvers may store
private base-location state. FIFO discovery preserves context. Parser leaves opaque
identities/locations uninterpreted, opens no paths, makes no network requests.
Resolver calls sequential.

Decode captures one-based line and Unicode-code-point columns; components retain
`Loc`, not source bytes.

## Diagnostics

Diagnostics classify invalid, unsupported, resolution, and internal failures; retain
stable codes, primary `Loc`, related/specification references, and causes; errors prevent
schema return. Unsupported features have stable report IDs.

## Schema model

Raw syntax is internal; immutable components retain `Loc`; queries use names/identities;
walks preserve discovery/lexical order and sort unordered sets. `Schema`, `SchemaDocument`,
`Component`, `ComponentID`, and expanded `QName` expose copied views; IDs use source/ordinal,
local particles are scoped, consumers are on demand.

Primitive: `DeclaredType`; typed scalar particles use bounded attribute-free extension choices/sequences over named empty-content bases or named `complexContent` restrictions of `xs:anyType`; retain anonymous refs (`SimpleTypeID`/`NodeID`/`AnonymousID`), inherited `##other`/`lax` facts; model-less identity/locations; extension consumers reject. precisionDecimal extension choices-only.
Local admission: direct choices/sequences/permitted extension choices admit `integer`, effective named/inline `negativeInteger`, and explicit built-in/supported named `unsignedLong`; direct `xs:negativeInteger` rejects when mapped nonzero. Mapped nonzero scalar exclusions return located `FeatureSchemaSyntax`/`FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at type/facet/element `Loc`, no schema; permitted `0/0` is absent only after syntax, occurrence, reference, and policy gates. Element-reference and top-level model-group refs are distinct queryable boundaries with QName/RefLoc/TargetID; effective `negativeInteger`/`unsignedLong` are consumer-only `FailureUnsupported`.
Attributes query: built-in/supported named atomic `xs:boolean`, `xs:integer`, `xs:decimal`, `xs:token`, `xs:negativeInteger`, `xs:language`, `xs:NCName`, `xs:anyURI`, and `xs:ID`, plus `xs:long`/`xs:unsignedLong` restrictions. Built-in/named `xs:precisionDecimal` is query-only under Compatibility/Strict11; Strict10 rejects at type `Loc` with `FeatureDatatypeFacets`/`FailureUnsupported`/`XSD3030`/`ErrUnsupported`. Excluded `xs:string`, `xs:NMTOKEN`, `xs:int`, `xs:nonNegativeInteger`, `xs:nonPositiveInteger`, narrower built-ins, and list/union refs report `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at type `Loc`; local/inline forms report at element/inline `simpleType` `Loc`. Earlier failures win; unsupported forms return no schema. Value constraints support Boolean/integer/decimal/token/precisionDecimal. Unsupported default/fixed values use `FailureUnsupported`/`UnsupportedSchemaSyntaxCode`/`ErrUnsupported` at value `Loc`; invalid supported values use `FailureInvalid`/`XSD3036` at value `Loc` with cause; default+fixed uses `FailureInvalid`/`XSD3010` with fixed primary/default related. Errors return no `Schema`. Built-in `xs:long`/`xs:unsignedLong` bounds are `[-9223372036854775808,9223372036854775807]` and `[0,18446744073709551615]`; named refs retain QName/type `Loc`, exact bounds/facets, locations, provenance, and target IDs; built-ins have no `ComponentID`. Type-only precision has no constraint; consumers reject.
Complexes accept omitted/`false`/`0`, reject `true`/`1`; malformed XSD 1.1 is invalid,
valid behavior unsupported. Diagnostics retain code/`Loc`/cause/`SpecRef`; `IsInheritable` accepts
Compatibility/Strict11 only. Untyped/inline attrs, `defaultAttributesApply`/XPath, and non-0/0
anonymous enumeration are unsupported. Direct checks use `Locs`; extension/model-less gates run
first, model-group refs use `RefLoc`, and broader forms reject.
`precisionDecimal` refs require default-occurrence direct/bounded extension
choices; nonzero sequences, inline/anonymous targets, and non-default choices
remain unsupported. PrecisionDecimal: Strict10 precedes permitted `0/0` omission;
Compatibility/Strict11 omits post-gate. Homogeneous local
built-in/supported named `token`/`NMTOKEN` sequences admit exact
finite/unbounded/above-`uint64` occurrences under all policies. Built-in/named
roots/direct defaults validate Compatibility/Strict11; extension choices
query-only, `GenerateGo` rejects targets, refs queryable; local token/NMTOKEN
particles/sequences `GenerateGo`-unsupported; `<all>` unsupported.

Complexes expose non-inherited `IsAbstract`; `Final()` uses declaring-document `finalDefault`,
explicit locals override, and `FinalLoc()` preserves provenance. Policies agree; `schema/@version`
is inert. Prohibited extension is `FailureInvalid` at use-site; unsupported base precedence remains.
Groups/extensions retain IDs/locations/wildcards; wildcard consumers reject. `xs:any` facts are
sorted; non-`0/0`/broader forms reject and `0/0` is absent. `openContent=none` supports
globals/extensions under Compatibility/Strict11 and mismatches Strict10; named groups retain
ordered refs/ranges; broader shapes unsupported.

## Datatypes

Lexical/value representations remain separate; QName values retain namespace context. Datatypes map
string enumeration and arbitrary-precision values; precisionDecimal retains exact values/facets under
Compatibility/Strict11. Boolean whitespace collapse is supported; broader facets/temporal values are
unsupported.

## Validation and code generation

`ValidateInstance` supports built-in/named Boolean/token/NMTOKEN/integer/decimal roots and
precisionDecimal roots only under Compatibility/Strict11; Strict10 rejects first, and
inline/anonymous targets remain query-only/consumer-rejected.
Built-in/named `nonNegativeInteger` is GenerateGo-only; validation returns
located `FailureUnsupported`/`XSD4004`/`ErrUnsupported`. Local Boolean/integer/decimal
sequences/default choices honor ranges; homogeneous token/NMTOKEN sequences honor exact
occurrences/value space. Anonymous/mixed-family/extension consumers reject; nonzero `xs:any`
is unsupported. Element refs retain QName/RefLoc/TargetID/order/occurrences without target gating;
only default direct-choice refs to global built-in/named Boolean/integer/decimal are eligible, other
forms remain queryable but excluded. Global `nonNegativeInteger` refs remain queryable;
direct-choice/sequence consumers reject with located unsupported diagnostics/nil output. Model-group
refs are top-level direct query only; broader forms reject.

Generation: named Boolean/integer/decimal/string/token/NMTOKEN components; global elements using
those built-in/named types; inline global string/token/NMTOKEN elements; global/named-typed
`nonNegativeInteger` elements; standalone named `nonNegativeInteger` components—all policies. Only elements require `abstract=false,nillable=false`
(either true: `FailureUnsupported`/`GOXSD9029`, nil). Built-in/standalone fields use
`StrictInteger`; named-typed fields use generated types. Canonical built-in facts require integer
kind/version, fixed `fractionDigits=0`, `minInclusive=0`, and no `totalDigits`/other bounds;
named bounds/facets remain, while final/variety/effective-facet gates reject
(`FailureUnsupported`/`GOXSD9029`, no output) and malformed/stale facts fail internally
(`FailureInternal`/`GOXSD9030`, nil). Mapped nonzero local `nonNegativeInteger` forms: no schema;
supported local Boolean/integer/decimal/token/NMTOKEN particles may have schema; consumer
exclusions apply; `0/0` admitted then absent all policies. `nonNegativeInteger` refs remain queryable; direct-choice/sequence
consumers reject, and inline/anonymous element/type forms remain query-only/rejected.
Global `long`/`unsignedLong` element/type facts query-only; validation/`GenerateGo` reject. Local
token/NMTOKEN particles/sequences remain `GenerateGo`-unsupported; inline Boolean/integer/decimal
elements query-only/rejected. Attributes remain query-only; `GenerateGo` rejects every
`ComponentKindAttributeDeclaration`. Local generation is limited to default-occurrence
Boolean/integer/decimal choices/sequences; `unsignedLong`, `precisionDecimal`, token/NMTOKEN,
anonymous, repeated, non-default forms excluded.

## Conformance

W3C XSD artifacts/outcomes are pinned; the harness reports pass, conformance, unsupported,
resolution, and internal failures without changing ranking.

XSD artifacts are URL/digest pinned; tooling verifies XSD 1.0 envelope/DTD ordering without
changing parser or resolver semantics.
