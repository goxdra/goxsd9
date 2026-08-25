# Envoy surface report
- Revision: `<commit>`
- Go: `<version>`

## Documentation
- Result: `pass | fail | blocked | unevaluated`
- Failure class: `documentation`
- Evidence: README and repeated `go doc .` output — `<observation>`
## API
- Result: `pass | fail | blocked | unevaluated`
- Failure class: `api`
- Evidence: consumer — `ParseSchema`; valid/invalid `ValidateInstance`; repeated `GenerateGo`; invalid `FailureInvalid`, `XSD2001`, located input `Loc`, related schema, and `xsd11-datatypes#integer`.
## Command
- Result: `pass | fail | blocked | unevaluated`
- Failure class: `command`
- Evidence: repeated public `go run .` output — `<observation>`
## Generated consumer
- Result: `pass | fail | blocked | unevaluated`
- Failure class: `generated-consumer`
- Evidence: temporary external module compiled complete returned Go source unchanged; repeated generation was byte-identical.
## Environment
- Result: `pass | fail | blocked | unevaluated`
- Failure class: `environment`
- Evidence: `<prerequisites>`
## Inventory boundary
- Inventory result: `unevaluated`
- Evidence: repeated `go tool conformance inventory -root .` — metadata only; it is not execution evidence.
- Catalog expected-validity, status, and headline fields remain metadata, separate from consumer execution.
## Unevaluated scope
- Product CLI: `unevaluated`; Direct-choice generation: `unevaluated`; Broader schema/validation features: `unevaluated`; W3C conformance scoring: `unevaluated`
## Boundary evidence
- Source inspection: `not used`; Search/source-view commands: `not used`
- Private implementation: `not used`; Test utilities: `not used`
- Repository imports: `not used`; Allowed evidence: README, `go doc .`, and public consumer/CLI behavior.
- Deviation: `<none or description>`
