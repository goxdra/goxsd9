package goxsd9

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

const (
	// InvalidXMLSyntaxCode identifies malformed XML in a schema source.
	InvalidXMLSyntaxCode = "XSD3001"
	// InvalidSchemaRootCode identifies a source that is not an XSD schema
	// document.
	InvalidSchemaRootCode = "XSD3002"
	// UnsupportedSchemaSyntaxCode identifies well-formed XSD syntax outside the
	// syntax kernel.
	UnsupportedSchemaSyntaxCode = "XSD3003"
	// SourceReadCode identifies a failure while draining a schema source.
	SourceReadCode = "XSD3004"
	// SourceCloseCode identifies a failure while closing a schema source.
	SourceCloseCode = "XSD3005"
)

const (
	xsdNamespaceURI           = "http://www.w3.org/2001/XMLSchema"
	xsdVersioningNamespaceURI = "http://www.w3.org/2007/XMLSchema-versioning"
	xmlNamespaceURI           = "http://www.w3.org/XML/1998/namespace"
	xmlnsNamespaceURI         = "http://www.w3.org/2000/xmlns/"
)

// syntaxDocument is the raw, internal result of decoding one schema source.
// Its slices preserve lexical order; namespace scopes are immutable linked
// frames so later phases can resolve QName-valued attributes without retaining
// source bytes.
type syntaxDocument struct {
	source SourceID
	root   *syntaxElement
}

type syntaxElement struct {
	name     syntaxName
	loc      Loc
	attrs    []syntaxAttribute
	children []syntaxNode
	scope    *syntaxScope
}

func (*syntaxElement) syntaxNode() {}

type syntaxNode interface {
	syntaxNode()
}

type syntaxText struct {
	data string
	loc  Loc
}

func (syntaxText) syntaxNode() {}

type syntaxAttribute struct {
	name  syntaxName
	value string
	loc   Loc
}

type syntaxName struct {
	namespace string
	local     string
}

type syntaxBinding struct {
	prefix    string
	namespace string
}

type syntaxScope struct {
	parent   *syntaxScope
	bindings []syntaxBinding
}

func (scope *syntaxScope) lookup(prefix string) (string, bool) {
	if prefix == "xml" {
		return xmlNamespaceURI, true
	}
	for current := scope; current != nil; current = current.parent {
		for index := len(current.bindings) - 1; index >= 0; index-- {
			binding := current.bindings[index]
			if binding.prefix == prefix {
				return binding.namespace, true
			}
		}
	}
	return "", false
}

type syntaxFrame struct {
	element *syntaxElement
	name    syntaxName
	scope   *syntaxScope
	loc     Loc
}

type syntaxDecoder struct {
	source    SourceID
	decoder   *xml.Decoder
	positions *syntaxPositionReader

	root       *syntaxElement
	stack      []syntaxFrame
	rootClosed bool
	seenToken  bool
	seenXML    bool
}

type syntaxDecodeConfig struct {
	sourceID SourceID
}

// decodeSyntax drains and closes source, returning a raw syntax document only
// after the complete XML stream has been consumed successfully.
func decodeSyntax(reader io.ReadCloser, config syntaxDecodeConfig) (document *syntaxDocument, err error) {
	if reader == nil {
		return nil, newDiagnostic(
			FailureInternal,
			diagnosticSyntaxNoReaderCode,
			Loc{},
			"schema source has no reader",
			nil,
		)
	}

	positions := newSyntaxPositionReader(config.sourceID, reader)
	decoder := xml.NewDecoder(positions)
	decoder.Strict = true
	parser := syntaxDecoder{
		source:    config.sourceID,
		decoder:   decoder,
		positions: positions,
	}

	defer func() {
		closeErr := reader.Close()
		if closeErr == nil {
			return
		}
		closeDiagnostic := newDiagnostic(
			FailureResolution,
			SourceCloseCode,
			positions.currentLoc(),
			"failed to close schema source",
			closeErr,
		)
		err = combineSyntaxErrors(err, closeDiagnostic)
		document = nil
	}()

	document, err = parser.decode()
	if err == nil {
		return document, nil
	}

	drainErr := positions.drain()
	if drainErr != nil && !errors.Is(err, drainErr) {
		drainDiagnostic := newDiagnostic(
			FailureResolution,
			SourceReadCode,
			positions.currentLoc(),
			"failed while draining schema source",
			drainErr,
		)
		err = combineSyntaxErrors(err, drainDiagnostic)
	}
	document = nil
	return document, err
}

