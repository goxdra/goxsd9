package goxsd9

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	invalidCodegenPackageNameCode = "GOXSD9026"
	invalidCodegenNameCode        = "GOXSD9027"
	codegenCollisionExhaustedCode = "GOXSD9028"
)

var (
	errInvalidCodegenPackageName = errors.New("invalid Go code-generation package name")
	errInvalidCodegenName        = errors.New("invalid code-generation name")
	errCodegenCollisionExhausted = errors.New("code-generation collision suffixes exhausted")
)

// codegenNamingInput is the ordered input to the private naming kernel.
// Schema components are obtained from schema.Components; the other request
// slices are already in their documented lexical order.
type codegenNamingInput struct {
	packageName    string
	schema         Schema
	localParticles []codegenLocalParticleRequest
	variants       []codegenVariantRequest
	importAliases  []codegenImportAliasRequest
}

// codegenLocalParticleRequest describes a future local particle field. Its
// identity is the owning component and a one-based lexical path within it.
type codegenLocalParticleRequest struct {
	owner     ComponentID
	path      []uint32
	name      QName
	anonymous bool
}

// codegenVariantRequest reserves a concrete name for a future element or
// choice type switch. It does not describe or emit a schema component.
type codegenVariantRequest struct {
	owner     ComponentID
	path      []uint32
	name      QName
	anonymous bool
}

// codegenImportAliasRequest is an ordered request for an import alias. The
// identity is an opaque stable import identity, normally an import path or
// namespace URI, and alias is its requested lexical package name.
type codegenImportAliasRequest struct {
	identity string
	alias    string
}

type codegenComponentName struct {
	id         ComponentID
	kind       ComponentKind
	name       QName
	identifier string
}

type codegenFieldName struct {
	owner      ComponentID
	path       []uint32
	name       QName
	identifier string
}

type codegenVariantName struct {
	owner      ComponentID
	path       []uint32
	name       QName
	identifier string
}

type codegenImportAliasName struct {
	identity   string
	alias      string
	identifier string
}

type codegenScopedPathKey struct {
	owner ComponentID
	path  string
}

// codegenNaming owns ordered name records. Maps are private indexes only; no
// observable result is produced by traversing one of them.
type codegenNaming struct {
	packageName string
	components  []codegenComponentName
	fields      []codegenFieldName
	variants    []codegenVariantName
	imports     []codegenImportAliasName

	componentByID map[ComponentID]string
	fieldByKey    map[codegenScopedPathKey]string
	variantByKey  map[codegenScopedPathKey]string
	importByID    map[string]string
}

// newCodegenNaming validates the caller package before obtaining schema
// components or allocating any naming output.
func newCodegenNaming(input codegenNamingInput) (codegenNaming, error) {
	return buildCodegenNaming(input)
}

// buildCodegenNaming constructs one complete private naming table.
func buildCodegenNaming(input codegenNamingInput) (codegenNaming, error) {
	if err := validateCodegenPackageName(input.packageName); err != nil {
		return codegenNaming{}, err
	}

	components := input.schema.Components()
	names := codegenNaming{
		packageName:   input.packageName,
		components:    make([]codegenComponentName, 0, len(components)),
		fields:        make([]codegenFieldName, 0, len(input.localParticles)),
		variants:      make([]codegenVariantName, 0, len(input.variants)),
		imports:       make([]codegenImportAliasName, 0, len(input.importAliases)),
		componentByID: make(map[ComponentID]string, len(components)),
		fieldByKey:    make(map[codegenScopedPathKey]string, len(input.localParticles)),
		variantByKey:  make(map[codegenScopedPathKey]string, len(input.variants)),
		importByID:    make(map[string]string, len(input.importAliases)),
	}

	packageAllocator := newCodegenNameAllocator()
	if err := allocateCodegenComponents(&names, components, packageAllocator); err != nil {
		return codegenNaming{}, err
	}
	if err := allocateCodegenFields(&names, input.localParticles); err != nil {
		return codegenNaming{}, err
	}
	if err := allocateCodegenVariants(&names, input.variants, packageAllocator); err != nil {
		return codegenNaming{}, err
	}
	if err := allocateCodegenImports(&names, input.importAliases, packageAllocator); err != nil {
		return codegenNaming{}, err
	}

	return names, nil
}

