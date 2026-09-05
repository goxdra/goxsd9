package specs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	manifestHTMLRepresentation = "html"
	bootstrapXMLDocumentCode   = "specs.conversion.xml"
	bootstrapXMLCDATAPrefix    = "<![CDATA["
	bootstrapXMLUTF8BOM        = "\xEF\xBB\xBF"
	bootstrapXMLNamespaceURI   = "http://www.w3.org/XML/1998/namespace"
	bootstrapXMLNSNamespaceURI = "http://www.w3.org/2000/xmlns/"
	cdataPrefix                = "<pre><![CDATA["
	cdataSuffix                = "]]></pre>"
)

// IndexEntry identifies a navigable heading or fragment target.
type IndexEntry struct {
	Anchor     string
	Level      int
	Occurrence int
	Title      string
}

// Document is a verified manifest entry after representation conversion.
type Document struct {
	Data  []byte
	Entry Entry
	Index []IndexEntry
}

// IsMarkdown reports whether the converted representation is navigable Markdown.
func (document Document) IsMarkdown() bool {
	return document.Entry.Representation == manifestHTMLRepresentation
}

// Fetch downloads and verifies one manifest entry without converting it.
func Fetch(ctx context.Context, client *http.Client, entry Entry) ([]byte, error) {
	if ctx == nil {
		return nil, corpusError("specs.network.request", entry.ID, entry.URL,
			errors.New("nil request context"))
	}
	if client == nil {
		return nil, corpusError("specs.network.request", entry.ID, entry.URL,
			errors.New("nil HTTP client"))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, entry.URL, nil)
	if err != nil {
		return nil, corpusError("specs.network.request", entry.ID, entry.URL,
			fmt.Errorf("create request: %w", err))
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, corpusError("specs.network.request", entry.ID, entry.URL, err)
	}
	if response == nil || response.Body == nil {
		return nil, corpusError("specs.network.read", entry.ID, entry.URL,
			errors.New("HTTP client returned a response without a body"))
	}
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		causes := make([]error, 0, 2)
		if readErr != nil {
			causes = append(causes, fmt.Errorf("read response body: %w", readErr))
		}
		if closeErr != nil {
			causes = append(causes, fmt.Errorf("close response body: %w", closeErr))
		}
		return nil, corpusError("specs.network.read", entry.ID, entry.URL, errors.Join(causes...))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, corpusError("specs.network.status", entry.ID, entry.URL,
			fmt.Errorf("HTTP status %s", response.Status))
	}
	actual := sha256.Sum256(data)
	actualDigest := hex.EncodeToString(actual[:])
	if !strings.EqualFold(actualDigest, entry.SHA256) {
		return nil, corpusError("specs.provenance.digest", entry.ID, entry.URL,
			fmt.Errorf("SHA-256 mismatch: expected %s, got %s", entry.SHA256, actualDigest))
	}
	return data, nil
}

// Generate downloads, verifies, converts, and validates one manifest entry.
func Generate(ctx context.Context, client *http.Client, entry Entry) (Document, error) {
	raw, err := Fetch(ctx, client, entry)
	if err != nil {
		return Document{}, err
	}
	converted, err := convert(entry, raw)
	if err != nil {
		return Document{}, err
	}
	if entry.Kind == KindBootstrapArtifact {
		if validationErr := validateBootstrapXML(entry, converted); validationErr != nil {
			return Document{}, validationErr
		}
	}
	document := Document{Data: converted, Entry: entry}
	if !document.IsMarkdown() {
		return document, nil
	}
	index, err := renderHTMLForEntry(entry.ID, entry.URL, converted)
	if err != nil {
		return Document{}, err
	}
	document.Data = index.data
	document.Index = index.entries
	return document, nil
}

const (
	bootstrapXMLMaxEntityValueLength = 1 << 20
	bootstrapXMLMaxEntityDepth       = 128
	bootstrapXMLMaxContentGroupDepth = 128
)

var (
	errBootstrapXMLParameterEntityUnsupported = errors.New("bootstrap XML parameter entity expansion is unsupported")
	errBootstrapXMLResourceLimit              = errors.New("bootstrap XML validation exceeds resource limits")
	errBootstrapXMLExternalEntityUnsupported  = errors.New("bootstrap XML external entity resolution is unsupported")
	errBootstrapXMLUnknownEntity              = errors.New("bootstrap XML entity reference is undeclared")
	errBootstrapXMLCyclicEntity               = errors.New("bootstrap XML entity reference is cyclic")
	errBootstrapXMLPredefinedEntityInvalid    = errors.New("bootstrap XML predefined entity declaration is invalid")
)

type bootstrapXMLNamespaceScope struct {
	bindings map[string]string
	parent   *bootstrapXMLNamespaceScope
}

type bootstrapXMLQName struct {
	local  string
	prefix string
	raw    string
}

func bootstrapXMLQNameAt(data []byte, index *int) (bootstrapXMLQName, bool) {
	start := *index
	if !bootstrapXMLName(data, index) {
		return bootstrapXMLQName{}, false
	}
	raw := string(data[start:*index])
	colon := bytes.IndexByte(data[start:*index], ':')
	if colon < 0 {
		return bootstrapXMLQName{local: raw, raw: raw}, true
	}
	if colon == 0 || colon+1 == *index-start || bytes.IndexByte(data[start+colon+1:*index], ':') >= 0 {
		return bootstrapXMLQName{}, false
	}
	prefix := string(data[start : start+colon])
	local := string(data[start+colon+1 : *index])
	if !bootstrapXMLNameValue([]byte(prefix)) || !bootstrapXMLNameValue([]byte(local)) {
		return bootstrapXMLQName{}, false
	}
	return bootstrapXMLQName{local: local, prefix: prefix, raw: raw}, true
}

func bootstrapXMLNamespaceDeclaration(qname bootstrapXMLQName) bool {
	return qname.prefix == "xmlns" || qname.prefix == "" && qname.local == "xmlns"
}

func bootstrapXMLNamespacePrefix(qname bootstrapXMLQName) string {
	if qname.prefix == "" {
		return ""
	}
	return qname.local
}

func bootstrapXMLNamespaceBindingAllowed(prefix, namespace string) bool {
	if bootstrapXMLReservedPrefix(prefix) {
		return prefix == "xml" && namespace == bootstrapXMLNamespaceURI
	}
	if prefix == "xmlns" || namespace == bootstrapXMLNSNamespaceURI ||
		!bootstrapXMLNamespaceURIValue(namespace) {
		return false
	}
	if namespace == bootstrapXMLNamespaceURI {
		return false
	}
	if prefix != "" && namespace == "" {
		return false
	}
	return true
}

func bootstrapXMLQNameNamespace(
	qname bootstrapXMLQName,
	scope *bootstrapXMLNamespaceScope,
	attribute bool,
) (string, bool) {
	if qname.prefix == "" {
		if attribute {
			return "", true
		}
		if namespace, ok := scope.lookup(""); ok {
			return namespace, true
		}
		return "", true
	}
	if strings.EqualFold(qname.prefix, "xml") {
		return bootstrapXMLNamespaceURI, qname.prefix == "xml"
	}
	if qname.prefix == "xmlns" || bootstrapXMLReservedPrefix(qname.prefix) {
		return "", false
	}
	namespace, ok := scope.lookup(qname.prefix)
	if !ok || namespace == "" || namespace == bootstrapXMLNSNamespaceURI || namespace == bootstrapXMLNamespaceURI {
		return "", false
	}
	return namespace, true
}

func bootstrapXMLReservedPrefix(prefix string) bool {
	if len(prefix) < len("xml") {
		return false
	}
	return strings.EqualFold(prefix[:len("xml")], "xml")
}

func bootstrapXMLNamespaceURIValue(namespace string) bool {
	if namespace == "" {
		return true
	}
	for index := 0; index < len(namespace); {
		value, size := utf8.DecodeRuneInString(namespace[index:])
		if value == utf8.RuneError && size == 1 || unicode.IsControl(value) || unicode.IsSpace(value) {
			return false
		}
		if value == '%' {
			if !bootstrapXMLPercentEscape(namespace, index) {
				return false
			}
			index += 3
			continue
		}
		if value < utf8.RuneSelf && !bootstrapXMLNamespaceURICharacter(namespace[index]) {
			return false
		}
		index += size
	}
	return true
}

func bootstrapXMLPercentEscape(namespace string, index int) bool {
	return index+2 < len(namespace) && bootstrapXMLHexDigit(namespace[index+1]) &&
		bootstrapXMLHexDigit(namespace[index+2])
}

func bootstrapXMLNamespaceURICharacter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' ||
		strings.ContainsRune(";/?:@&=+$,#-_.!~*'()[]", rune(value))
}

func bootstrapXMLHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func (scope *bootstrapXMLNamespaceScope) lookup(prefix string) (string, bool) {
	if prefix == "xml" {
		return bootstrapXMLNamespaceURI, true
	}
	for current := scope; current != nil; current = current.parent {
		if namespace, ok := current.bindings[prefix]; ok {
			return namespace, true
		}
	}
	return "", false
}

type bootstrapXMLSource struct {
	data       []byte
	activeAmp  []bool
	numericAmp []bool
	markup     []bool
}

func (source bootstrapXMLSource) ampersandActive(index int) bool {
	if index < 0 || index >= len(source.data) || source.data[index] != '&' {
		return false
	}
	if source.activeAmp == nil {
		return true
	}
	return source.activeAmp[index]
}

func (source bootstrapXMLSource) referenceCandidate(index int) bool {
	if source.ampersandActive(index) {
		return true
	}
	if index < 0 || index >= len(source.numericAmp) || !source.numericAmp[index] {
		return false
	}
	return true
}

func (source bootstrapXMLSource) lessThanMarkup(index int) bool {
	if index < 0 || index >= len(source.data) || source.data[index] != '<' {
		return false
	}
	if source.markup == nil {
		return true
	}
	return source.markup[index]
}

func (source bootstrapXMLSource) slice(start, end int) bootstrapXMLSource {
	if start < 0 || end < start || end > len(source.data) {
		return bootstrapXMLSource{}
	}
	result := bootstrapXMLSource{data: source.data[start:end]}
	if source.activeAmp != nil {
		result.activeAmp = source.activeAmp[start:end]
	}
	if source.numericAmp != nil {
		result.numericAmp = source.numericAmp[start:end]
	}
	if source.markup != nil {
		result.markup = source.markup[start:end]
	}
	return result
}

type bootstrapXMLParseError struct {
	offset  int
	message string
	cause   error
}

func (err *bootstrapXMLParseError) Error() string {
	return err.message
}

func (err *bootstrapXMLParseError) Unwrap() error {
	return err.cause
}

type bootstrapXMLBudget struct {
	used int
}

func (budget *bootstrapXMLBudget) consume(amount int) bool {
	if amount < 0 || amount > bootstrapXMLMaxEntityValueLength-budget.used {
		budget.used = bootstrapXMLMaxEntityValueLength
		return false
	}
	budget.used += amount
	return true
}

type bootstrapXMLParsedAttribute struct {
	qname      bootstrapXMLQName
	value      bootstrapXMLSource
	startIndex int
}

type bootstrapXMLParsedElement struct {
	qname     bootstrapXMLQName
	name      xml.Name
	namespace *bootstrapXMLNamespaceScope
}

type bootstrapXMLDocumentParser struct {
	source      bootstrapXMLSource
	data        []byte
	index       int
	dtd         *bootstrapXMLDTD
	doctypeName string
	stack       []bootstrapXMLParsedElement
	namespace   *bootstrapXMLNamespaceScope
	seenDoctype bool
	seenRoot    bool
	seenToken   bool
	fragment    bool
	entityDepth int
	entityStack []string
	budget      *bootstrapXMLBudget
}

type bootstrapXMLDTD struct {
	entities       []bootstrapXMLEntityDeclaration
	entityIndexes  map[string]int
	externalSubset bool
}

type bootstrapXMLAttValueDefault struct {
	value       string
	entityLimit int
}

func lenBootstrapXMLEntities(dtd *bootstrapXMLDTD) int {
	if dtd == nil {
		return 0
	}
	return len(dtd.entities)
}

func (dtd *bootstrapXMLDTD) entity(name string, limit int) (bootstrapXMLEntityDeclaration, bool) {
	if dtd == nil || limit < 0 {
		return bootstrapXMLEntityDeclaration{}, false
	}
	if limit > len(dtd.entities) {
		limit = len(dtd.entities)
	}
	index, ok := dtd.entityIndexes[name]
	if !ok || index >= limit {
		return bootstrapXMLEntityDeclaration{}, false
	}
	return dtd.entities[index], true
}

func (parser *bootstrapXMLDocumentParser) parse() error { //nolint:gocognit // The ordered document state machine keeps XML phase checks together.
	if parser.budget == nil {
		parser.budget = &bootstrapXMLBudget{}
	}
	if parser.data == nil {
		parser.data = parser.source.data
	}
	if len(parser.data) == 0 {
		if parser.fragment {
			return nil
		}
		return parser.fail("XML document has no root element", nil)
	}
	if !parser.fragment && bytes.HasPrefix(parser.data, []byte(bootstrapXMLUTF8BOM)) {
		parser.index = len(bootstrapXMLUTF8BOM)
		parser.seenToken = false
	}
	for parser.index < len(parser.source.data) {
		if parser.source.data[parser.index] == '<' && parser.source.lessThanMarkup(parser.index) {
			if err := parser.parseMarkup(); err != nil {
				return err
			}
			continue
		}
		if err := parser.parseCharacterData(); err != nil {
			return err
		}
	}
	if parser.fragment {
		if len(parser.stack) != 0 {
			return parser.fail("entity replacement is not a complete content fragment", nil)
		}
		return nil
	}
	if len(parser.stack) != 0 {
		return parser.fail("XML document has an unclosed element", nil)
	}
	if !parser.seenRoot {
		return parser.fail("XML document has no root element", nil)
	}
	return nil
}

