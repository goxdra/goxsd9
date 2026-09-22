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
walks preserve discovery/lexical order and sort unordered sets. `Schema`,
`SchemaDocument`, `Component`, `ComponentID`, and expanded `QName` expose copied
views; IDs use source/ordinal, local particles are scoped, consumers are on demand.

Primitive: `DeclaredType`; direct-local choice/sequence and bounded attribute-free extension choice/sequence over named empty-content bases retain anonymous Boolean/integer/decimal refs; only default direct choices and bounded attribute-free extension choices retain local built-in/named-effective `precisionDecimal` refs (QName/facets/locations/occurrences/bounds). Anonymous refs preserve `SimpleTypeID`/`NodeID`/`AnonymousID`; no `ComponentID`/global-walk ownership. Model-less extensions retain only base identity/locations. After admission, `0/0` absent: Compatibility/Strict11 omit; Strict10 rejects first, including zero.
Mapped non-`0/0` anonymous integer particles allow only `integer`/`negativeInteger` through named/forward/imported/included/chameleon chains; excluded `long`/`unsignedLong`/`nonNegativeInteger`/`nonPositiveInteger` valid but unsupported at type/facet `Loc`, with `FailureUnsupported`/`ErrUnsupported` and no schema.
Global attributes retain at most one immutable declaration-owned `AttributeValueConstraint` for `default`/`fixed`. `AttributeDeclaration.ValueConstraint()` returns a defensive copy with `Kind()`, collapsed `Lexical()`, source `Loc()`, and exact defensive `StrictPrecisionDecimal` via `PrecisionDecimalValue()`. Built-in/named effective-facet types admit query facts under Compatibility/Strict11; Strict10 policy diagnostic: type `Loc` precedes conversion. Invalid lexical/facet values retain outer diagnostic and nested details with no schema; inline/local attributes and validation/`GenerateGo` consumers reject.
Global built-in/named long-family refs remain queryable with bounds: `long`
`[-9223372036854775808, 9223372036854775807]`, `unsignedLong`
`[0, 18446744073709551615]`, `negativeInteger` upper `-1`, `nonNegativeInteger`
lower `0`, and `nonPositiveInteger` upper `0`; malformed refs are invalid. These
facts stay separate; global nonNegativeInteger elements generate.
Named complexes accept omitted/`false`/`0`, reject `true`/`1`; malformed XSD 1.1
is invalid, valid behavior outside this slice unsupported. Diagnostics retain code,
primary `Loc`, cause, `SpecRef`. `IsInheritable` accepts Compatibility/Strict11
and mismatches Strict10; untyped/inline attrs, `defaultAttributesApply`, XPath
unsupported/inert.
Anonymous facets query; unsupported mapped non-0/0 non-string enumeration reports facet Loc with no schema. Direct checks use element/particle Locs; extension/model-less gates run first. Model-group refs use group RefLoc and retain locations; nested/local/recursive/broader refs reject.
Global precisionDecimal element/type facts are queryable under Compatibility/Strict11; built-in/named roots validate, inline targets do not, and Strict10 rejects at type Loc before conversion. Local built-in/named-effective refs require default choices or bounded attribute-free extension choices; owners and typed alternatives require default occurrences. Compatibility/Strict11 omits 0/0; Strict10 rejects it first. Only default non-extension choices validate; non-default/nonzero sequences, inline/anonymous forms, extensions, and GenerateGo targets reject.
Mapped non-`0/0` anonymous string/token/NMTOKEN unsupported; `<all>` unsupported.

Complexes expose non-inherited `IsAbstract`; named `final`/simple-type `finalDefault`/local `final` retain immutable `FinalLoc`. Named complex `Final()` uses declaring document's `finalDefault` only when local `final` is absent; explicit empty/non-empty local values override it; default projects only `extension`/`restriction`. Policies agree; `schema/@version` inert. `FinalLoc()` preserves local/default provenance for non-empty controls; occurrence/validation/`GenerateGo` limits unchanged. Prohibited extension is `FailureInvalid` at use-site, related to local/default control; unsupported-base precedence remains. Groups/extensions retain IDs/locations, model-less bases, nil particles, inherited `##other`/lax. Named globals expose `anyAttribute` facts; wildcard consumers unsupported. Direct `xs:any` exposes sorted facts; non-`0/0` and broader placements are consumer-unsupported; `0/0` absent. `openContent=none` supports globals/extensions under Compatibility/Strict11; Strict10 mismatches. Named groups retain ordered refs/ranges; broader shapes unsupported.

## Datatypes

Lexical/value representations remain separate; QName values retain namespace
context. Datatypes map string enumeration and arbitrary-precision scalar values;
precisionDecimal retains exact values/facets under Compatibility/Strict11. Boolean
whitespace collapse supported; Boolean facets, temporal distinctions, broader values
unsupported.

## Validation and code generation

`ValidateInstance` supports global built-in/named scalar roots (`Boolean`/`token`/`NMTOKEN`/`integer`/`decimal`/`precisionDecimal`) and named complexes. Local built-in/named Boolean/integer/decimal sequences honor exact finite, unbounded, and above-`uint64` ranges; named Boolean validates only facet-free restrictions. Non-extension default choices use local built-in/named Boolean/token/NMTOKEN/integer/decimal or explicitly typed built-in/named-effective `precisionDecimal`; homogeneous Boolean/token/NMTOKEN and integer/decimal mixtures validate. Local anonymous Boolean/integer/decimal forms remain query-only; mixed Boolean/numeric or token/NMTOKEN choices, repetition/non-default choices, extensions, anonymous consumers unsupported.
Token/NMTOKEN sequences unsupported. Element refs retain QName/`RefLoc`/`TargetID`/order/exact occurrences; only non-extension default-occurrence direct-choice refs to global built-in/named Boolean/integer/decimal targets are eligible, while sequence/repetition/nested/recursive/broader/anonymous-target/mixed refs are consumer-excluded but queryable. Model-group refs are a separate top-level direct query boundary; nested/local/recursive/broader forms remain unsupported. Other string/list/union/attribute forms are unsupported; token/NMTOKEN collapse XML whitespace, and `xs:any` is query-only (`0/0` absent, nonzero rejected).

Generation: global built-in/named/inherited/included/imported Boolean/integer/decimal/nonNegativeInteger/string/token/NMTOKEN and global inline string/token/NMTOKEN components generate. Only non-extension default-occurrence direct-choice refs to global built-in/named Boolean/integer/decimal targets are eligible; sequences, repetition/non-default, nested/recursive/broader refs, and anonymous targets are rejected.
Global inline Boolean/integer/decimal/long-family/identity-only declarations retain query facts; local inline Boolean/integer/decimal facts are limited to the admitted direct choice/sequence/bounded-extension shapes, while mapped local long-family/identity-only forms remain unsupported. Anonymous consumers reject. Local built-in/named Boolean/integer/decimal default all-Boolean/numeric choices and default-bounded sequences generate; anonymous/token/NMTOKEN and repeated/non-default consumers reject.

## Conformance

W3C XSD artifacts and outcomes are pinned; the harness reports pass, conformance,
unsupported, resolution, and internal failures without changing ranking.

XSD 1.0/1.1 artifacts are URL/digest pinned; tooling indexes them and verifies the
XSD 1.0 envelope/DTD ordering without changing parser or resolver semantics.