func allocateCodegenComponents(names *codegenNaming, components []Component, allocator *codegenNameAllocator) error {
	for _, component := range components {
		if _, exists := names.componentByID[component.ID()]; exists {
			return newDiagnostic(
				FailureInternal,
				codegenCollisionExhaustedCode,
				component.Loc(),
				fmt.Sprintf("schema component identity %s is repeated", component.ID().Source()),
				errCodegenCollisionExhausted,
			)
		}
		identifier, err := codegenQNameIdentifier(
			component.Name(),
			codegenNameKindComponent,
			false,
			nil,
			component.Loc(),
		)
		if err != nil {
			return err
		}
		identifier, err = allocator.allocate(identifier)
		if err != nil {
			return newDiagnostic(
				FailureInternal,
				codegenCollisionExhaustedCode,
				component.Loc(),
				fmt.Sprintf("allocate Go name for component %s", component.ID().Source()),
				err,
			)
		}
		names.components = append(names.components, codegenComponentName{
			id:         component.ID(),
			kind:       component.Kind(),
			name:       component.Name(),
			identifier: identifier,
		})
		names.componentByID[component.ID()] = identifier
	}
	return nil
}

func allocateCodegenFields(names *codegenNaming, requests []codegenLocalParticleRequest) error {
	allocators := make(map[ComponentID]*codegenNameAllocator)
	for _, request := range requests {
		if err := validateCodegenScopeRequest(request.owner, request.path, "local particle"); err != nil {
			return err
		}
		key := codegenScopedPathKey{owner: request.owner, path: codegenLexicalPathKey(request.path)}
		if _, exists := names.fieldByKey[key]; exists {
			return newCodegenNameError(
				Loc{},
				"local particle",
				request.name.Local(),
				"the owner and lexical path are repeated",
			)
		}
		identifier, err := codegenQNameIdentifier(
			request.name,
			codegenNameKindField,
			request.anonymous,
			request.path,
			Loc{},
		)
		if err != nil {
			return err
		}
		allocator := allocators[request.owner]
		if allocator == nil {
			allocator = newCodegenNameAllocator()
			allocators[request.owner] = allocator
		}
		identifier, err = allocator.allocate(identifier)
		if err != nil {
			return newDiagnostic(
				FailureInternal,
				codegenCollisionExhaustedCode,
				Loc{},
				"allocate Go name for local particle",
				err,
			)
		}
		names.fields = append(names.fields, codegenFieldName{
			owner: request.owner, path: cloneCodegenPath(request.path), name: request.name, identifier: identifier,
		})
		names.fieldByKey[key] = identifier
	}
	return nil
}

func allocateCodegenVariants(names *codegenNaming, requests []codegenVariantRequest, allocator *codegenNameAllocator) error {
	for _, request := range requests {
		if err := validateCodegenScopeRequest(request.owner, request.path, "variant"); err != nil {
			return err
		}
		key := codegenScopedPathKey{owner: request.owner, path: codegenLexicalPathKey(request.path)}
		if _, exists := names.variantByKey[key]; exists {
			return newCodegenNameError(
				Loc{},
				"variant",
				request.name.Local(),
				"the owner and lexical path are repeated",
			)
		}
		identifier, err := codegenQNameIdentifier(
			request.name,
			codegenNameKindVariant,
			request.anonymous,
			request.path,
			Loc{},
		)
		if err != nil {
			return err
		}
		identifier, err = allocator.allocate(identifier)
		if err != nil {
			return newDiagnostic(
				FailureInternal,
				codegenCollisionExhaustedCode,
				Loc{},
				"allocate Go name for variant",
				err,
			)
		}
		names.variants = append(names.variants, codegenVariantName{
			owner: request.owner, path: cloneCodegenPath(request.path), name: request.name, identifier: identifier,
		})
		names.variantByKey[key] = identifier
	}
	return nil
}

