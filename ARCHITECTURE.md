# Architecture

## Pipeline

```mermaid
flowchart LR
  A["Root byte stream"] --> B["XML and XSD syntax"]
  B --> C["Document discovery queue"]
  C --> D["Component declarations and identities"]
  D --> E["Reference and derivation ordering"]
  E --> F["Constraints and facets"]
  F --> G["Immutable Schema"]
  G --> H["Instance validator"]
  G --> I["Go code generator"]
```

Identities precede references; slices order walks/output; no backpatching.

## Resolution

`ParseSchema` defaults Compatibility; `ParseSchemaWithPolicy` selects policy.
Streams close; identities decode once.

```go
type Resolver interface {
    Resolve(ctx context.Context, namespaceURN, schemaLocation string) (ResolvedSource, error)
}
```

Sources retain identity/context; resolvers own locations.
FIFO discovery; sequential calls. `Loc`: one-based Unicode positions.

## Diagnostics

Diagnostics retain class/code, primary/related `Loc`, spec reference, and cause;
errors prevent `Schema`.

## Schema model

Typed globals retain identity constraints, XPath, scopes, `Loc`s, keyref targets.
IDs/duplicates precede refs; unresolved/invisible/ambiguous targets yield
`FailureResolution`; wrong-kind/field-count yield `FailureInvalid`.

Extensions/restrictions retain IDs/`Loc`s/wildcards. Named-base restrictions need
completed particle-free, use-free bases. Derived namespace/process constraints
must narrow; ##other retains origin under edition-specific rules.
Negative/QName-constrained restriction comparisons remain unsupported.
Missing bases, widening, final, cycles invalidate; missing derived
wildcards, richer bodies, consumers are unsupported.
Grouped/sequence IDs persist; ID/list/union and nested `inheritable` excluded.
SimpleContent: base/type/use `Loc`s, nil particle; dependencies before owner gates. Boolean/string/integer/decimal, policy-gated `precisionDecimal`.
Attribute-free named simpleContent queries facet-free language/anyURI; restrictions/consumers reject.
Selected direct/extension integer particles; named-direct sign restrictions. Built-in/named integers validate; other extensions query-only.
Syntax/policy/occurrences precede mapping; refs precede omission.
Named direct choice/sequence and named-group sequence/nested-choice locals match `targetNamespace`.
Compatibility/Strict11 admit; Strict10, absent/mismatched owners, form/ref conflicts invalidate.
Named-group choices reject element-ref substitution overlaps beside group refs; group-ref attribution unchecked. Atomic-local/opaque-ref facts: [contract](doc.go).
Empty-base anonymous extensions admit ref-only choice, nested group/element-ref sequence, or both; bounds/order/targets/`Loc`s/wildcard persist.
Broader nesting/direct groups/wildcards/locals/nested attributes reject.
Type conflicts: XSD3063; ambiguity relates `Loc`s.
Valid inline-simple/untyped `0/0` omits; broader nesting/wildcard/`all` reject. Cycles XSD3062; consumers reject at group `RefLoc`.
Named direct `all`: ordered locals/refs, bounds/`Loc`s; 1.0 maxima≤1,
1.1 repeats/outer `0/0`; anonymous members/consumers reject.
Models→ordered uses→base; `0/0`/prohibited omit; chameleons adopt; 1.1 `targetNamespace` matches.
Compatibility/Strict11 omit false/0 local `inheritable` after owner checks; Strict10/true/1/refs reject.
Direct-sequence: empty bases. Named grouped extensions: use-free
optional-ref-sequence base, zero own uses; completed sequence-extension base,
one optional direct local ID. Both: default 1/1 sequence, 0/1 global ref.
Collisions; Strict10 second effective IDs fail.
Named effective `xs:short`/`xs:int`/`xs:byte`/`xs:unsignedLong` grouped uses: empty/use-free bases.
Named empty/choice/sequence/all/direct-group/grouped/sequence extensions retain facet-free `xs:ID`; these retain built-in/named atomic `xs:NCName`/`xs:normalizedString`.
Named empty/direct-choice/direct-sequence/direct-all/direct-group owners: built-in/facet-free named `xs:language`/`xs:anyURI`; named direct-sequence/grouped extensions likewise. Consumers reject.
Named owners: token/NMTOKEN built-in/named empty/choice/sequence/direct-sequence/grouped/direct-group; V/G reject.
Anonymous empty-base ref-sequences: built-in `xs:token`/`xs:anyURI`; consumers reject.
`xs:long`: named empty/choice/sequence/all/direct-group/direct-sequence/grouped owners, built-in/named uses.
Named empty/choice/sequence/all and empty-base direct-sequence/grouped extensions:
built-in/named `xs:short`/`xs:int`/`xs:byte`/`xs:unsignedLong`. Consumers reject.
Global attributes: Boolean/integer/decimal/normalizedString/token/negativeInteger/positiveInteger/nonPositiveInteger/language/
NCName/anyURI/ID/long/int/short/byte/unsignedLong, facet-free QName,
policy-gated `precisionDecimal`.
Compatibility/Strict11 omit untyped-global `inheritable=false/0`;
Strict10/true/1 reject; refs reject at use `Loc`.
Inline varieties retain IDs/refs/`Loc`s/facets/`finalDefault`; atomic inline
`normalizedString` admits. Atomic NCName preserves enumeration order, inherited
narrowing, fixed collapse. Enumerated NCName list/union members reject;
facet-free admit. Explicit whiteSpace/pattern/length, local inline NCName,
list/union attribute refs, default/fixed, and consumers reject.
Default/fixed covers built-in/named Boolean/integer/decimal/token/negativeInteger/nonPositiveInteger/positiveInteger/long/int/short/byte/unsignedLong and policy-gated `precisionDecimal`.
Integer exact; unsignedLong digits-only Strict10, plus/signed-zero
Compatibility/Strict11. Local refs to global inline lists/unions reject; atomic
refs query-only. Inline defaults/fixed fail at value `Loc`, retaining
causes/related `Loc`s. Conflicts locate fixed/default; type-only unconstrained;
standalone attributes: ValidateInstance N/A; GenerateGo rejects at declaration
`Loc` with nil output.

