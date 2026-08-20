# Scheduled operations

Paseo schedules outside this repository. Jobs start from the clean coordination
checkout in America/New_York. Skills are executable procedures; this page
is the operator contract.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:30 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | Sol, maximum effort | `Run $retro for this repository.` |
Jobs are non-interactive. Develop starts from canonical clean `main`, equal
to fetched `origin/main`, with recursive pins. `doctor` enforces;
stale launches use `base-sync`. Develop claims one Ready issue, uses its
worktree, opens draft PR, and squash-merges evaluated head; one
companion allowed. Managed-document heads receive exact docs audit and fresh
Curator review after remediation; Examiner gates.
Claims have a four-hour deadline. Renew at durable boundaries, including before
pushes; never wake solely to renew. Expired claims without open PRs are archived
under `agent/archive/`; open PRs require `needs-human`.
Three failed rounds add `needs-human` and return issue to Backlog;
retries continue.
`go tool workflowctl sync` updates Project status and fetches claim refs; it does
not sync canonical `main` or recursive submodules.
Run-local `agent/issue-N-run-ID` refs never affect ownership, Project status, or PR heads; they are inventory-only.
`base-sync` fetches `origin/main`, fast-forwards clean canonical `main`, and
checks/updates recursive pins; no reset/rebase/stash/discard.
Every pushed head saves JSON from
`go tool workflowctl develop-signals --base BASE_SHA --format json` before
`pr evidence update`; non-relevant packets: `no-relevant-target`/`not-measured`
valid. Text: coverage/fuzz; JSON: exact affected/repository deltas,
selected-target evidence. Parser/datatype replays: bounded offline single-worker
fuzz; request corpus replay. Regression JSON: exact base/head/reason; totals
context. Signals stay separate from catalog inventory/XSD conformance/evaluation
fuzz.
PR evidence is versioned JSON between owned body markers; `pr evidence update`
uses exact signal/audit JSON, preserves other body bytes, and is idempotent.
Challenge/finish require exact REST base/head, audit, and Curator/no-doc result;
challenges bind body/evidence digests. Before each Examiner, `evaluation challenge`
records a one-use head-bound challenge. Examiner JSON is versioned;
`workflowctl` rejects wrong-head, stale, reused, malformed, or caller-selected
results. Fresh context is required; receipts are evidence, not identity proof.
REST squash merge is SHA-bound; `pr finish` base-syncs, removing exact-head refs/clean claim worktrees/expected-SHA branches. Cleanup requires immutable pre-merge proof/drift preservation, exact issue/run/SHA/evaluated-head/merge/current-value proof/leased-exact deletion, idempotent `pr recover`, merged-proof `claim prune`; archive/unrelated refs remain.

If draft-to-ready GraphQL is unavailable, close the draft and create an
identical-head ready PR via REST, then require a new challenge and fresh
Examiner. Post-merge Project failures converge on `workflowctl sync`.

The GitHub issue, claim ref, checks, challenge, attestation, receipt, and merge
commit are the communication record. At finalization, pass a plain-text summary
outside the repository to `workflowctl pr finish PR --summary-file FILE`; it covers
problem, outcome, rationale, invariants, and process learning, omitting status,
commands and claim/review metadata. The command validates it as squash body;
later runs read it through `workflowctl history`. Use Markdown body files for evidence.

Summary is non-empty UTF-8 text in a file of at most 8 KiB, LF-only; one final LF
is accepted. Surrounding/line-trailing whitespace, control, format, other
line-separator characters, and generated claim trailers are rejected.