func decodeResolvedSyntax(source ResolvedSource) (*syntaxDocument, error) {
	return decodeSyntax(source.stream(), syntaxDecodeConfig{sourceID: source.SourceID()})
}

func (parser *syntaxDecoder) decode() (*syntaxDocument, error) {
	for {
		offset := parser.decoder.InputOffset()
		loc := parser.positions.locAt(offset)
		token, err := parser.decoder.RawToken()
		if err != nil {
			return parser.handleTokenError(err, loc)
		}
		if token == nil {
			return nil, newDiagnostic(
				FailureInternal,
				diagnosticSyntaxEmptyTokenCode,
				loc,
				"XML decoder returned an empty token",
				nil,
			)
		}
		if err := parser.handleToken(token, loc); err != nil {
			return nil, err
		}
	}
}

func (parser *syntaxDecoder) handleToken(token xml.Token, loc Loc) error {
	switch value := token.(type) {
	case xml.StartElement:
		return parser.startElement(value, loc)
	case xml.EndElement:
		return parser.endElement(value, loc)
	case xml.CharData:
		return parser.characterData(value, loc)
	case xml.ProcInst:
		return parser.processingInstruction(value, loc)
	case xml.Comment, xml.Directive:
		parser.seenToken = true
		return nil
	default:
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxUnsupportedTokenCode,
			loc,
			fmt.Sprintf("XML decoder returned unsupported token %T", token),
			nil,
		)
	}
}

func (parser *syntaxDecoder) handleTokenError(err error, loc Loc) (*syntaxDocument, error) {
	if errors.Is(err, io.EOF) {
		if len(parser.stack) > 0 {
			open := parser.stack[len(parser.stack)-1]
			return nil, newDiagnostic(
				FailureInvalid,
				InvalidXMLSyntaxCode,
				open.loc,
				fmt.Sprintf("unexpected end of document; element <%s> is not closed", open.name.local),
				err,
			)
		}
		if parser.root == nil {
			return nil, newDiagnostic(
				FailureInvalid,
				InvalidSchemaRootCode,
				loc,
				"schema document has no root element",
				err,
			)
		}
		return &syntaxDocument{source: parser.source, root: parser.root}, nil
	}

	if parser.positions.isReadError(err) {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, newDiagnostic(
				FailureInvalid,
				InvalidXMLSyntaxCode,
				loc,
				"malformed XML in schema source",
				err,
			)
		}
		return nil, newDiagnostic(
			FailureResolution,
			SourceReadCode,
			loc,
			"failed while reading schema source",
			err,
		)
	}
	return nil, newDiagnostic(
		FailureInvalid,
		InvalidXMLSyntaxCode,
		loc,
		"malformed XML in schema source",
		err,
	)
}

