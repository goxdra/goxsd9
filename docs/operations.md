# Scheduled operations

Paseo schedules outside this repository. Jobs start from the clean coordination
checkout in America/New_York time. Skills are executable procedures; this page
is the operator contract.

| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 02:00, 08:00, 14:00, 20:00 daily | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:00 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 12:00 Sunday | Sol, maximum effort | `Run $retro for this repository.` |

Jobs are non-interactive. Develop starts only from the canonical primary on a
clean `main` exactly equal to freshly fetched `origin/main`, with recursive
pinned submodules ready. `doctor` enforces this; stale/linked launches recover
with `go tool workflowctl base-sync` there, then relaunch. Develop claims one
Ready issue, uses its worktree, opens a draft PR, and squash-merges only the
evaluated head; one companion is allowed. Every pushed head containing a
managed-document change receives the exact docs audit and a fresh read-only
Curator review; repeat both after each remediation push. Examiner is the gate.

Claims have a four-hour deadline after issuance. Renew at durable boundaries,
including before pushes; never wake solely to renew. Expired claims without open PRs are archived under `agent/archive/`; open PRs require a `needs-human` handoff.

Three failed evaluation rounds add `needs-human` and return issue to Backlog;
bounded transient retries continue successful runs.

`go tool workflowctl sync` updates Project status and fetches claim refs for lease classification; it does not sync canonical `main` or recursive submodules.
`base-sync` fetches `origin/main`, fast-forwards clean canonical `main`, and
checks/updates recursive pins; no reset/rebase/stash/discard.

Before each fresh Examiner, `evaluation challenge` records a one-use head-bound
challenge. The Examiner returns versioned JSON; `workflowctl` rejects wrong-head,
stale, reused, malformed, or caller-selected results. Fresh context is required;
shared credentials make receipts evidence, not identity proof.

REST squash merge is SHA-bound. After GitHub returns the merge SHA, `pr finish`
runs base-sync and removes only exact-head remote refs, clean uniquely registered
claim worktrees, and expected-SHA local branches. Squash drops topic ancestry, so
cleanup/recovery use latest trusted pre-merge proof with immutable
base/head/closure/body metadata; refuse receipt or PR/local/remote/worktree drift.
Failures report completed merge, preserve artifacts, and name idempotent `go tool
workflowctl pr recover PR`; `claim prune ISSUE` requires merged proof.

If draft-to-ready GraphQL is unavailable, close the draft and create an
identical-head ready PR through REST, then obtain a new challenge and Examiner.
Post-merge Project failures converge on the next `workflowctl sync`.

The GitHub issue, claim ref, checks, challenge, attestation, receipt, and merge
commit are the communication record. At finalization, pass a plain-text summary
outside the repository to `workflowctl pr finish PR --summary-file FILE`; it covers
problem, outcome, rationale, invariants, and process learning, omitting status,
commands, and claim/review metadata. The command validates it as the squash body;
later runs read it through `workflowctl history`. Use Markdown body files for evidence.

The summary is UTF-8 plain text in a regular file of at most 8 KiB, LF-only (one
final LF accepted), non-empty, and without leading/trailing or line-trailing
whitespace. Control/format/line-separator characters and generated claim trailers
are rejected; Markdown punctuation has no special meaning.
