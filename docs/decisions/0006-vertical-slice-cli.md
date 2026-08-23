# 0006: Vertical-slice CLI contract

Status: accepted

## Decision

The first product CLI is one binary, `goxsd9`, with these canonical forms:

```text
goxsd9 parse [--schema-root DIR] [--diagnostics human|json] SCHEMA
goxsd9 validate [--schema-root DIR] [--diagnostics human|json] SCHEMA INSTANCE
goxsd9 generate [--schema-root DIR] [--diagnostics human|json] --package NAME [--output FILE|-] [--force] SCHEMA
```

The `parse` form is implemented by `cmd/goxsd9` in the first slice;
`validate` and `generate` remain future commands. Flags occur before operands,
each option is supplied at most once, and
unknown options or extra operands are usage errors. `parse` and `generate`
take exactly one `SCHEMA`; `validate` takes exactly one `SCHEMA` and one
`INSTANCE`. Human diagnostics are the default. `--force` is valid only with an
explicit file destination, not with omitted `--output` or `--output -`.

An operand `-` means stdin. At most one input operand may use stdin, so
`goxsd9 validate - -` is a status-2 usage error. `--schema-root` applies to
the schema graph, is required when `SCHEMA` is `-`, and otherwise defaults to
the canonical parent directory of a file `SCHEMA`. `--output` omitted or set
to `-` writes generated Go to stdout; a file destination writes no status text.

Success output is narrow and exact: `parse` writes only
`documents=N components=M\n`; `validate` is silent on stdout and stderr; and
`generate` writes the complete formatted Go source to its selected destination.
Diagnostics never share stdout with command data.

## Boundary and offline resolution

The CLI opens schema and instance operands and constructs the root
`ResolvedSource`. The library retains opaque `SourceID` and context values;
the parser passes `namespaceURN` and the exact lexical `schemaLocation` to
sequential resolver calls without interpreting paths or opening resources.
The CLI resolver carries the importing document's canonical base in private
context and performs the local lookup. It does not normalize the lexical
argument before passing it across the library boundary.

`--schema-root` is a canonical containment policy, not the normative base for
every reference. The CLI canonicalizes the root and each schema file, follows
symlinks for the containment check, and requires every resolved schema file to
remain beneath the canonical root. A symlink escaping that root is rejected.
Relative locations are resolved from the canonical directory of the importing
document; a schema read from stdin uses the canonical schema root as its base.
The root file's default parent is used only when no root is supplied. Missing,
unreadable, non-regular, or escaping sources are resolution failures.

The policy is local and offline: only bare local filesystem paths may be
resolved beneath the root. Any URI scheme, including `file:`, `http:`, and
`https:`, plus catalogs, environment expansion, namespace-only lookup, and
other search paths, is rejected by CLI resolution policy. This does not change
XSD meaning. A valid document that is inaccessible or cannot be retrieved
under this policy is a resolution/resource failure, never invalid XSD.
`include` is distinct from `import`: the library retains include
target-namespace compatibility, including the specification's
no-target-namespace/chameleon rule; current unsupported chameleon behavior
remains an explicit library boundary. Normatively, XSD 1.0/1.1 treat
`schemaLocation` on `include` and `redefine`, and XSD 1.1 `override`, as
dereference expectations rather than namespace lookup hints. This first-slice
CLI boundary covers only the currently handled `include`/`import` references.
The current parser reports `redefine` and `override` as explicit unsupported
behavior before resolution; the CLI preserves that diagnostic and never
relabels either construct as invalid input or a resolution failure. Their
normative dereference behavior remains future library work. For handled
`include`, an absent required location is invalid schema representation and a
failed local dereference is a resolution failure. An `import` may legally omit
`schemaLocation`; that normative namespace-only case is not invalid input, but
this CLI has no namespace-only policy and reports a resolution failure when it
cannot obtain the imported components. When an import location is present, its
namespace and lexical location still pass unchanged to the resolver.