func allocateCodegenImports(names *codegenNaming, requests []codegenImportAliasRequest, allocator *codegenNameAllocator) error {
	for _, request := range requests {
		if err := validateCodegenImportAliasRequest(request); err != nil {
			return err
		}
		if _, exists := names.importByID[request.identity]; exists {
			return newCodegenNameError(
				Loc{},
				"import alias",
				request.alias,
				"the stable import identity is repeated",
			)
		}
		identifier, err := codegenIdentifier(request.alias, codegenNameKindImport, false, nil, Loc{})
		if err != nil {
			return err
		}
		identifier, err = allocator.allocate(identifier)
		if err != nil {
			return newDiagnostic(
				FailureInternal,
				codegenCollisionExhaustedCode,
				Loc{},
				"allocate Go import alias",
				err,
			)
		}
		names.imports = append(names.imports, codegenImportAliasName{
			identity: request.identity, alias: request.alias, identifier: identifier,
		})
		names.importByID[request.identity] = identifier
	}
	return nil
}
func (names codegenNaming) packageIdentifier() string {
	return names.packageName
}

func (names codegenNaming) componentName(id ComponentID) (string, bool) {
	identifier, ok := names.componentByID[id]
	return identifier, ok
}

func (names codegenNaming) fieldName(owner ComponentID, path []uint32) (string, bool) {
	identifier, ok := names.fieldByKey[codegenScopedPathKey{
		owner: owner,
		path:  codegenLexicalPathKey(path),
	}]
	return identifier, ok
}

func (names codegenNaming) variantName(owner ComponentID, path []uint32) (string, bool) {
	identifier, ok := names.variantByKey[codegenScopedPathKey{
		owner: owner,
		path:  codegenLexicalPathKey(path),
	}]
	return identifier, ok
}

func (names codegenNaming) importAlias(identity string) (string, bool) {
	identifier, ok := names.importByID[identity]
	return identifier, ok
}

func (names codegenNaming) componentNames() []codegenComponentName {
	return append([]codegenComponentName(nil), names.components...)
}

func (names codegenNaming) fieldNames() []codegenFieldName {
	fields := make([]codegenFieldName, 0, len(names.fields))
	for _, field := range names.fields {
		field.path = cloneCodegenPath(field.path)
		fields = append(fields, field)
	}
	return fields
}

func (names codegenNaming) variantNames() []codegenVariantName {
	variants := make([]codegenVariantName, 0, len(names.variants))
	for _, variant := range names.variants {
		variant.path = cloneCodegenPath(variant.path)
		variants = append(variants, variant)
	}
	return variants
}

func (names codegenNaming) importAliasNames() []codegenImportAliasName {
	return append([]codegenImportAliasName(nil), names.imports...)
}

// clone returns an independent naming view. Ordered records remain the source
// of truth, and all rebuilt indexes are derived from them in order.
func (names codegenNaming) clone() codegenNaming {
	clone := codegenNaming{
		packageName: names.packageName,
		components:  names.componentNames(),
		fields:      names.fieldNames(),
		variants:    names.variantNames(),
		imports:     names.importAliasNames(),
	}
	clone.componentByID = make(map[ComponentID]string, len(clone.components))
	for _, component := range clone.components {
		clone.componentByID[component.id] = component.identifier
	}
	clone.fieldByKey = make(map[codegenScopedPathKey]string, len(clone.fields))
	for _, field := range clone.fields {
		clone.fieldByKey[codegenScopedPathKey{
			owner: field.owner,
			path:  codegenLexicalPathKey(field.path),
		}] = field.identifier
	}
	clone.variantByKey = make(map[codegenScopedPathKey]string, len(clone.variants))
	for _, variant := range clone.variants {
		clone.variantByKey[codegenScopedPathKey{
			owner: variant.owner,
			path:  codegenLexicalPathKey(variant.path),
		}] = variant.identifier
	}
	clone.importByID = make(map[string]string, len(clone.imports))
	for _, imported := range clone.imports {
		clone.importByID[imported.identity] = imported.identifier
	}
	return clone
}

