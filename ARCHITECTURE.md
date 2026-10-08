# Architecture

## Boundaries

goxsd9 builds immutable query models; validation/generation are leaves.

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

Phases never backpatch. Identities intern before discovery; repeats/cycles close. Stable
topological order resolves acyclic dependencies; slices order walks/output.

## Input and resolution

`ParseSchema(root ResolvedSource, resolver Resolver)` uses Compatibility;
`ParseSchemaWithPolicy` selects policy. Resolvers acquire sources; streams close,
identities decode once.

```go
type Resolver interface {
    Resolve(ctx context.Context, namespaceURN, schemaLocation string) (ResolvedSource, error)
}
```

Sources carry opaque identity, reader-closer, child context; resolvers may keep private base-location state.
FIFO preserves context; parser leaves identities/locations opaque,
opens no paths/network resources, resolves sequentially.
`Loc` uses one-based lines and Unicode-code-point columns.

## Diagnostics

Invalid/unsupported/resolution/internal diagnostics retain code, primary/related
`Loc`, spec reference, cause; errors prevent `Schema`. Unsupported
features have stable report IDs.

## Schema model

Components are immutable; walks ordered, particles scoped. Global elements retain
ordered identity constraints, XPath, namespace scopes, `Loc`s, and keyref targets.
IDs/duplicates precede refer resolution; unresolved/invisible/ambiguous targets yield
`FailureResolution` at `refer`, wrong-kind/field-count `FailureInvalid`. Publication is atomic.

`DeclaredType` is primitive. Bounded attribute-free complexContent extensions over
named empty bases and restrictions over `xs:anyType` retain refs, base IDs/`Loc`s,
and inherited `##other`/`lax` wildcards. Scalar simpleContent retains base/type/use
`Loc`s and nil particle; restrictions reject. Bases are Boolean/string/integer/decimal
or policy-gated `precisionDecimal`.
Direct/extension choices and sequences admit `integer`, built-in/named/inline
`negativeInteger`, built-in/named `long`/`int`/`short`/`byte`/`unsignedLong`/
`nonNegativeInteger`; built-in `positiveInteger` is direct-global only.
Built-in/named `integer` validate broadly; anonymous `integer`/`negativeInteger`
need bounded list/union context. Other derivatives/extensions are query-only;
named effective-long retains identity/facets/order. SimpleContent excludes local derivatives.
Syntax/occurrence/reference/policy gates precede mapping and `0/0` omission;
errors keep cause/`Loc` and prevent `Schema`. Sequences resolve children first;
choices resolve refs once; named groups before omission. Refs retain
QName/RefLoc/TargetID/order without expansion; broader consumers reject.
Valid `0/0` omits; non-`0/0` inline long-family locals fail at type/simpleType `Loc`.
Direct named-complex `all` retains ordered integer/decimal/Boolean, built-in `string`,
built-in/named effective `token`/`NMTOKEN`/`negativeInteger`/`nonNegativeInteger`/`long`/`int`/`short`/`byte`/`unsignedLong` locals,
and refs with exact bounds, `0/0` omission, and duplicate locations. XSD 1.0
caps maxima at one; XSD 1.1 permits repeats and outer `0/0`.
Resolved anonymous simple-type `0/0` terms omit; direct-`all` anonymous complex members reject even at `0/0`, and `all` consumers reject.
AttributeUse preserves order, ownership, locations, and targets; grouped extensions resolve group/uses/base and omit valid `0/0` groups.
Forms select local names; XSD 1.1 `targetNamespace` must match the container;
chameleon adopts; prohibited uses omit. Local values/inheritable and
attributeGroup/broader extensions reject; refs locate failures.
Global typed attributes include built-in/named Boolean/integer/decimal/normalizedString/token/negativeInteger/positiveInteger/nonPositiveInteger/language/
NCName/anyURI/ID/long/int/short/byte/unsignedLong, plus policy-gated `precisionDecimal`.
Global inline restriction/list/union retains anonymous IDs, ordered refs, `Loc`s,
facets, and `finalDefault`; leaves exclude `normalizedString`, and lists/unions
exclude `string`. Forward/imported references resolve.
Default/fixed covers built-in/named Boolean/integer/decimal/token/negativeInteger/nonPositiveInteger/positiveInteger/long/int/short/byte/unsignedLong and policy-gated `precisionDecimal`.
Integer facets/values are exact; unsignedLong bounds/enumerations use digits-only
Strict10 versus optional plus/signed zero Compatibility/Strict11.
Unsupported types/local refs to global inline lists/unions have located diagnostics;
local refs to inline atomics stay query-only. Global inline defaults/fixed fail at
value `Loc`; invalid values retain lexical/facet causes/related `Loc`s. Conflicts
locate fixed/default; type-only unconstrained; global attribute consumers reject.