func (parser *syntaxDecoder) startElement(token xml.StartElement, loc Loc) error {
	scope, err := childSyntaxScope(parser.currentScope(), token.Attr, loc)
	if err != nil {
		return err
	}
	name, err := resolveSyntaxName(token.Name, scope, true, loc)
	if err != nil {
		return err
	}
	if parser.root == nil {
		if name != (syntaxName{namespace: xsdNamespaceURI, local: "schema"}) {
			return newDiagnostic(
				FailureInvalid,
				InvalidSchemaRootCode,
				loc,
				fmt.Sprintf("expected XSD schema root, got <%s>", renderSyntaxName(name)),
				nil,
			)
		}
	}
	if parser.root != nil {
		if len(parser.stack) == 0 {
			return newDiagnostic(
				FailureInvalid,
				InvalidXMLSyntaxCode,
				loc,
				"schema document has more than one root element",
				nil,
			)
		}
		if supportErr := parser.checkSupportedElement(name, loc); supportErr != nil {
			return supportErr
		}
	}

	attrs, err := syntaxAttributes(token.Attr, scope, loc)
	if err != nil {
		return err
	}
	element := &syntaxElement{
		name:     name,
		loc:      loc,
		attrs:    attrs,
		children: make([]syntaxNode, 0),
		scope:    scope,
	}
	if parser.root == nil {
		parser.root = element
	}
	if parser.root != element {
		parent := parser.stack[len(parser.stack)-1].element
		parent.children = append(parent.children, element)
	}
	parser.stack = append(parser.stack, syntaxFrame{
		element: element,
		name:    name,
		scope:   scope,
		loc:     loc,
	})
	parser.seenToken = true
	return nil
}

func (parser *syntaxDecoder) endElement(token xml.EndElement, loc Loc) error {
	if len(parser.stack) == 0 {
		return newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			fmt.Sprintf("unexpected end element </%s>", token.Name.Local),
			nil,
		)
	}
	frame := parser.stack[len(parser.stack)-1]
	name, err := resolveSyntaxName(token.Name, frame.scope, true, loc)
	if err != nil {
		return err
	}
	if name != frame.name {
		return newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			fmt.Sprintf("end element </%s> does not match <%s>", renderSyntaxName(name), renderSyntaxName(frame.name)),
			nil,
		)
	}
	parser.stack = parser.stack[:len(parser.stack)-1]
	if len(parser.stack) == 0 {
		parser.rootClosed = true
	}
	return nil
}

func (parser *syntaxDecoder) characterData(data xml.CharData, loc Loc) error {
	if len(parser.stack) == 0 {
		if parser.root == nil && !parser.seenToken && bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
			data = data[3:]
			if len(data) == 0 {
				return nil
			}
		}
		if xmlWhitespace(data) {
			parser.seenToken = true
			return nil
		}
		return newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			"character data is outside the schema root",
			nil,
		)
	}
	if parser.rootClosed {
		return newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			"character data follows the schema root",
			nil,
		)
	}
	frame := parser.stack[len(parser.stack)-1]
	frame.element.children = append(frame.element.children, syntaxText{
		data: string(data),
		loc:  loc,
	})
	parser.seenToken = true
	return nil
}

func (parser *syntaxDecoder) processingInstruction(token xml.ProcInst, loc Loc) error {
	if token.Target == "xml" {
		if parser.seenToken || parser.seenXML {
			return newDiagnostic(
				FailureInvalid,
				InvalidXMLSyntaxCode,
				loc,
				"XML declaration must be the first document token",
				nil,
			)
		}
		parser.seenXML = true
	}
	parser.seenToken = true
	return nil
}

func (parser *syntaxDecoder) currentScope() *syntaxScope {
	if len(parser.stack) == 0 {
		return nil
	}
	return parser.stack[len(parser.stack)-1].scope
}

func (parser *syntaxDecoder) checkSupportedElement(name syntaxName, loc Loc) error {
	if name.namespace != xsdNamespaceURI {
		return newUnsupportedSyntax(name, loc)
	}
	if name.local == "assertion" {
		feature, ok := LookupUnsupportedFeature("xsd.assertion")
		if !ok {
			return newDiagnostic(
				FailureInternal,
				diagnosticSyntaxAssertionFeatureCode,
				loc,
				"XSD assertion feature is not registered",
				nil,
			)
		}
		return newUnsupported(feature, UnsupportedSchemaSyntaxCode, loc, "XSD assertions are not implemented")
	}
	if _, ok := supportedSyntaxElements[name.local]; ok {
		return nil
	}
	return newUnsupportedSyntax(name, loc)
}

