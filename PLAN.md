# Plan

Current phase: **Vertical Slice**

The project delivers a useful vertical slice before broadening to full measured
conformance. GitHub Issues contain executable work; this file contains outcomes
and sequencing only.

## 0. Bootstrap

- Deterministic Go module, quality gates, environment doctor, and documentation
  checks.
- Worktree claim, handoff, Project synchronization, and history tooling.
- Checked-in Guild agents and develop, backlog, and retro skills.
- W3C tests pinned and specification manifest established.
- Agent regression scenarios for recurring development and review failures.

Exit measure: a scheduled development prompt can pick, claim, implement,
evaluate, and merge one issue without human interaction.

## 1. Vertical slice

- Stream a small compatible XSD 1.0 or 1.1 graph through a caller resolver.
- Produce an immutable schema with deterministic query and walk APIs.
- Validate a small XML instance with located diagnostics.
- Generate compiling Go for the same schema, including a choice type switch.

Exit measure: documented library and CLI examples complete end to end.

## 2. Schema model and bootstrap

- Download and index the normative specification set.
- Parse the separately pinned XSD 1.0 and 1.1 `XMLSchema.xsd` and
  `datatypes.xsd` graphs, including the explicit `xml.xsd` import policy, using
  the minimal syntax kernel.
- Verify raw artifact digests and apply only the representation conversion
  declared by the manifest before parsing.
- Generate the canonical schema syntax declarations.
- Verify regeneration is byte-identical.
- Complete references, derivation, substitution, and content-model invariants.

Exit measure: bootstrap regeneration and model invariants pass deterministically.

## 3. Datatypes and facets

- Implement all strict built-in lexical and value spaces.
- Implement every standard facet with exact numeric behavior.
- Add the idiomatic Go profile and custom datatype-library contract.
- Cover QName and other context-sensitive lexical conversions.

Exit measure: datatype conformance is reported by feature with no silent gaps.

## 4. Validation

- Complete particle, wildcard, identity, derivation, and instance constraints.
- Preserve instance and related schema locations in every diagnostic.
- Calculate validation programs on demand without schema caches.
- Add deterministic error aggregation and recovery boundaries.

Exit measure: all implemented W3C validation tests have stable outcomes.

## 5. Go code generation

- Define stable naming and package policies.
- Generate types for derivation, unions, lists, choices, and occurrence ranges.
- Integrate strict or idiomatic datatype profiles.
- Validate generated packages with Go compile and round-trip fixtures.

Exit measure: representative real-world schemas generate usable documented Go.

## 6. Conformance breadth

- Close unsupported feature clusters by tests unlocked and user impact.
- Exercise mixed XSD 1.0 and 1.1 document graphs.
- Investigate queried, disputed-test, and disputed-spec cases with explicit
  specification evidence.
- Preserve catalog status independently from execution outcome and publish
  reports that exclude all three classes from headline and unlock rankings.

Exit measure: no known unsupported XSD parser or validator feature remains.

## 7. XPath 2.0 completion

- Implement the XSD-required subset first.
- Expand the data model, operators, functions, and edge cases toward full XPath
  2.0 conformance.
- Keep XPath isolated behind schema and validation contracts.

Exit measure: the supported XPath surface and remaining gaps are test-measured.