Untyped: `block=""` retains `Loc`; no authored type QName/ID/ref;
consumers reject. Nonempty block rejects.
Named direct choice/sequence `xs:any`: empty/positive all policies; Compatibility/Strict11 add negatives, `##defined`/`##definedSibling`, strict negative QNames, paired empty `namespace`+`notQName` (strict/lax/skip). Membership before omission; symbolic negatives/derivations/consumers reject.
Named direct `xs:anyAttribute`: Compatibility/Strict11 negative strict admits member QNames, lax/skip only absent/empty `notQName`; negative symbols reject. Strict/default admits QNames/`##defined` for absent/positive namespace, empty `notQName`/`##defined` for empty namespace. QName membership precedes publication; derivations/consumers reject.
Global-element anonymous complexType negative `xs:anyAttribute`: Compatibility/Strict11, written empty default-1/1 sequence, no uses. `notQName` absent for omitted/strict/lax; absent/empty for skip. Strict10/broader owners/derivations/consumers reject.
`openContent=none` excludes Strict10; named groups retain refs/ranges.
Inline-complex `mixed/abstract/nillable=false/0` omit; true/1 reject. Untyped
globals retain Boolean values/`Loc`s; malformed values retain causes.
Compatibility/Strict11 omit `defaultAttributesApply`; Strict10/schema `defaultAttributes` reject.
Inline IDs omit from walks; precisionDecimal sequences validate.
Local lists use precisionDecimal; unions pair precisionDecimal/negativeInteger.
Named grouped extensions query optional local `final` through structurally
matched named `xs:simpleDerivationSet`; its union/list values and consumers reject.
Facet-free `QName` refs, global attributes, and local uses on named
empty/choice/sequence owners query. Other local forms and consumers reject.

## Datatypes

Schema `anyURI`: Compatibility/Strict11 admit XML-legal characters after whitespace
collapse; Strict10 checks XLink-escaped URI references. Nonempty `targetNamespace`
and wildcard `##` tokens have separate checks. Resolvers receive written
`schemaLocation`. XML Base checks URI-reference syntax without datatype collapse;
resolution remains unsupported.
Datatypes map string enumeration, arbitrary precision, Compatibility/Strict11
precisionDecimal facets, Boolean whitespace; broader facets/temporal values reject.

## Validation and code generation

Validation handles scalar roots, lists/unions, ordered typed/ref sequences.
Lists split XML whitespace; unions preserve order. Bounded variety sequences
admit selected typed/anonymous locals/refs; outer occurrences default, child
ranges exact. Default direct-choice
refs to unconstrained Boolean/integer/decimal globals validate; constrained
targets relate `Loc`.
SimpleContent accepts built-in string plus selected precisionDecimal attributes
or effective precisionDecimal; effective named string excludes.
Inline roots validate selected bounded list/union sequences, attributes,
simpleContent; complex-content extensions reject after 0/0/prohibited omission.
Mixed-content validation and inline-complex generation remain unsupported.
Nonzero `xs:any` query-only. Identity-constrained roots reject at instance-use `Loc`,
relating first constraint. QName elements/element refs reject validation/generation;
attribute refs fail admission.
Structure precedes facets; first failure retains instance/schema `Loc`s.
`xsi:schemaLocation` never resolves;
other byte/short/int/long/unsignedLong validation uses reject.

Generation covers named/inline scalars, default Boolean/integer/decimal
choices/sequences, all-token choices; other locals/attributes/targets reject.
Untyped, identity-constrained, or abstract/nillable=true globals yield `GOXSD9029`, nil output.
Default integer/decimal sequence refs retain TargetID/order. Built-in bounded
integers use `StrictInteger`; `IntegerBounds()` copies effective bounds.
Byte bounds [-128,127]; unsignedLong bounds [0,18446744073709551615]. Named
facets retain provenance. Unsupported final/
variety/effective facets yield `GOXSD9029`; malformed facts `GOXSD9030`.
Nonzero inline `nonNegativeInteger` has no schema; valid `0/0` terms omit.

## Conformance

Pinned W3C artifacts classify pass/conformance/unsupported/resolution/internal
outcomes; tooling checks XSD 1.0 envelope/DTD order separately.