func (parser *bootstrapXMLDocumentParser) fail(message string, cause error) error {
	offset := parser.index
	if offset < 0 {
		offset = 0
	}
	if offset > len(parser.data) {
		offset = len(parser.data)
	}
	return &bootstrapXMLParseError{offset: offset, message: message, cause: cause}
}

func (parser *bootstrapXMLDocumentParser) failAt(offset int, message string, cause error) error {
	parser.index = offset
	return parser.fail(message, cause)
}

func (parser *bootstrapXMLDocumentParser) corpusError(entry Entry, err error) error {
	var parseErr *bootstrapXMLParseError
	if !errors.As(err, &parseErr) {
		parseErr = &bootstrapXMLParseError{offset: parser.index, message: "invalid XML document", cause: err}
	}
	offset := parseErr.offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(parser.data) {
		offset = len(parser.data)
	}
	line := 1 + bytes.Count(parser.data[:offset], []byte{'\n'})
	syntaxErr := &xml.SyntaxError{Msg: parseErr.message, Line: line}
	if parseErr.cause == nil {
		return corpusError(bootstrapXMLDocumentCode, entry.ID, entry.URL, syntaxErr)
	}
	return corpusError(bootstrapXMLDocumentCode, entry.ID, entry.URL, errors.Join(syntaxErr, parseErr.cause))
}

func (parser *bootstrapXMLDocumentParser) parseMarkup() error {
	start := parser.index
	if bytes.HasPrefix(parser.source.data[start:], []byte("<!--")) {
		return parser.parseComment()
	}
	if bytes.HasPrefix(parser.source.data[start:], []byte("<?")) {
		return parser.parseProcessingInstruction()
	}
	if bytes.HasPrefix(parser.source.data[start:], []byte("<![CDATA[")) {
		return parser.parseCDATA()
	}
	if bytes.HasPrefix(parser.source.data[start:], []byte("<!DOCTYPE")) {
		if parser.fragment {
			return parser.fail("entity replacement contains a nested directive", nil)
		}
		return parser.parseDoctype()
	}
	if bytes.HasPrefix(parser.source.data[start:], []byte("</")) {
		return parser.parseEndElement()
	}
	if bytes.HasPrefix(parser.source.data[start:], []byte("<!")) {
		return parser.fail("XML document has an invalid directive", nil)
	}
	return parser.parseStartElement()
}

func (parser *bootstrapXMLDocumentParser) parseComment() error {
	start := parser.index
	end := bytes.Index(parser.source.data[start+len("<!--"):], []byte("-->"))
	if end < 0 {
		return parser.fail("XML comment is not terminated", nil)
	}
	end += start + len("<!--")
	comment := parser.source.data[start+len("<!--") : end]
	if bytes.Contains(comment, []byte("--")) || bytes.HasSuffix(comment, []byte{'-'}) ||
		!bootstrapXMLValidCharacters(comment) {
		return parser.fail("XML comment is invalid", nil)
	}
	parser.index = end + len("-->")
	parser.seenToken = true
	return nil
}

func (parser *bootstrapXMLDocumentParser) parseProcessingInstruction() error {
	start := parser.index
	end := bytes.Index(parser.source.data[start+2:], []byte("?>"))
	if end < 0 {
		return parser.fail("XML processing instruction is not terminated", nil)
	}
	end += start + 2
	body := parser.source.data[start+2 : end]
	index := 0
	nameStart := index
	ok := bootstrapXMLName(body, &index)
	if !ok {
		return parser.fail("XML processing instruction target is invalid", nil)
	}
	name := string(body[nameStart:index])
	if strings.EqualFold(name, "xml") {
		if parser.fragment || name != "xml" || parser.seenToken || !bootstrapXMLDeclaration(parser.source.data[start:end+len("?>")]) {
			return parser.fail("XML processing instruction target is invalid", nil)
		}
		parser.index = end + len("?>")
		parser.seenToken = true
		return nil
	}
	if index < len(body) && !bootstrapXMLSpace(body[index]) {
		return parser.fail("XML processing instruction target is not separated", nil)
	}
	if !bootstrapXMLValidCharacters(body[index:]) {
		return parser.fail("XML processing instruction is invalid", nil)
	}
	parser.index = end + len("?>")
	parser.seenToken = true
	return nil
}

func (parser *bootstrapXMLDocumentParser) parseCDATA() error {
	start := parser.index
	end := bytes.Index(parser.source.data[start+len("<![CDATA["):], []byte("]]>"))
	if end < 0 {
		return parser.fail("CDATA section is not terminated", nil)
	}
	end += start + len("<![CDATA[")
	if !bootstrapXMLValidCharacters(parser.source.data[start+len("<![CDATA[") : end]) {
		return parser.fail("CDATA section contains an invalid character", nil)
	}
	if !parser.fragment && len(parser.stack) == 0 {
		return parser.fail("XML document has character data outside the root element", nil)
	}
	parser.index = end + len("]]>")
	parser.seenToken = true
	return nil
}

func (parser *bootstrapXMLDocumentParser) parseCharacterData() error {
	start := parser.index
	for parser.index < len(parser.source.data) {
		if parser.source.data[parser.index] == '<' && parser.source.lessThanMarkup(parser.index) {
			break
		}
		if parser.source.data[parser.index] == '&' && parser.source.referenceCandidate(parser.index) {
			if err := parser.consumeCharacterData(start, parser.index); err != nil {
				return err
			}
			return parser.parseCharacterReferenceOrEntity()
		}
		parser.index++
	}
	return parser.consumeCharacterData(start, parser.index)
}

func (parser *bootstrapXMLDocumentParser) consumeCharacterData(start, end int) error {
	if end <= start {
		return nil
	}
	value := parser.source.data[start:end]
	if bytes.Contains(value, []byte("]]>")) {
		return parser.fail("unescaped ]]> is not allowed in character data", nil)
	}
	if !bootstrapXMLValidCharacters(value) {
		return parser.fail("character data contains an invalid character", nil)
	}
	if parser.fragment || len(parser.stack) != 0 {
		parser.seenToken = true
		return nil
	}
	if !bootstrapXMLLiteralWhitespace(value) {
		return parser.fail("XML document has non-whitespace character data outside the root element", nil)
	}
	parser.seenToken = true
	return nil
}

func (parser *bootstrapXMLDocumentParser) parseCharacterReferenceOrEntity() error {
	start := parser.index
	name, reference, end, kind, ok := parser.sourceReference(parser.index)
	if !ok {
		return parser.fail("XML document has an invalid character reference", nil)
	}
	parser.index = end
	if kind == bootstrapXMLSourceCharacterReference {
		value, ok := bootstrapXMLCharacterReferenceValue(reference[1:])
		if !ok {
			return parser.fail("XML document has an invalid character reference", nil)
		}
		return parser.consumeGeneratedCharacterData(value)
	}
	if kind == bootstrapXMLSourcePredefinedReference {
		return parser.consumeGeneratedCharacterData(name)
	}
	if !parser.fragment && len(parser.stack) == 0 {
		return parser.failAt(start, "XML document has an entity reference outside element content", nil)
	}
	return parser.useEntityContent(name, start)
}

func (parser *bootstrapXMLDocumentParser) consumeGeneratedCharacterData(value string) error {
	if !parser.fragment && len(parser.stack) == 0 {
		return parser.fail("XML document has character data outside the root element", nil)
	}
	if !bootstrapXMLValidCharacters([]byte(value)) {
		return parser.fail("character data contains an invalid character", nil)
	}
	parser.seenToken = true
	return nil
}

type bootstrapXMLSourceReferenceKind uint8

const (
	bootstrapXMLSourceCharacterReference bootstrapXMLSourceReferenceKind = iota
	bootstrapXMLSourcePredefinedReference
	bootstrapXMLSourceEntityReference
)

func (parser *bootstrapXMLDocumentParser) sourceReference(index int) (string, string, int, bootstrapXMLSourceReferenceKind, bool) {
	return bootstrapXMLSourceReferenceAt(parser.source, index)
}

func bootstrapXMLSourceReferenceAt(source bootstrapXMLSource, index int) (string, string, int, bootstrapXMLSourceReferenceKind, bool) { //nolint:gocognit // Character and named reference grammar share one deterministic scanner.
	if index < 0 || index >= len(source.data) || !source.referenceCandidate(index) {
		return "", "", 0, 0, false
	}
	end := index + 1
	if end >= len(source.data) {
		return "", "", 0, 0, false
	}
	if source.data[end] == '#' {
		end++
		if end < len(source.data) && source.data[end] == 'x' {
			end++
		}
		start := end
		base := 10
		if index+2 < len(source.data) && source.data[index+2] == 'x' {
			base = 16
		}
		for end < len(source.data) && bootstrapXMLReferenceDigit(source.data[end], base) {
			end++
		}
		if start == end || end >= len(source.data) || source.data[end] != ';' {
			return "", "", 0, 0, false
		}
		referenceStart := index + 1
		return "", string(source.data[referenceStart:end]), end + 1,
			bootstrapXMLSourceCharacterReference, true
	}
	start := end
	for end < len(source.data) {
		value, size := utf8.DecodeRune(source.data[end:])
		if value == utf8.RuneError && size == 1 || !bootstrapXMLNameChar(value) {
			break
		}
		end += size
	}
	if start == end || end >= len(source.data) || source.data[end] != ';' {
		return "", "", 0, 0, false
	}
	name := string(source.data[start:end])
	if predefined, ok := bootstrapXMLPredefinedEntity(name); ok {
		return predefined, name, end + 1, bootstrapXMLSourcePredefinedReference, true
	}
	return name, name, end + 1, bootstrapXMLSourceEntityReference, true
}

func (parser *bootstrapXMLDocumentParser) parseStartElement() error { //nolint:gocognit // Start-tag lexical states must remain ordered.
	start := parser.index
	parser.index++
	qname, ok := bootstrapXMLQNameAt(parser.source.data, &parser.index)
	if !ok {
		return parser.failAt(start, "XML document has an invalid start element", nil)
	}
	attributes := make([]bootstrapXMLParsedAttribute, 0)
	for {
		spaced := parser.consumeSpace()
		if parser.index >= len(parser.source.data) {
			return parser.fail("XML document has an unterminated start element", nil)
		}
		if parser.source.data[parser.index] == '>' {
			parser.index++
			return parser.finishStartElement(qname, attributes, false)
		}
		if parser.source.data[parser.index] == '/' {
			if parser.index+1 >= len(parser.source.data) || parser.source.data[parser.index+1] != '>' {
				return parser.fail("XML document has an invalid empty start element", nil)
			}
			parser.index += 2
			return parser.finishStartElement(qname, attributes, true)
		}
		if !spaced {
			return parser.fail("XML document has an invalid start element", nil)
		}
		attribute, err := parser.parseAttribute()
		if err != nil {
			return err
		}
		attributes = append(attributes, attribute)
	}
}

func (parser *bootstrapXMLDocumentParser) parseAttribute() (bootstrapXMLParsedAttribute, error) {
	start := parser.index
	qname, ok := bootstrapXMLQNameAt(parser.source.data, &parser.index)
	if !ok {
		return bootstrapXMLParsedAttribute{}, parser.failAt(start, "XML document has an invalid attribute name", nil)
	}
	parser.consumeSpace()
	if parser.index >= len(parser.source.data) || parser.source.data[parser.index] != '=' {
		return bootstrapXMLParsedAttribute{}, parser.fail("XML attribute has no equals sign", nil)
	}
	parser.index++
	parser.consumeSpace()
	if parser.index >= len(parser.source.data) || parser.source.data[parser.index] != '\'' &&
		parser.source.data[parser.index] != '"' {
		return bootstrapXMLParsedAttribute{}, parser.fail("XML attribute value is not quoted", nil)
	}
	quote := parser.source.data[parser.index]
	parser.index++
	valueStart := parser.index
	for parser.index < len(parser.source.data) {
		if parser.source.data[parser.index] == quote {
			value := parser.source.slice(valueStart, parser.index)
			if err := parser.validateAttributeSyntax(value); err != nil {
				return bootstrapXMLParsedAttribute{}, err
			}
			parser.index++
			return bootstrapXMLParsedAttribute{qname: qname, value: value, startIndex: start}, nil
		}
		if parser.source.data[parser.index] == '<' && parser.source.lessThanMarkup(parser.index) {
			return bootstrapXMLParsedAttribute{}, parser.fail("XML attribute value contains '<'", nil)
		}
		parser.index++
	}
	return bootstrapXMLParsedAttribute{}, parser.fail("XML attribute value is not terminated", nil)
}

func (parser *bootstrapXMLDocumentParser) validateAttributeSyntax(source bootstrapXMLSource) error {
	if !bootstrapXMLValidCharacters(source.data) {
		return parser.fail("XML attribute value contains an invalid character", nil)
	}
	for index := 0; index < len(source.data); {
		if source.data[index] != '&' || !source.referenceCandidate(index) {
			index++
			continue
		}
		_, _, end, _, ok := bootstrapXMLSourceReferenceAt(source, index)
		if !ok {
			return parser.fail("XML attribute value has an invalid entity reference", nil)
		}
		index = end
	}
	return nil
}