func newUnsupportedSyntax(name syntaxName, loc Loc) Diagnostic {
	feature, ok := LookupUnsupportedFeature(FeatureSchemaSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticSyntaxFeatureCode,
			loc,
			"schema syntax feature is not registered",
			nil,
		)
	}
	return newUnsupported(
		feature,
		UnsupportedSchemaSyntaxCode,
		loc,
		fmt.Sprintf("XSD element <%s> is not supported by the syntax kernel", renderSyntaxName(name)),
	)
}

var supportedSyntaxElements = map[string]struct{}{
	"all":            {},
	"annotation":     {},
	"appinfo":        {},
	"attribute":      {},
	"attributeGroup": {},
	"choice":         {},
	"complexContent": {},
	"complexType":    {},
	"documentation":  {},
	"element":        {},
	"extension":      {},
	"field":          {},
	"group":          {},
	"import":         {},
	"include":        {},
	"key":            {},
	"keyref":         {},
	"list":           {},
	"notation":       {},
	"restriction":    {},
	"redefine":       {},
	"selector":       {},
	"sequence":       {},
	"simpleContent":  {},
	"simpleType":     {},
	"unique":         {},
}

func childSyntaxScope(parent *syntaxScope, attrs []xml.Attr, loc Loc) (*syntaxScope, error) {
	bindings := make([]syntaxBinding, 0)
	for _, attr := range attrs {
		prefix, ok := namespaceDeclaration(attr)
		if !ok {
			continue
		}
		binding, err := makeSyntaxBinding(prefix, attr.Value, bindings, loc)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	if len(bindings) == 0 {
		return parent, nil
	}
	return &syntaxScope{parent: parent, bindings: bindings}, nil
}

func makeSyntaxBinding(prefix, namespace string, bindings []syntaxBinding, loc Loc) (syntaxBinding, error) {
	for _, binding := range bindings {
		if binding.prefix == prefix {
			return syntaxBinding{}, newDiagnostic(
				FailureInvalid,
				InvalidXMLSyntaxCode,
				loc,
				fmt.Sprintf("duplicate namespace declaration for prefix %q", prefix),
				nil,
			)
		}
	}
	if prefix == "xmlns" || (prefix == "xml" && namespace != xmlNamespaceURI) {
		return syntaxBinding{}, newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			fmt.Sprintf("invalid namespace declaration for prefix %q", prefix),
			nil,
		)
	}
	if namespace == xmlnsNamespaceURI || (prefix != "" && namespace == "") {
		return syntaxBinding{}, newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			fmt.Sprintf("invalid namespace URI %q for prefix %q", namespace, prefix),
			nil,
		)
	}
	return syntaxBinding{prefix: prefix, namespace: namespace}, nil
}

func namespaceDeclaration(attr xml.Attr) (string, bool) {
	if attr.Name.Space == "xmlns" {
		return attr.Name.Local, true
	}
	if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
		return "", true
	}
	return "", false
}

func resolveSyntaxName(name xml.Name, scope *syntaxScope, element bool, loc Loc) (syntaxName, error) {
	prefix := name.Space
	if prefix == "xmlns" {
		return syntaxName{}, newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			"the xmlns prefix cannot name an element or attribute",
			nil,
		)
	}
	if prefix == "" {
		if !element {
			return syntaxName{local: name.Local}, nil
		}
		namespace, ok := scope.lookup(prefix)
		if !ok {
			return syntaxName{local: name.Local}, nil
		}
		return syntaxName{namespace: namespace, local: name.Local}, nil
	}
	namespace, ok := scope.lookup(prefix)
	if !ok {
		return syntaxName{}, newDiagnostic(
			FailureInvalid,
			InvalidXMLSyntaxCode,
			loc,
			fmt.Sprintf("unbound namespace prefix %q", prefix),
			nil,
		)
	}
	return syntaxName{namespace: namespace, local: name.Local}, nil
}

