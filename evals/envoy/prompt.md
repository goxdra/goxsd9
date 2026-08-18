# Envoy surface evaluation

Use only the repository README, the public package documentation returned by `go doc .`, and the documented conformance CLI. Do not inspect repository source, private implementation details, or imports. Do not use source-viewing flags or search commands. If a step would need a parser, XML validator, or Go generator API, record it as unsupported rather than inventing an API.

Run the inventory from `evals/envoy/fixture`; its `-root .` is the fixture root.

```sh
go tool conformance inventory -root .
```

Run the package documentation command from the repository root.

```sh
go doc .
```

Repeat the commands when checking reproducibility. The inventory is deterministic catalog metadata only; it is not schema or instance test execution. The package documentation is the public API surface for this evaluation. Parser, validator, and generator APIs are intentionally outside the contract.

Record the result with `report.md`. Keep evidence to short observations and classify each problem as documentation, public API, CLI, or environment. Keep boundary evidence separate from those outcomes.
