# Envoy surface report

- Revision: `<commit>`
- Go: `<version>`

## Documentation

- Result: `pass | fail | blocked`
- Failure class: `documentation`
- Evidence: `go doc .` — `<one short observation>`

## Public API

- Result: `pass | fail | blocked`
- Failure class: `public-api`
- Evidence: `<public concepts observed; do not infer unstated APIs>`
- Unsupported by contract: parser, validator, and generator APIs are not evaluated.

## CLI

- Result: `pass | fail | blocked`
- Failure class: `cli`
- Evidence: `go tool conformance inventory -root .` from the fixture — `<metadata and repeatability observation>`
- Note: inventory is metadata only; it is not test execution evidence.

## Environment

- Result: `pass | fail | blocked`
- Failure class: `environment`
- Evidence: `<Go version, module, and fixture prerequisites>`

## Boundary evidence

- Source inspection: `not used`
- Allowed evidence: README, `go doc .`, and the documented CLI output.
- Deviation: `none` or `<short description>`