The first slice does not interpret `xml:base`. A schema graph containing
`xml:base` is rejected with an explicit unsupported-resolution diagnostic and
the XML Base specification reference; it is not classified as invalid XSD.
This is a product restriction, not a conformance claim. In an instance,
`xsi:schemaLocation` and `xsi:noNamespaceSchemaLocation` are optional hints.
Because the schema operand is explicit, the CLI never uses them to select or
open another schema. It passes the instance unchanged to current
`ValidateInstance`; current scalar validation reports these attributes as
explicit unsupported behavior. The CLI does not filter them semantically.

## Source identities and language policy

The CLI gives the library deterministic, role-scoped identities while the
library treats them as opaque:

| Source | `SourceID` |
| --- | --- |
| Schema file relative to the canonical schema root | `schema/<relative/path.xsd>` |
| Schema stdin | `schema/stdin` |
| Instance file relative to the invocation directory | `instance/<relative/path.xml>` |
| Instance stdin | `instance/stdin` |

Paths use `/` and are relative; they are not absolute URIs. The `schema/` and
`instance/` roles ensure that schema stdin and instance stdin cannot collide in
diagnostics. An identity is never a target namespace, `schema/@version`, XML
declaration version, generated Go name, or filesystem URI.

The CLI exposes no edition flag and does not choose an edition from
`schema/@version`. The parser applies policy graph-wide: `ParseSchema` selects
graph-wide `Compatibility` for the complete graph, while
`ParseSchemaWithPolicy` applies one validated policy to the complete graph.
Normatively, `schema/@version` is an inert optional label and never selects or
mismatches a policy, as recorded in [0004-xsd-language-policy.md](0004-xsd-language-policy.md).

## Fixed offline limits

The first slice has no unlimited setting and exposes no flag that raises these
limits:

| Resource | Limit |
| --- | ---: |
| One schema source, including root and resolver sources | 16 MiB |
| Total schema bytes in one invocation | 64 MiB |
| Sequential resolver calls | 256 |
| One instance source | 16 MiB |
| Complete generated output | 16 MiB |

There is no separate unique-document or reference-edge limit. Repeated and
cyclic edges consume the sequential resolver-call budget, so call accounting
bounds them without inventing a second graph counter. A limit breach is a
resource or output processing failure, never invalid input or unsupported XSD
behavior. Readers and generators stop before emitting partial command data.

## Diagnostics and statuses

Human mode emits deterministic one-line diagnostics to stderr. The stable
shape is a fixed field order:

```text
command stage=STAGE class=CLASS kind=KIND source_id=SOURCE_ID location=LINE:COLUMN code=CODE [related=...] [feature=...] [spec_ref=...] message
```

The library classes are `invalid`, `unsupported`, `resolution`, and `internal`.
The CLI stage is separate: a schema failure during `validate` has
`command=validate stage=parse`, while an instance failure has
`stage=validate`; generation and destination failures use `generate` and
`output` respectively. Usage/configuration uses `stage=usage` and no library
class. For a CLI-owned diagnostic with no library class, JSON uses `class: null`
and human mode uses `class=-`; its `kind` is `usage`, `path-policy`, `resource`,
`limit`, `output`, or `internal`. All lists and diagnostics retain processing
order; no observable field comes from map iteration.

CLI-owned diagnostics reserve the non-colliding `CLI1xxx` namespace. These
codes are structural, not parsed from messages or library errors:

| Code | Meaning | Kind | Status | Source identity and location |
| --- | --- | --- | ---: | --- |
| `CLI1001` | Usage or configuration | `usage` | 2 | Known operand role ID, otherwise `-`; no `Loc` |
| `CLI1002` | Path policy rejection, including URI schemes or containment | `path-policy` | 1 | Rejected schema/instance role ID; no `Loc` unless one is already available |
| `CLI1003` | Source/resource acquisition or retrieval failure | `resource` | 1 | Subject role ID assigned before opening; no `Loc` for CLI-owned open failures |
| `CLI1004` | Fixed schema, instance, or generated-output limit | `limit` | 1 | Limited role ID; no `Loc` |
| `CLI1005` | Output transaction, overwrite, symlink, write, close, or rename failure | `output` | 1 | `output/<relative/path>`; no `Loc` |
| `CLI1006` | CLI internal invariant failure | `internal` | 1 | Best available role ID, otherwise `-`; no `Loc` |

