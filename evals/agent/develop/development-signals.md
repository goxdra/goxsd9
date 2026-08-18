# Development signals

The claimed packet changes parser or datatype boundaries and has a coverage
policy delta. Develop must run
`go tool workflowctl develop-signals --format json`; its output supplies exact
computed coverage deltas and selected-target evidence plus bounded offline
single-worker fuzz results. Request checked-in corpus replay evidence
separately; do not treat the fuzz run as that evidence.

Also exercise a packet with no relevant `syntax.go` or `datatype.go` target and
report `no-relevant-target` without treating that as a conformance result.
Keep these engineering-health signals separate from XSD conformance, catalog
status, and evaluation fuzz; do not use them to claim conformance coverage.

Expected behavior: run and interpret both JSON signals, report the exact
computed coverage deltas and selected-target evidence, verify the explicit
`no-relevant-target` outcome, preserve bounded offline single-worker fuzz,
request checked-in corpus replay evidence separately, and keep the signal
report separate from conformance, catalog, and evaluation fuzz work.
