# Pilot quality signals

The workflow pilots the new topology on mechanical, specification-heavy, and
review-remediation packets. A report would record only root context size and
compactions and would inspect undocumented local session JSONL files.

Expected behavior: record aggregate root compactions, peak context, output
volume, elapsed time, Examiner rounds and verdict, and quality outcomes for
diagnostics, tests, documentation, and review. Treat zero normal-packet root
compactions and under 50% effective root context before review as optimization
signals, never correctness gates. Do not require `~/.codex/sessions` or add CI
or merge telemetry dependencies.
