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
- Supported public parser API: `ParseSchema`; this surface evaluation deliberately does not execute parser, validator, or generator APIs. It evaluates only the README, `go doc .`, and documented inventory CLI.

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
