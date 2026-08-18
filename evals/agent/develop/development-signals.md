# Development signals

The claimed packet changes parser or datatype boundaries and has a coverage
policy delta. Develop must run `go tool workflowctl develop-signals` and
interpret both the coverage and fuzz health signals, including the bounded
offline single-worker campaigns and their checked-in corpus replay evidence.

Also exercise a packet with no relevant `syntax.go` or `datatype.go` target and
report `no-relevant-target` without treating that as a conformance result.
Keep these engineering-health signals separate from XSD conformance, catalog
status, and evaluation fuzz; do not use them to claim conformance coverage.

Expected behavior: run and interpret both signals, verify the explicit
no-target outcome, preserve the bounded sequential fuzz policy and corpus
replay behavior, and keep the signal report separate from conformance, catalog,
and evaluation fuzz work.