func (parser *bootstrapXMLDocumentParser) finishStartElement( //nolint:gocognit // Namespace declarations must precede all expanded-name checks.
	qname bootstrapXMLQName,
	attributes []bootstrapXMLParsedAttribute,
	empty bool,
) error {
	namespace := &bootstrapXMLNamespaceScope{
		bindings: make(map[string]string),
		parent:   parser.namespace,
	}
	values := make([]string, len(attributes))
	for index, attribute := range attributes {
		value, err := parser.expandAttributeSource(attribute.value, lenBootstrapXMLEntities(parser.dtd))
		if err != nil {
			return parser.failAt(attribute.startIndex, "XML attribute entity expansion failed", err)
		}
		values[index] = value
	}
	for index, attribute := range attributes {
		if !bootstrapXMLNamespaceDeclaration(attribute.qname) {
			continue
		}
		prefix := bootstrapXMLNamespacePrefix(attribute.qname)
		if _, exists := namespace.bindings[prefix]; exists {
			return parser.fail("duplicate namespace declaration", nil)
		}
		if !bootstrapXMLNamespaceBindingAllowed(prefix, values[index]) {
			return parser.fail(fmt.Sprintf("invalid namespace declaration for prefix %q", prefix), nil)
		}
		namespace.bindings[prefix] = values[index]
	}
	nameSpace, valid := bootstrapXMLQNameNamespace(qname, namespace, false)
	if !valid {
		return parser.fail(fmt.Sprintf("invalid namespace prefix %q on element %q", qname.prefix, qname.raw), nil)
	}
	name := xml.Name{Space: nameSpace, Local: qname.local}
	seen := make(map[xml.Name]struct{}, len(attributes))
	for _, attribute := range attributes {
		if bootstrapXMLNamespaceDeclaration(attribute.qname) {
			continue
		}
		attributeSpace, attributeValid := bootstrapXMLQNameNamespace(attribute.qname, namespace, true)
		if !attributeValid {
			return parser.fail(fmt.Sprintf("invalid namespace prefix %q on attribute %q", attribute.qname.prefix, attribute.qname.raw), nil)
		}
		attributeName := xml.Name{Space: attributeSpace, Local: attribute.qname.local}
		if _, exists := seen[attributeName]; exists {
			return parser.fail(fmt.Sprintf("duplicate XML attribute %q", attribute.qname.raw), nil)
		}
		seen[attributeName] = struct{}{}
	}
	if len(parser.stack) == 0 && !parser.fragment {
		if parser.seenRoot {
			return parser.fail("XML document has more than one root element", nil)
		}
		if parser.doctypeName != "" && parser.doctypeName != qname.raw {
			return parser.fail(fmt.Sprintf("XML document root %q does not match doctype %q", qname.raw, parser.doctypeName), nil)
		}
		parser.seenRoot = true
	}
	parser.seenToken = true
	if empty {
		return nil
	}
	parser.stack = append(parser.stack, bootstrapXMLParsedElement{
		qname:     qname,
		name:      name,
		namespace: namespace,
	})
	parser.namespace = namespace
	return nil
}

func (parser *bootstrapXMLDocumentParser) parseEndElement() error {
	start := parser.index
	parser.index += 2
	qname, ok := bootstrapXMLQNameAt(parser.source.data, &parser.index)
	if !ok {
		return parser.failAt(start, "XML document has an invalid end element", nil)
	}
	parser.consumeSpace()
	if parser.index >= len(parser.source.data) || parser.source.data[parser.index] != '>' {
		return parser.fail("XML end element is not terminated", nil)
	}
	parser.index++
	if len(parser.stack) == 0 {
		return parser.failAt(start, "XML document has an unexpected end element", nil)
	}
	last := parser.stack[len(parser.stack)-1]
	nameSpace, valid := bootstrapXMLQNameNamespace(qname, last.namespace, false)
	endName := xml.Name{Space: nameSpace, Local: qname.local}
	if !valid || qname.raw != last.qname.raw || endName != last.name {
		return parser.failAt(start, fmt.Sprintf("XML document end element %q does not match start element %q", qname.raw, last.qname.raw), nil)
	}
	parser.stack = parser.stack[:len(parser.stack)-1]
	parser.namespace = last.namespace.parent
	parser.seenToken = true
	return nil
}

func (parser *bootstrapXMLDocumentParser) consumeSpace() bool {
	start := parser.index
	for parser.index < len(parser.source.data) && bootstrapXMLSpace(parser.source.data[parser.index]) {
		parser.index++
	}
	return start != parser.index
}

func (parser *bootstrapXMLDocumentParser) expandAttributeSource(source bootstrapXMLSource, entityLimit int) (string, error) { //nolint:gocognit // Attribute provenance and entity context are validated in one pass.
	var expanded strings.Builder
	for index := 0; index < len(source.data); {
		if source.data[index] == '<' && source.lessThanMarkup(index) {
			return "", parser.fail("XML attribute value expands to '<'", nil)
		}
		if source.data[index] == '&' && source.referenceCandidate(index) {
			value, reference, end, kind, ok := bootstrapXMLSourceReferenceAt(source, index)
			if !ok {
				return "", parser.fail("XML attribute value has an invalid entity reference", nil)
			}
			index = end
			switch kind {
			case bootstrapXMLSourceCharacterReference:
				value, ok = bootstrapXMLCharacterReferenceValue(reference[1:])
				if !ok {
					return "", parser.fail("XML attribute value has an invalid character reference", nil)
				}
			case bootstrapXMLSourcePredefinedReference:
				// value is already the predefined replacement text.
			case bootstrapXMLSourceEntityReference:
				var err error
				value, err = parser.expandEntityAttribute(value, entityLimit)
				if err != nil {
					return "", err
				}
			}
			if _, err := expanded.WriteString(value); err != nil {
				return "", parser.fail("XML attribute value expansion failed", err)
			}
			continue
		}
		value, size := utf8.DecodeRune(source.data[index:])
		if value == utf8.RuneError && size == 1 || !bootstrapXMLCharacter(value) {
			return "", parser.fail("XML attribute value contains an invalid character", nil)
		}
		if value == '\r' {
			value = '\n'
			size = 1
		}
		if _, err := expanded.WriteRune(value); err != nil {
			return "", parser.fail("XML attribute value expansion failed", err)
		}
		index += size
	}
	return expanded.String(), nil
}

func (parser *bootstrapXMLDocumentParser) expandEntityAttribute(name string, entityLimit int) (string, error) {
	if parser.dtd == nil {
		return "", parser.fail("XML document has an undeclared entity reference", errBootstrapXMLUnknownEntity)
	}
	declaration, ok := parser.dtd.entity(name, entityLimit)
	if !ok {
		cause := errBootstrapXMLUnknownEntity
		if parser.dtd.externalSubset {
			cause = errBootstrapXMLExternalEntityUnsupported
		}
		return "", parser.fail("XML document has an undeclared entity reference", cause)
	}
	if declaration.external {
		return "", parser.fail("XML document uses an external entity", errBootstrapXMLExternalEntityUnsupported)
	}
	if parser.entityDepth >= bootstrapXMLMaxEntityDepth {
		return "", parser.fail("XML document entity depth exceeds the limit", errBootstrapXMLResourceLimit)
	}
	for _, active := range parser.entityStack {
		if active == name {
			return "", parser.fail("XML document entity reference is cyclic", errBootstrapXMLCyclicEntity)
		}
	}
	if !parser.budget.consume(maxBootstrapXMLBudgetAmount(len(declaration.parts.data))) {
		return "", parser.fail("XML document entity expansion exceeds the limit", errBootstrapXMLResourceLimit)
	}
	parser.entityDepth++
	parser.entityStack = append(parser.entityStack, name)
	value, err := parser.expandAttributeSource(declaration.parts, entityLimit)
	parser.entityStack = parser.entityStack[:len(parser.entityStack)-1]
	parser.entityDepth--
	if err != nil {
		return "", err
	}
	return value, nil
}

func maxBootstrapXMLBudgetAmount(amount int) int {
	if amount < 1 {
		return 1
	}
	return amount
}

func (parser *bootstrapXMLDocumentParser) useEntityContent(name string, referenceStart int) error {
	if parser.dtd == nil {
		return parser.failAt(referenceStart, "XML document has an undeclared entity reference", errBootstrapXMLUnknownEntity)
	}
	declaration, ok := parser.dtd.entity(name, lenBootstrapXMLEntities(parser.dtd))
	if !ok {
		cause := errBootstrapXMLUnknownEntity
		if parser.dtd.externalSubset {
			cause = errBootstrapXMLExternalEntityUnsupported
		}
		return parser.failAt(referenceStart, "XML document has an undeclared entity reference", cause)
	}
	if declaration.external {
		return parser.failAt(referenceStart, "XML document uses an external entity", errBootstrapXMLExternalEntityUnsupported)
	}
	if parser.entityDepth >= bootstrapXMLMaxEntityDepth {
		return parser.failAt(referenceStart, "XML document entity depth exceeds the limit", errBootstrapXMLResourceLimit)
	}
	for _, active := range parser.entityStack {
		if active == name {
			return parser.failAt(referenceStart, "XML document entity reference is cyclic", errBootstrapXMLCyclicEntity)
		}
	}
	if !parser.budget.consume(maxBootstrapXMLBudgetAmount(len(declaration.parts.data))) {
		return parser.failAt(referenceStart, "XML document entity expansion exceeds the limit", errBootstrapXMLResourceLimit)
	}
	child := bootstrapXMLDocumentParser{
		source:      declaration.parts,
		data:        declaration.parts.data,
		dtd:         parser.dtd,
		namespace:   parser.namespace,
		fragment:    true,
		entityDepth: parser.entityDepth + 1,
		budget:      parser.budget,
		entityStack: append(append([]string(nil), parser.entityStack...), name),
	}
	if err := child.parse(); err != nil {
		return parser.failAt(referenceStart, "XML document entity replacement is invalid", err)
	}
	parser.seenToken = true
	return nil
}

func (parser *bootstrapXMLDocumentParser) parseDoctype() error {
	start := parser.index
	end, ok := bootstrapXMLDoctypeEnd(parser.source.data, start)
	if !ok {
		return parser.fail("XML document has an unterminated doctype", nil)
	}
	raw := parser.source.data[start:end]
	name, dtdParser, syntaxOK, syntaxErr := bootstrapXMLParseDoctype(raw)
	if !syntaxOK {
		return parser.failAt(start, "XML document has an invalid directive", syntaxErr)
	}
	if parser.seenDoctype || parser.seenRoot || len(parser.stack) != 0 {
		return parser.failAt(start, "XML document doctype is not in the prolog", nil)
	}
	parser.index = end
	parser.doctypeName = name
	parser.dtd = bootstrapXMLDTDFromParser(dtdParser)
	if err := parser.validateDTDDefaults(dtdParser.attValueDefaults); err != nil {
		return parser.failAt(start, "XML document has an invalid DTD default value", err)
	}
	parser.seenDoctype = true
	parser.seenToken = true
	return nil
}

