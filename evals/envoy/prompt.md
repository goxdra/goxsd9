# Envoy surface evaluation

Use only README.md, `go doc .`, and public command/consumer behavior. Evidence from source files, implementation-only details, test utilities, and repository import declarations are outside the permitted evidence. Do not use source-viewing or search commands.

The public consumer exercises the supported scalar path, so these API checks are mandatory: `ParseSchema` must succeed with a nil resolver; `ValidateInstance` must return nil for the valid instance and must return an invalid `Diagnostic` for the invalid instance; and `GenerateGo` must be called twice with byte-identical results. Check the invalid diagnostic through accessors: `FailureInvalid`, `XSD2001`, the primary input `Loc`, related schema evidence, and `xsd11-datatypes#integer`. The consumer also compiles the complete returned Go source in a temporary external module. SourceID is opaque; CLI source IDs and catalog statuses do not apply to direct API evidence.

Run these fixed commands in order. Repeat each command once and compare its complete output byte-for-byte. Run the inventory from `evals/envoy/fixture`, `go doc .` from the repository root, and the consumer from `evals/envoy/fixture/consumer`.

```sh
go tool conformance inventory -root .
```

```sh
go doc .
```

```sh
go run .
```

The catalog inventory is deterministic catalog metadata only; it is not schema or instance test execution evidence. Keep catalog expected-validity, status, and headline fields separate from consumer execution. Keep product CLI, direct-choice generation, broader schema/validation features, and W3C conformance scoring explicitly unevaluated.

Record the result in `report.md`. Give documentation, api, command, generated-consumer, and environment separate failure classes and use only pass, fail, blocked, or unevaluated statuses. Keep boundary evidence separate.
