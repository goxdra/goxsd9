# Architecture

## Boundaries

goxsd9 parses schemas into immutable query models; validation/generation are leaves.

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

Phases do not backpatch. Identities intern before discovery; repeats/cycles close.
Acyclic dependencies use stable topological order; slices order walks/output.

## Input and resolution

Entrypoint: `ParseSchema(root ResolvedSource, resolver Resolver)`. The caller selects
graph language policy; resolvers acquire sources under their resolution policy.
Streams close; identities decode once; repeats/cycles close.

```go
type Resolver interface {
    Resolve(
        ctx context.Context,
        namespaceURN string,
        schemaLocation string,
    ) (ResolvedSource, error)
}
```

Sources carry opaque identity, reader-closer, child context; resolvers may keep private
base-location state. FIFO discovery preserves context. Parser leaves identities/locations uninterpreted,
opens no paths/network resources, and resolves sequentially.

`Loc` uses one-based lines and Unicode-code-point columns.

## Diagnostics

Diagnostics classify invalid/unsupported/resolution/internal failures; retain codes,
primary `Loc`, related/specification references, and causes; errors prevent schema
return. Unsupported features have stable report IDs.

## Schema model

Immutable components; ordered walks; scoped particles.
Global elements retain ordered unique/key/keyref kind, name, source/ordinal ID, `Loc`, XML-decoded selector/field XPath/`Loc`s, copied namespace scopes/defaults,
and resolved keyref QName/target ID. IDs/duplicates precede visible refer resolution: unresolved/invisible/ambiguous targets yield `FailureResolution` at `refer`;
wrong-kind/field-count yield `FailureInvalid`. Publication is atomic.

`DeclaredType` is primitive. Bounded attribute-free complexContent extensions
over named empty bases and restrictions over `xs:anyType` retain refs, base
IDs/`Loc`s, inherited `##other`/`lax` wildcards. Scalar simpleContent retains
base/type/use `Loc`s and nil particle; restrictions reject. Bases:
Boolean/string/integer/decimal or policy-gated `precisionDecimal`.
Direct/extension choices/sequences admit `integer`, built-in/named/anonymous-inline
`negativeInteger`, and built-in/named `long`, `int`, `short`, `byte`, `unsignedLong`,
`nonNegativeInteger`. Built-in `positiveInteger`: direct globals only.
Direct built-in/named-effective `integer` supports consumers;
derivatives/extensions are query-only. Built-in `long` retains bounds; named
effective-long retains identity, facets, QName, occurrences, order.
SimpleContent excludes these local derivatives. Failures locate type/facet/element,
particles at nested `Loc`.
Syntax/occurrence/reference/policy gates precede mapping, including `0/0` inline
bases/facets. Graph/reference/policy errors retain cause/`Loc`; no `Schema`.
Sequences resolve children before omission; choices resolve refs once; named
groups resolve/check before omission; child refs first.
Element/model-group refs retain QName/RefLoc/TargetID/order without expansion;
nested/local/recursive/broader forms reject or exclude consumers. Non-`0/0` local inline/anonymous
`long`/`int`/`short`/`byte`/`unsignedLong`/`nonNegativeInteger` fail at type/
simpleType `Loc`; applicable `0/0` forms omit after gates.
AttributeUse preserves order, locations, ownership, use, QName/RefLoc/TargetID
across particles/groups/extensions/attribute-only/simpleContent.
Grouped extensions resolve group/uses/base in order; `0/0` omits group.
Local uses retain name/type/use locations and named/anonymous `AnonymousID`/`NodeID`;
references retain QName/RefLoc/TargetID/use. Forms select names; XSD 1.1
`targetNamespace` must match the container; chameleon adopts; prohibited uses omit.
Local values/inheritable, attributeGroup/broader extensions, and consumers are
unsupported; refs located; no schema.
Attributes query built-in/named Boolean/integer/decimal/token/negativeInteger/language/
NCName/anyURI/ID/long/int/short/byte/unsignedLong; `precisionDecimal` policy-gated.
Default/fixed: built-in/named Boolean/integer/decimal/token/negativeInteger/long/int/short/unsignedLong; policy-gated `precisionDecimal`.
Integer values/facets exact; unsignedLong lexical: digits-only XSD 1.0, signed/-0 XSD 1.1.
Unsupported types/local/inline: located `FailureUnsupported`; unsupported values: constraint `Loc`;
invalid values retain lexical/facet causes/related `Loc`s. Conflicts locate fixed/default; type-only
unconstrained; attribute consumers reject.