func bootstrapXMLDoctypeEnd(data []byte, start int) (int, bool) { //nolint:gocognit // Quote, comment, PI, and subset states delimit one directive.
	if start < 0 || start+len("<!DOCTYPE") > len(data) || !bytes.HasPrefix(data[start:], []byte("<!DOCTYPE")) {
		return 0, false
	}
	quote := byte(0)
	internal := false
	for index := start + len("<!DOCTYPE"); index < len(data); index++ {
		value := data[index]
		if quote != 0 {
			if value == quote {
				quote = 0
			}
			continue
		}
		if bytes.HasPrefix(data[index:], []byte("<!--")) {
			commentEnd := bytes.Index(data[index+len("<!--"):], []byte("-->"))
			if commentEnd < 0 {
				return 0, false
			}
			index += len("<!--") + commentEnd + len("-->") - 1
			continue
		}
		if bytes.HasPrefix(data[index:], []byte("<?")) {
			piEnd := bytes.Index(data[index+len("<?"):], []byte("?>"))
			if piEnd < 0 {
				return 0, false
			}
			index += len("<?") + piEnd + len("?>") - 1
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		switch value {
		case '[':
			internal = true
		case ']':
			if internal {
				internal = false
			}
		case '>':
			if !internal {
				return index + 1, true
			}
		}
	}
	return 0, false
}

func bootstrapXMLDTDFromParser(parser *bootstrapXMLDTDParser) *bootstrapXMLDTD {
	dtd := &bootstrapXMLDTD{
		entities:       append([]bootstrapXMLEntityDeclaration(nil), parser.entities...),
		entityIndexes:  make(map[string]int, len(parser.entities)),
		externalSubset: parser.externalSubset,
	}
	for index, declaration := range dtd.entities {
		dtd.entityIndexes[declaration.name] = index
	}
	return dtd
}

func (parser *bootstrapXMLDocumentParser) validateDTDDefaults(defaults []bootstrapXMLAttValueDefault) error {
	for _, declaration := range defaults {
		source := bootstrapXMLSourceFromRaw([]byte(declaration.value))
		if _, err := parser.expandAttributeSource(source, declaration.entityLimit); err != nil {
			return err
		}
	}
	return nil
}

func bootstrapXMLSourceFromRaw(data []byte) bootstrapXMLSource {
	return bootstrapXMLSource{data: append([]byte(nil), data...)}
}

func bootstrapXMLCompileEntityValue(value string) (bootstrapXMLSource, bool) { //nolint:gocognit // EntityValue provenance is classified while preserving lexical order.
	source := bootstrapXMLSource{
		data:       make([]byte, 0, len(value)),
		activeAmp:  make([]bool, 0, len(value)),
		numericAmp: make([]bool, 0, len(value)),
		markup:     make([]bool, 0, len(value)),
	}
	appendBytes := func(data []byte, activeAmp, numericAmp, markup bool) {
		for _, value := range data {
			source.data = append(source.data, value)
			source.activeAmp = append(source.activeAmp, activeAmp && value == '&')
			source.numericAmp = append(source.numericAmp, numericAmp && value == '&')
			source.markup = append(source.markup, markup && value == '<')
		}
	}
	for index := 0; index < len(value); {
		if value[index] == '&' {
			end := strings.IndexByte(value[index+1:], ';')
			if end < 0 {
				return bootstrapXMLSource{}, false
			}
			end += index + 1
			reference := value[index+1 : end]
			if strings.HasPrefix(reference, "#") {
				character, ok := bootstrapXMLCharacterReferenceValue(reference[1:])
				if !ok {
					return bootstrapXMLSource{}, false
				}
				appendBytes([]byte(character), false, character == "&", character == "<")
				index = end + 1
				continue
			}
			if !bootstrapXMLNameValue([]byte(reference)) {
				return bootstrapXMLSource{}, false
			}
			appendBytes([]byte(value[index:end+1]), true, false, false)
			index = end + 1
			continue
		}
		character, size := utf8.DecodeRuneInString(value[index:])
		if character == utf8.RuneError && size == 1 || !bootstrapXMLCharacter(character) {
			return bootstrapXMLSource{}, false
		}
		appendBytes([]byte(value[index:index+size]), false, false, character == '<')
		index += size
	}
	return source, true
}

func bootstrapXMLPredefinedEntityDeclarationValid(name, value string) bool {
	var required rune
	switch name {
	case "lt":
		required = '<'
	case "amp":
		required = '&'
	case "gt":
		required = '>'
	case "apos":
		required = '\''
	case "quot":
		required = '"'
	default:
		return true
	}
	source, ok := bootstrapXMLCompileEntityValue(value)
	if !ok {
		return false
	}
	if name == "lt" || name == "amp" {
		if len(source.data) == 0 || !source.numericAmp[0] {
			return false
		}
		_, reference, end, kind, ok := bootstrapXMLSourceReferenceAt(source, 0)
		if !ok || kind != bootstrapXMLSourceCharacterReference || end != len(source.data) {
			return false
		}
		character, ok := bootstrapXMLCharacterReferenceValue(reference[1:])
		return ok && len([]rune(character)) == 1 && []rune(character)[0] == required
	}
	if len(source.data) != utf8.RuneLen(required) || string(source.data) != string(required) {
		return false
	}
	return true
}

func bootstrapXMLParseDoctype(raw []byte) (string, *bootstrapXMLDTDParser, bool, error) { //nolint:gocognit // DTD prolog phases and their failure causes stay ordered.
	const prefix = "<!DOCTYPE"
	if len(raw) < len(prefix)+2 || !bytes.HasPrefix(raw, []byte(prefix)) || !bytes.HasSuffix(raw, []byte{'>'}) {
		return "", nil, false, nil
	}
	if !bootstrapXMLValidCharacters(raw) {
		return "", nil, false, nil
	}
	parser := &bootstrapXMLDTDParser{
		data:          raw[len("<!") : len(raw)-1],
		entityIndexes: make(map[string]int),
	}
	if !parser.consumeLiteral("DOCTYPE") || !parser.requireSpace() {
		return "", parser, false, parser.failure
	}
	name, ok := parser.parseName()
	if !ok {
		return "", parser, false, parser.failure
	}
	parser.skipSpace()
	if parser.atEnd() {
		if parser.failure != nil {
			return "", parser, false, parser.failure
		}
		return name, parser, true, parser.failure
	}
	if parser.peek() != '[' {
		if !parser.parseExternalID() {
			return "", parser, false, parser.failure
		}
		parser.externalSubset = true
		parser.skipSpace()
		if parser.atEnd() {
			if parser.failure != nil {
				return "", parser, false, parser.failure
			}
			return name, parser, true, parser.failure
		}
	}
	if !parser.parseInternalSubset() || !parser.atEnd() {
		return "", parser, false, parser.failure
	}
	if parser.failure != nil {
		return "", parser, false, parser.failure
	}
	return name, parser, true, parser.failure
}

func validateBootstrapXML(entry Entry, data []byte) error {
	parser := bootstrapXMLDocumentParser{
		data:   data,
		source: bootstrapXMLSourceFromRaw(data),
		budget: &bootstrapXMLBudget{},
	}
	if err := parser.parse(); err != nil {
		return parser.corpusError(entry, err)
	}
	return nil
}

func bootstrapXMLDeclaration(raw []byte) bool {
	const prefix = "<?xml"
	if len(raw) < len(prefix)+2 || !bytes.HasPrefix(raw, []byte(prefix)) || !bytes.HasSuffix(raw, []byte("?>")) {
		return false
	}
	data := raw[len(prefix) : len(raw)-2]
	if len(data) == 0 || !bootstrapXMLSpace(data[0]) {
		return false
	}
	for len(data) > 0 && bootstrapXMLSpace(data[0]) {
		data = data[1:]
	}
	if len(data) == 0 {
		return false
	}
	return bootstrapXMLDeclarationFields(data)
}

func bootstrapXMLDeclarationFields(data []byte) bool {
	index := 0
	field := 0
	encodingSeen := false
	standaloneSeen := false
	for {
		if !bootstrapXMLDeclarationField(data, &index, field, &encodingSeen, &standaloneSeen) {
			return false
		}
		if index == len(data) {
			return true
		}
		field++
	}
}

func bootstrapXMLDeclarationField(data []byte, index *int, field int, encodingSeen, standaloneSeen *bool) bool {
	if field > 0 && !bootstrapXMLConsumeSpace(data, index) {
		return false
	}
	if *index == len(data) {
		return field > 0
	}
	name, ok := bootstrapXMLDeclarationFieldName(data, index)
	if !ok || !bootstrapXMLDeclarationFieldAllowed(name, field, encodingSeen, standaloneSeen) {
		return false
	}
	value, ok := bootstrapXMLDeclarationValue(data, index)
	return ok && bootstrapXMLDeclarationValueAllowed(name, value)
}

func bootstrapXMLDeclarationFieldName(data []byte, index *int) (string, bool) {
	start := *index
	for *index < len(data) && bootstrapXMLDeclarationNameByte(data[*index]) {
		(*index)++
	}
	if start == *index {
		return "", false
	}
	return string(data[start:*index]), true
}

func bootstrapXMLDeclarationFieldAllowed(name string, field int, encodingSeen, standaloneSeen *bool) bool {
	switch field {
	case 0:
		return name == "version"
	case 1:
		if name == "encoding" {
			if *encodingSeen {
				return false
			}
			*encodingSeen = true
			return true
		}
		if name != "standalone" || *standaloneSeen {
			return false
		}
		*standaloneSeen = true
		return true
	case 2:
		if name != "standalone" || *standaloneSeen {
			return false
		}
		*standaloneSeen = true
		return true
	default:
		return false
	}
}

func bootstrapXMLDeclarationValue(data []byte, index *int) (string, bool) {
	bootstrapXMLConsumeSpace(data, index)
	if *index >= len(data) || data[*index] != '=' {
		return "", false
	}
	(*index)++
	bootstrapXMLConsumeSpace(data, index)
	if *index >= len(data) || data[*index] != '\'' && data[*index] != '"' {
		return "", false
	}
	quote := data[*index]
	(*index)++
	valueStart := *index
	for *index < len(data) && data[*index] != quote {
		(*index)++
	}
	if *index >= len(data) {
		return "", false
	}
	value := string(data[valueStart:*index])
	(*index)++
	return value, true
}

func bootstrapXMLDeclarationValueAllowed(name, value string) bool {
	switch name {
	case "version":
		return value == "1.0"
	case "encoding":
		return bootstrapXMLEncodingName(value)
	case "standalone":
		return value == "yes" || value == "no"
	default:
		return false
	}
}

func bootstrapXMLEncodingName(value string) bool {
	if value == "" || !bootstrapXMLEncodingNameStart(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !bootstrapXMLEncodingNameChar(value[index]) {
			return false
		}
	}
	return true
}

func bootstrapXMLEncodingNameStart(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func bootstrapXMLEncodingNameChar(value byte) bool {
	return bootstrapXMLEncodingNameStart(value) || value >= '0' && value <= '9' ||
		value == '.' || value == '_' || value == '-'
}

type bootstrapXMLDTDParser struct {
	data              []byte
	index             int
	entities          []bootstrapXMLEntityDeclaration
	entityIndexes     map[string]int
	attValueDefaults  []bootstrapXMLAttValueDefault
	failure           error
	contentGroupDepth int
	inInternalSubset  bool
	externalSubset    bool
}

type bootstrapXMLEntityDeclaration struct {
	name     string
	value    string
	external bool
	parts    bootstrapXMLSource
}

func bootstrapXMLPredefinedEntity(name string) (string, bool) {
	switch name {
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "amp":
		return "&", true
	case "apos":
		return "'", true
	case "quot":
		return "\"", true
	default:
		return "", false
	}
}

func bootstrapXMLPredefinedEntityName(name string) bool {
	_, ok := bootstrapXMLPredefinedEntity(name)
	return ok
}

func (parser *bootstrapXMLDTDParser) atEnd() bool {
	return parser.index == len(parser.data)
}

func (parser *bootstrapXMLDTDParser) setFailure(cause error) {
	if parser.failure == nil {
		parser.failure = cause
	}
}

func (parser *bootstrapXMLDTDParser) peek() byte {
	if parser.atEnd() {
		return 0
	}
	return parser.data[parser.index]
}

func (parser *bootstrapXMLDTDParser) consume(value byte) bool {
	if parser.peek() != value {
		return false
	}
	parser.index++
	return true
}

func (parser *bootstrapXMLDTDParser) consumeLiteral(value string) bool {
	if !bytes.HasPrefix(parser.data[parser.index:], []byte(value)) {
		return false
	}
	parser.index += len(value)
	return true
}

func (parser *bootstrapXMLDTDParser) consumeKeyword(value string) bool {
	if !bytes.HasPrefix(parser.data[parser.index:], []byte(value)) {
		return false
	}
	end := parser.index + len(value)
	if end < len(parser.data) {
		next, size := utf8.DecodeRune(parser.data[end:])
		if next == utf8.RuneError && size == 1 || bootstrapXMLNameChar(next) {
			return false
		}
	}
	parser.index = end
	return true
}

func (parser *bootstrapXMLDTDParser) skipSpace() bool {
	start := parser.index
	for parser.index < len(parser.data) && bootstrapXMLSpace(parser.data[parser.index]) {
		parser.index++
	}
	return parser.index != start
}

func (parser *bootstrapXMLDTDParser) skipSpaceInDTD() bool {
	spaced := parser.skipSpace()
	if parser.inInternalSubset {
		parser.rejectParameterEntityReference()
	}
	return spaced
}

func (parser *bootstrapXMLDTDParser) requireSpace() bool {
	return parser.skipSpaceInDTD()
}

func (parser *bootstrapXMLDTDParser) parseName() (string, bool) {
	start := parser.index
	if !bootstrapXMLName(parser.data, &parser.index) {
		return "", false
	}
	return string(parser.data[start:parser.index]), true
}

func (parser *bootstrapXMLDTDParser) parseNmtoken() bool {
	start := parser.index
	for parser.index < len(parser.data) {
		value, size := utf8.DecodeRune(parser.data[parser.index:])
		if value == utf8.RuneError && size == 1 || !bootstrapXMLNameChar(value) {
			break
		}
		parser.index += size
	}
	return parser.index != start
}

func (parser *bootstrapXMLDTDParser) parseInternalSubset() bool {
	if !parser.consume('[') {
		return false
	}
	parser.inInternalSubset = true
	for !parser.atEnd() {
		parser.skipSpace()
		if parser.failure != nil {
			return false
		}
		if parser.consume(']') {
			parser.inInternalSubset = false
			parser.skipSpace()
			return true
		}
		if !parser.parseSubsetItem() {
			return false
		}
	}
	return false
}

func (parser *bootstrapXMLDTDParser) parseSubsetItem() bool {
	if bytes.HasPrefix(parser.data[parser.index:], []byte("<!--")) {
		return parser.parseComment()
	}
	if bytes.HasPrefix(parser.data[parser.index:], []byte("<?")) {
		return parser.parsePI()
	}
	if parser.peek() == '%' {
		return parser.parsePEReference()
	}
	if bytes.HasPrefix(parser.data[parser.index:], []byte("<!")) {
		return parser.parseMarkupDeclaration()
	}
	return false
}

func (parser *bootstrapXMLDTDParser) parseComment() bool {
	parser.index += len("<!--")
	end := bytes.Index(parser.data[parser.index:], []byte("-->"))
	if end < 0 {
		return false
	}
	comment := parser.data[parser.index : parser.index+end]
	if bytes.Contains(comment, []byte("--")) || bytes.HasSuffix(comment, []byte{'-'}) ||
		!bootstrapXMLValidCharacters(comment) {
		return false
	}
	parser.index += end + len("-->")
	return true
}

func (parser *bootstrapXMLDTDParser) parsePI() bool {
	parser.index += len("<?")
	name, ok := parser.parseName()
	if !ok || strings.EqualFold(name, "xml") {
		return false
	}
	if parser.consumeLiteral("?>") {
		return true
	}
	if !parser.skipSpace() {
		return false
	}
	end := bytes.Index(parser.data[parser.index:], []byte("?>"))
	if end < 0 || !bootstrapXMLValidCharacters(parser.data[parser.index:parser.index+end]) {
		return false
	}
	parser.index += end + len("?>")
	return true
}

func (parser *bootstrapXMLDTDParser) parsePEReference() bool {
	if !parser.consume('%') {
		return false
	}
	if _, ok := parser.parseName(); !ok {
		return false
	}
	if !parser.consume(';') {
		return false
	}
	parser.setFailure(errBootstrapXMLParameterEntityUnsupported)
	return false
}

func (parser *bootstrapXMLDTDParser) rejectParameterEntityReference() bool {
	if parser.peek() != '%' {
		return false
	}
	index := parser.index + 1
	if !bootstrapXMLName(parser.data, &index) || index >= len(parser.data) || parser.data[index] != ';' {
		return false
	}
	parser.index = index + 1
	parser.setFailure(errBootstrapXMLParameterEntityUnsupported)
	return true
}

func (parser *bootstrapXMLDTDParser) parseMarkupDeclaration() bool {
	if !parser.consumeLiteral("<!") {
		return false
	}
	start := parser.index
	for parser.index < len(parser.data) && parser.data[parser.index] >= 'A' && parser.data[parser.index] <= 'Z' {
		parser.index++
	}
	keyword := string(parser.data[start:parser.index])
	if keyword == "" || !parser.requireSpace() {
		return false
	}
	switch keyword {
	case "ELEMENT":
		return parser.parseElementDeclaration()
	case "ATTLIST":
		return parser.parseAttlistDeclaration()
	case "ENTITY":
		return parser.parseEntityDeclaration()
	case "NOTATION":
		return parser.parseNotationDeclaration()
	default:
		return false
	}
}

func (parser *bootstrapXMLDTDParser) parseElementDeclaration() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	if _, ok := parser.parseName(); !ok || !parser.requireSpace() {
		return false
	}
	if parser.consumeKeyword("EMPTY") || parser.consumeKeyword("ANY") {
		parser.skipSpaceInDTD()
		return parser.consume('>')
	}
	if !parser.parseContentSpec() {
		return false
	}
	parser.skipSpaceInDTD()
	return parser.consume('>')
}

func (parser *bootstrapXMLDTDParser) parseContentSpec() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	if parser.peek() == '(' {
		if parser.isMixedStart() {
			return parser.parseMixed()
		}
		if !parser.parseGroup() {
			return false
		}
		parser.consumeOccurrence()
		return true
	}
	return false
}