func syntaxAttributes(attrs []xml.Attr, scope *syntaxScope, loc Loc) ([]syntaxAttribute, error) {
	result := make([]syntaxAttribute, 0, len(attrs))
	seen := make([]syntaxName, 0, len(attrs))
	for _, attr := range attrs {
		if _, ok := namespaceDeclaration(attr); ok {
			continue
		}
		name, err := resolveSyntaxName(attr.Name, scope, false, loc)
		if err != nil {
			return nil, err
		}
		for _, previous := range seen {
			if previous == name {
				return nil, newDiagnostic(
					FailureInvalid,
					InvalidXMLSyntaxCode,
					loc,
					fmt.Sprintf("duplicate attribute %q", renderSyntaxName(name)),
					nil,
				)
			}
		}
		seen = append(seen, name)
		result = append(result, syntaxAttribute{name: name, value: attr.Value, loc: loc})
	}
	return result, nil
}

func renderSyntaxName(name syntaxName) string {
	if name.namespace == "" {
		return name.local
	}
	return "{" + name.namespace + "}" + name.local
}

func xmlWhitespace(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, character := range data {
		switch character {
		case ' ', '\t', '\r', '\n':
		default:
			return false
		}
	}
	return true
}

func combineSyntaxErrors(primary error, additional Diagnostic) error {
	if primary == nil {
		return additional
	}
	items := syntaxDiagnostics(primary)
	items = append(items, additional)
	if len(items) == 1 {
		return items[0]
	}
	return makeDiagnostics(items)
}

func syntaxDiagnostics(err error) []Diagnostic {
	var aggregate Diagnostics
	if errors.As(err, &aggregate) {
		return aggregate.All()
	}
	var aggregatePointer *Diagnostics
	if errors.As(err, &aggregatePointer) && aggregatePointer != nil {
		return aggregatePointer.All()
	}
	var diagnostic Diagnostic
	if errors.As(err, &diagnostic) {
		return []Diagnostic{diagnostic}
	}
	var diagnosticPointer *Diagnostic
	if errors.As(err, &diagnosticPointer) && diagnosticPointer != nil {
		return []Diagnostic{*diagnosticPointer}
	}
	return []Diagnostic{newDiagnostic(
		FailureInternal,
		diagnosticSyntaxUnclassifiedErrorCode,
		Loc{},
		"syntax decoder returned an unclassified error",
		err,
	)}
}

type syntaxPositionSample struct {
	offset int64
	loc    Loc
}

// syntaxPositionReader exposes a byte reader so encoding/xml does not add an
// opaque buffer. It counts Unicode code points rather than UTF-8 bytes and
// retains only a short offset history needed to account for XML decoder
// lookahead.
type syntaxPositionReader struct {
	reader io.Reader
	source SourceID

	offset           int64
	line             int
	column           int
	utf8Continuation int
	previousCR       bool
	pending          error
	lastReadError    error
	emptyReads       int
	history          []syntaxPositionSample
	prefix           []byte
}

func newSyntaxPositionReader(source SourceID, reader io.Reader) *syntaxPositionReader {
	position := &syntaxPositionReader{
		reader: reader,
		source: source,
		line:   1,
		column: 1,
		prefix: make([]byte, 0, 3),
	}
	position.history = append(position.history, syntaxPositionSample{
		offset: 0,
		loc:    position.currentLoc(),
	})
	return position
}

func (position *syntaxPositionReader) ReadByte() (byte, error) {
	if position.pending != nil {
		err := position.pending
		position.pending = nil
		position.lastReadError = err
		return 0, err
	}
	var one [1]byte
	for {
		n, err := position.reader.Read(one[:])
		if n < 0 || n > len(one) {
			readErr := fmt.Errorf("schema source returned invalid byte count %d", n)
			position.lastReadError = readErr
			return 0, readErr
		}
		if n == 1 {
			position.recordByte(one[0])
			if err != nil {
				position.pending = err
			}
			return one[0], nil
		}
		if err != nil {
			position.lastReadError = err
			return 0, err
		}
		position.emptyReads++
		if position.emptyReads >= 100 {
			readErr := io.ErrNoProgress
			position.lastReadError = readErr
			return 0, readErr
		}
	}
}