Complexes retain non-inherited `IsAbstract`, `finalDefault` provenance, ordered groups/extensions,
and wildcards. `xs:any` supports positive sets and XSD 1.1 strict/lax/skip
`notNamespace`; chameleon markers expand after adoption. Consumers reject
wildcards; broader forms reject; `0/0` omits. `openContent=none` works except
under Strict10; named groups retain ordered refs/ranges.
Inline complexes expose IDs and ordered sequence/ref/use outside walks; consumers reject. SimpleContent admits string/Boolean/integer/decimal and policy-gated
`precisionDecimal` with nil particles. Compatibility/Strict11 admit nonzero
direct `precisionDecimal` sequences and non-extension list/union locals without QName; consumers reject.
Element refs retain targets/occurrences, including repeats to global inline
restrictions; nonzero local inline forms reject.
Built-in/named `string` particles are query-only.
`normalizedString`: replace whitespace and lexical facets; restriction/list/union refs.
Atomic direct/named/inline globals and refs query; nonzero locals reject, `0/0`
omits after gates. Global attributes/consumers, standalone named generation reject.
Facet-free `QName` restriction/list/union and direct/named/inline global refs
retain datatype QName, use `Loc`, named ID. Nonzero QName-bearing local declarations, attributes,
facets, default/fixed reject; validated `0/0` omit after gates.

## Datatypes

Lexical/value forms differ; QName lexical-to-value conversion remains unsupported
and requires namespace context.
Datatypes map string enumeration, arbitrary precision, exact Compatibility/Strict11
precisionDecimal facets, and Boolean whitespace; broader facets/temporal values reject.

## Validation and code generation

`ValidateInstance` supports built-in/named Boolean/token/NMTOKEN/integer/nonNegativeInteger/decimal,
direct/named/anonymous `xs:string`-atomic roots (whiteSpace/enumeration), and
Compatibility/Strict11 precisionDecimal roots; Strict10 rejects precisionDecimal.
Identity-constrained roots reject at instance use `Loc`, relating the first constraint `Loc`.
Local Boolean/integer/decimal sequences/default choices honor
ranges; homogeneous token/NMTOKEN sequences honor exact above-`uint64`/unbounded occurrences/value space.
Local anonymous/mixed-family/extension consumers reject; nonzero `xs:any` is queryable but consumer-
unsupported. Element refs retain QName/RefLoc/TargetID/order/occurrences without target gating;
only default direct-choice refs to unconstrained global built-in/named Boolean/integer/decimal validate.
Constrained targets reject at instance use `Loc` with related constraint `Loc`; other forms remain queryable.
Global `nonNegativeInteger` refs remain queryable;
direct-choice/sequence consumers reject with located unsupported diagnostics/nil output. Model-group
refs query in direct complex-type bodies and supported grouped extensions; nested/broader forms reject.
AttributeUse/simpleContent are query-only. QName globals/refs reject validation and generation with located diagnostics and nil output.

Generation admits named Boolean/integer/decimal/token/NMTOKEN/effective-`xs:string`-atomic
types, global built-in/named elements of those types, inline global
string/token/NMTOKEN, and named/global `nonNegativeInteger`. Standalone named
`normalizedString` rejects. Global elements require
`abstract=false,nillable=false`; violations yield `GOXSD9029` and nil output.
Identity-constrained elements/targets yield `FailureUnsupported`/`GOXSD9029` at the first constraint `Loc` and no output.
Built-in/standalone `nonNegativeInteger` uses `StrictInteger`; named fields use
generated types. Canonical built-in facts require integer kind, fixed
`fractionDigits=0` and `minInclusive=0`; named bounds/facets survive.
Unsupported final/variety/effective-facet states yield `GOXSD9029`; malformed
facts yield `GOXSD9030`, all with nil output. Nonzero local inline
`nonNegativeInteger` has no schema; built-in/named locals are query-only, `0/0`
absent. Global `int`/`long`/`short`/`byte`/`unsignedLong`/`positiveInteger`
and direct `nonNegativeInteger` refs are query-only. `IntegerBounds()` copies
built-in/named bounds; negativeInteger max=-1, positiveInteger min=1, named
restrictions retain provenance. Local generation admits default
Boolean/integer/decimal choices/sequences and all-token choices; other local
shapes and global attributes reject.

## Conformance

URL/digest-pinned W3C artifacts drive pass, conformance, unsupported, resolution, and
internal outcomes. Tooling verifies XSD 1.0 envelope/DTD order without changing parser or resolver semantics.
