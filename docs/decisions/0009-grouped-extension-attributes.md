# 0009: Grouped complex-content extensions with local attributes

Status: accepted

## Decision

The next schema-model boundary is one named global `complexType` whose
`complexContent/extension` uses the already-supported named-base boundary from
[#414](https://github.com/goxdra/goxsd9/issues/414), one direct named
`<group ref="...">` particle, and one or more ordered direct local
`<attribute>` uses from [#317](https://github.com/goxdra/goxsd9/issues/317).
The extension body may contain its optional annotation. The model child is
the group reference; local attributes follow it in lexical order. This is a
future implementation contract, not a claim about current parser behavior,
validation, generation, or conformance.

The group remains one opaque particle. Preserve its expanded written QName,
`ref` and use-site locations, exact occurrence range, and target
`ComponentID`; never copy or expand the target's members. Reuse #317's sole
immutable `AttributeUse` representation for declaration/reference forms,
effective names, `use`, form/chameleon policy, type identity, locations, and
lexical order. Attribute uses are a semantic set, so duplicate effective names
are invalid, but their ordered model view is part of the observable contract.

An effective group range of `0/0` is validated against its target first, then
omitted from the public particle view. The completed body consequently uses
#317's attribute-only shape; `0/0` is not a published zero-valued particle.
Validation and Go generation remain explicit unsupported consumer boundaries.

## Normative and pinned-artifact evidence

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

The authoritative pinned raw inputs are the four files below. The manifest
selects the edition-specific artifacts and `bootstrap_probe_test.go` records
the stable first parser observation, including class, feature, location, and
specification reference:

| Artifact and exact raw location | Current bounded observation |
| --- | --- |
| [`xsd10-schema-for-schemas.raw:128:8`](../../internal/specs/testdata/bootstrap/xsd10-schema-for-schemas.raw#L128), `xs:attribute name="id" type="xs:ID"` | Strict10 stops at XSD3003 (`xsd.schema.syntax` / `FeatureSchemaSyntax`), with `xsd10-structures#cos-ct-extends`. |
| [`xsd11-schema-for-schemas.raw:121:9`](../../internal/specs/testdata/bootstrap/xsd11-schema-for-schemas.raw#L121), `xs:attribute name="id" type="xs:ID"` | Strict11 stops at XSD3003 (`xsd.schema.syntax` / `FeatureSchemaSyntax`), with `xsd11-structures#cos-ct-extends`. |
| [`xsd10-datatypes-schema.raw:827:19`](../../internal/specs/testdata/bootstrap/xsd10-datatypes-schema.raw#L827), `xs:group ref="xs:simpleDerivation"` | Strict10 stops at XSD3003 (`xsd.schema.syntax` / `FeatureSchemaSyntax`), with `xsd10-structures#cos-particle-extend`. |
| [`xsd11-datatypes-schema.raw:99:19`](../../internal/specs/testdata/bootstrap/xsd11-datatypes-schema.raw#L99), `xs:group ref="xs:simpleDerivation"` | Strict11 stops at XSD3003 (`xsd.schema.syntax` / `FeatureSchemaSyntax`), with `xsd11-structures#cos-particle-extend`. |

The artifact IDs, edition, representation, and dependency order are pinned in
[`specs/manifest.json`](../../specs/manifest.json#L166); the probe rows and
no-partial-schema assertion are in
[`internal/specs/bootstrap_probe_test.go`](../../internal/specs/bootstrap_probe_test.go#L58).
These blockers are bounded evidence, not an implementation target for this
record. `xs:ID` identity, lexical space, value space, and instance uniqueness
are distinct concerns and none is claimed here.

## Exact supported shape and non-goals

The supported future input is:

- a named global complex type;
- `complexContent/extension` with a named base accepted by #414;
- an optional annotation, exactly one direct `<group ref="...">` model
  child, and one or more direct local `<attribute>` uses after that child;
- a group target resolved as a named model group using #392's direct-reference
  facts; and
- local uses accepted by #317's scalar/type and namespace allowlists,
  including its supported anonymous local simple-type identities.

The tagged implementation contract is ordered: the group is resolved as an
opaque #392 particle, local uses are resolved through #317, and the named base
follows through #414. No consumer model is introduced by the composition.

The group may be `0/0`, subject to target validation and the normalization
above. A syntactically absent group is not silently inferred for this packet.
The base remains a named completed empty-content base with only the inherited
wildcard facts that #414 can represent. This packet does not add or expand
`attributeGroup`, multiple/nested/local/anonymous groups, `all`, extension
`anyAttribute`, open content, assertions, mixed or simple content, nonempty or
broader bases, `xs:anyType` direct extension, another derivation kind, value
constraints, or consumer behavior.

The simple-type variety and reference identities from
[#213](https://github.com/goxdra/goxsd9/issues/213) remain distinct from
list/union value semantics. Named simple-type `final` facts are retained by
[#385](https://github.com/goxdra/goxsd9/issues/385), and merged
[#429](https://github.com/goxdra/goxsd9/issues/429) enforces those controls.
This packet excludes union value semantics. The identity-only built-ins from
[#312](https://github.com/goxdra/goxsd9/issues/312)—`xs:language`,
`xs:NCName`, `xs:anyURI`, and `xs:ID`—are not widened into local attribute
lexical or value support. For a local use, `name` is an
unqualified NCName and `ref` is an expanded QName; they are mutually exclusive
and do not share the global-declaration name path.

## Phase representation and invariants

Use a tagged, phase-specific extension-body variant for `group + attribute
uses`. Do not add generic nullable particle/attribute fields that can discard a
child or publish an impossible combination. The syntax input owns the
extension, group, base, and ordered-use locations; the resolved result owns a
resolved group particle, an ordered immutable use slice, and the existing #414
base facts. The `0/0` result selects the existing attribute-only variant rather
than retaining a zero particle or a second attribute API.

The construction order is deterministic:

1. Allocate all named component identities and anonymous local simple-type
   identities in discovery/declaration order.
2. Convert and validate the direct group reference and its exact occurrence;
   resolve its visible named model-group target first.
3. Resolve local attribute uses in lexical order through #317, including
   effective names, form/chameleon namespace policy, global targets, and
   supported type identities.
4. Resolve the extension base through #414's existing seam, including final,
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

## Classification and sibling-axis matrix

Structural violations and referenced-component target failures are invalid
input. Unresolved, inaccessible, ambiguous, or wrong-kind component targets
retain their existing `FailureInvalid` XSD codes, causes, primary reference-use
`Loc`, related target locations, and edition-specific `SpecRef`;
`FailureResolution` is reserved for acquisition of a referenced source through
the caller's resolver. A well-formed, specification-valid form outside the
exact slice is explicit unsupported behavior with a registered feature ID,
stable diagnostic code, primary `Loc`, and edition-specific `SpecRef`. No
error-level result returns a partial schema.

| Affected axis | Supported | Invalid | Resolution failure | Explicit unsupported | N/A |
| --- | --- | --- | --- | --- | --- |
| Edition/policy | XSD 1.0/1.1 under Compatibility, Strict10, or Strict11; one policy for the whole graph. | Malformed edition-specific attributes or grammar. | N/A. | Valid XSD 1.1 open-content/assertion behavior, or any valid shape outside this slice, gets a feature/`Loc`/`SpecRef`; a label never selects policy. | Edition selection from `schema/@version` is N/A. |
| Named/anonymous/inline/ref shape | Named global owner and named #414 base; one direct named group ref; #317 local declaration/ref uses and its supported anonymous scalar types. | Missing/duplicate model child; malformed QName/NCName; both or neither `name`/`ref`; invalid use/form/occurrences; duplicate effective name; unresolved, inaccessible, ambiguous, or wrong-kind component targets remain `FailureInvalid`. | Referenced-source acquisition failures only, through the caller's resolver. | Anonymous/local complex owners, multiple/nested/anonymous groups, attribute groups, unsupported #213 varieties, and #312 identity-only value semantics. | Global inline attributes (#333) and local element inline types (#400) are N/A. |
| Graph visibility/cycles | Forward, included, imported, chameleon, repeated, and interned discovery identities with existing visibility. | Inaccessible/ambiguous/unresolved/wrong-kind component targets; base/simple-type cycles at existing diagnostics. | Referenced-source acquisition failures only at the resolver/discovery boundary. | Group or attribute-group recursive expansion and broader graph composition are not followed. | Group-member traversal is N/A because the particle is opaque. |
| Supported/invalid/explicit unsupported | Exact slice publishes facts; malformed structure is invalid; valid unavailable behavior is explicit unsupported. | Stable structural/component-target `FailureInvalid` code, primary source `Loc`, related locations where useful, cause, and edition `SpecRef`; existing direct-group XSD3047–XSD3050 and #317 XSD3030/XSD3045–XSD3052 families retain these details. | Only referenced-source acquisition failures are `FailureResolution` at the resolver/discovery boundary. | Registered feature ID, stable code, `Loc`, `SpecRef`, `ErrUnsupported`, and no schema; validation/generation reject explicitly. | Conformance and instance execution are N/A to this research record. |
| Location/order/provenance | Preserve group QName/ref/use locations, exact range, target ID, ordered #317 uses, effective names, type/form locations, and declaration order; validate before `0/0` omission. | Primary reference-use `Loc`, related target declaration/duplicate/bounds locations, and existing cause remain attached. | Resolver/discovery acquisition location and underlying cause remain attached. | The unsupported construct's source `Loc` and versioned `SpecRef` remain attached. | Unordered map iteration is N/A to observable order; ordered slices are authoritative. |

## Dependency and decomposition

| Packet or foundation | Contract used here | Boundary preserved |
| --- | --- | --- |
| [#317](https://github.com/goxdra/goxsd9/issues/317) | Required implementation prerequisite: the single immutable local `AttributeUse` API, order, form/chameleon policy, use kind, type identity, locations, and diagnostics. | No second attribute representation; no copied global declaration facts. |
| [#213](https://github.com/goxdra/goxsd9/issues/213) | Simple-type variety and reference identities. | No list/union value semantics. |
| [#312](https://github.com/goxdra/goxsd9/issues/312) | Identity-only XML built-in reference facts. | `xs:ID`/`xs:NCName`/`xs:anyURI`/`xs:language` lexical, value, and uniqueness work remains separate. |
| [#392](https://github.com/goxdra/goxsd9/issues/392) | Direct group reference QName, locations, exact range, target identity, and XSD3047–XSD3050 diagnostics. | Opaque particle only; no member expansion. |
| [#404](https://github.com/goxdra/goxsd9/issues/404) | Named-group direct sequence/choice facts. | Target members remain owned by the named group and are not flattened here. |
| [#414](https://github.com/goxdra/goxsd9/issues/414) | Empty/particle-free extension packet and named empty-base, inherited bounded wildcard seam. | No nonempty/broader base, new wildcard, or derivation widening. |
| [#437](https://github.com/goxdra/goxsd9/issues/437) | The independently executable implementation packet for exactly this decision, after #317. | Query-only composition first; explicit validation/generation rejection. |

[#215](https://github.com/goxdra/goxsd9/issues/215) remains the later broad
auxiliary composition/consumer work and may consume this fact boundary. [#333](https://github.com/goxdra/goxsd9/issues/333)
remains global inline attributes, and [#400](https://github.com/goxdra/goxsd9/issues/400)
remains local element inline types. None is reopened or duplicated here.
[#414](https://github.com/goxdra/goxsd9/issues/414) remains the empty/particle-free
extension packet; this record only composes its already-supported base seam.

## Risks and next actions

The main risks are flattening an opaque group into #404 members, losing source
causes or locations while composing #317 uses, exposing a semantic `0/0` as a
zero-valued particle, and accidentally turning #312 identity facts or #213
varieties into value support. Visibility and base-cycle handling can also
regress if composition bypasses the existing seams. Ordered slices and tagged
variants are the safeguards against map-order and impossible-state bugs.

Implementation remains scoped to #437 and depends on #317's local-attribute
foundation. Validation, generation, and broad auxiliary consumption remain
separate packets. This record defines future behavior only; it reports neither
implementation, validation, nor conformance.