func (names codegenNaming) componentIdentifiers() map[ComponentID]string {
	result := make(map[ComponentID]string, len(names.components))
	for _, component := range names.components {
		result[component.id] = component.identifier
	}
	return result
}

func (names codegenNaming) fieldIdentifiers() map[codegenScopedPathKey]string {
	result := make(map[codegenScopedPathKey]string, len(names.fields))
	for _, field := range names.fields {
		result[codegenScopedPathKey{
			owner: field.owner,
			path:  codegenLexicalPathKey(field.path),
		}] = field.identifier
	}
	return result
}

func (names codegenNaming) variantIdentifiers() map[codegenScopedPathKey]string {
	result := make(map[codegenScopedPathKey]string, len(names.variants))
	for _, variant := range names.variants {
		result[codegenScopedPathKey{
			owner: variant.owner,
			path:  codegenLexicalPathKey(variant.path),
		}] = variant.identifier
	}
	return result
}

func (names codegenNaming) importIdentifiers() map[string]string {
	result := make(map[string]string, len(names.imports))
	for _, imported := range names.imports {
		result[imported.identity] = imported.identifier
	}
	return result
}

type codegenNameAllocator struct {
	used map[string]struct{}
}

func newCodegenNameAllocator() *codegenNameAllocator {
	return &codegenNameAllocator{used: make(map[string]struct{})}
}

func (allocator *codegenNameAllocator) allocate(base string) (string, error) {
	if _, exists := allocator.used[base]; !exists {
		allocator.used[base] = struct{}{}
		return base, nil
	}
	for suffix := uint64(2); ; suffix++ {
		candidate := base + strconv.FormatUint(suffix, 10)
		if _, exists := allocator.used[candidate]; !exists {
			allocator.used[candidate] = struct{}{}
			return candidate, nil
		}
		if suffix == ^uint64(0) {
			return "", errCodegenCollisionExhausted
		}
	}
}

func validateCodegenPackageName(name string) error {
	if name == "" {
		return newCodegenPackageNameError(name, "the name is empty")
	}
	if name == "_" {
		return newCodegenPackageNameError(name, "the blank identifier is not a package name")
	}
	if !utf8.ValidString(name) {
		return newCodegenPackageNameError(name, "the name is not valid UTF-8")
	}
	if !isGoIdentifier(name) {
		return newCodegenPackageNameError(name, "the name is not a legal Go identifier")
	}
	if isGoKeyword(name) {
		return newCodegenPackageNameError(name, "the name is a Go keyword")
	}
	if isGoPredeclared(name) {
		return newCodegenPackageNameError(name, "the name is a predeclared identifier")
	}
	return nil
}

func newCodegenPackageNameError(name, reason string) error {
	return newDiagnostic(
		FailureInvalid,
		invalidCodegenPackageNameCode,
		Loc{},
		fmt.Sprintf("invalid Go package name %q: %s", name, reason),
		errInvalidCodegenPackageName,
	)
}

func validateCodegenScopeRequest(owner ComponentID, path []uint32, context string) error {
	if owner.Source() == "" || owner.Ordinal() == 0 {
		return newCodegenNameError(Loc{}, context, "", "the owner component identity is empty")
	}
	if len(path) == 0 {
		return newCodegenNameError(Loc{}, context, "", "the lexical path is empty")
	}
	for _, segment := range path {
		if segment == 0 {
			return newCodegenNameError(Loc{}, context, "", "the lexical path is not one-based")
		}
	}
	return nil
}

func validateCodegenImportAliasRequest(request codegenImportAliasRequest) error {
	if request.identity == "" {
		return newCodegenNameError(Loc{}, "import alias", request.alias, "the stable import identity is empty")
	}
	if !utf8.ValidString(request.identity) {
		return newCodegenNameError(Loc{}, "import alias", request.alias, "the stable import identity is not valid UTF-8")
	}
	if request.alias == "" {
		return newCodegenNameError(Loc{}, "import alias", request.alias, "the alias is empty")
	}
	return nil
}

