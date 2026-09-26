# 0009: Grouped complex-content extensions with local attributes

Status: accepted

## Decision

This schema-model boundary is one named global `complexType` whose
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
grammar. Executable outcomes remain in
[`bootstrap_probe_test.go`](../../internal/specs/bootstrap_probe_test.go).
This boundary makes no `xs:ID` lexical, value, or uniqueness claim.

## Exact supported shape and non-goals

The supported input is:

- a named global complex type;
- `complexContent/extension` with a supported named empty-content base;
- an optional annotation, exactly one direct `<group ref="...">` model
  child, and one or more direct local `<attribute>` uses after that child;
- a group target resolved as an opaque named model group; and
- local uses accepted by the scalar/type and namespace allowlists,
  including its supported anonymous local simple-type identities.

Construction resolves the opaque group particle, ordered local uses, then the
named base. No consumer model is introduced by the composition.

The group may be `0/0`, subject to target validation and the normalization
above. A syntactically absent group is not silently inferred for this boundary.
The base remains a named completed empty-content base with only representable
inherited wildcard facts. Duplicate direct model children are invalid. This
boundary does not add or expand `attributeGroup`, nested/local/anonymous groups,
attribute-bearing extensions without a direct group, `all`, extension
`anyAttribute`, open content, assertions, mixed or simple content, nonempty or
broader bases, `xs:anyType` direct extension, another derivation kind, value
constraints, or consumer behavior.

Simple-type variety and reference identities remain distinct from list/union
value semantics; named simple-type `final` controls remain enforced. The
identity-only built-ins—`xs:language`,
`xs:NCName`, `xs:anyURI`, and `xs:ID`—are not widened into local attribute
lexical or value support. For a local use, `name` is an
unqualified NCName and `ref` is an expanded QName; they are mutually exclusive
and do not share the global-declaration name path.

## Phase representation and invariants

Use a tagged, phase-specific extension-body variant for `group + attribute
uses`. Do not add generic nullable particle/attribute fields that can discard a
child or publish an impossible combination. The syntax input owns the
extension, group, base, and ordered-use locations; the resolved result owns a
resolved group particle, an ordered immutable use slice, and the bounded
base facts. The `0/0` result selects the existing attribute-only variant rather
than retaining a zero particle or a second attribute API.

The construction order is deterministic:

1. Allocate all named component identities and anonymous local simple-type
   identities in discovery/declaration order.
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
An inaccessible or ambiguous target does not become visible because a map
happens to expose it. Discovery identity cycles remain interned; complex-base
and simple-type cycles fail at their existing located diagnostics. Group and
attribute-group recursion is not followed because group members are never
expanded here.

## Classification and axes

Structural violations and referenced-component target failures are invalid
input. Unresolved, inaccessible, ambiguous, or wrong-kind component targets
retain their existing `FailureInvalid` XSD codes, causes, primary reference-use
`Loc`, related target locations, and edition-specific `SpecRef`;
`FailureResolution` is reserved for acquisition of a referenced source through
the caller's resolver. A well-formed, specification-valid unimplemented form outside the
exact slice is explicit unsupported behavior with a registered feature ID,
stable diagnostic code, primary `Loc`, and edition-specific `SpecRef`. No
error-level result returns a partial schema.

| Affected axis | Supported | Invalid | Resolution failure | Explicit unsupported | N/A |
| --- | --- | --- | --- | --- | --- |
| Edition/policy | XSD 1.0/1.1 under Compatibility, Strict10, or Strict11; one policy for the whole graph. | Malformed edition-specific attributes or grammar. | N/A. | Valid unimplemented XSD 1.1 open-content/assertion behavior gets a feature/`Loc`/`SpecRef`; a label never selects policy. | Edition selection from `schema/@version` is N/A. |
| Named/anonymous/inline/ref shape | Named global owner and supported named empty base; one direct named group ref; local declaration/ref uses and supported anonymous scalar types. | Duplicate direct model child (`XSD3010`); malformed QName/NCName; both or neither `name`/`ref`; invalid use/form/occurrences; duplicate effective name; unresolved, inaccessible, ambiguous, or wrong-kind component targets remain `FailureInvalid`. | Referenced-source acquisition failures only, through the caller's resolver. | Attribute-bearing extensions without a direct group; anonymous/local complex owners, nested/anonymous groups, attribute groups, unsupported list/union varieties and identity-only value semantics. | Global inline attributes are outside this boundary. The local element-inline scope is outside this grouped-extension decision. |
| Graph visibility/cycles | Forward, included, imported, chameleon, repeated, and interned discovery identities with existing visibility. | Inaccessible/ambiguous/unresolved/wrong-kind component targets; base/simple-type cycles at existing diagnostics. | Referenced-source acquisition failures only at the resolver/discovery boundary. | Group or attribute-group recursive expansion and broader graph composition are not followed. | Group-member traversal is N/A because the particle is opaque. |
| Supported/invalid/explicit unsupported | Exact slice publishes facts; malformed structure is invalid; valid unavailable behavior is explicit unsupported. | Stable structural/component-target `FailureInvalid` code, primary source `Loc`, related locations where useful, cause, and edition `SpecRef`; direct-group XSD3047–XSD3050 and attribute-use XSD3030/XSD3045–XSD3052 families retain these details. | Only referenced-source acquisition failures are `FailureResolution` at the resolver/discovery boundary. | Registered feature ID, stable code, `Loc`, `SpecRef`, `ErrUnsupported`, and no schema; validation/generation reject explicitly. | W3C instance conformance scores are outside this decision. |
| Location/order/provenance | Preserve group QName/ref/use locations, exact range, target ID, ordered attribute uses, effective names, type/form locations, and declaration order; validate before `0/0` omission. | Primary reference-use `Loc`, related target declaration/duplicate/bounds locations, and existing cause remain attached. | Resolver/discovery acquisition location and underlying cause remain attached. | The unsupported construct's source `Loc` and versioned `SpecRef` remain attached. | Unordered map iteration is N/A to observable order; ordered slices are authoritative. |

## Durable boundaries

This composition reuses the immutable local attribute-use API, direct opaque
group-reference facts, and bounded named empty-base and inherited wildcard
resolution. It does not expand group members, introduce another attribute
representation, or admit list/union or identity-only built-in value semantics.
Validation and generation reject the completed grouped shape.
