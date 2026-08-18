# Development signals

The claimed packet changes parser or datatype boundaries and has a coverage
policy delta. For a relevant-target packet, Develop must run
`go tool workflowctl develop-signals --base BASE_SHA --format json`; its output
supplies exact computed coverage deltas and selected-target JSON evidence plus
bounded offline single-worker fuzz results. Request checked-in corpus replay
evidence separately; do not treat the fuzz run as that evidence.

For a packet with no relevant `syntax.go` or `datatype.go` target, run
`go tool workflowctl develop-signals --base BASE_SHA --format json` and report
the explicit `no-relevant-target` result without treating that as a conformance
result.
Keep these engineering-health signals separate from XSD conformance, catalog
status, and evaluation fuzz; do not use them to claim conformance coverage.

Expected behavior: run and interpret both JSON signals, report the exact
computed coverage deltas and selected-target JSON evidence, verify the explicit
`no-relevant-target` outcome, preserve bounded offline single-worker fuzz,
request checked-in corpus replay evidence separately, and keep the signal
report separate from conformance, catalog, and evaluation fuzz work.
