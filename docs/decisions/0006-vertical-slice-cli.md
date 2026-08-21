# 0006: Vertical-slice CLI contract

Status: accepted

## Decision

The first product command is one binary, `goxsd9`, with these canonical forms:

```text
goxsd9 parse SCHEMA
goxsd9 validate SCHEMA INSTANCE
goxsd9 generate --package NAME [--output FILE|-] [--force] SCHEMA
```

`SCHEMA` and `INSTANCE` are local paths or `-` for stdin. The first slice also
accepts `--diagnostics human|json` and the canonical `--schema-root DIR` in the
positions shown below. Human diagnostics are the default. `SCHEMA` is the only
input to `parse` and `generate`; `validate` requires two distinct input
streams, so `goxsd9 validate - -` is a usage error.

`--force` applies only to an explicit file destination; stdout has no existing
destination to overwrite.

The commands are a future implementation contract, not an assertion that the
binary exists today. Success output is deliberately narrow:

- `parse` writes exactly `documents=N components=M` and a newline to stdout.
- `validate` writes nothing to stdout on success.
- `generate` writes complete formatted Go to stdout when `--output` is absent
  or `--output -` is used. An explicit file destination writes no status text.

Stdout never carries diagnostics, progress, or success text beyond the command
data above.

## CLI-owned input and resolution

The CLI alone interprets operands, opens local files, and constructs the root
`ResolvedSource`. The library continues to receive opaque source identities,
contexts, namespace URNs, and lexical schema locations. Its resolver calls stay
sequential, and the CLI resolver passes `namespaceURN` and the exact lexical
`schemaLocation` unchanged across the library boundary. The private context
may carry only the importing document's canonical base path.

`--schema-root DIR` is the canonical spelling and applies to the schema graph.
For a filesystem schema operand it defaults to the canonical parent directory
of the schema file. For a schema operand of `-`, it is required. The CLI
canonicalizes the root and every graph file, evaluates symlinks, and requires
each resolved graph file to remain beneath the canonical root. A missing root,
escaping path, missing file, or unreadable file is a resolution failure.

Relative `include` and `import` locations are resolved from the canonical
directory of the importing document, not from the root or the invocation
directory. A schema read from stdin uses the canonical schema root as its
private base directory. Local filesystem paths and local `file:` locations are accepted.
HTTP, HTTPS, other network locations, namespace-only lookup, environment
expansion, catalogs, and other search paths are rejected in this slice. An
`include` without `schemaLocation` is invalid schema representation; an
`import` without `schemaLocation` is a resolution failure because this CLI has
no namespace-only lookup policy. `xsi:schemaLocation` and
`xsi:noNamespaceSchemaLocation` hints in an instance are rejected rather than
silently used.