Complexes retain abstract/final provenance, ordered groups/extensions, and
wildcards. `xs:any` admits positive sets, XSD 1.1 strict/lax/skip `notNamespace`,
chameleon adoption, and `0/0` omission. Compatibility/Strict11 direct strict
`xs:any` retains normalized `notQName` tokens, bindings, `Loc`, and sorted names.
Inconsistent exclusions fail before omission; consumers/broader forms reject.
`openContent=none` excludes Strict10; named groups retain refs/ranges.
Supported global inline complexes accept whitespace-collapsed unqualified `mixed="false"`/`"0"` as omission under every policy.
True/1 is located unsupported with edition SpecRef; malformed Boolean input is located invalid with lexical cause.
No lexical-presence fact; shape/occurrence/consumer boundaries stay fixed.
Inline IDs stay outside walks; selected precisionDecimal sequences validate. Lists/unions, sequences, and attributes retain order.
Bounded local lists use precisionDecimal; unions pair precisionDecimal/negativeInteger.
Strict10 rejects precisionDecimal. Element refs retain ranges. Strings validate
with bounded varieties; `normalizedString` supports replace whitespace/facets.
Facet-free `QName` varieties/global refs retain context; QName locals/attributes/facets/values and consumers reject.

## Datatypes

Lexical/value forms differ.
QName value conversion needs namespace context and remains unsupported.
Datatypes map string enumeration, arbitrary precision, exact Compatibility/Strict11
precisionDecimal facets, Boolean whitespace; broader facets/temporal values reject.

## Validation and code generation

`ValidateInstance` handles built-in/named Boolean/token/NMTOKEN/integer/
nonNegativeInteger/decimal/byte/short/int/long/unsignedLong and atomic `xs:string`
roots. Compatibility/Strict11 also handle precisionDecimal roots, named/inline
global lists, bounded unions, and ordered typed/ref sequences with anonymous
targets/inline roots; Strict10 rejects before publication. Lists split XML
whitespace; unions preserve member order. Selected variety attributes validate
on empty/sequence roots. Bounded variety sequences accept typed string/integer/
negativeInteger/precisionDecimal locals, anonymous integer/negativeInteger
locals, and refs with exact child ranges. Local Boolean/integer/decimal
sequences/default choices honor ranges; token/NMTOKEN sequences retain large/
unbounded occurrences. Default direct-choice refs to unconstrained global
Boolean/integer/decimal validate; constrained targets relate `Loc`.
SimpleContent accepts built-in string with selected precisionDecimal attributes
or built-in/named effective precisionDecimal; named effective string excludes.
Selected inline roots validate precisionDecimal-only sequences and supported precisionDecimal attribute uses.
Other inline/mixed-content uses reject; `GenerateGo` rejects inline complex roots.
Nonzero `xs:any` is query-only. Identity-constrained roots reject at instance-use `Loc`,
relating first constraint. QName globals/refs reject validation/generation with
located diagnostics. Structure precedes facets; first failure retains instance/
schema `Loc`s. Occurrence and scalar failures differ. `xsi:schemaLocation` never
resolves. Other byte/short/int/long/unsignedLong validation uses reject.

Generation admits named Boolean/integer/decimal/token/NMTOKEN/effective-atomic
`xs:string` types and supported globals, including inline string/token/NMTOKEN
and named/global `nonNegativeInteger`/`long`. Local Boolean/integer/decimal
default choices/sequences and all-token choices generate; other locals/attributes
reject. Standalone `normalizedString` and global `int`/`short`/`byte`/`unsignedLong`/
`positiveInteger`/`nonNegativeInteger`/`long` refs reject. Global abstract/nillable and identity constraints return located
`GOXSD9029` with nil output. Default integer/decimal sequence refs retain
TargetID/order. `nonNegativeInteger` uses `StrictInteger`; `IntegerBounds()`
copies built-in/named bounds. Unsupported final/variety/effective facets yield
`GOXSD9029`, malformed facts `GOXSD9030`. Nonzero inline `nonNegativeInteger`
has no schema; built-in/named locals remain query-only, while valid `0/0` terms omit.

## Conformance

URL/digest-pinned W3C artifacts classify pass/conformance/unsupported/resolution/internal
outcomes; tooling checks XSD 1.0 envelope/DTD order without altering parser/resolver behavior.
