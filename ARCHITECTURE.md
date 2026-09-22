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

Primitive: `DeclaredType`; direct-local/bounded attribute-free extension choice/sequence over named empty bases retain anonymous Boolean/integer/decimal refs; default choices and bounded attribute-free extension choices retain local built-in/named-effective `precisionDecimal` refs (QName/facets/locations/occurrences/bounds). Anonymous refs preserve `SimpleTypeID`/`NodeID`/`AnonymousID`, not `ComponentID`/global-walk ownership. Model-less extensions retain base identity/locations. After admission, `0/0` absent: Compatibility/Strict11 omit; Strict10 rejects first, including zero.
Mapped non-`0/0` anonymous integer particles allow only `integer`/`negativeInteger` through named/forward/imported/included/chameleon chains; excluded long-family types are valid but unsupported at type/facet `Loc` (`FailureUnsupported`/`ErrUnsupported`), with no schema. Attrs retain default/fixed lexical/location facts.
Global built-in/named long-family refs remain queryable with bounds: `long`
`[-9223372036854775808, 9223372036854775807]`, `unsignedLong`
`[0, 18446744073709551615]`, `negativeInteger` upper `-1`, `nonNegativeInteger`
lower `0`, `nonPositiveInteger` upper `0`; malformed refs invalid. These facts are
separate from local admission.
Named complexes accept omitted/`false`/`0`, reject `true`/`1`; malformed XSD 1.1 is
invalid; behavior outside this slice unsupported. Diagnostics retain code, `Loc`,
cause, `SpecRef`. `IsInheritable` accepts Compatibility/Strict11, mismatches
Strict10; untyped/inline attrs unsupported.
`defaultAttributesApply` (`true|false|1|0`) for named globals validates/discards
under Compatibility/Strict11 without schema-level `defaultAttributes`; Strict10
mismatches. XPath unsupported/inert.
Anonymous facets queryable; mapped non-`0/0` non-string enumeration reports
`FailureUnsupported`/`ErrUnsupported` at facet `Loc`, with no schema. Direct checks
use element/particle `Loc`; extension/model-less gates run first (codegen extension
primary; validation owner/sequence primary). Top-level model-group refs use group
`RefLoc`, retaining particle/group/component/reference/target locations; nested/
local/recursive/broader refs unsupported; no output.
`precisionDecimal` queryable only under Compatibility/Strict11; built-in/named roots
validate, inline does not; Strict10 rejects first. Policy admits local declared
built-in `xs:precisionDecimal` or named effective-facet types only in default direct
or bounded attribute-free extension choices; owners and typed children/alternatives
require default occurrences; nonprecision alternatives remain query-only. Inline anonymous
`<xs:simpleType><xs:restriction base="xs:precisionDecimal">` forms unsupported.
Compatibility/Strict11 omit `0/0` in both forms; Strict10 rejects both before
omission, including zero. Non-default choices/nonzero direct sequences reject; only
non-extension default choices validate; extension, inline/anonymous, and
`GenerateGo` consumers reject.
Mapped non-`0/0` anonymous string/token/NMTOKEN unsupported; `<all>` unsupported.
Particle-plus-use/attribute-only bodies expose ordered scalar local/ref/inline
`AttributeUse` facts; bounded scalar `simpleContent` retains base/type refs/uses.
Optional/required effective; prohibited omitted. Local value/default/fixed/
inheritable semantics and attribute consumers unsupported.

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

Generation: global built-in/named/inherited/included/imported Boolean/integer/decimal/string/token/NMTOKEN and global inline string/token/NMTOKEN components generate. Only non-extension default-occurrence direct-choice refs to global built-in/named Boolean/integer/decimal targets are eligible; sequences, repetition/non-default, nested/recursive/broader refs, and anonymous targets are rejected.
Global inline Boolean/integer/decimal/long-family/identity-only declarations retain query facts; local inline Boolean/integer/decimal facts are limited to the admitted direct choice/sequence/bounded-extension shapes, while mapped local long-family/identity-only forms remain unsupported. Anonymous consumers reject. Local built-in/named Boolean/integer/decimal default all-Boolean/numeric choices and default-bounded sequences generate; anonymous/token/NMTOKEN and repeated/non-default consumers reject.

## Conformance

W3C XSD artifacts and outcomes are pinned; the harness reports pass, conformance,
unsupported, resolution, and internal failures without changing ranking.

XSD 1.0/1.1 artifacts are URL/digest pinned; tooling indexes them and verifies the
XSD 1.0 envelope/DTD ordering without changing parser or resolver semantics.