The W3C XSD 1.0 and 1.1 composition clauses define schema documents and the
include/import relationships, while leaving document location and retrieval
to processor/application policy: [XSD 1.0 §4.2](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#composition),
[XSD 1.1 §4.2](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#composition),
[XSD 1.0 include/import](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#src-include),
[XSD 1.0 import](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#src-import),
[XSD 1.1 include](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#src-include),
and [XSD 1.1 import](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#src-import).
This local-only policy is narrower than a Web-aware processor and makes no
conformance claim. XML Base is not interpreted by this slice; any packet that
needs base-dependent composition must report explicit unsupported behavior and
provide the relevant specification reference.

## Source identities and language policy

The CLI chooses identities; the parser treats them as opaque:

| Source | `SourceID` |
| --- | --- |
| Filesystem schema source | Canonical slash-separated path relative to the canonical schema root, such as `root.xsd` or `types/base.xsd` |
| Schema stdin | Reserved string `<stdin>` |
| Filesystem instance source | Canonical slash-separated path relative to the invocation working directory |
| Instance stdin | Reserved string `<stdin>` |

Canonicalization happens before identity derivation. The same canonical
resource receives the same identity within one invocation. An identity is never
the target namespace, `schema/@version`, XML declaration version, or generated
Go name; it is also not emitted as an absolute file URI. `SourceID` remains an
opaque library value even though this CLI gives it a reproducible spelling.

The CLI always uses the repository `Compatibility`/default parser pipeline.
It exposes no language-edition flag, and `schema/@version` never selects a
policy. This preserves the accepted boundary in
[0004-xsd-language-policy.md](0004-xsd-language-policy.md).

## Finite offline limits

These are first-slice policy limits, not XSD conformance limits. The CLI does
not expose an unlimited sentinel or a flag that raises them. Any internal
limit configuration with a zero or negative value is a usage/configuration
failure.

| Resource | Limit |
| --- | ---: |
| Bytes in one schema source, including root and resolver sources | 16 MiB |
| Total schema bytes consumed for one invocation | 64 MiB |
| Unique schema document identities; resolver calls | 256 each |
| Schema reference edges | 1024, when enforceable without duplicating parser state |
| Instance bytes | 16 MiB |
| Generated output bytes | 16 MiB |

Repeated or cyclic identities do not permit unbounded discovery. The edge
bound must not be silently treated as unlimited: if a packet cannot account for
it through the existing parser/resolver boundary without a second graph state,
that enforcement gap is explicit and separately accepted before claiming the
limit. A limit breach is a resource/I/O processing failure, never invalid XML
or unsupported XSD behavior. Limit readers stop before producing partial command
data; a breach is not converted to EOF.

## Diagnostics and statuses

Diagnostics are emitted in processing order: schema discovery order, resolver
call order, and then the command's validation or generation order. No
observable diagnostic, JSON field, or related-location list is produced by
map iteration.

Human mode writes one deterministic line per diagnostic to stderr in this
shape, with optional fields in the fixed order shown:

```text
<stage> <class> <source:line:column|<unknown>> <code>: <message> [kind=...] [related=...] [feature=...] [spec_ref=...]
```

`class` uses the existing `FailureClass` values `invalid`, `unsupported`,
`resolution`, and `internal`. A missing `Loc` renders as `<unknown>`.
Resource and destination failures are CLI-owned processing diagnostics and add
a structural `kind=resource` or `kind=output`; they are not relabeled as
invalid or unsupported. For a CLI-owned diagnostic with no library
`FailureClass` (including usage), human mode renders `class=<none>` and JSON
uses `class: null`. Causes remain in the Go error chain at library boundaries,
while displayed messages and machine fields stay stable.

`--diagnostics json` writes one ordered JSON envelope to stderr and no human
text. Its stable shape is:

```json
{
  "format": "goxsd9-diagnostics/v1",
  "command": "validate",
  "stage": "validate",
  "exit_status": 1,
  "diagnostics": [
    {
      "class": "invalid",
      "kind": "processing",
      "code": "...",
      "message": "...",
      "location": {"source": "examples/invalid.xml", "line": 3, "column": 5},
      "related": [],
      "feature": "",
      "spec_ref": ""
    }
  ]
}
```

The envelope and diagnostic arrays preserve processing order. Unknown
locations use `source: "<unknown>"` and zero line/column. `related`, `feature`,
and `spec_ref` are present as empty values when absent. Consumers use these
fields, not rendered text or serialized causes. A successful command emits no
diagnostic envelope; its stdout remains the only command data.

| Outcome | Status | Stdout | Stderr and identity policy |
| --- | ---: | --- | --- |
| Success | 0 | Command data only | No diagnostics |
| Usage/configuration | 2 | Empty | Deterministic usage diagnostic; no source `Loc` when parsing arguments failed |
| `FailureInvalid` | 1 | Empty | Located schema/instance diagnostics when available; otherwise `<unknown>` |
| `FailureUnsupported` | 1 | Empty | Feature, related locations, and `spec_ref` when registered |
| `FailureResolution` | 1 | Empty | Source acquisition/location cause with the best available location |
| `FailureInternal` | 1 | Empty | Invariant diagnostic; preserve the underlying cause internally |
| CLI resource/output processing | 1 | Empty, except a pipe may already have received bytes | `kind=resource` or `kind=output`; never a successful status |

All status-1 paths leave no schema result available to later stages. A
malformed command, missing required schema root for stdin, both validation
operands set to `-`, invalid package name, incompatible destination flags, or
non-positive configured limit uses status 2. XML contents, unsupported XSD
features, source failures, limit breaches, and output failures use status 1.

## Ordering and output transactions

Every command parses the schema before consuming an instance or mutating a
generation destination. `validate` opens and consumes the instance only after
schema parsing succeeds. `generate` builds and formats the complete Go output
before writing either stdout or a file. A schema is never returned after an
error-level diagnostic.

For an explicit `--output FILE`, generation refuses an existing destination
unless `--force` is supplied. It does not create parent directories. It writes
to a temporary file in the destination's directory, checks the write and close,
and atomically renames that file into place. Any parse, unsupported-feature,
formatting, size, write, close, or rename failure leaves the destination
unchanged. Stdout is buffered before its first write, but a pipe cannot roll
back bytes after a downstream failure; callers needing atomic delivery should
use an explicit file.

## Contract examples

The examples are normative command shapes for the future CLI. The counts and
generated declarations are illustrative; they do not claim that these commands
exist in the current tree.

Single-file parse; the schema-root default is the canonical parent of the file:

```console
$ goxsd9 parse examples/root.xsd
documents=1 components=2
```

Parse a graph whose relative references are under `examples/`:

```console
$ goxsd9 parse --schema-root examples examples/root.xsd
documents=3 components=7
```

Validate a supported scalar instance. Success has no stdout:

```console
$ goxsd9 validate examples/root.xsd examples/valid.xml
$
```

An invalid scalar instance has no stdout and reports its located diagnostic on
stderr; JSON mode changes only the diagnostic representation:

```console
$ goxsd9 validate --diagnostics human examples/root.xsd examples/invalid.xml
validate invalid examples/invalid.xml:3:5 <stable-code>: value is not valid for the declared integer
$ goxsd9 validate --diagnostics json examples/root.xsd examples/invalid.xml
# ordered JSON envelope on stderr
```

Generate complete Go on stdout or atomically deliver it to a destination:

```console
$ goxsd9 generate --package sample examples/root.xsd
package sample

type Root int
$ goxsd9 generate --package sample --output generated.go --force examples/root.xsd
$
```

Schema stdin requires an explicit root for referenced local files. Only one
input stream may be stdin, and the validation instance remains a separate
operand:

```console
$ cat examples/root.xsd | goxsd9 parse --schema-root examples -
documents=3 components=7
$ cat examples/root.xsd | goxsd9 validate --schema-root examples - examples/valid.xml
$ goxsd9 validate - -
```

The last command exits 2 and emits a usage diagnostic. These examples prove
that path interpretation belongs to the CLI while `ParseSchema` and `Resolver`
retain their current opaque, sequential boundary.

## Repository and specification evidence

The current library and product invariants remain canonical in
[README.md](../../README.md), [ARCHITECTURE.md](../../ARCHITECTURE.md),
[PLAN.md](../../PLAN.md), [0004-xsd-language-policy.md](0004-xsd-language-policy.md),
and [0005-codegen-naming.md](0005-codegen-naming.md). This decision records
only the CLI boundary choices and does not duplicate those documents.

XSD 1.0 and 1.1 define schema-document composition and include/import
relationships in §4.2 ([XSD 1.0](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#composition),
[XSD 1.1](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#composition)),
while their §4.3 access rules leave document location and retrieval to
processor/application policy. The first slice therefore chooses bounded local
retrieval while preserving the normative graph relationships and makes no W3C
conformance claim.

Current gaps and non-goals are intentional: this file defines no command
implementation, does not broaden the schema or scalar validation slices, does
not expose private syntax/validator/naming representations, and does not claim
W3C conformance. A future CLI may widen local resolution only through a new
decision that preserves opaque library inputs, deterministic identities, finite
limits, located causes, and atomic explicit-file output.