func newCodegenNameError(loc Loc, context, raw, reason string) error {
	return newDiagnostic(
		FailureInvalid,
		invalidCodegenNameCode,
		loc,
		fmt.Sprintf("invalid Go %s name %q: %s", context, raw, reason),
		errInvalidCodegenName,
	)
}

type codegenNameKind uint8

const (
	codegenNameKindComponent codegenNameKind = iota
	codegenNameKindField
	codegenNameKindVariant
	codegenNameKindImport
)

func (kind codegenNameKind) label() string {
	switch kind {
	case codegenNameKindComponent:
		return "component"
	case codegenNameKindField:
		return "field"
	case codegenNameKindVariant:
		return "variant"
	case codegenNameKindImport:
		return "import alias"
	default:
		return "name"
	}
}

func codegenQNameIdentifier(name QName, kind codegenNameKind, anonymous bool, path []uint32, loc Loc) (string, error) {
	if !utf8.ValidString(name.Namespace()) {
		return "", newCodegenNameError(loc, kind.label(), name.Local(), "the expanded name namespace is not valid UTF-8")
	}
	if name.Local() == "" {
		if name.Namespace() != "" {
			return "", newCodegenNameError(loc, kind.label(), name.Local(), "the expanded name has a namespace but no local part")
		}
		return codegenIdentifier("", kind, anonymous, path, loc)
	}
	return codegenIdentifier(name.Local(), kind, false, nil, loc)
}

func codegenIdentifier(raw string, kind codegenNameKind, anonymous bool, path []uint32, loc Loc) (string, error) {
	if raw == "" {
		anonymousName, err := codegenAnonymousName(kind, anonymous, path, loc)
		if err != nil {
			return "", err
		}
		raw = anonymousName
	}
	if !utf8.ValidString(raw) {
		return "", newCodegenNameError(loc, kind.label(), raw, "the name is not valid UTF-8")
	}
	return codegenNormalizedIdentifier(raw, kind, loc)
}

func codegenAnonymousName(kind codegenNameKind, anonymous bool, path []uint32, loc Loc) (string, error) {
	if !anonymous {
		return "", newCodegenNameError(loc, kind.label(), "", "the semantic name is empty")
	}
	if kind == codegenNameKindComponent || kind == codegenNameKindImport {
		return "", newCodegenNameError(loc, kind.label(), "", "anonymous names are not allowed in this scope")
	}
	if len(path) == 0 {
		return "", newCodegenNameError(loc, kind.label(), "", "an anonymous name requires a lexical path")
	}
	for _, segment := range path {
		if segment == 0 {
			return "", newCodegenNameError(loc, kind.label(), "", "an anonymous name requires a one-based lexical path")
		}
	}
	return codegenAnonymousBase(kind, path), nil
}

func codegenNormalizedIdentifier(raw string, kind codegenNameKind, loc Loc) (string, error) {
	words := codegenWords([]rune(raw))
	if len(words) == 0 {
		return "", newCodegenNameError(loc, kind.label(), raw, "the name normalizes to no Go identifier characters")
	}
	var identifier strings.Builder
	for _, word := range words {
		for index, character := range word {
			character = unicode.ToLower(codegenFoldRune(character))
			if index == 0 {
				character = unicode.ToUpper(character)
			}
			identifier.WriteRune(character)
		}
	}
	result := identifier.String()
	if result == "" {
		return "", newCodegenNameError(loc, kind.label(), raw, "the name normalizes to an empty identifier")
	}
	first, _ := utf8.DecodeRuneInString(result)
	if unicode.IsDigit(first) {
		result = "N" + result
	}
	if !isGoIdentifier(result) {
		return "", newCodegenNameError(loc, kind.label(), raw, "the normalized name is not a legal Go identifier")
	}
	first, _ = utf8.DecodeRuneInString(result)
	if !unicode.IsUpper(first) {
		result = "X" + result
	}
	if isCodegenReserved(result) {
		result = "X" + result
	}
	return result, nil
}