`CLI1001` uses `stage=usage`, `CLI1005` uses `stage=output`, and the other
codes use the active command phase (`parse` for schema resolution, `validate`
for instance processing, or `output` for generated-output limits). Library
diagnostics retain their existing stable codes and classes, including
`XSD2001`; CLI-owned codes are additional structural identifiers.

JSON mode emits one ordered `goxsd9-diagnostics/v1` envelope to stderr and no
human lines:

```json
{
  "format": "goxsd9-diagnostics/v1",
  "command": "validate",
  "stage": "validate",
  "exit_status": 1,
  "diagnostics": [{
    "class": "invalid",
    "kind": "processing",
    "code": "XSD2001",
    "source_id": "instance/examples/invalid.xml",
    "location": {"line": 3, "column": 5},
    "related": [],
    "feature": "",
    "spec_ref": "",
    "message": "integer lexical form is invalid"
  }]
}
```

`class`, `kind`, `code`, `source_id`, `location`, `related`, `feature`, and
`spec_ref` are structural fields; `message` is optional and never authoritative.
Unknown locations use line and column zero. Related locations carry their own
role-scoped source IDs and locations. Root-open and destination failures may
have no `Loc`, but still carry the schema or instance source ID; a destination
uses `output/<relative/path>` as its diagnostic source ID.
The CLI reads `Diagnostic` accessors and preserved error causes; it never parses
`Diagnostic.Error()` to recover fields.

| Outcome | Status | Stdout | Source identity | Stderr and machine behavior |
| --- | ---: | --- | --- | --- |
| Success | 0 | Command data only | None; no diagnostic is emitted | Empty; no diagnostic envelope |
| Usage or configuration | 2 | Empty | Operand role ID when known; otherwise `-` | Deterministic usage diagnostic, class absent, no location required |
| Invalid, unsupported, resolution, or internal | 1 | Empty | Primary `SourceID` plus related source IDs | Ordered library fields, locations and causes preserved |
| Resource, path-policy, limit, output, or CLI-internal processing | 1 | Empty, except a pipe may already have bytes | Schema/instance or `output/...` role ID, even without `Loc` | Ordered CLI fields with the mapped CLI kind |

`parse` reports schema failures at `stage=parse`. `validate` parses the schema
before opening or consuming the instance; schema failures remain at `stage=parse`
and instance failures use `stage=validate`. `generate` parses before generation
and uses `stage=generate` for the in-memory API. Invalid package/options,
missing schema root for schema stdin, and `validate - -` are status 2. XML
content failures, unsupported features, source failures, limit breaches, and
output failures are status 1.

## Output transactions

Generation builds and formats the complete Go source, including the 16 MiB
check, before writing anywhere. A library failure therefore produces no
partial command data. For an explicit file destination, an existing file is
refused unless `--force` is supplied; parent directories are never created.
The implementation writes in the destination directory, checks every write
and close, and atomically renames the temporary file. The destination remains
unchanged on parse, unsupported-feature, formatting, size, write, close, or
rename failure. A symlink destination is rejected and is never replaced,
including with `--force`. Stdout is buffered until generation completes, but a
broken pipe can still leave a downstream reader with a prefix; callers needing
rollback use an explicit file.

## Contract examples

The parse examples in this section, including the stdin example, document
current executable behavior. The validate and generate examples are future
contract examples. Counts and diagnostic text are illustrative except for the
required success stream shapes.

Single-file schema, with the default root:

```console
$ goxsd9 parse examples/root.xsd
documents=1 components=2
```

Resolved multi-document graph, beneath an explicit containment root:

