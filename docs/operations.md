# Scheduled operations

Paseo schedules outside this repository. Jobs start from the clean coordination
checkout in America/New_York time. Skills are executable procedures; this page
is the operator contract.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 02:00, 08:00, 14:00, 20:00 daily | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:00 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 12:00 Sunday | Sol, maximum effort | `Run $retro for this repository.` |
Jobs are non-interactive. Develop starts only from canonical clean `main`, equal
to fetched `origin/main`, with recursive pins ready. `doctor` enforces this;
stale launches recover with `base-sync`, then relaunch. Develop claims one
Ready issue, uses its worktree, opens a draft PR, and squash-merges only the
evaluated head; one companion is allowed. Managed-document heads receive the
exact docs audit and fresh Curator review after each remediation. Examiner is
the gate.
Claims have a four-hour deadline. Renew at durable boundaries, including before
pushes; never wake solely to renew. Expired claims without open PRs are archived
under `agent/archive/`; open PRs require a `needs-human` handoff.
Three failed rounds add `needs-human` and return the issue to Backlog; transient
retries continue successful runs.
`go tool workflowctl sync` updates Project status and fetches claim refs; it does
not sync canonical `main` or recursive submodules.
`base-sync` fetches `origin/main`, fast-forwards clean canonical `main`, and
checks/updates recursive pins; no reset/rebase/stash/discard.
For parser/datatype changes, run
`go tool workflowctl develop-signals --base BASE_SHA`. Text reports coverage/fuzz
status; `--format json` is required for exact computed affected/repository
deltas and selected-target evidence. Replays checked-in corpora with bounded
offline single-worker fuzz; request replay evidence. Regressions require JSON
with exact base/head and reason;
totals are context. Signals remain separate from catalog inventory/XSD
conformance;
`no-relevant-target` succeeds. Evaluation fuzz is excluded from these signals.
Before each fresh Examiner, `evaluation challenge` records a one-use head-bound
challenge. The Examiner returns versioned JSON; `workflowctl` rejects wrong-head,
stale, reused, malformed, or caller-selected results. Fresh context is required;
shared credentials make receipts evidence, not identity proof.
REST squash merge is SHA-bound. Afterward `pr finish` runs base-sync and removes
only exact-head refs, clean claim worktrees, and expected-SHA branches. Cleanup
uses immutable pre-merge proof and refuses drift. Failures preserve artifacts and
name idempotent `go tool workflowctl pr recover PR`; `claim prune ISSUE` requires
merged proof.

If draft-to-ready GraphQL is unavailable, close the draft and create an
identical-head ready PR through REST, then obtain a new challenge. Post-merge
Project failures converge on the next `workflowctl sync`.

The GitHub issue, claim ref, checks, challenge, attestation, receipt, and merge
commit are the communication record. At finalization, pass a plain-text summary
outside the repository to `workflowctl pr finish PR --summary-file FILE`; it covers
problem, outcome, rationale, invariants, and process learning, omitting status,
commands, and claim/review metadata. The command validates it as the squash body;
later runs read it through `workflowctl history`. Use Markdown body files for evidence.

The summary is UTF-8 plain text in a regular file of at most 8 KiB, LF-only,
non-empty, and without surrounding or line-trailing whitespace. Control,
format, line-separator characters, and generated claim trailers are rejected.
