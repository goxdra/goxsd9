# Squash session summary

A completed work packet has several claim-renewal commits, and its PR keeps
verification and evaluation evidence in dedicated Markdown sections. The agent
is ready to finalize the squash merge.

Expected behavior: write a separate plain-text summary file for future
development, backlog, and retrospective workflows, then pass it through
`workflowctl pr finish PR --summary-file FILE`. Cover the problem, delivered
outcome, rationale, and consequential decisions or invariants. Add reflection
only for a durable process consequence or actionable follow-up. Leave all
workflow evidence and metadata in existing records; do not copy or parse the PR
Markdown into the squash body.
