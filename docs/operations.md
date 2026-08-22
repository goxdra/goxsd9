# Scheduled operations

Paseo schedules outside this repository. Jobs start from the clean coordination
checkout in America/New_York. Skills are executable procedures; this page
is the operator contract.
| Job | Schedule | Agent | Prompt |
| --- | --- | --- | --- |
| Develop | 00:00, then every 3 hours | Luna, maximum effort | `Run $develop for this repository.` |
| Backlog | 10:30 daily | Sol, maximum effort | `Run $backlog for this repository.` |
| Retro | 13:30 Sunday | Sol, maximum effort | `Run $retro for this repository.` |
Jobs are non-interactive. Develop starts from clean canonical `main` matching
fetched `origin/main` with recursive pins; `doctor` enforces; stale launches:
`base-sync`. Develop claims one Ready issue, uses worktree, opens draft PR,
squash-merges evaluated head; one companion. All managed-document heads: exact
audit + fresh read-only Curator review; repeat after each remediation push;
preserve review records in PR evidence; Examiner gates.
Claims have a four-hour deadline. Renew at durable boundaries, including before
pushes; never wake solely to renew. Expired claims without open PRs are archived
under `agent/archive/`; open PRs require `needs-human`.
Three failed rounds add `needs-human`; return issue to Backlog;
retries continue.
`go tool workflowctl sync` updates Project status and fetches claim refs; it does
not sync canonical `main` or recursive submodules.
Run-local refs never affect ownership, Project status, or PR heads; inventory-only.
`base-sync` fetches `origin/main`, fast-forwards `main`, checks recursive pins;
no reset/rebase/stash/discard.
After draft opens, set `PR_NUMBER` from open PR:
`PR_NUMBER="$(gh pr view --json number --jq '.number')"`; then set
`BASE_SHA="$(gh api repos/goxdra/goxsd9/pulls/$PR_NUMBER --jq '.base.sha')"` to
exact REST base SHA; use it for `develop-signals --base "$BASE_SHA"`,
`docs audit --base "$BASE_SHA"`, and `pr evidence update` JSON; never
`origin/main` or local merge-base.
Non-relevant: `no-relevant-target`/`not-measured` valid. Text coverage/fuzz; JSON
deltas. Bounded offline fuzz; corpus replay. Repeatable bounded
`--additional-fuzz PACKAGE:TARGET` is current-head validated, separate. Regression
JSON: base/head/reason. Fuzz is engineering health, not conformance. Evidence
versioned; before mutation workflowctl resolves REST base/head/local match,
recomputes signals/policy, and compares canonical payload. Updates preserve bytes.
Challenge/finish require REST base/head, audit, and Curator/no-doc result;
challenges bind body/evidence digests. Before Examiner, `evaluation challenge`
records one-use head-bound challenge. Examiner JSON is versioned;
`workflowctl` rejects wrong-head, stale, reused, malformed, or caller-selected
results. Fresh context required; receipts are evidence, not identity proof.
REST squash merge is SHA-bound; `pr finish` base-syncs, removing exact-head refs/clean claim worktrees/expected-SHA branches. Cleanup requires immutable pre-merge proof/drift preservation, exact issue/run/SHA/evaluated-head/merge/current-value proof/leased-exact deletion, idempotent `pr recover`, merged-proof `claim prune`; archive/unrelated refs remain.

If draft-to-ready GraphQL is unavailable, close the draft and create an
identical-head ready PR via REST, then require a new challenge and fresh
Examiner. Post-merge Project failures converge on `workflowctl sync`.

Issue, claim, checks, challenge, attestation, receipt, and merge commit record
communication. At finalization, pass a plain-text summary outside the repository
to `workflowctl pr finish PR --summary-file FILE`; it covers problem, outcome,
rationale, invariants, and process learning; omit status, commands, and
claim/review metadata. The command validates the squash body; history reads it.
Use Markdown body files for evidence.

Summary is non-empty UTF-8 text in a file of at most 8 KiB, LF-only; one final LF
is accepted. Surrounding/line-trailing whitespace, control, format, other
line-separator characters, and generated claim trailers are rejected.