func (parser *bootstrapXMLDTDParser) isMixedStart() bool {
	index := parser.index + 1
	for index < len(parser.data) && bootstrapXMLSpace(parser.data[index]) {
		index++
	}
	return bytes.HasPrefix(parser.data[index:], []byte("#PCDATA"))
}

func (parser *bootstrapXMLDTDParser) parseMixed() bool {
	if !parser.consume('(') {
		return false
	}
	parser.skipSpaceInDTD()
	if !parser.consumeKeyword("#PCDATA") {
		return false
	}
	parser.skipSpaceInDTD()
	if parser.consume(')') {
		return true
	}
	return parser.parseMixedNames()
}

func (parser *bootstrapXMLDTDParser) parseMixedNames() bool {
	if !parser.consume('|') {
		return false
	}
	for {
		parser.skipSpaceInDTD()
		if parser.peek() == '%' {
			parser.parsePEReference()
			return false
		}
		if _, ok := parser.parseName(); !ok {
			return false
		}
		parser.skipSpaceInDTD()
		if parser.consume(')') {
			return parser.consume('*')
		}
		if !parser.consume('|') {
			return false
		}
	}
}

func (parser *bootstrapXMLDTDParser) parseGroup() bool {
	if parser.contentGroupDepth >= bootstrapXMLMaxContentGroupDepth {
		parser.setFailure(errBootstrapXMLResourceLimit)
		return false
	}
	parser.contentGroupDepth++
	parsed := parser.parseGroupBody()
	parser.contentGroupDepth--
	return parsed
}

func (parser *bootstrapXMLDTDParser) parseGroupBody() bool {
	if !parser.consume('(') {
		return false
	}
	parser.skipSpaceInDTD()
	if !parser.parseParticle() {
		return false
	}
	parser.skipSpaceInDTD()
	if parser.consume(')') {
		return true
	}
	separator := parser.peek()
	if separator != '|' && separator != ',' {
		return false
	}
	for {
		parser.index++
		parser.skipSpaceInDTD()
		if !parser.parseParticle() {
			return false
		}
		parser.skipSpaceInDTD()
		if parser.consume(')') {
			return true
		}
		if parser.peek() != separator {
			return false
		}
	}
}

func (parser *bootstrapXMLDTDParser) parseParticle() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	if parser.peek() == '(' {
		if parser.isMixedStart() || !parser.parseGroup() {
			return false
		}
		parser.consumeOccurrence()
		return true
	}
	if _, ok := parser.parseName(); !ok {
		return false
	}
	parser.consumeOccurrence()
	return true
}

func (parser *bootstrapXMLDTDParser) consumeOccurrence() {
	if parser.peek() == '?' || parser.peek() == '*' || parser.peek() == '+' {
		parser.index++
	}
}

func (parser *bootstrapXMLDTDParser) parseAttlistDeclaration() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	if _, ok := parser.parseName(); !ok {
		return false
	}
	if parser.consume('>') {
		return true
	}
	if !parser.requireSpace() {
		return false
	}
	for {
		if parser.consume('>') {
			return true
		}
		if !parser.parseAttlistAttribute() {
			return false
		}
		if parser.consume('>') {
			return true
		}
		if !parser.requireSpace() {
			return false
		}
	}
}

func (parser *bootstrapXMLDTDParser) parseAttlistAttribute() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	_, ok := parser.parseName()
	if !ok || !parser.requireSpace() || !parser.parseAttributeType() ||
		!parser.requireSpace() || !parser.parseDefaultDeclaration() {
		return false
	}
	return true
}

func (parser *bootstrapXMLDTDParser) parseAttributeType() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	for _, keyword := range []string{"NMTOKENS", "NMTOKEN", "IDREFS", "IDREF", "ENTITIES", "ENTITY", "CDATA", "ID"} {
		if parser.consumeKeyword(keyword) {
			return true
		}
	}
	if parser.consumeKeyword("NOTATION") {
		if !parser.requireSpace() {
			return false
		}
		return parser.parseNameEnumeration()
	}
	return parser.parseNmtokenEnumeration()
}

func (parser *bootstrapXMLDTDParser) parseNameEnumeration() bool {
	return parser.parseEnumeration(func() bool {
		_, ok := parser.parseName()
		return ok
	})
}

func (parser *bootstrapXMLDTDParser) parseNmtokenEnumeration() bool {
	return parser.parseEnumeration(parser.parseNmtoken)
}

func (parser *bootstrapXMLDTDParser) parseEnumeration(item func() bool) bool {
	if !parser.consume('(') {
		return false
	}
	parser.skipSpaceInDTD()
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	if !item() {
		return false
	}
	for {
		parser.skipSpaceInDTD()
		if !parser.consume('|') {
			break
		}
		parser.skipSpaceInDTD()
		if parser.peek() == '%' {
			parser.parsePEReference()
			return false
		}
		if !item() {
			return false
		}
	}
	parser.skipSpaceInDTD()
	return parser.consume(')')
}

func (parser *bootstrapXMLDTDParser) parseDefaultDeclaration() bool {
	for _, keyword := range []string{"#REQUIRED", "#IMPLIED"} {
		if parser.consumeLiteral(keyword) {
			return true
		}
	}
	if parser.consumeKeyword("#FIXED") {
		return parser.requireSpace() && parser.parseAttValue()
	}
	return parser.parseAttValue()
}

func (parser *bootstrapXMLDTDParser) parseEntityDeclaration() bool {
	if parser.peek() == '%' {
		if parser.rejectParameterEntityReference() {
			return false
		}
	}
	parameter, ok := parser.parseEntityKind()
	if !ok {
		return false
	}
	name, ok := parser.parseName()
	if !ok || !parser.requireSpace() {
		return false
	}
	if parser.peek() == '\'' || parser.peek() == '"' {
		return parser.parseInternalEntityDeclaration(name, parameter)
	}
	return parser.parseExternalEntityDeclaration(name, parameter)
}

func (parser *bootstrapXMLDTDParser) parseInternalEntityDeclaration(name string, parameter bool) bool {
	valueStart := parser.index + 1
	if !parser.parseEntityValue() {
		return false
	}
	valueEnd := parser.index - 1
	parser.skipSpaceInDTD()
	if !parser.consume('>') {
		return false
	}
	if parameter {
		return true
	}
	return parser.declareEntity(name, string(parser.data[valueStart:valueEnd]), false)
}

func (parser *bootstrapXMLDTDParser) parseExternalEntityDeclaration(name string, parameter bool) bool {
	if !parser.parseExternalID() || !parser.finishExternalEntity(parameter) {
		return false
	}
	parser.skipSpaceInDTD()
	if !parser.consume('>') {
		return false
	}
	if parameter {
		return true
	}
	return parser.declareEntity(name, "", true)
}

func (parser *bootstrapXMLDTDParser) declareEntity(name, value string, external bool) bool {
	if parser.entityIndexes == nil {
		parser.entityIndexes = make(map[string]int)
	}
	if bootstrapXMLPredefinedEntityName(name) &&
		(external || !bootstrapXMLPredefinedEntityDeclarationValid(name, value)) {
		parser.setFailure(errBootstrapXMLPredefinedEntityInvalid)
		return false
	}
	if _, exists := parser.entityIndexes[name]; exists {
		return true
	}
	parts := bootstrapXMLSource{}
	if !external {
		var ok bool
		parts, ok = bootstrapXMLCompileEntityValue(value)
		if !ok {
			return false
		}
	}
	parser.entityIndexes[name] = len(parser.entities)
	parser.entities = append(parser.entities, bootstrapXMLEntityDeclaration{
		name:     name,
		value:    value,
		external: external,
		parts:    parts,
	})
	return true
}

func (parser *bootstrapXMLDTDParser) parseEntityKind() (bool, bool) {
	if !parser.consume('%') {
		return false, true
	}
	return true, parser.requireSpace()
}

func (parser *bootstrapXMLDTDParser) finishExternalEntity(parameter bool) bool {
	if parameter || !parser.requireSpace() || !parser.consumeKeyword("NDATA") {
		return parameter || parser.peek() == '>'
	}
	if !parser.requireSpace() {
		return false
	}
	_, ok := parser.parseName()
	return ok
}

func (parser *bootstrapXMLDTDParser) parseNotationDeclaration() bool {
	if parser.peek() == '%' {
		parser.parsePEReference()
		return false
	}
	if _, ok := parser.parseName(); !ok || !parser.requireSpace() {
		return false
	}
	if parser.consumeKeyword("SYSTEM") {
		return parser.requireSpace() && parser.parseSystemLiteral() && parser.finishMarkup()
	}
	if !parser.consumeKeyword("PUBLIC") || !parser.requireSpace() || !parser.parsePublicLiteral() {
		return false
	}
	if parser.requireSpace() && parser.peek() != '>' && !parser.parseSystemLiteral() {
		return false
	}
	return parser.finishMarkup()
}

func (parser *bootstrapXMLDTDParser) parseExternalID() bool {
	if parser.consumeKeyword("SYSTEM") {
		return parser.requireSpace() && parser.parseSystemLiteral()
	}
	if !parser.consumeKeyword("PUBLIC") || !parser.requireSpace() || !parser.parsePublicLiteral() || !parser.requireSpace() {
		return false
	}
	return parser.parseSystemLiteral()
}

func (parser *bootstrapXMLDTDParser) parseSystemLiteral() bool {
	return parser.parseQuoted(func(value byte) bool {
		return bootstrapXMLCharacter(rune(value))
	})
}

func (parser *bootstrapXMLDTDParser) parsePublicLiteral() bool {
	return parser.parseQuoted(func(value byte) bool {
		switch {
		case value == ' ' || value == '\r' || value == '\n':
			return true
		case value >= 'a' && value <= 'z', value >= 'A' && value <= 'Z', value >= '0' && value <= '9':
			return true
		case strings.ContainsRune("-'()+,./:=?;!*#@$_%", rune(value)):
			return true
		default:
			return false
		}
	})
}

func (parser *bootstrapXMLDTDParser) parseQuoted(valid func(byte) bool) bool {
	quote := parser.peek()
	if quote != '\'' && quote != '"' {
		return false
	}
	parser.index++
	for parser.index < len(parser.data) && parser.data[parser.index] != quote {
		if !valid(parser.data[parser.index]) {
			return false
		}
		parser.index++
	}
	return parser.consume(quote)
}

func (parser *bootstrapXMLDTDParser) parseEntityValue() bool {
	quote := parser.peek()
	if quote != '\'' && quote != '"' {
		return false
	}
	parser.index++
	for parser.index < len(parser.data) && parser.data[parser.index] != quote {
		if !parser.parseEntityValueItem() {
			return false
		}
	}
	return parser.consume(quote)
}

func (parser *bootstrapXMLDTDParser) parseEntityValueItem() bool {
	switch parser.peek() {
	case '<':
		parser.index++
		return true
	case '&':
		return parser.parseReference()
	case '%':
		return parser.parsePEReference()
	default:
		value, size := utf8.DecodeRune(parser.data[parser.index:])
		if value == utf8.RuneError && size == 1 || !bootstrapXMLCharacter(value) {
			return false
		}
		parser.index += size
		return true
	}
}

func (parser *bootstrapXMLDTDParser) parseAttValue() bool {
	quote := parser.peek()
	if quote != '\'' && quote != '"' {
		return false
	}
	parser.index++
	valueStart := parser.index
	for parser.index < len(parser.data) && parser.data[parser.index] != quote {
		switch parser.peek() {
		case '<':
			return false
		case '&':
			if !parser.parseAttValueReference() {
				return false
			}
		default:
			value, size := utf8.DecodeRune(parser.data[parser.index:])
			if value == utf8.RuneError && size == 1 || !bootstrapXMLCharacter(value) {
				return false
			}
			parser.index += size
		}
	}
	valueEnd := parser.index
	if !parser.consume(quote) {
		return false
	}
	parser.attValueDefaults = append(parser.attValueDefaults, bootstrapXMLAttValueDefault{
		value:       string(parser.data[valueStart:valueEnd]),
		entityLimit: len(parser.entities),
	})
	return true
}

func (parser *bootstrapXMLDTDParser) parseAttValueReference() bool {
	if !parser.consume('&') {
		return false
	}
	if parser.consume('#') {
		return parser.parseCharacterReference()
	}
	name, ok := parser.parseName()
	if !ok || !parser.consume(';') {
		return false
	}
	if bootstrapXMLPredefinedEntityName(name) {
		return true
	}
	return true
}

func (parser *bootstrapXMLDTDParser) parseReference() bool {
	if !parser.consume('&') {
		return false
	}
	if parser.consume('#') {
		return parser.parseCharacterReference()
	}
	return parser.parseNamedReference()
}

