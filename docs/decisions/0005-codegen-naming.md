# 0005: Deterministic Go code-generation naming

Status: accepted

## Decision

Issue [#117](https://github.com/goxdra/goxsd9/issues/117) establishes one
private, deterministic naming kernel before semantic Go emission. It accepts a
caller-supplied package name, derives legal exported identifiers for generated
types, fields, variants, and import aliases, and stores ordered records with
private lookup indexes. The kernel does not reinterpret schema names as Go
package paths or discard the expanded-name and owner identities used for
lookup.

## Normative facts and chosen policy

XSD names are not one global pool. XSD 1.0 [§2.5 Names and Symbol
Spaces](https://www.w3.org/TR/2004/REC-xmlschema-1-20041028/#concepts-nameSymbolSpaces)
defines distinct symbol spaces by component kind within a target namespace,
except that simple and complex type definitions share one. XSD 1.1
[§2.6 Names and Symbol
Spaces](https://www.w3.org/TR/2012/REC-xmlschema11-1-20120405/#concepts-nameSymbolSpaces)
defines the same global distinction and says an expanded name is a namespace
name/local-name pair; local element and attribute declarations are scoped by
their containing complex type rather than the global symbol spaces. Thus an
XSD element and type may share an expanded name, while two components in one
symbol space may not.

Go has a different target. The [identifier
grammar](https://go.dev/ref/spec#Identifiers) permits letters and Unicode
digits, but keywords are reserved by the [keyword
list](https://go.dev/ref/spec#Keywords). Top-level declarations occupy the
package block; an imported package name occupies a file block, and Go forbids
an identifier from being declared in both blocks ([declarations and
scope](https://go.dev/ref/spec#Declarations_and_scope)).

The chosen mapping is therefore intentionally stricter than XSD:

- Every schema component represented by this kernel, regardless of component
  kind or namespace, reserves one package-level type identifier.
- Concrete element and choice variants reserve that same package-level type
  space. They are not scoped only by their owner because future emission puts
  them at package level.
- Local particle fields have one identifier allocator per owner component.
  Different owners may reuse a field identifier; fields in one owner may not.
- Import aliases have one allocator for the naming table's generated file.
  The allocator is shared with package-level type reservations so an alias
  cannot conflict with a package declaration, while fields do not consume
  import reservations. The package clause name is validated independently;
  it is not a declaration in the Go package block.

Each QName remains an expanded `QName`; the namespace affects collision
behavior only through the shared Go reservation scope. Component lookup uses
`ComponentID` (source identity plus declaration ordinal). Local fields and
variants use `(owner ComponentID, lexical path)`. A generated identifier is
never used as schema identity.

## Exact mapping corpus

The following rows are one ordered naming input. Component rows precede
variant rows, and all rows in each request slice are already in documented
schema-discovery or lexical order. The package type allocator is populated
before the import rows.

| Input | Identifier or result |
| --- | --- |
| package `generated_2` | `generated_2` |
| component #1 `{urn:billing}purchase-order` | `PurchaseOrder` |
| component #2 `{urn:shipping}purchase_order` | `PurchaseOrder2` |
| component #3 `{urn:billing}choice-a` | `ChoiceA` |
| field of owner #1, path `/1/2`, `line-item` | `LineItem` |
| field of owner #1, path `/3`, `line_item` | `LineItem2` |
| field of owner #2, path `/1/2`, `line-item` | `LineItem` |
| anonymous field of owner #1, path `/5/1` | `FieldAtP5P1` |
| variant of owner #1, path `/4`, `{urn:one}choice-a` | `ChoiceA2` |
| variant of owner #2, path `/4`, `{urn:two}choice_a` | `ChoiceA3` |
| anonymous variant of owner #2, path `/5/1` | `VariantAtP5P1` |
| import `urn:billing`, alias `billing-client` | `BillingClient` |
| import `urn:shipping`, alias `billing_client` | `BillingClient2` |
| import `urn:collision`, alias `choice-a` | `ChoiceA4` |
| package `type` or `generated-name` | invalid, `GOXSD9026` |

## Normalization and invalid input

For generated names, punctuation and separators are dropped as word
boundaries; lower-to-upper, acronym, and digit-to-letter boundaries are
camel-cased. The first rune is uppercased for an exported type, field, or
alias. A leading digit receives `N` (`123-lives` becomes `N123Lives`). Unicode
letters and digits are retained without Unicode normalization. Unicode simple
folding makes case-only and fold-equivalent inputs collide (`Kelvin` becomes
`Kelvin`); an uncased name is prefixed with `X` for export (`東京` becomes
`X東京`). Invalid UTF-8 is rejected.

Generated `type`, `TYPE`, `int`, `INT`, and other case-fold-equivalent Go
keywords or predeclared identifiers receive `X` (`XType`, `XInt`). A caller
package name is not normalized: it must be valid UTF-8, a Go identifier, not
`_`, a keyword, or a predeclared identifier. Its exact Go spelling is retained.

An empty semantic local name, a namespace with no local name, or a name that
normalizes to no identifier characters (such as `---`) is invalid. Anonymous
names are allowed only for fields and variants, require a non-empty one-based
lexical path, and use the `FieldAtP...` or `VariantAtP...` form. Anonymous
components and imports are invalid. Invalid package input is checked first and
returns a zero naming table. Other invalid input returns no partial table;
diagnostics retain their stable code, classification, cause, and source
`Loc` whenever the input supplies one.

## Collisions, order, and table contract

Allocation is first-come, first-served. A used base receives the smallest
available decimal suffix beginning at `2`: `Foo`, `Foo2`, `Foo3`, and so on.
An independently requested `Foo2` also reserves that spelling. Components
follow `Schema.Components()` order (document discovery, then lexical
declaration order); variants and imports follow their ordered request slices.
No observable result ranges a map. Repeated owner/path or import identity is
invalid, so the stable lookup key cannot be silently replaced.

The ordered records are the source of truth. Private maps only accelerate
identity lookup. Constructors clone lexical paths before storing them;
accessors return copied records, paths, and maps; and `clone` rebuilds indexes
from the ordered records. No accessor or lookup mutates a completed table, and
mutating an accessor result cannot affect later lookups. The implementation is
the private kernel in [`codegen_naming.go`](../../codegen_naming.go), with the
collision and immutability corpus in
[`codegen_naming_test.go`](../../codegen_naming_test.go). The documentation
registry is maintained in [`internal/workflowctl/docs.go`](../../internal/workflowctl/docs.go).

## Non-goals

This decision does not add a public generator API, emit semantic schema types,
build choice or content models, validate XML, resolve imports or package
loading, map namespaces to Go modules, write files, or claim complete XSD
conformance. It does not change the immutable schema model described in
[`ARCHITECTURE.md`](../../ARCHITECTURE.md), the phase outcomes in
[`PLAN.md`](../../PLAN.md), or the pinned specification metadata in
[`specs/manifest.json`](../../specs/manifest.json). Future multi-file
generation must supply an explicit file/import scope; it must not infer one
from map order or use a generated spelling as an expanded-name identity.
