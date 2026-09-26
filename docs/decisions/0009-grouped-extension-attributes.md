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
the group reference; local attributes follow it in lexical order. The current
parser rejects local attributes inside every `complexContent` body with
`XSD3003` at the attribute and returns no schema. Its extension input has no
group-plus-use variant. Existing standalone group references and local
`AttributeUse` facts therefore do not admit this combination yet.

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
group. XSD 1.1 additionally has open-content and assertion constructs; they
are outside this slice. Attribute uses are set-like semantically, while this
future model must retain lexical and provenance order.

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
These blockers show current parser limits; the pinned group probes stop at
`XSD3003` at the group reference, separately from the local-attribute rejection
above. `xs:ID` identity,
lexical space, value space, and instance uniqueness are distinct concerns
and none is claimed here.

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
`anyAttribute`, open content, assertions, mixed or simple content, nonempty or
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

Today a structurally admitted local `complexContent` attribute returns
`FeatureSchemaSyntax`/`FailureUnsupported`/`XSD3003` at its source `Loc`, with
`ErrUnsupported` and no schema. The pinned standalone group probes above stop
at `XSD3003` at their group references. The matrix specifies requirements for
the future combined admission. Existing resolver, group-reference, and
`AttributeUse` diagnostics remain separate until that admission exists.

| Axis | Proposed admitted shape | Invalid after admission | Resolution failure | Explicit unsupported | N/A |
| --- | --- | --- | --- | --- | --- |
| Edition/policy | One graph policy must cover the XSD 1.0/1.1 shape under Compatibility, Strict10, or Strict11. | Malformed edition-specific syntax must retain its invalid diagnostic. | N/A; policy selection does not acquire sources. | Open content, assertions, and valid shapes beyond this contract remain unsupported. | `schema/@version` must never select an edition. |
| Named/anonymous/inline/ref | One named global owner, named #414 base, direct named group ref, and ordered #317 local declaration/ref uses must be admitted. | Missing/duplicate model child, malformed QName/NCName, invalid name/ref/use/form/occurrences, duplicate effective name, and unresolved/wrong-kind/ambiguous/inaccessible targets must fail with existing invalid causes. | Only source acquisition through the caller's resolver is a resolution failure. | Current group-plus-use input fails `XSD3003` at the local attribute; future anonymous/local owners, multiple/nested groups, attribute groups, and unsupported scalar varieties stay excluded. | Global inline attributes and local element-inline content are outside this contract. |
| Graph visibility/cycles | The builder must reuse forward/include/import/chameleon/repeat identities and visibility; target members stay opaque. | Target visibility, ambiguity, wrong-kind, and base/simple-type cycles must retain located invalid diagnostics. | Referenced-source acquisition must retain its resolver cause. | Recursive group expansion and broader graph composition remain outside the contract. | Group-member traversal is unnecessary because no expansion occurs. |
| Failure class | On success the future builder must publish one immutable fact; on error no schema. | Structural and target failures must retain stable code, primary/related `Loc`s, cause, and edition `SpecRef`. | Acquisition alone uses `FailureResolution` at the discovery boundary. | Valid unavailable behavior needs a registered feature, stable code, `Loc`, `SpecRef`, and `ErrUnsupported`; validation/generation of the admitted shape must reject explicitly. | No conformance outcome follows from this query contract. |
| Location/order/provenance | Preserve group QName/ref/use `Loc`s, exact range, target ID, ordered local uses, effective names, type/form `Loc`s, and declaration order; validate before `0/0` omission. | Reference-use primary and target/duplicate/bound related locations must survive. | Preserve acquisition location and underlying cause. | Preserve the unsupported construct's source `Loc` and versioned `SpecRef`. | Map iteration cannot define observable order; ordered slices do. |

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
