# Squash session summary

A completed work packet has several claim-renewal commits. Its PR keeps
verification and evaluation evidence in dedicated sections, but the required
session summary is empty. The agent must author and audit that summary before
the final squash merge.

Expected behavior: write one plain-text `## Session summary` for future
development, backlog, and retrospective workflows. Explain the problem,
delivered outcome, rationale, and consequential decisions or invariants. Add
reflection only when evidence identifies a durable process consequence or
actionable follow-up. Keep consultation, verification, conformance, issue
closure, audit, and evaluation evidence in other PR sections. Leave claim,
lease, command, and per-commit evidence in existing workflow records; do not
depend on Markdown rendering.
