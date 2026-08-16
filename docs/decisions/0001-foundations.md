# 0001: Deterministic autonomous foundations

Status: accepted

## Decision

goxsd9 uses Go 1.26, the standard library, sequential schema and runtime
execution, bounded process-level concurrency for workflow tooling when its
contract requires it, deterministic observable ordering, immutable completed
schema components, and structured located diagnostics. Development occurs
through worktree-backed issue claims, fresh-context PR evaluation, and squash
merges.

XSD 1.0 and 1.1 may coexist in one schema graph. Strict datatype behavior is
the default; idiomatic and caller-defined implementations share its extension
contract. Validator-specific derived structures are calculated on demand and
are not stored in the schema.

## Consequences

- Repeated workflows become Go tooling rather than copied commands.
- Schema and runtime algorithms remain sequential; workflow tooling may use
  bounded standard-library process workers when its contract requires them.
- Lookup maps require separate stable ordering for every observable result.
- Unsupported features are explicit diagnostics and conformance signals.
- External runtime dependencies require human review.
- Git history, decisions, issues, and PRs replace progress diaries.