```console
$ goxsd9 parse --schema-root examples examples/root.xsd
documents=3 components=7
```

Valid scalar instance; success is silent:

```console
$ goxsd9 validate examples/root.xsd examples/valid.xml
$
```

Invalid scalar instance; human and JSON diagnostics both stay on stderr:

```console
$ goxsd9 validate --diagnostics human examples/root.xsd examples/invalid.xml
validate stage=validate class=invalid kind=processing source_id=instance/examples/invalid.xml location=3:5 code=XSD2001: integer lexical form is invalid
$ goxsd9 validate --diagnostics json examples/root.xsd examples/invalid.xml
{"format":"goxsd9-diagnostics/v1", "command":"validate", "stage":"validate", "exit_status":1, "diagnostics":[...]}
```

The current private scalar emitter formats output like this future example:

```console
$ goxsd9 generate --package sample examples/scalars.xsd
package sample

import Runtime "github.com/goxdra/goxsd9"

type Amount struct {
	Value Runtime.StrictDecimal
}

type WholeNumber struct {
	Value Runtime.StrictInteger
}
```

An explicit destination emits no stdout and writes the same complete bytes
atomically:

```console
$ goxsd9 generate --package sample --output generated.go --force examples/scalars.xsd
$
```

Schema stdin requires a root; the two-stdin form is a usage error:

```console
$ cat examples/root.xsd | goxsd9 parse --schema-root examples -
documents=3 components=7
$ goxsd9 validate - -
```

The last command exits 2. These examples show that path interpretation belongs
to the CLI while `ParseSchema` and `Resolver` retain their opaque, sequential
boundary. The generated shape comes from the current private emitter; the public
`generate` command remains future work.

## Bounded follow-up packets

The linked GitHub issues are the canonical work packets; this decision records
only their dependency order and responsibility boundaries:

- [#136 — parse](https://github.com/goxdra/goxsd9/issues/136) (XS) owns the
  shared schema-source boundary and first-slice parse command.
- [#137 — validate](https://github.com/goxdra/goxsd9/issues/137) (S) follows
  #136 and owns the schema-first instance-validation command while preserving
  current scalar validation's explicit unsupported treatment of instance
  attributes, including schema-location hints.
- [#138 — generate](https://github.com/goxdra/goxsd9/issues/138) (M) follows
  #136 and the scalar emitter work. It must first expose a deliberate
  in-memory `GenerateGo`-like API over public inputs, without exposing private
  naming tables or filesystem policy, then own the CLI output transaction.

The linked issues retain the complete scope, dependencies, and acceptance
proofs for each packet.

The current codegen emitter and naming kernel are private. No packet here claims
choice generation, broad validation, network access, catalogs, environment
lookup, XML Base interpretation, strict edition flags, or W3C conformance.

## Repository and specification evidence

This decision changes no library boundary or current architecture. The
[README schema-parsing contract](../../README.md),
[ARCHITECTURE input/resolution rules](../../ARCHITECTURE.md#input-and-resolution),
[0004 language-policy decision](0004-xsd-language-policy.md), and
[0005 naming decision](0005-codegen-naming.md) are the repository evidence.
The pinned [specification manifest](../../specs/manifest.json) remains the
source of edition-specific repository evidence.

Relevant normative references are [XSD 1.0 §4 access and composition](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#layer2),
[XSD 1.0 include](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#src-include),
[XSD 1.0 import](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#src-import),
[XSD 1.0 redefine](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#src-redefine),
and [XSD 1.0 document access](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#composition-instances);
[XSD 1.1 access and composition](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#layer2),
[XSD 1.1 include](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#src-include),
[XSD 1.1 import](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#src-import),
[XSD 1.1 redefine](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#src-redefine),
[XSD 1.1 override](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#src-override),
and [XSD 1.1 document access](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#composition-instances);
and [XML Base](https://www.w3.org/TR/xmlbase/#matching). These links support
the boundary decisions above; they do not turn the first slice into a full
XSD conformance implementation.
