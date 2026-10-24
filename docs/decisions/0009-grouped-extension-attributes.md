# 0009: Grouped complex-content extensions with local attributes

Status: accepted

## Decision

This schema-model boundary is one named global or global-element inline `complexType` whose
`complexContent/extension` uses a supported named empty-content base, one
direct named `<group ref="...">` particle, and one or more ordered direct
local `<attribute>` uses.
The extension body may contain its optional annotation. The model child is
the group reference; local attributes follow it in lexical order. The parser
implements this bounded queryable fact; validation and generation reject it.

The group remains one opaque particle. Preserve its expanded written QName,
`ref` and use-site locations, exact occurrence range, and target
`ComponentID`; never copy or expand the target's members. Reuse the sole
immutable `AttributeUse` representation for declaration/reference forms,
effective names, `use`, form/chameleon policy, type identity, locations, and
lexical order. Attribute uses are a semantic set, so duplicate effective names
are invalid, but their ordered model view is part of the observable contract.

An effective group range of `0/0` is validated against its target first, then
omitted from the public particle view. The completed body consequently uses
the attribute-only shape; `0/0` is not a published zero-valued particle.
Validation and Go generation remain explicit unsupported consumer boundaries.

## Normative basis

The paired XSD 1.0 and XSD 1.1 normative anchors for
[`complexContent/extension`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#element-complexContent..extension),
[`group`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#element-group),
[`declare-contentModel`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#declare-contentModel),
[`cAttributeUse`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#cAttributeUse),
[`cos-ct-extends`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#cos-ct-extends),
and [`cos-particle-extend`](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#cos-particle-extend),
and the corresponding XSD 1.1 anchors [`complexContent/extension`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#element-complexContent..extension),
[`group`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#element-group),
[`declare-contentModel`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#declare-contentModel),
[`cAttributeUse`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#cAttributeUse),
[`cos-ct-extends`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#cos-ct-extends),
and [`cos-particle-extend`](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#cos-particle-extend).
Those rules permit an optional annotation, at most one model child, then
attribute uses/attribute groups and an optional wildcard. A group reference is
one particle with its own occurrences, not an instruction to flatten a named
group. XSD 1.1 additionally has open-content and assertion constructs; they
are outside this slice. Attribute uses are set-like semantically, while this
model retains lexical and provenance order.

The pinned XSD schema-for-schemas and datatypes artifacts in
[`specs/manifest.json`](../../specs/manifest.json) contain the motivating
grammar. Full-artifact outcomes remain in
[`bootstrap_probe_test.go`](../../internal/specs/bootstrap_probe_test.go);
the copied datatype declarations are exercised through a synthetic grouped
owner and local `final` use, with admission, exclusions, and located diagnostics
tested in
[`bootstrap_simple_derivation_set_probe_test.go`](../../internal/specs/bootstrap_simple_derivation_set_probe_test.go).
This boundary makes no `xs:ID` lexical, value, or uniqueness claim.

## Exact supported shape and non-goals

The supported input is:

- a named global complex type or an anonymous inline complex type on a global element;
- `complexContent/extension` with a supported named empty-content base;
- an optional annotation, exactly one direct `<group ref="...">` model
  child, and one or more direct local `<attribute>` uses after that child;
- a group target resolved as an opaque named model group; and
- local uses accepted by the scalar/type and namespace allowlists,
  including its supported anonymous local simple-type identities. Anonymous
  owners admit non-ID atomic uses and exclude local list/union varieties;
  named owners retain their separately supported varieties and ID rules.

For this grouped form, named owners and anonymous global-element inline owners
treat explicit `inheritable=false/0` on supported name-bearing local declarations
as omission under Compatibility/Strict11. Anonymous owners retain their non-ID
atomic type boundary. Explicit `true/1` and `inheritable` on a
`ref` use (including false) remain unsupported; Strict10 rejects the XSD 1.1
attribute. Malformed Boolean values remain invalid. The [package contract](../../doc.go)
states other owner and consumer limits.

Construction resolves the opaque group particle, ordered local uses, then the
named base. No consumer model is introduced by the composition.

The group may be `0/0`, subject to target validation and the normalization
above. A syntactically absent group is not silently inferred for this boundary.
The base remains a named completed empty-content base with only representable
inherited wildcard facts. Duplicate direct model children are invalid. This
grouped boundary does not add or expand `attributeGroup`, nested/local/anonymous
groups, attribute-bearing choices, `all`, extension
`anyAttribute`, open content, assertions, mixed or simple content, nonempty or
broader bases, `xs:anyType` direct extension, another derivation kind, value
constraints, or consumer behavior.
Direct-sequence extensions with local uses follow the separate
[package contract](../../doc.go).

Simple-type variety and reference identities remain distinct from list/union
value semantics; named simple-type `final` controls remain enforced.
The sole nested union/list local-use exception is a direct local declaration
`name="final"`, with absent or explicit `use="optional"`, on a named grouped
extension. Its `type` directly references the named `xs:simpleDerivationSet`
component. Admission checks completed copied facts, not just that QName: two
ordered inline union members, first an `xs:token` restriction enumerating
`#all`, then a list with an inline item restriction of named
`xs:derivationControl`. That dependency restricts `xs:NMTOKEN` with ordered
`substitution`, `extension`, `restriction`, `list`, `union` enumerations. The
item restricts those values to `list`, `union`, `restriction` in the pinned
XSD 1.0 slice and additionally `extension` in the XSD 1.1 slice. The parser
publishes identities, references, facets, order, and lexical locations;
this fragment slice does not claim support for either full schema-for-schemas.
The local attribute name is separate from simple-type `final` controls. For
this type, local inline simple types, attribute references, required uses,
default/fixed values, other owners, generic union/list uses, and consumers
remain unsupported.
Named grouped owners admit direct built-in `xs:language` and `xs:anyURI` uses;
named aliases, inline varieties, references, values, and consumers remain outside
that admission. `xs:QName` remains outside grouped-extension local attribute
admission. Named grouped owners admit built-in and named effective atomic
`xs:NCName` declarations with copied enumeration facts. Built-in and named
`xs:normalizedString` declarations retain effective whitespace and enumeration
facts on the same named owners; inline, reference, value, and consumer forms
remain unsupported. Built-in and facet-free named
`xs:ID` local uses retain query facts only; lexical, value, uniqueness, validation, and generation
semantics remain unsupported. For a local use, `name` is an
unqualified NCName and `ref` is an expanded QName; they are mutually exclusive
and do not share the global-declaration name path.
Named grouped owners over supported empty use-free bases also query direct
built-in `xs:token` and `xs:NMTOKEN` uses. Their effective whitespace is
collapse, and the built-ins have no enumeration or local type ID. Named
aliases, inline atomic/list/union types, attribute refs, value constraints,
other owners, and both consumers remain outside this admission.
The same named grouped owners query direct built-in and supported named
effective atomic `xs:long` uses. They retain intrinsic inclusive bounds and
effective inherited or narrowed inclusive/exclusive bounds with facet values
and source locations, supported digits and enumeration facets, and type
provenance. Anonymous owners, inline atomic/list/union types, attribute refs,
value constraints, richer bases, and both consumers remain outside this long
admission.

## Phase representation and invariants

Use a tagged, phase-specific extension-body variant for `group + attribute
uses`. Do not add generic nullable particle/attribute fields that can discard a
child or publish an impossible combination. The syntax input owns the
extension, group, base, and ordered-use locations; the resolved result owns a
resolved group particle, an ordered immutable use slice, and the bounded
base facts. The `0/0` result selects the existing attribute-only variant rather
than retaining a zero particle or a second attribute API.

The construction order is deterministic:

1. Allocate named component identities, anonymous global-element inline
   complex-owner `ComplexTypeID`s, and anonymous local simple-type identities
   in discovery/declaration order. Anonymous owners stay out of global walks.
2. Convert and validate the direct group reference and its exact occurrence;
   resolve its visible named model-group target first.
3. Resolve local attribute uses in lexical order, including
   effective names, form/chameleon namespace policy, global targets, inline
   simple-type dependencies, and supported type identities.
4. Resolve the extension base through its bounded seam, including final,
   visibility, completed-content, wildcard, and cycle checks.
5. Publish one immutable completed fact only after every step succeeds.

All resolver calls are sequential. Completed components are never backpatched;
returned slices, QNames, IDs, locations, and occurrence values are owned or
copied at the phase boundary. Maps are lookup-only; ordered slices define
attribute order, diagnostics, walks, and output. Consumer content models are
calculated on demand and are not cached in the schema.

## Edition, policy, and graph behavior

The one immutable graph-wide policy from [Decision 0004](0004-xsd-language-policy.md)
applies: Compatibility admits the supported mixed XSD 1.0/1.1 graph, while
Strict10 and Strict11 select one profile. `schema/@version` remains an inert
label. The grouped shape is specified for XSD 1.0, XSD 1.1, Compatibility,
Strict10, and Strict11; edition-specific open-content/assertion behavior stays
explicitly outside it.

Forward, included, imported, chameleon, repeated, and cyclic discovery graphs
use existing opaque source identity, visibility, and sequential resolver rules.
An inaccessible target does not become visible because a map exposes it.
Duplicate declarations fail before group-reference lookup. Discovery identity cycles remain interned; complex-base
and simple-type cycles fail at their existing located diagnostics. Named choice
and sequence group dependencies fail XSD3062 on cycles before publication, even
when grouped extensions keep targets opaque. Target members are not expanded;
attributeGroup recursion remains outside this boundary.

## Classification and axes

Within grouped-extension references, structural violations and referenced-component target failures are invalid
input. Unresolved, inaccessible, or wrong-kind targets retain their existing
`FailureInvalid` XSD codes, causes, primary reference-use `Loc`, related target
locations, and edition-specific `SpecRef`. Public duplicate group declarations
fail first with invalid `XSD3023` at the second declaration and the first
declaration related, before reference resolution. Conditional ambiguity at a
reference-use `Loc` requires internally preloaded duplicate candidates;
it is not a public parse outcome.
`FailureResolution` arises here only from acquisition of a referenced source through
the caller's resolver. A well-formed, specification-valid unimplemented form outside the
exact slice is explicit unsupported behavior with a registered feature ID,
stable diagnostic code, primary `Loc`, and edition-specific `SpecRef`. No
error-level result returns a partial schema.

| Affected axis | Supported | Invalid | Resolution failure | Explicit unsupported | N/A |
| --- | --- | --- | --- | --- | --- |
| Edition/policy | XSD 1.0/1.1 under Compatibility, Strict10, or Strict11; one policy for the whole graph. | Malformed edition-specific attributes or grammar. | N/A. | Valid unimplemented XSD 1.1 open-content/assertion behavior gets a feature/`Loc`/`SpecRef`; a label never selects policy. | Edition selection from `schema/@version` is N/A. |
| Named/anonymous/inline/ref shape | Named global and global-element inline owners over a supported named empty base; one direct named group ref; local declaration/ref uses and supported anonymous scalar types. Named owners also admit their existing bounded precisionDecimal list/two-member union uses. A named grouped owner additionally admits one optional direct local `final` declaration with a named `xs:simpleDerivationSet` type reference and the exact copied nested union/list facts above. | Duplicate direct model child (`XSD3010`); malformed QName/NCName; both or neither `name`/`ref`; invalid use/form/occurrences; duplicate effective name; unresolved, inaccessible, or wrong-kind targets remain `FailureInvalid`. Duplicate global group declarations fail earlier with `XSD3023`. | Grouped-extension referenced-source acquisition failures only, through the caller's resolver. | Attribute-bearing choices beyond the supported named direct-choice subset, and extension shapes beyond this grouped form and the supported direct-sequence form; local complex owners, nested/anonymous groups, attribute groups, ID and list/union uses on anonymous owners, true/1 or ref `inheritable`, broader varieties and identity-only value semantics. The `final` exception excludes inline local types, attribute refs, required use, value constraints, other owners, and generic union/list varieties. Grouped-extension validation and generation reject admitted uses. | Global attribute declarations with inline simple types are handled separately; local element-inline owners are outside this boundary. Internal duplicate-candidate ambiguity is not a public parse outcome. |
| Graph visibility/cycles | Forward, included, imported, chameleon, repeated, and interned discovery identities with existing visibility. | Inaccessible/unresolved/wrong-kind grouped-extension targets; duplicate global group declarations fail before refs; named-group cycles fail XSD3062 before publication; base/simple-type cycles retain existing diagnostics. | Grouped-extension referenced-source acquisition failures only at the resolver/discovery boundary. | AttributeGroup recursion and broader graph composition remain unsupported. | Group-member expansion is N/A because the particle is opaque. |
| Supported/invalid/explicit unsupported | Exact slice publishes facts; malformed structure is invalid; valid unavailable behavior is explicit unsupported. | Grouped-extension structural/target `FailureInvalid` keeps code, `Loc`, related evidence, cause, edition `SpecRef`; duplicate declarations use XSD3023, direct-group refs XSD3047–XSD3050, named-group cycles XSD3062 at closing ref with ordered related path, and attribute-use XSD3030/XSD3045–XSD3052/XSD3061 retain these details. | Only grouped-extension referenced-source acquisition failures are `FailureResolution` at the resolver/discovery boundary. | Registered feature ID, stable code, `Loc`, `SpecRef`, `ErrUnsupported`, and no schema; validation/generation reject explicitly. | W3C instance conformance scores are outside this decision. |
| Location/order/provenance | Preserve group QName/ref/use locations, exact range, target ID, ordered attribute uses, effective names, type/form locations, and declaration order; validate before `0/0` omission. | Reference failures locate the use and relate targets; duplicate global groups locate the second declaration and relate the first; bounds retain their locations and causes. | Resolver/discovery acquisition location and underlying cause remain attached. | The unsupported construct's source `Loc` and versioned `SpecRef` remain attached. | Unordered map iteration is N/A to observable order; ordered slices are authoritative. |
