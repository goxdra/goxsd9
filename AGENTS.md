# Repository instructions

These instructions apply to every file in this repository.

## Start here

1. Work from an issue claimed through `go tool workflowctl`.
2. Make changes only in the issue worktree. Keep the coordination checkout on
   `main` clean.
3. Read the issue, `README.md`, `ARCHITECTURE.md`, and the relevant part of
   `PLAN.md` before editing.
4. Run `go tool workflowctl doctor` before work and `go tool workflowctl check`
   before handoff.

## Go style

- Use the standard library. An external dependency requires a `needs-human`
  issue and explicit human approval. Development-only linters are the approved
  exception.
- Do not start goroutines or use locks, atomics, channels, or synchronization
  primitives.
- Avoid `else`. Keep the happy path at the left margin with early returns.
- Return errors for every recoverable condition. Panic only for an impossible
  variant that proves an internal invariant is broken.
- Check every returned error, including errors reported after loop iteration.
- Decorate errors at package boundaries. Preserve the cause, stable diagnostic
  code, and `Loc` whenever source input is involved.
- Keep exported API minimal. Export only deliberate user concepts with concise,
  current Go documentation.
- Keep comments concise and about current behavior. A change owns every nearby
  comment and reference that it makes stale.
- Avoid duplicate state. Compute values on demand unless the representation is
  itself the source of truth.
- Prefer phase-specific representations that make invalid states
  unrepresentable over repeatedly checking partially built objects.
- Never range over a map when the order can affect output, traversal,
  diagnostics, generated code, tests, logs, or API behavior. Sort explicit
  keys or use an ordered primary representation.
- Use `slog` through injected loggers. Do not use a global logger.
- Use `gofmt`, `go vet`, `go test`, and the configured `golangci-lint` checks.
  Do not add a Makefile.

## Diagnostics and unsupported behavior

- User input must not cause a panic.
- Classify failures as invalid input, unsupported behavior, resolution failure,
  or internal failure.
- Every user-facing diagnostic has a stable code and primary `Loc`. Add related
  locations and a specification reference when they clarify the cause.
- Return explicit unsupported diagnostics for unfinished specification
  behavior. Never skip, approximate, or misclassify it as invalid input.
- Do not return a schema after any error-level diagnostic.

## Architecture

- Parse XSD 1.0 and XSD 1.1 documents in mixed schema graphs. Compatibility is
  the default; strict version policies exist for conformance runs.
- Keep syntax trees internal. The public schema component model is immutable.
- Build components through deterministic phases. Allocate identities before
  resolving references; do not backpatch completed components.
- Resolver calls are sequential. The parser passes standard-library contexts,
  namespace URNs, and lexical schema locations without interpreting paths or
  opening resources itself.
- Keep lexical and value representations separate. QName conversion must retain
  the namespace context needed for interpretation.
- Calculate validator-specific content models on demand. Do not cache them in
  the schema.
- Walk components in documented schema-discovery and lexical declaration order.
- Generate Go type switches for XSD choices.

## Workflow

- Work commits and PR titles use `<type>(<scope>): <imperative summary>`.
  Scope is optional and `!` may follow the type or scope for a breaking change.
  Allowed types are `feat`, `fix`, `test`, `docs`, `refactor`, `perf`, `ci`,
  and `chore`; scopes use the canonical suffixes from `area/*` labels. Start
  the summary with a lowercase letter or digit, omit a trailing period, and
  keep the title within 72 characters. Explain why and affected invariants in
  the body when the subject is insufficient. Generated `chore(workflow): claim
  issue #N` commits retain their `Agent-*` trailers. `workflowctl pr open`
  validates branch-only commit subjects and the requested PR title; `pr finish`
  revalidates the subjects at the exact PR head and the actual GitHub title
  before merge.
- One PR has one primary issue and may include only separately claimed, closely
  related issues.
- The branch claim lease is two hours. Renew it at least every 30 minutes and
  before each shared mutation.
- Scribe and Mason consultation is the default. Record a concise exemption only
  for demonstrably mechanical work.
- Every PR receives an Examiner review in a fresh, read-only context. Smith
  fixes findings and uses a new Examiner context for each round. Three failed
  rounds mark the work `needs-human`.
- Create a head-bound `workflowctl evaluation challenge` before spawning each
  Examiner. Record its returned JSON attestation unchanged; Smith never chooses
  or rewrites the verdict.
- Use body files for GitHub Markdown. Do not pass escaped newlines through
  command arguments.
- A deferred item must become an issue only when it is independently actionable.
  Finish small related work in the current packet when doing so stays coherent.
- Mechanize any repeated or error-prone workflow in `workflowctl` or another Go
  tool, and add a regression test.

## Documentation

- `README.md` is the concise user entrypoint.
- `ARCHITECTURE.md` describes only current design.
- `PLAN.md` describes phases and measurable outcomes, not individual tasks.
- `docs/operations.md` is the scheduler and operator contract. Skills contain
  executable procedures, and the PR template contains only evidence headings.
- Decision records explain durable choices and link superseding decisions.
- Git and GitHub preserve history. Do not add session transcripts, stale
  progress diaries, or duplicate issue lists.
- After a draft PR exists, audit its final head with `workflowctl docs audit
  --base origin/main`. Any managed-document change requires a fresh read-only
  Curator review for placement, current relevance, duplication, history, and
  replacement opportunities. Deletion alone does not prove improvement.
- Examiner receives the exact audit and Curator result, independently checks
  the documentation, and remains the only authenticated merge gate.
- Envoy user evaluations may read library documentation and use public CLIs.
  They must not inspect source code.