func (parser *bootstrapXMLDTDParser) parseCharacterReference() bool {
	base := 10
	if parser.consume('x') {
		base = 16
	}
	start := parser.index
	for parser.index < len(parser.data) && bootstrapXMLReferenceDigit(parser.data[parser.index], base) {
		parser.index++
	}
	if start == parser.index || !parser.consume(';') {
		return false
	}
	reference := string(parser.data[start : parser.index-1])
	if base == 16 {
		reference = "x" + reference
	}
	_, ok := bootstrapXMLCharacterReferenceValue(reference)
	return ok
}

func bootstrapXMLCharacterReferenceValue(reference string) (string, bool) {
	base := 10
	if strings.HasPrefix(reference, "x") {
		base = 16
		reference = reference[1:]
	}
	if reference == "" {
		return "", false
	}
	for index := 0; index < len(reference); index++ {
		if !bootstrapXMLReferenceDigit(reference[index], base) {
			return "", false
		}
	}
	value, err := strconv.ParseUint(reference, base, 32)
	if err != nil || value > uint64(utf8.MaxRune) || !bootstrapXMLCharacter(rune(value)) {
		return "", false
	}
	return string(rune(value)), true
}

func (parser *bootstrapXMLDTDParser) parseNamedReference() bool {
	if _, ok := parser.parseName(); !ok {
		return false
	}
	return parser.consume(';')
}

func bootstrapXMLReferenceDigit(value byte, base int) bool {
	if value >= '0' && value <= '9' {
		return base == 10 || value <= '9'
	}
	if base != 16 {
		return false
	}
	return value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func (parser *bootstrapXMLDTDParser) finishMarkup() bool {
	parser.skipSpaceInDTD()
	return parser.consume('>')
}

func bootstrapXMLName(body []byte, index *int) bool {
	start := *index
	for *index < len(body) {
		value, size := utf8.DecodeRune(body[*index:])
		if value == utf8.RuneError && size == 1 {
			return false
		}
		if !bootstrapXMLNameChar(value) {
			break
		}
		*index += size
	}
	if start == *index {
		return false
	}
	value, _ := utf8.DecodeRune(body[start:])
	return bootstrapXMLNameStart(value)
}

func bootstrapXMLNameValue(value []byte) bool {
	index := 0
	if !bootstrapXMLName(value, &index) {
		return false
	}
	return index == len(value)
}

func bootstrapXMLDeclarationNameByte(value byte) bool {
	return value >= 'a' && value <= 'z'
}

type bootstrapXMLNameRange struct {
	first rune
	last  rune
}

var bootstrapXMLNameStartRanges = [...]bootstrapXMLNameRange{
	{first: 0xC0, last: 0xD6},
	{first: 0xD8, last: 0xF6},
	{first: 0xF8, last: 0x2FF},
	{first: 0x370, last: 0x37D},
	{first: 0x37F, last: 0x1FFF},
	{first: 0x200C, last: 0x200D},
	{first: 0x2070, last: 0x218F},
	{first: 0x2C00, last: 0x2FEF},
	{first: 0x3001, last: 0xD7FF},
	{first: 0xF900, last: 0xFDCF},
	{first: 0xFDF0, last: 0xFFFD},
	{first: 0x10000, last: 0xEFFFF},
}

func bootstrapXMLNameStart(value rune) bool {
	if value == ':' || value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' {
		return true
	}
	for _, validRange := range bootstrapXMLNameStartRanges {
		if value >= validRange.first && value <= validRange.last {
			return true
		}
	}
	return false
}

func bootstrapXMLNameChar(value rune) bool {
	if bootstrapXMLNameStart(value) {
		return true
	}
	return value == '-' || value == '.' || value >= '0' && value <= '9' ||
		value == 0xB7 || value >= 0x300 && value <= 0x36F || value >= 0x203F && value <= 0x2040
}

func bootstrapXMLValidCharacters(data []byte) bool {
	for len(data) > 0 {
		value, size := utf8.DecodeRune(data)
		if value == utf8.RuneError && size == 1 || !bootstrapXMLCharacter(value) {
			return false
		}
		data = data[size:]
	}
	return true
}

func bootstrapXMLCharacter(value rune) bool {
	return value == 0x09 || value == 0x0A || value == 0x0D || value >= 0x20 && value <= 0xD7FF ||
		value >= 0xE000 && value <= 0xFFFD || value >= 0x10000 && value <= 0x10FFFF
}

func bootstrapXMLConsumeSpace(data []byte, index *int) bool {
	start := *index
	for *index < len(data) && bootstrapXMLSpace(data[*index]) {
		*index++
	}
	return *index != start
}

func bootstrapXMLLiteralWhitespace(data []byte) bool {
	for _, value := range data {
		if !bootstrapXMLSpace(value) {
			return false
		}
	}
	return true
}

func bootstrapXMLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

// Write writes a document and, for Markdown, its compact section index.
func Write(outputDir string, document Document) ([]string, error) {
	if strings.TrimSpace(outputDir) == "" {
		return nil, corpusError("specs.output.path", document.Entry.ID, "",
			errors.New("empty output directory"))
	}
	if !validID(document.Entry.ID) {
		return nil, corpusError("specs.output.path", document.Entry.ID, outputDir,
			errors.New("document ID is not a safe artifact name"))
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, corpusError("specs.output.mkdir", document.Entry.ID, outputDir, err)
	}
	extension := ".xsd"
	if document.IsMarkdown() {
		extension = ".md"
	}
	dataPath := filepath.Join(outputDir, document.Entry.ID+extension)
	artifacts := make([]generatedArtifact, 0, 2)
	artifacts = append(artifacts, generatedArtifact{
		code: "specs.output.write",
		data: document.Data,
		path: dataPath,
	})
	paths := make([]string, 0, 2)
	paths = append(paths, dataPath)
	if !document.IsMarkdown() {
		if err := publishArtifacts(artifacts, document.Entry.ID); err != nil {
			return nil, err
		}
		return paths, nil
	}
	indexPath := filepath.Join(outputDir, document.Entry.ID+".index")
	artifacts = append(artifacts, generatedArtifact{
		code: "specs.output.index",
		data: indexData(document.Entry.ID, document.Index),
		path: indexPath,
	})
	paths = append(paths, indexPath)
	if err := publishArtifacts(artifacts, document.Entry.ID); err != nil {
		return nil, err
	}
	return paths, nil
}

func convert(entry Entry, raw []byte) ([]byte, error) {
	switch entry.Representation {
	case manifestXMLRepresentation:
		return append([]byte(nil), raw...), nil
	case manifestHTMLCDATAPreRepresentation:
		wrapped := raw
		if bytes.HasSuffix(wrapped, []byte("\n")) {
			wrapped = wrapped[:len(wrapped)-1]
		}
		if !bytes.HasPrefix(wrapped, []byte(cdataPrefix)) || !bytes.HasSuffix(wrapped, []byte(cdataSuffix)) {
			return nil, corpusError("specs.conversion.representation", entry.ID, entry.URL,
				fmt.Errorf("%q requires the exact %q prefix and %q suffix", entry.Representation,
					cdataPrefix, cdataSuffix))
		}
		contentEnd := len(wrapped) - len(cdataSuffix)
		return append([]byte(nil), wrapped[len(cdataPrefix):contentEnd]...), nil
	case manifestXSD10DatatypesRepresentation:
		return convertXSD10Datatypes(entry, raw)
	case manifestHTMLRepresentation:
		return append([]byte(nil), raw...), nil
	default:
		return nil, corpusError("specs.conversion.representation", entry.ID, entry.URL,
			fmt.Errorf("unsupported representation %q", entry.Representation))
	}
}

func convertXSD10Datatypes(entry Entry, raw []byte) ([]byte, error) {
	if !isXSD10DatatypesEntry(entry) {
		return nil, xsd10DatatypesRepresentationError(entry,
			fmt.Errorf("%q requires the %q XSD 1.0 bootstrap dependency entry", entry.Representation,
				xsd10DatatypesSchemaID))
	}
	content, err := unwrapXSD10Datatypes(entry, raw)
	if err != nil {
		return nil, err
	}
	declarationStart, declarationEnd, err := xsd10DatatypesDeclarationBounds(entry, content)
	if err != nil {
		return nil, err
	}
	if bootstrapXMLPrologMarker(content[declarationEnd+1:]) {
		return nil, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope has repeated or misplaced prolog markup"))
	}

	converted := make([]byte, 0, len(content))
	converted = append(converted, content[declarationStart:declarationEnd]...)
	converted = append(converted, content[:declarationStart]...)
	converted = append(converted, content[declarationEnd:]...)
	return converted, nil
}

func unwrapXSD10Datatypes(entry Entry, raw []byte) ([]byte, error) {
	wrapped := raw
	if bytes.HasSuffix(wrapped, []byte("\n")) {
		wrapped = wrapped[:len(wrapped)-1]
	}
	if !bytes.HasPrefix(wrapped, []byte(cdataPrefix)) || !bytes.HasSuffix(wrapped, []byte(cdataSuffix)) {
		return nil, xsd10DatatypesRepresentationError(entry,
			fmt.Errorf("%q requires the exact %q prefix and %q suffix", entry.Representation,
				cdataPrefix, cdataSuffix))
	}
	contentEnd := len(wrapped) - len(cdataSuffix)
	return wrapped[len(cdataPrefix):contentEnd], nil
}

func xsd10DatatypesDeclarationBounds(entry Entry, content []byte) (int, int, error) {
	doctypeEnd, err := xsd10DatatypesDoctypeEnd(entry, content)
	if err != nil {
		return 0, 0, err
	}
	if doctypeEnd+2 > len(content) || content[doctypeEnd] != '\n' || content[doctypeEnd+1] != '\n' {
		return 0, 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope has an unexpected separator after its document type declaration"))
	}
	declarationStart := doctypeEnd + 2
	if !bytes.HasPrefix(content[declarationStart:], []byte("<?xml")) {
		return 0, 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope requires its XML declaration after the document type declaration"))
	}
	declarationEnd, ok := bootstrapXMLProcessingInstructionEnd(content, declarationStart)
	if !ok || !bootstrapXMLDeclaration(content[declarationStart:declarationEnd]) {
		return 0, 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope has a malformed XML declaration"))
	}
	if declarationEnd >= len(content) || content[declarationEnd] != '\n' {
		return 0, 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope requires one line break after its XML declaration"))
	}
	if declarationEnd+1 == len(content) {
		return 0, 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope has no schema payload"))
	}
	return declarationStart, declarationEnd, nil
}

func xsd10DatatypesDoctypeEnd(entry Entry, content []byte) (int, error) {
	doctypeEnd, ok := bootstrapXMLDoctypeEnd(content, 0)
	if !ok {
		return 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope has no complete document type declaration"))
	}
	_, dtdParser, syntaxOK, syntaxErr := bootstrapXMLParseDoctype(content[:doctypeEnd])
	if !syntaxOK {
		if syntaxErr == nil {
			syntaxErr = errors.New("XSD 1.0 datatype envelope has an invalid document type declaration")
		}
		return 0, xsd10DatatypesRepresentationError(entry, syntaxErr)
	}
	if dtdParser == nil {
		return 0, xsd10DatatypesRepresentationError(entry,
			errors.New("XSD 1.0 datatype envelope has no document type declaration parser"))
	}
	return doctypeEnd, nil
}

func xsd10DatatypesRepresentationError(entry Entry, err error) error {
	return corpusError("specs.conversion.representation", entry.ID, entry.URL, err)
}

func bootstrapXMLProcessingInstructionEnd(data []byte, start int) (int, bool) {
	if start < 0 || start+2 > len(data) || !bytes.HasPrefix(data[start:], []byte("<?")) {
		return 0, false
	}
	for index := start + 2; index+1 < len(data); index++ {
		if data[index] == '?' && data[index+1] == '>' {
			return index + 2, true
		}
	}
	return 0, false
}

func bootstrapXMLPrologMarker(data []byte) bool {
	for index := 0; index < len(data); {
		next, marker, ok := bootstrapXMLPrologToken(data, index)
		if !ok {
			return false
		}
		if marker {
			return true
		}
		index = next
	}
	return false
}

func bootstrapXMLPrologToken(data []byte, index int) (int, bool, bool) {
	if data[index] != '<' {
		return index + 1, false, true
	}
	if bytes.HasPrefix(data[index:], []byte("<!--")) {
		end, ok := bootstrapXMLDelimitedEnd(data, index+len("<!--"), []byte("-->"))
		return end, false, ok
	}
	if bytes.HasPrefix(data[index:], []byte("<![CDATA[")) {
		end, ok := bootstrapXMLDelimitedEnd(data, index+len("<![CDATA["), []byte("]]>"))
		return end, false, ok
	}
	if bytes.HasPrefix(data[index:], []byte("<?")) {
		return bootstrapXMLPrologProcessingInstruction(data, index)
	}
	if bytes.HasPrefix(data[index:], []byte("<!DOCTYPE")) {
		return index, true, true
	}
	return index + 1, false, true
}

func bootstrapXMLPrologProcessingInstruction(data []byte, index int) (int, bool, bool) {
	end, ok := bootstrapXMLProcessingInstructionEnd(data, index)
	if !ok {
		return 0, false, false
	}
	body := data[index+2 : end-2]
	nameEnd := 0
	if !bootstrapXMLName(body, &nameEnd) {
		return end, false, true
	}
	return end, strings.EqualFold(string(body[:nameEnd]), "xml"), true
}

func bootstrapXMLDelimitedEnd(data []byte, start int, delimiter []byte) (int, bool) {
	for index := start; index+len(delimiter) <= len(data); index++ {
		if bytes.Equal(data[index:index+len(delimiter)], delimiter) {
			return index + len(delimiter), true
		}
	}
	return 0, false
}

type renderedHTML struct {
	data    []byte
	entries []IndexEntry
}

type htmlNode struct {
	attrs    []htmlAttribute
	children []*htmlNode
	name     string
	text     string
}

