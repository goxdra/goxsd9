# 0009: Grouped complex-content extensions with local attributes

Status: accepted

## Decision

This accepted decision specifies a future query representation for one named
global `complexType` whose
`complexContent/extension` uses the already-supported named-base boundary from
[#414](https://github.com/goxdra/goxsd9/issues/414), one direct named
`<group ref="...">` particle, and one or more ordered direct local
`<attribute>` uses from [#317](https://github.com/goxdra/goxsd9/issues/317).
The extension body may contain its optional annotation. The model child is
the group reference; local attributes follow it in lexical order. A bounded
group-only extension currently parses. A structurally admitted attribute-only
extension fails `XSD3003` at its local attribute; adding a valid group reference
before that attribute fails earlier at the group `ref` with `XSD3003`.
Malformed attributes retain invalid-input diagnostics when reached. Neither
failure returns a schema; the current extension input carries particles but no
local attribute uses. Standalone group and `AttributeUse` models remain separate.

The future group particle must remain opaque and preserve its expanded written
QName, `ref` and use-site locations, exact occurrence range, and target
`ComponentID`; it must never copy or expand the target's members. It must reuse #317's sole
immutable `AttributeUse` representation for declaration/reference forms,
effective names, `use`, form/chameleon policy, type identity, locations, and
lexical order. Attribute uses are a semantic set, so duplicate effective names
must be invalid; their ordered model view is part of the proposed contract.

The future builder must validate an effective `0/0` group against its target
before omitting it from the public particle view. It would then publish #317's
attribute-only shape, never a zero-valued particle. After schema admission,
validation and Go generation must reject the composed shape explicitly.

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
group. Existing `openContent=none` support applies only to eligible shapes
under Compatibility/Strict11; this decision adds no open-content behavior to
the grouped composition. Other modes and assertions remain outside it.
Attribute uses are set-like semantically; the future model must retain lexical
and provenance order.

The edition-specific schema-for-schema and datatype artifacts are pinned in
[`specs/manifest.json`](../../specs/manifest.json); exact parser observations
live in [`bootstrap_probe_test.go`](../../internal/specs/bootstrap_probe_test.go).
Those observations retain the current `XSD3003` boundary: attribute-only
`complexContent` stops at the local attribute; the datatype-schema group
probes stop at the group reference. A bounded group-only extension remains
queryable, while the combined group-plus-use shape stops at its group `ref`.
No failing parse returns a partial schema. These separate observations do not
establish support for the proposed composition.
`xs:ID` identity, lexical space, value space, and instance uniqueness remain
distinct from this decision.

## Proposed shape and non-goals

The future query contract would admit:

- a named global complex type;
- `complexContent/extension` with a named base accepted by #414;
- an optional annotation, exactly one direct `<group ref="...">` model
  child, and one or more direct local `<attribute>` uses after that child;
- a group target resolved as a named model group using #392's direct-reference
  facts; and
- local uses accepted by #317's scalar/type and namespace allowlists,
  including its supported anonymous local simple-type identities.

The future tagged composition must resolve the group as an opaque #392 particle,
then local uses through #317, then the named base through #414. It introduces
no validation or generation content model.

The proposed group may be `0/0`, subject to target validation and the
normalization above. The builder must not infer a syntactically absent group.
The base must be a named completed empty-content base with only the inherited
wildcard facts that #414 can represent. This decision does not add or expand
`attributeGroup`, multiple/nested/local/anonymous groups, `all`, extension
`anyAttribute`, open content in this composition, assertions, mixed or simple content, nonempty or
broader bases, `xs:anyType` direct extension, another derivation kind, value
constraints, or consumer behavior.

The simple-type variety and reference identities from
[#213](https://github.com/goxdra/goxsd9/issues/213) remain distinct from
list/union value semantics. Named simple-type `final` facts and their controls remain in effect.
This decision excludes union value semantics. The identity-only built-ins from
[#312](https://github.com/goxdra/goxsd9/issues/312)—`xs:language`,
`xs:NCName`, `xs:anyURI`, and `xs:ID`—are not widened into local attribute
lexical or value support. For a local use, `name` is an
unqualified NCName and `ref` is an expanded QName; they are mutually exclusive
and do not share the global-declaration name path.

## Phase representation and invariants

The future builder must use a tagged, phase-specific extension-body variant
for `group + attribute uses`. Generic nullable particle/attribute fields could
discard a child or publish an impossible combination. Its syntax input would
own extension, group, base, and ordered-use locations; its resolved result
would own the opaque group particle, an immutable ordered use slice, and #414
base facts. A `0/0` result must select the attribute-only variant without
retaining a zero particle or creating a second attribute API.

The required construction order is deterministic:

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

The future composition must preserve sequential resolver calls and allocate
identities before resolution. It must not backpatch completed components;
returned slices, QNames, IDs, locations, and occurrences must be owned or
copied at the phase boundary. Maps remain lookup-only; ordered slices define
attribute order, diagnostics, walks, and output. Consumer content models stay
on demand and out of the schema.

## Edition, policy, and graph behavior

The existing immutable graph-wide policy from [Decision 0004](0004-xsd-language-policy.md)
admits supported mixed XSD 1.0/1.1 graphs in Compatibility; Strict10 and
Strict11 select one profile, and `schema/@version` remains inert. The future
grouped contract must use that policy in either edition. It adds no
open-content or assertion behavior.

Existing discovery supports forward, included, imported, chameleon, repeated,
and cyclic source identities with visibility and sequential resolver rules.
The future composition must reuse those rules: inaccessible or ambiguous
targets stay excluded, discovery cycles stay interned, and complex-base and
simple-type cycles retain located failures. It must not traverse group-member
or attribute-group recursion because target members remain opaque.

## Classification and sibling-axis matrix

Today the bounded group-only shape parses. Structurally admitted attribute-only
extensions return `FeatureSchemaSyntax`/`FailureUnsupported`/`XSD3003` at the
attribute `Loc`; the bounded combined shape with a valid group ref returns
the same class/code earlier at group `RefLoc`. Both preserve `ErrUnsupported`
and return no schema. Their
edition-specific structures `SpecRef`s end in
`#element-complexContent..extension` and `#cos-particle-extend`, respectively.
The matrix specifies future combined admission; malformed attributes retain
invalid diagnostics when reached. Existing resolver, group-reference, and
`AttributeUse` diagnostics remain separate until that admission exists.

| Axis | Proposed admitted shape | Invalid after admission | Resolution failure | Explicit unsupported | N/A |
| --- | --- | --- | --- | --- | --- |
| Edition/policy | One graph policy must cover the XSD 1.0/1.1 shape under Compatibility, Strict10, or Strict11. | Malformed edition-specific syntax must retain its invalid diagnostic. | N/A; policy selection does not acquire sources. | This composition adds no open-content support; elsewhere only `openContent=none` is admitted under Compatibility/Strict11. Other modes, assertions, and broader shapes stay unsupported. | `schema/@version` must never select an edition. |
| Named/anonymous/inline/ref | One named global owner, named #414 base, direct named group ref, and ordered #317 local declaration/ref uses must be admitted. | Duplicate model children, malformed QName/NCName, invalid name/ref/use/form/occurrences, duplicate effective name, and unresolved/wrong-kind/ambiguous/inaccessible targets must fail with existing invalid causes. | Only source acquisition through the caller's resolver is a resolution failure. | Current structurally admitted attribute-only input fails `XSD3003` at the attribute, while bounded group-plus-use with a valid ref fails earlier at group `RefLoc`; a syntactically absent group is valid but outside the proposed slice. Future anonymous/local owners, multiple/nested groups, attribute groups, and unsupported scalar varieties stay excluded. | Global inline attributes and local element-inline content are outside this contract. |
| Graph visibility/cycles | The builder must reuse forward/include/import/chameleon/repeat identities and visibility; target members stay opaque. | Target visibility, ambiguity, wrong-kind, and base/simple-type cycles must retain located invalid diagnostics. | Referenced-source acquisition must retain its resolver cause. | Recursive group expansion and broader graph composition remain outside the contract. | Group-member traversal is unnecessary because no expansion occurs. |
| Failure class | On success the future builder must publish one immutable fact; on error no schema. | Structural and target failures must retain stable code, primary/related `Loc`s, cause, and edition `SpecRef`. | Acquisition alone uses `FailureResolution` at the discovery boundary. | Valid unavailable behavior needs a registered feature, stable code, `Loc`, `SpecRef`, and `ErrUnsupported`; validation/generation of the admitted shape must reject explicitly. | No conformance outcome follows from this query contract. |
| Location/order/provenance | Preserve group QName/ref/use `Loc`s, exact range, target ID, ordered local uses, effective names, type/form `Loc`s, and declaration order; validate before `0/0` omission. | Reference-use primary and target/duplicate/bound related locations must survive. | Preserve acquisition location and underlying cause. | For those well-formed shapes, attribute-only primary is attribute `Loc`; combined primary is group `RefLoc`. Preserve versioned `SpecRef`. | Map iteration cannot define observable order; ordered slices do. |

## Design dependencies

The future grouped body must reuse [#317](https://github.com/goxdra/goxsd9/issues/317)'s
single immutable ordered `AttributeUse` view,
[#392](https://github.com/goxdra/goxsd9/issues/392)'s opaque direct group
reference, and [#414](https://github.com/goxdra/goxsd9/issues/414)'s named
empty-base and inherited wildcard seam. The target must retain its own
[#404](https://github.com/goxdra/goxsd9/issues/404) members; they are never
flattened into the extension. [#213](https://github.com/goxdra/goxsd9/issues/213)
variety and [#312](https://github.com/goxdra/goxsd9/issues/312) XML built-in
identities add no list/union or attribute value semantics.

Global inline complex and attribute declarations and local element-inline
complex content stay outside the proposed grouped-extension contract. Its base
must remain within the existing empty/particle-free seam.

## Risks and boundaries

Implementing the contract could flatten an opaque group into #404 members,
lose causes or locations while composing #317 uses, publish a semantic `0/0`
as a zero particle, or turn #312/#213 identities into value support. Reusing
existing visibility and base-cycle checks, ordered slices, and tagged variants
would guard those boundaries.

The combined group-plus-use shape is currently schema-unsupported with no
completed component. This decision requires a future immutable query fact
over the bounded base seam, followed by explicit validation and generation
rejection. Broader derivation and group expansion remain outside its scope.