func (position *syntaxPositionReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if position.pending != nil {
		err := position.pending
		position.pending = nil
		position.lastReadError = err
		return 0, err
	}
	for {
		n, err := position.readBuffer(buffer)
		if n > 0 {
			return n, err
		}
		if err != nil {
			return 0, err
		}
		position.emptyReads++
		if position.emptyReads >= 100 {
			readErr := io.ErrNoProgress
			position.lastReadError = readErr
			return 0, readErr
		}
	}
}

func (position *syntaxPositionReader) readBuffer(buffer []byte) (int, error) {
	n, err := position.reader.Read(buffer)
	if n < 0 || n > len(buffer) {
		readErr := fmt.Errorf("schema source returned invalid byte count %d", n)
		position.lastReadError = readErr
		return 0, readErr
	}
	for index := 0; index < n; index++ {
		position.recordByte(buffer[index])
	}
	if n == 0 {
		if err != nil {
			position.lastReadError = err
		}
		return 0, err
	}
	if err != nil {
		position.pending = err
	}
	return n, nil
}

func (position *syntaxPositionReader) recordByte(value byte) {
	position.emptyReads = 0
	position.history = append(position.history, syntaxPositionSample{
		offset: position.offset,
		loc:    position.currentLoc(),
	})
	if len(position.history) > 16 {
		position.history = position.history[1:]
	}
	position.offset++
	if len(position.prefix) < 3 {
		position.prefix = append(position.prefix, value)
		if len(position.prefix) == 3 && bytes.Equal(position.prefix, []byte{0xef, 0xbb, 0xbf}) {
			position.removeUTF8BOM()
			return
		}
	}
	position.advance(value)
}

func (position *syntaxPositionReader) removeUTF8BOM() {
	position.line = 1
	position.column = 1
	position.utf8Continuation = 0
	position.previousCR = false
	for index := range position.history {
		if position.history[index].offset <= position.offset {
			position.history[index].loc = position.currentLoc()
		}
	}
}

func (position *syntaxPositionReader) advance(value byte) {
	if value == '\r' {
		position.line++
		position.column = 1
		position.utf8Continuation = 0
		position.previousCR = true
		return
	}
	if value == '\n' {
		if position.previousCR {
			position.previousCR = false
			return
		}
		position.line++
		position.column = 1
		position.utf8Continuation = 0
		return
	}
	position.previousCR = false
	if position.utf8Continuation > 0 {
		if value&0xc0 == 0x80 {
			position.utf8Continuation--
			return
		}
		position.utf8Continuation = 0
	}
	position.column++
	switch {
	case value < 0x80:
		return
	case value >= 0xc2 && value <= 0xdf:
		position.utf8Continuation = 1
	case value >= 0xe0 && value <= 0xef:
		position.utf8Continuation = 2
	case value >= 0xf0 && value <= 0xf4:
		position.utf8Continuation = 3
	}
}

func (position *syntaxPositionReader) currentLoc() Loc {
	if position.source == "" {
		return Loc{}
	}
	return Loc{source: position.source, line: position.line, column: position.column}
}

func (position *syntaxPositionReader) locAt(offset int64) Loc {
	if offset == position.offset {
		return position.currentLoc()
	}
	for index := len(position.history) - 1; index >= 0; index-- {
		if position.history[index].offset == offset {
			return position.history[index].loc
		}
	}
	return position.currentLoc()
}

func (position *syntaxPositionReader) isReadError(err error) bool {
	return position.lastReadError != nil && errors.Is(err, position.lastReadError)
}

func (position *syntaxPositionReader) drain() error {
	_, err := io.Copy(io.Discard, position)
	return err
}