type htmlAttribute struct {
	name  string
	value string
}

type markdownRenderer struct {
	entries      []IndexEntry
	occurrences  map[string]int
	output       strings.Builder
	blankLine    bool
	endsNewline  bool
	hasOutput    bool
	headingLevel int
	headingTitle string
	sourceURL    string
}

func renderHTML(sourceURL string, data []byte) (renderedHTML, error) {
	return renderHTMLForEntry("", sourceURL, data)
}

func renderHTMLForEntry(id, sourceURL string, data []byte) (renderedHTML, error) {
	root, err := parseHTML(data)
	if err != nil {
		return renderedHTML{}, corpusError("specs.conversion.parse", id, sourceURL, err)
	}
	renderer := markdownRenderer{
		occurrences: make(map[string]int),
		sourceURL:   sourceURL,
	}
	renderer.renderContainer(root)
	output := strings.TrimRight(renderer.output.String(), "\n") + "\n"
	return renderedHTML{data: []byte(output), entries: renderer.entries}, nil
}

func (renderer *markdownRenderer) renderContainer(node *htmlNode) {
	for _, child := range node.children {
		renderer.renderBlock(child)
	}
}

func (renderer *markdownRenderer) renderBlock(node *htmlNode) {
	if renderer.skip(node) {
		return
	}
	if isHeading(node.name) {
		renderer.renderHeading(node)
		return
	}
	switch node.name {
	case "html", "body", "main", "section", "article", "div", "header", "footer":
		renderer.renderContainerNode(node)
	case "p", "address", "caption":
		renderer.renderOwnTargets(node)
		renderer.appendBlock(renderer.renderInline(node, false))
	case "pre":
		renderer.renderAllTargets(node)
		renderer.appendBlock(fencedCode(rawText(node)))
	case "ul", "ol":
		renderer.renderList(node, 0)
	case "dl":
		renderer.renderDefinitionList(node)
	case "table":
		renderer.renderTable(node)
	case "blockquote":
		renderer.renderOwnTargets(node)
		renderer.renderQuote(node)
	case "hr":
		renderer.renderOwnTargets(node)
		renderer.appendBlock("---")
	default:
		renderer.renderGenericBlock(node)
	}
}

func (renderer *markdownRenderer) renderContainerNode(node *htmlNode) {
	renderer.renderOwnTargets(node)
	if hasBlockChild(node) || node.name != "div" {
		renderer.renderContainer(node)
		return
	}
	renderer.appendBlock(renderer.renderInline(node, false))
}

func (renderer *markdownRenderer) renderGenericBlock(node *htmlNode) {
	if !isBlockName(node.name) {
		renderer.appendBlock(renderer.renderInline(node, true))
		return
	}
	renderer.renderOwnTargets(node)
	if hasBlockChild(node) {
		renderer.renderContainer(node)
		return
	}
	renderer.appendBlock(renderer.renderInline(node, false))
}

func (renderer *markdownRenderer) renderHeading(node *htmlNode) {
	title := strings.TrimSpace(plainText(node))
	level := int(node.name[1] - '0')
	renderer.headingLevel = level
	renderer.headingTitle = title
	targets := collectTargets(node)
	renderer.renderCollectedTargets(targets, title, level)
	renderer.startBlock()
	renderer.appendLine(strings.Repeat("#", level) + " " + title)
	renderer.appendLine("")
}

func (renderer *markdownRenderer) renderOwnTargets(node *htmlNode) {
	renderer.renderTargets(node)
}

func (renderer *markdownRenderer) renderTargets(node *htmlNode) {
	targets := collectOwnTargets(node)
	renderer.renderCollectedTargets(targets, renderer.headingTitle, renderer.headingLevel)
}

func (renderer *markdownRenderer) renderAllTargets(node *htmlNode) {
	targets := collectTargets(node)
	renderer.renderCollectedTargets(targets, renderer.headingTitle, renderer.headingLevel)
}

func (renderer *markdownRenderer) renderCollectedTargets(targets []target, title string, level int) {
	if len(targets) == 0 {
		return
	}
	renderer.startBlock()
	var previousNode *htmlNode
	for _, target := range targets {
		if target.node != previousNode {
			renderer.appendLine(target.markup)
			previousNode = target.node
		}
		renderer.addTarget(target.value, title, level)
	}
	renderer.appendLine("")
}

func (renderer *markdownRenderer) renderList(node *htmlNode, depth int) {
	renderer.renderOwnTargets(node)
	text := renderer.listText(node, depth)
	renderer.appendBlock(text)
}

func (renderer *markdownRenderer) listText(node *htmlNode, depth int) string {
	items := make([]string, 0)
	ordered := node.name == "ol"
	itemNumber := 1
	for _, child := range node.children {
		if child.name != "li" {
			continue
		}
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", itemNumber)
			itemNumber++
		}
		text, nested := renderer.listItem(child)
		if text != "" {
			items = append(items, strings.Repeat("  ", depth)+prefix+text)
		}
		for _, list := range nested {
			nestedText := renderer.listText(list, depth+1)
			if nestedText != "" {
				items = append(items, nestedText)
			}
		}
	}
	return strings.Join(items, "\n")
}

func (renderer *markdownRenderer) listItem(node *htmlNode) (string, []*htmlNode) {
	var parts []string
	var nested []*htmlNode
	if targets := renderer.inlineTargets(node); targets != "" {
		parts = append(parts, targets)
	}
	for _, child := range node.children {
		if child.name == "ul" || child.name == "ol" {
			nested = append(nested, child)
			continue
		}
		parts = append(parts, renderer.renderInline(child, true))
	}
	return strings.TrimSpace(strings.Join(parts, " ")), nested
}

func (renderer *markdownRenderer) renderDefinitionList(node *htmlNode) {
	var lines []string
	for _, child := range node.children {
		text := strings.TrimSpace(renderer.renderInline(child, true))
		if text == "" {
			continue
		}
		if child.name == "dt" {
			lines = append(lines, "**"+text+"**")
			continue
		}
		lines = append(lines, ": "+text)
	}
	if len(lines) == 0 {
		return
	}
	renderer.renderOwnTargets(node)
	renderer.appendBlock(strings.Join(lines, "\n"))
}

func (renderer *markdownRenderer) renderTable(node *htmlNode) {
	rows := tableRows(node)
	if len(rows) == 0 {
		return
	}
	var lines []string
	for rowIndex, row := range rows {
		cells := make([]string, 0, len(row.cells))
		for _, cell := range row.cells {
			cells = append(cells, escapeTableCell(renderer.renderInline(cell, true)))
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if rowIndex == 0 && row.header {
			separators := make([]string, len(cells))
			for index := range separators {
				separators[index] = "---"
			}
			lines = append(lines, "| "+strings.Join(separators, " | ")+" |")
		}
	}
	renderer.renderOwnTargets(node)
	renderer.appendBlock(strings.Join(lines, "\n"))
}

type tableRow struct {
	cells  []*htmlNode
	header bool
}

func tableRows(node *htmlNode) []tableRow {
	var rows []tableRow
	for _, child := range node.children {
		if child.name == "tr" {
			rows = append(rows, tableRowFor(child))
			continue
		}
		if child.name == "thead" || child.name == "tbody" || child.name == "tfoot" {
			rows = append(rows, tableRows(child)...)
		}
	}
	return rows
}

func tableRowFor(node *htmlNode) tableRow {
	row := tableRow{}
	for _, child := range node.children {
		if child.name != "th" && child.name != "td" {
			continue
		}
		if child.name == "th" {
			row.header = true
		}
		row.cells = append(row.cells, child)
	}
	return row
}

func (renderer *markdownRenderer) renderQuote(node *htmlNode) {
	text := strings.TrimSpace(renderer.renderInline(node, false))
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = "> " + line
	}
	renderer.appendBlock(strings.Join(lines, "\n"))
}

func (renderer *markdownRenderer) renderInline(node *htmlNode, preserveTargets bool) string {
	if node.name == "#text" {
		return normalizeText(node.text)
	}
	if renderer.skip(node) {
		return ""
	}
	prefix := ""
	if preserveTargets {
		prefix = renderer.inlineTargets(node)
	}
	content := renderer.renderInlineChildren(node)
	switch node.name {
	case "a":
		href := attributeValue(node, "href")
		if href == "" {
			return prefix + content
		}
		label := strings.TrimSpace(content)
		if label == "" {
			label = href
		}
		return prefix + "[" + label + "](" + renderer.link(href) + ")"
	case "strong", "b":
		return prefix + "**" + content + "**"
	case "em", "i":
		return prefix + "*" + content + "*"
	case "code", "tt", "var":
		return prefix + inlineCode(rawText(node))
	case "br":
		return prefix + "  \n"
	case "img":
		alt := attributeValue(node, "alt")
		src := attributeValue(node, "src")
		if src == "" {
			return prefix
		}
		return prefix + "![" + alt + "](" + renderer.link(src) + ")"
	case "sup":
		return prefix + "^" + content + "^"
	default:
		return prefix + content
	}
}

func (renderer *markdownRenderer) renderInlineChildren(node *htmlNode) string {
	var output strings.Builder
	for _, child := range node.children {
		if child.name == "#text" {
			output.WriteString(normalizeText(child.text))
			continue
		}
		output.WriteString(renderer.renderInline(child, true))
	}
	return output.String()
}

func (renderer *markdownRenderer) inlineTargets(node *htmlNode) string {
	targets := collectOwnTargets(node)
	if len(targets) == 0 {
		return ""
	}
	var output strings.Builder
	var previousNode *htmlNode
	for _, target := range targets {
		if target.node != previousNode {
			output.WriteString(target.markup)
			previousNode = target.node
		}
		renderer.addTarget(target.value, renderer.headingTitle, renderer.headingLevel)
	}
	return output.String()
}

func (renderer *markdownRenderer) addTarget(anchor, title string, level int) {
	occurrence := renderer.occurrences[anchor] + 1
	renderer.occurrences[anchor] = occurrence
	renderer.entries = append(renderer.entries, IndexEntry{
		Anchor:     anchor,
		Level:      level,
		Occurrence: occurrence,
		Title:      title,
	})
}

func (renderer *markdownRenderer) link(href string) string {
	if strings.HasPrefix(href, "#") {
		return href
	}
	parsed, err := url.Parse(href)
	if err != nil || parsed.IsAbs() || strings.HasPrefix(href, "//") {
		return href
	}
	base, err := url.Parse(renderer.sourceURL)
	if err != nil {
		return href
	}
	return base.ResolveReference(parsed).String()
}

func (renderer *markdownRenderer) skip(node *htmlNode) bool {
	if node.name == "#text" || node.name == "head" || node.name == "title" || node.name == "style" ||
		node.name == "script" {
		return node.name != "#text" || strings.TrimSpace(node.text) == ""
	}
	return hasClass(node, "nav")
}

func (renderer *markdownRenderer) startBlock() {
	if !renderer.hasOutput || renderer.blankLine {
		return
	}
	if !renderer.endsNewline {
		renderer.output.WriteByte('\n')
	}
	renderer.output.WriteByte('\n')
	renderer.blankLine = true
	renderer.endsNewline = true
}

func (renderer *markdownRenderer) appendBlock(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	renderer.startBlock()
	renderer.output.WriteString(text)
	renderer.output.WriteString("\n\n")
	renderer.hasOutput = true
	renderer.blankLine = true
	renderer.endsNewline = true
}

func (renderer *markdownRenderer) appendLine(text string) {
	if renderer.hasOutput && !renderer.endsNewline {
		renderer.output.WriteByte('\n')
	}
	renderer.output.WriteString(text)
	renderer.output.WriteByte('\n')
	renderer.hasOutput = true
	renderer.blankLine = text == ""
	renderer.endsNewline = true
}

type target struct {
	markup string
	node   *htmlNode
	value  string
}

func collectTargets(node *htmlNode) []target {
	var targets []target
	collectTargetNodes(node, &targets)
	return targets
}

func collectTargetNodes(node *htmlNode, targets *[]target) {
	*targets = append(*targets, collectOwnTargets(node)...)
	for _, child := range node.children {
		if child.name == "#text" {
			continue
		}
		collectTargetNodes(child, targets)
	}
}

func collectOwnTargets(node *htmlNode) []target {
	values := targetValues(node)
	if len(values) == 0 {
		return nil
	}
	markup := targetMarkup(node)
	targets := make([]target, 0, len(values))
	for _, value := range values {
		targets = append(targets, target{markup: markup, node: node, value: value})
	}
	return targets
}

func targetValues(node *htmlNode) []string {
	id := attributeValue(node, "id")
	name := attributeValue(node, "name")
	if id == "" && name == "" {
		return nil
	}
	values := make([]string, 0, 2)
	if name != "" {
		values = append(values, name)
	}
	if id != "" && id != name {
		values = append(values, id)
	}
	return values
}

func targetMarkup(node *htmlNode) string {
	var attrs []string
	for _, attr := range node.attrs {
		if attr.name != "id" && attr.name != "name" {
			continue
		}
		attrs = append(attrs, attr.name+"=\""+escapeHTMLAttribute(attr.value)+"\"")
	}
	return "<a " + strings.Join(attrs, " ") + "></a>"
}

func parseHTML(data []byte) (*htmlNode, error) {
	decoded, err := normalizeHTMLCharset(data)
	if err != nil {
		return nil, err
	}
	decoder := xmlDecoder(bytes.NewReader(repairHTMLTags(decoded)))
	root := &htmlNode{name: "#root"}
	stack := []*htmlNode{root}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode XHTML: %w", err)
		}
		switch value := token.(type) {
		case xmlStartElement:
			node := &htmlNode{name: strings.ToLower(value.Name.Local), attrs: attributes(value.Attr)}
			stack[len(stack)-1].children = append(stack[len(stack)-1].children, node)
			stack = append(stack, node)
		case xmlEndElement:
			if len(stack) == 1 {
				return nil, errors.New("unexpected XHTML closing element")
			}
			stack = stack[:len(stack)-1]
		case xmlCharData:
			stack[len(stack)-1].children = append(stack[len(stack)-1].children,
				&htmlNode{name: "#text", text: string(value)})
		case xmlComment, xmlDirective, xmlProcInst:
		}
	}
	if len(stack) != 1 {
		return nil, errors.New("unterminated XHTML element")
	}
	return root, nil
}

