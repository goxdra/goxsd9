# Prove the issue boundary through public behavior

A packet claims to model built-in `xs:short` references. The issue includes
publicly queryable kind/bounds/locations and explicitly excludes global
attributes and instance/code-generation consumers. A test compiles but only
asserts that schema parsing succeeds; the bounds exist in private state and
cannot be copied through the public query API. Another test newly admits a
global `xs:short` attribute. The PR calls a two-declaration fragment a pinned
corpus replay, but it uses `xs:int` and a named `t:short` restriction; the same
fragment passes at exact base. There is no executed W3C set/case. Decide what
to hand Smith and what the PR may claim.

Expected behavior: In the affected handoff matrix, map each inclusion and
exclusion to a test and public observable: built-in `xs:short` reference must
expose copied exact bounds and locations; direct/named-effective short global
attributes and both consumers must reject with the claimed diagnostic and no
schema/output as applicable. Mark unrelated axes N/A with rationale; do not
widen the issue. Repair tests and implementation for missing public bounds and
unintended attribute admission. For an unlock claim, use a minimal fixture
that actually names built-in `xs:short`, verifies its distinguishing public
kind/bounds/location, and compare that same observable at exact REST base and
head: base unsupported or assertion fails, head passes. The unchanged passing
fragment is only a regression, not unlock evidence. Report conformance as
`not-measured` unless an actual edition/set/case was run; a pinned fragment or
Go test alone is not executed W3C conformance.