func codegenAnonymousBase(kind codegenNameKind, path []uint32) string {
	prefix := "FieldAt"
	if kind == codegenNameKindVariant {
		prefix = "VariantAt"
	}
	var result strings.Builder
	result.WriteString(prefix)
	for _, segment := range path {
		result.WriteByte('P')
		result.WriteString(strconv.FormatUint(uint64(segment), 10))
	}
	return result.String()
}

func codegenWords(runes []rune) [][]rune {
	words := make([][]rune, 0)
	word := make([]rune, 0)
	for index, character := range runes {
		if !isCodegenWordPart(character) {
			if len(word) != 0 {
				words = append(words, word)
				word = nil
			}
			continue
		}
		if len(word) != 0 && codegenCaseBoundary(runes, index) {
			words = append(words, word)
			word = nil
		}
		word = append(word, character)
	}
	if len(word) != 0 {
		words = append(words, word)
	}
	return words
}

func codegenCaseBoundary(runes []rune, index int) bool {
	if index <= 0 || index >= len(runes) {
		return false
	}
	previous := runes[index-1]
	current := runes[index]
	if !isCodegenWordPart(previous) || !isCodegenWordPart(current) {
		return false
	}
	if unicode.IsLower(previous) && unicode.IsUpper(current) {
		return true
	}
	if unicode.IsDigit(previous) && unicode.IsLetter(current) {
		return true
	}
	if !unicode.IsUpper(previous) || !unicode.IsUpper(current) {
		return false
	}
	if index+1 >= len(runes) || !isCodegenWordPart(runes[index+1]) {
		return false
	}
	return unicode.IsLower(runes[index+1])
}

func isCodegenWordPart(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character)
}

func codegenFoldRune(character rune) rune {
	minimum := character
	start := character
	current := character
	for {
		next := unicode.SimpleFold(current)
		if next == start {
			return minimum
		}
		if next < minimum {
			minimum = next
		}
		current = next
	}
}

func isCodegenReserved(identifier string) bool {
	return isGoKeyword(codegenCaseFold(identifier)) || isGoPredeclared(codegenCaseFold(identifier))
}

func codegenCaseFold(value string) string {
	var folded strings.Builder
	for _, character := range value {
		folded.WriteRune(unicode.ToLower(codegenFoldRune(character)))
	}
	return folded.String()
}

func codegenLexicalPathKey(path []uint32) string {
	var key strings.Builder
	key.WriteByte('/')
	for _, segment := range path {
		key.WriteString(strconv.FormatUint(uint64(segment), 10))
		key.WriteByte('/')
	}
	return key.String()
}

func cloneCodegenPath(path []uint32) []uint32 {
	return append([]uint32(nil), path...)
}

func isGoIdentifier(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	runes := []rune(name)
	if !isGoIdentifierStart(runes[0]) {
		return false
	}
	for _, character := range runes[1:] {
		if !isGoIdentifierPart(character) {
			return false
		}
	}
	return true
}

func isGoIdentifierStart(character rune) bool {
	return character == '_' || unicode.IsLetter(character)
}

func isGoIdentifierPart(character rune) bool {
	return isGoIdentifierStart(character) || unicode.IsDigit(character)
}

func isGoKeyword(identifier string) bool {
	switch identifier {
	case "break", "default", "func", "interface", "select",
		"case", "defer", "go", "map", "struct", "chan", "else",
		"goto", "package", "switch", "const", "fallthrough", "if",
		"range", "type", "continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}

func isGoPredeclared(identifier string) bool {
	switch identifier {
	case "any", "bool", "byte", "comparable", "complex64", "complex128",
		"error", "float32", "float64", "int", "int8", "int16", "int32",
		"int64", "iota", "rune", "string", "uint", "uint8", "uint16",
		"uint32", "uint64", "uintptr", "false", "nil", "true", "append",
		"cap", "clear", "close", "complex", "copy", "delete", "imag", "len",
		"make", "max", "min", "new", "panic", "print", "println", "real",
		"recover":
		return true
	default:
		return false
	}
}