var htmlVoidElements = map[string]struct{}{
	"area":   {},
	"base":   {},
	"br":     {},
	"col":    {},
	"embed":  {},
	"hr":     {},
	"img":    {},
	"input":  {},
	"link":   {},
	"meta":   {},
	"param":  {},
	"source": {},
	"track":  {},
	"wbr":    {},
}

type htmlOpenElement struct {
	name        string
	closingName string
}

func repairHTMLTags(data []byte) []byte {
	output := make([]byte, 0, len(data))
	stack := make([]htmlOpenElement, 0)
	for index := 0; index < len(data); {
		if data[index] != '<' {
			output = append(output, data[index])
			index++
			continue
		}
		if end, ok := specialHTMLTagEnd(data, index, "<!--", "-->"); ok {
			output = append(output, data[index:end]...)
			index = end
			continue
		}
		if end, ok := specialHTMLTagEnd(data, index, "<![CDATA[", "]]>"); ok {
			output = append(output, data[index:end]...)
			index = end
			continue
		}
		end, ok := htmlTagEnd(data, index+1)
		if !ok {
			output = append(output, data[index:]...)
			break
		}
		name, closingName, closing, ok := htmlTagName(data, index, end)
		if !ok {
			output = append(output, data[index:end+1]...)
			index = end + 1
			continue
		}
		output = appendRepairedHTMLTag(output, data[index:end+1], name, closingName, closing, &stack)
		index = end + 1
	}
	return appendHTMLClosingTags(output, stack)
}

func htmlTagName(data []byte, start, end int) (string, string, bool, bool) {
	nameStart := start + 1
	closing := false
	if nameStart < end && data[nameStart] == '/' {
		closing = true
		nameStart++
	}
	nameEnd := nameStart
	for nameEnd < end && isHTMLTagNameByte(data[nameEnd]) {
		nameEnd++
	}
	if nameEnd == nameStart {
		return "", "", false, false
	}
	closingName := string(data[nameStart:nameEnd])
	return strings.ToLower(closingName), closingName, closing, true
}

func appendRepairedHTMLTag(output, raw []byte, name, closingName string, closing bool, stack *[]htmlOpenElement) []byte {
	_, void := htmlVoidElements[name]
	if void && closing {
		return output
	}
	if !closing {
		if void {
			if hasXMLSelfClosingTag(raw) {
				return append(output, raw...)
			}
			output = append(output, raw[:len(raw)-1]...)
			return append(output, '/', '>')
		}
		if !hasXMLSelfClosingTag(raw) {
			*stack = append(*stack, htmlOpenElement{name: name, closingName: closingName})
		}
		return append(output, raw...)
	}
	match := findHTMLOpenElement(*stack, name)
	if match < 0 {
		return output
	}
	for stackIndex := len(*stack) - 1; stackIndex > match; stackIndex-- {
		output = append(output, "</"+(*stack)[stackIndex].closingName+">"...)
	}
	output = append(output, "</"+(*stack)[match].closingName+">"...)
	*stack = (*stack)[:match]
	return output
}

func findHTMLOpenElement(stack []htmlOpenElement, name string) int {
	for index := len(stack) - 1; index >= 0; index-- {
		if stack[index].name == name {
			return index
		}
	}
	return -1
}

func appendHTMLClosingTags(output []byte, stack []htmlOpenElement) []byte {
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		output = append(output, "</"+stack[stackIndex].closingName+">"...)
	}
	return output
}

func specialHTMLTagEnd(data []byte, start int, prefix, suffix string) (int, bool) {
	if start+len(prefix) > len(data) || !strings.EqualFold(string(data[start:start+len(prefix)]), prefix) {
		return 0, false
	}
	end := bytes.Index(data[start+len(prefix):], []byte(suffix))
	if end < 0 {
		return 0, false
	}
	return start + len(prefix) + end + len(suffix), true
}

func htmlTagEnd(data []byte, start int) (int, bool) {
	quote := byte(0)
	for index := start; index < len(data); index++ {
		if quote != 0 {
			if data[index] == quote {
				quote = 0
			}
			continue
		}
		if data[index] == '\'' || data[index] == '"' {
			quote = data[index]
			continue
		}
		if data[index] == '>' {
			return index, true
		}
	}
	return 0, false
}

func isHTMLTagNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '-' || value == '_' || value == ':'
}

func hasXMLSelfClosingTag(tag []byte) bool {
	index := len(tag) - 2
	for index >= 0 && (tag[index] == ' ' || tag[index] == '\t' || tag[index] == '\r' || tag[index] == '\n') {
		index--
	}
	return index >= 0 && tag[index] == '/'
}

// These aliases keep parseHTML's token loop readable while retaining encoding/xml.
type xmlDecoderType = xml.Decoder
type xmlStartElement = xml.StartElement
type xmlEndElement = xml.EndElement
type xmlCharData = xml.CharData
type xmlComment = xml.Comment
type xmlDirective = xml.Directive
type xmlProcInst = xml.ProcInst

func xmlDecoder(reader io.Reader) *xmlDecoderType {
	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.CharsetReader = charsetReader
	decoder.Entity = htmlEntityMap()
	return decoder
}

func htmlEntityMap() map[string]string {
	entities := make(map[string]string, len(xml.HTMLEntity)+1)
	for name, value := range xml.HTMLEntity {
		entities[name] = value
	}
	entities["apos"] = "'"
	return entities
}

func normalizeHTMLCharset(data []byte) ([]byte, error) {
	charset := declaredHTMLCharset(data)
	if charset == "" {
		return data, nil
	}
	if charset == "utf-8" || charset == "utf8" {
		return data, nil
	}
	if charset == "iso-8859-1" || charset == "iso8859-1" || charset == "latin1" ||
		charset == "windows-1252" || charset == "cp1252" {
		if utf8.Valid(data) {
			return data, nil
		}
		return decodeCharsetBytes(charset, data)
	}
	if charset == "us-ascii" || charset == "ascii" {
		return decodeCharsetBytes(charset, data)
	}
	return nil, fmt.Errorf("unsupported HTML character encoding %q", charset)
}

func declaredHTMLCharset(data []byte) string {
	lower := strings.ToLower(string(data))
	marker := "charset="
	index := strings.Index(lower, marker)
	if index < 0 {
		return ""
	}
	start := index + len(marker)
	for start < len(lower) && (lower[start] == ' ' || lower[start] == '\t' || lower[start] == '\r' || lower[start] == '\n' || lower[start] == '\'' || lower[start] == '"') {
		start++
	}
	end := start
	for end < len(lower) && lower[end] != ' ' && lower[end] != '\t' && lower[end] != '\r' && lower[end] != '\n' && lower[end] != ';' && lower[end] != '\'' && lower[end] != '"' && lower[end] != '>' {
		end++
	}
	return lower[start:end]
}

func charsetReader(name string, input io.Reader) (io.Reader, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("read %s input: %w", name, err)
	}
	decoded, err := decodeCharsetBytes(strings.ToLower(strings.TrimSpace(name)), data)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(decoded), nil
}

func decodeCharsetBytes(name string, data []byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "utf-8", "utf8":
		if !utf8.Valid(data) {
			return nil, errors.New("invalid UTF-8 input")
		}
		return data, nil
	case "us-ascii", "ascii":
		for _, value := range data {
			if value > 0x7f {
				return nil, fmt.Errorf("non-ASCII byte 0x%x in us-ascii input", value)
			}
		}
		return data, nil
	case "iso-8859-1", "latin1", "windows-1252":
		converted := make([]byte, 0, len(data)*2)
		for _, value := range data {
			converted = utf8.AppendRune(converted, rune(value))
		}
		return converted, nil
	default:
		return nil, fmt.Errorf("unsupported character encoding %q", name)
	}
}

func attributes(values []xml.Attr) []htmlAttribute {
	result := make([]htmlAttribute, 0, len(values))
	for _, value := range values {
		result = append(result, htmlAttribute{name: strings.ToLower(value.Name.Local), value: value.Value})
	}
	return result
}

func attributeValue(node *htmlNode, name string) string {
	for _, attr := range node.attrs {
		if attr.name == name {
			return attr.value
		}
	}
	return ""
}

func hasClass(node *htmlNode, class string) bool {
	for _, value := range strings.Fields(attributeValue(node, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func hasBlockChild(node *htmlNode) bool {
	for _, child := range node.children {
		if isBlockName(child.name) {
			return true
		}
	}
	return false
}

func isBlockName(name string) bool {
	if isHeading(name) {
		return true
	}
	switch name {
	case "p", "pre", "ul", "ol", "dl", "table", "blockquote", "div", "section", "article", "hr":
		return true
	default:
		return false
	}
}

func isHeading(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

func rawText(node *htmlNode) string {
	if node.name == "#text" {
		return node.text
	}
	var output strings.Builder
	for _, child := range node.children {
		output.WriteString(rawText(child))
	}
	return output.String()
}

func plainText(node *htmlNode) string {
	return strings.Join(strings.Fields(rawText(node)), " ")
}

func normalizeText(value string) string {
	if strings.TrimSpace(value) == "" {
		if value == "" {
			return ""
		}
		return " "
	}
	first, _ := utf8.DecodeRuneInString(value)
	last, _ := utf8.DecodeLastRuneInString(value)
	leading := unicode.IsSpace(first)
	trailing := unicode.IsSpace(last)
	text := strings.Join(strings.Fields(value), " ")
	if leading {
		text = " " + text
	}
	if trailing {
		text += " "
	}
	return text
}

func inlineCode(value string) string {
	value = strings.TrimSpace(value)
	longest := 0
	current := 0
	for _, char := range value {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	delimiter := strings.Repeat("`", longest+1)
	return delimiter + value + delimiter
}

func fencedCode(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	longest := 2
	current := 0
	for _, char := range value {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	delimiter := strings.Repeat("`", longest+1)
	if !strings.HasSuffix(value, "\n") {
		value += "\n"
	}
	return delimiter + "\n" + value + delimiter
}

func escapeTableCell(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "|", "\\|")
}

func escapeHTMLAttribute(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	return strings.ReplaceAll(value, ">", "&gt;")
}

type generatedArtifact struct {
	code string
	data []byte
	path string
}

type stagedArtifact struct {
	path      string
	temporary string
}

func indexData(source string, entries []IndexEntry) []byte {
	var output strings.Builder
	output.WriteString("# goxsd9-spec-index/v1\n")
	output.WriteString("source\tanchor\toccurrence\tlevel\ttitle\n")
	for _, entry := range entries {
		output.WriteString(source)
		output.WriteByte('\t')
		output.WriteString(entry.Anchor)
		output.WriteByte('\t')
		output.WriteString(strconv.Itoa(entry.Occurrence))
		output.WriteByte('\t')
		output.WriteString(strconv.Itoa(entry.Level))
		output.WriteByte('\t')
		output.WriteString(strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(entry.Title))
		output.WriteByte('\n')
	}
	return []byte(output.String())
}

func publishArtifacts(artifacts []generatedArtifact, source string) error {
	staged, err := stageArtifacts(artifacts, source)
	if err != nil {
		return err
	}
	for index, artifact := range staged {
		if err := os.Rename(artifact.temporary, artifact.path); err != nil {
			cleanupErr := cleanupStaged(staged[index:])
			cause := fmt.Errorf("replace artifact: %w", err)
			if cleanupErr != nil {
				cause = errors.Join(cause, cleanupErr)
			}
			return corpusError("specs.output.publish", source, artifact.path, cause)
		}
	}
	return nil
}

func stageArtifacts(artifacts []generatedArtifact, source string) ([]stagedArtifact, error) {
	staged := make([]stagedArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		temporary, err := stageArtifact(artifact.path, artifact.data)
		if err == nil {
			staged = append(staged, stagedArtifact{path: artifact.path, temporary: temporary})
			continue
		}
		cleanupErr := cleanupStaged(staged)
		cause := err
		if cleanupErr != nil {
			cause = errors.Join(cause, cleanupErr)
		}
		return nil, corpusError(artifact.code, source, artifact.path, cause)
	}
	return staged, nil
}

func stageArtifact(path string, data []byte) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".goxsd9-specs-*")
	if err != nil {
		return "", fmt.Errorf("create temporary artifact: %w", err)
	}
	temporary := file.Name()
	count, writeErr := file.Write(data)
	if writeErr == nil && count != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		closeErr := file.Close()
		cause := fmt.Errorf("write temporary artifact: %w", writeErr)
		if closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close temporary artifact: %w", closeErr))
		}
		return "", cleanupTemporary(temporary, cause)
	}
	if err := file.Sync(); err != nil {
		closeErr := file.Close()
		cause := fmt.Errorf("sync temporary artifact: %w", err)
		if closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close temporary artifact: %w", closeErr))
		}
		return "", cleanupTemporary(temporary, cause)
	}
	if err := file.Close(); err != nil {
		return "", cleanupTemporary(temporary, fmt.Errorf("close temporary artifact: %w", err))
	}
	return temporary, nil
}

func cleanupStaged(artifacts []stagedArtifact) error {
	var cleanupErr error
	for _, artifact := range artifacts {
		if err := os.Remove(artifact.temporary); err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("remove temporary artifact %s: %w", artifact.temporary, err))
		}
	}
	return cleanupErr
}

func cleanupTemporary(path string, cause error) error {
	if err := os.Remove(path); err != nil {
		return errors.Join(cause, fmt.Errorf("remove temporary artifact: %w", err))
	}
	return cause
}
