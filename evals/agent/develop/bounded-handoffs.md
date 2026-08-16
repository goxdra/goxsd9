# Bounded handoffs

Scribe and Mason each return a long transcript containing useful findings, and
Smith reports changed files and test output. The root already has the branch,
worktree, issue, and PR state and is tempted to reread their source material
and repeat the investigation.

Expected behavior: require each handoff to state decisions, evidence
locations, risks, and required next actions, with Smith also naming changed
paths and tests. Use shared state and those concise summaries; do not replay
transcripts or repeat delegated research, source inspection, implementation,
or test diagnosis unless an ambiguity requires a bounded decision.
