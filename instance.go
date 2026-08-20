package goxsd9

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// InvalidInstanceXMLCode identifies malformed XML instance input.
	InvalidInstanceXMLCode = "XML3001"
	// InvalidInstanceNamespaceCode identifies an invalid XML namespace binding.
	InvalidInstanceNamespaceCode = "XML3002"
	// InvalidInstanceRootCode identifies an instance without exactly one root.
	InvalidInstanceRootCode = "XML3003"
	// UnsupportedInstanceSyntaxCode identifies valid XML outside this decoder
	// slice, such as DTDs and non-UTF-8 encodings.
	UnsupportedInstanceSyntaxCode = "XML3005"
)

const (
	instanceXMLWellFormedSpecRef   = "xml10#sec-well-formed"
	instanceXMLStartTagsSpecRef    = "xml10#sec-starttags"
	instanceXMLNamespacesSpecRef   = "xml-names#scoping-defaulting"
	diagnosticInstanceNoReaderCode = "XML3004"
	diagnosticInstanceFeatureCode  = "XML3006"
)

var instanceUTF8BOM = []byte{0xef, 0xbb, 0xbf}

var errInstanceUnsupportedEncoding = errors.New("non-UTF-8 instance encoding")

// instanceDocument is the completed, ordered representation consumed by the
// first validator phase. It retains no decoder, reader, or source bytes.
type instanceDocument struct {
	source SourceID
	root   *instanceElement
}

type instanceElement struct {
	name     syntaxName
	loc      Loc
	attrs    []instanceAttribute
	children []instanceNode
	scope    *syntaxScope
}

func (*instanceElement) instanceNode() {}

type instanceNode interface {
	instanceNode()
}

type instanceText struct {
	data string
	loc  Loc
}

func (instanceText) instanceNode() {}

type instanceAttribute struct {
	name  syntaxName
	value string
	loc   Loc
}

type instanceRawAttribute struct {
	value string
	loc   Loc
}

type instanceFrame struct {
	element *instanceElement
	name    syntaxName
	scope   *syntaxScope
	loc     Loc
}

type instanceDecoder struct {
	source    SourceID
	decoder   *xml.Decoder
	positions *syntaxPositionReader
	lexical   *instanceLexicalReader

	root        *instanceElement
	stack       []instanceFrame
	rootClosed  bool
	seenToken   bool
	seenXML     bool
	unsupported error
}

type instanceDecodeConfig struct {
	sourceID SourceID
}

type instanceLexicalReader struct {
	positions    *syntaxPositionReader
	capture      []byte
	captureStart int64
}

func (reader *instanceLexicalReader) ReadByte() (byte, error) {
	value, err := reader.positions.ReadByte()
	if err != nil {
		return 0, err
	}
	reader.captureByte(value)
	return value, nil
}

func (reader *instanceLexicalReader) Read(buffer []byte) (int, error) {
	n, err := reader.positions.Read(buffer)
	for index := 0; index < n; index++ {
		reader.captureByte(buffer[index])
	}
	return n, err
}

func (reader *instanceLexicalReader) beginCapture(logicalOffset int64) {
	reader.capture = reader.capture[:0]
	reader.captureStart = logicalOffset
	missing := reader.positions.offset - logicalOffset
	if missing == 1 {
		reader.capture = append(reader.capture, '<')
	}
}

func (reader *instanceLexicalReader) captureByte(value byte) {
	reader.capture = append(reader.capture, value)
}

func (reader *instanceLexicalReader) endCapture(logicalOffset int64) []byte {
	defer func() {
		reader.capture = nil
	}()
	length := logicalOffset - reader.captureStart
	if length < 0 || length > int64(len(reader.capture)) {
		return nil
	}
	return append([]byte(nil), reader.capture[:length]...)
}

// decodeInstance drains and closes reader, returning an instance only after
// the complete XML stream has been consumed successfully.
func decodeInstance(reader io.ReadCloser, config instanceDecodeConfig) (document *instanceDocument, err error) {
	if reader == nil {
		return nil, newDiagnostic(
			FailureInternal,
			diagnosticInstanceNoReaderCode,
			Loc{},
			"instance source has no reader",
			nil,
		)
	}

	positions := newSyntaxPositionReader(config.sourceID, reader)
	lexical := &instanceLexicalReader{positions: positions}
	decoder := xml.NewDecoder(lexical)
	decoder.Strict = true
	decoder.CharsetReader = func(charset string, _ io.Reader) (io.Reader, error) {
		return nil, fmt.Errorf("%w: %s", errInstanceUnsupportedEncoding, charset)
	}
	parser := instanceDecoder{
		source:    config.sourceID,
		decoder:   decoder,
		positions: positions,
		lexical:   lexical,
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
			"failed to close instance source",
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
			"failed while draining instance source",
			drainErr,
		)
		err = combineSyntaxErrors(err, drainDiagnostic)
	}
	document = nil
	return document, err
}

func (parser *instanceDecoder) decode() (*instanceDocument, error) {
	for {
		offset := parser.decoder.InputOffset()
		loc := parser.positions.locAt(offset)
		parser.lexical.beginCapture(offset)
		token, err := parser.decoder.RawToken()
		rawToken := parser.lexical.endCapture(parser.decoder.InputOffset())
		if err != nil {
			return parser.handleTokenError(err, loc, rawToken)
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
		if err := parser.handleToken(token, loc, rawToken); err != nil {
			return nil, err
		}
	}
}

func (parser *instanceDecoder) handleTokenError(err error, loc Loc, rawToken []byte) (*instanceDocument, error) {
	if errors.Is(err, io.EOF) {
		return parser.finishAtEOF(err, loc)
	}
	if errors.Is(err, errInstanceUnsupportedEncoding) {
		if declarationErr := parser.validateUnsupportedEncodingDeclaration(rawToken, loc); declarationErr != nil {
			return nil, declarationErr
		}
		return nil, newInstanceUnsupported(
			loc,
			"non-UTF-8 instance encodings are not supported",
			err,
		)
	}
	if parser.positions.isReadError(err) {
		return nil, newDiagnostic(
			FailureResolution,
			SourceReadCode,
			loc,
			"failed while reading instance source",
			err,
		)
	}
	if parser.positions.hasNonUTF8BOM() {
		return nil, newInstanceUnsupported(
			loc,
			"non-UTF-8 instance encodings are not supported",
			err,
		)
	}
	return nil, newInstanceInvalid(
		InvalidInstanceXMLCode,
		loc,
		"malformed XML in instance source",
		instanceXMLWellFormedSpecRef,
		err,
	)
}

func (parser *instanceDecoder) validateUnsupportedEncodingDeclaration(rawToken []byte, loc Loc) error {
	if parser.seenToken || parser.seenXML || parser.root != nil || len(parser.stack) > 0 || parser.rootClosed {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"XML declaration must be the first document token",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	data, ok := instanceXMLDeclarationData(rawToken)
	if !ok {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"invalid XML declaration",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	return validateInstanceXMLDeclaration(data, loc)
}

func (parser *instanceDecoder) finishAtEOF(cause error, loc Loc) (*instanceDocument, error) {
	if len(parser.stack) > 0 {
		open := parser.stack[len(parser.stack)-1]
		return nil, newInstanceInvalid(
			InvalidInstanceXMLCode,
			open.loc,
			fmt.Sprintf("unexpected end of document; element <%s> is not closed", renderSyntaxName(open.name)),
			instanceXMLWellFormedSpecRef,
			cause,
		)
	}
	if parser.root == nil {
		return nil, newInstanceInvalid(
			InvalidInstanceRootCode,
			loc,
			"instance document has no root element",
			instanceXMLWellFormedSpecRef,
			cause,
		)
	}
	if parser.unsupported != nil {
		return nil, parser.unsupported
	}
	return &instanceDocument{source: parser.source, root: parser.root}, nil
}

func (parser *instanceDecoder) handleToken(token xml.Token, loc Loc, rawToken []byte) error {
	switch value := token.(type) {
	case xml.StartElement:
		return parser.startElement(value, loc, rawToken)
	case xml.EndElement:
		return parser.endElement(value, loc)
	case xml.CharData:
		return parser.characterData(value, loc, rawToken)
	case xml.Comment:
		if err := validateInstanceComment(value, loc); err != nil {
			return err
		}
		parser.seenToken = true
		return nil
	case xml.ProcInst:
		return parser.processingInstruction(value, loc)
	case xml.Directive:
		if instanceIsDoctype(value) {
			if parser.root != nil || len(parser.stack) > 0 || parser.rootClosed {
				return newInstanceInvalid(
					InvalidInstanceXMLCode,
					loc,
					"DTD declaration is not allowed after the instance prolog",
					instanceXMLWellFormedSpecRef,
					nil,
				)
			}
			if parser.unsupported != nil {
				return newInstanceInvalid(
					InvalidInstanceXMLCode,
					loc,
					"multiple DTD declarations are not allowed",
					instanceXMLWellFormedSpecRef,
					nil,
				)
			}
			if err := validateInstanceDoctype(rawToken, loc); err != nil {
				return err
			}
			parser.unsupported = newInstanceUnsupported(loc, "DTD declarations are not supported", nil)
			parser.seenToken = true
			return nil
		}
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"invalid XML directive in instance source",
			instanceXMLWellFormedSpecRef,
			nil,
		)
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

func instanceIsDoctype(directive xml.Directive) bool {
	const keyword = "DOCTYPE"
	if !bytes.HasPrefix(directive, []byte(keyword)) {
		return false
	}
	if len(directive) == len(keyword) || !isInstanceXMLSpace(directive[len(keyword)]) {
		return false
	}
	for index := len(keyword); index < len(directive); index++ {
		if !isInstanceXMLSpace(directive[index]) {
			return true
		}
	}
	return false
}

func validateInstanceDoctype(raw []byte, loc Loc) error {
	body, ok := instanceDoctypeBody(raw)
	if !ok {
		return invalidInstanceDoctype(loc, "invalid DTD declaration")
	}
	index, ok := consumeInstanceDoctypeRoot(body)
	if !ok {
		return invalidInstanceDoctype(loc, "invalid DTD root name")
	}
	if index == len(body) {
		return nil
	}
	if body[index] != '[' {
		var externalOK bool
		index, externalOK = consumeInstanceDoctypeExternalID(body, index)
		if !externalOK {
			return invalidInstanceDoctype(loc, "invalid DTD external identifier")
		}
		if index == len(body) {
			return nil
		}
		if body[index] != '[' {
			return invalidInstanceDoctype(loc, "invalid DTD declaration")
		}
	}
	return validateInstanceDoctypeSubset(body, index, loc)
}

func instanceDoctypeBody(raw []byte) ([]byte, bool) {
	if len(raw) < len("<!DOCTYPE>") || !bytes.HasPrefix(raw, []byte("<!DOCTYPE")) || !bytes.HasSuffix(raw, []byte{'>'}) {
		return nil, false
	}
	return raw[len("<!") : len(raw)-1], true
}

func consumeInstanceDoctypeRoot(body []byte) (int, bool) {
	index := len("DOCTYPE")
	if index == len(body) || !isInstanceXMLSpace(body[index]) {
		return 0, false
	}
	consumeInstanceDoctypeSpace(body, &index)
	nameStart := index
	for index < len(body) && !isInstanceXMLSpace(body[index]) && body[index] != '[' {
		index++
	}
	if nameStart == index || !validInstanceXMLName(string(body[nameStart:index])) {
		return 0, false
	}
	consumeInstanceDoctypeSpace(body, &index)
	return index, true
}

func consumeInstanceDoctypeExternalID(body []byte, index int) (int, bool) {
	keywordStart := index
	for index < len(body) && !isInstanceXMLSpace(body[index]) && body[index] != '[' {
		index++
	}
	keyword := string(body[keywordStart:index])
	if keyword != "SYSTEM" && keyword != "PUBLIC" {
		return 0, false
	}
	if !consumeInstanceDoctypeSpace(body, &index) || !consumeInstanceDoctypeLiteral(body, &index) {
		return 0, false
	}
	if keyword == "PUBLIC" && (!consumeInstanceDoctypeSpace(body, &index) || !consumeInstanceDoctypeLiteral(body, &index)) {
		return 0, false
	}
	consumeInstanceDoctypeSpace(body, &index)
	return index, true
}

func validateInstanceDoctypeSubset(body []byte, index int, loc Loc) error {
	depth := 0
	var quote byte
	for index < len(body) {
		var closed, ok bool
		index, closed, ok = consumeInstanceDoctypeSubsetToken(body, index, &depth, &quote)
		if !ok {
			return invalidInstanceDoctype(loc, "invalid DTD internal subset")
		}
		if closed {
			consumeInstanceDoctypeSpace(body, &index)
			if index != len(body) {
				return invalidInstanceDoctype(loc, "invalid DTD internal subset")
			}
			return nil
		}
	}
	if quote != 0 {
		return invalidInstanceDoctype(loc, "unterminated DTD literal")
	}
	return invalidInstanceDoctype(loc, "unterminated DTD internal subset")
}

func consumeInstanceDoctypeSubsetToken(body []byte, index int, depth *int, quote *byte) (int, bool, bool) {
	if *quote != 0 {
		if body[index] == *quote {
			*quote = 0
		}
		return index + 1, false, true
	}
	if bytes.HasPrefix(body[index:], []byte("<!--")) {
		commentEnd := bytes.Index(body[index+4:], []byte("-->"))
		if commentEnd < 0 {
			return index, false, false
		}
		return index + 4 + commentEnd + len("-->"), false, true
	}
	switch body[index] {
	case '\'', '"':
		*quote = body[index]
	case '[':
		(*depth)++
	case ']':
		(*depth)--
		if *depth < 0 {
			return index, false, false
		}
		if *depth == 0 {
			return index + 1, true, true
		}
	}
	return index + 1, false, true
}

func consumeInstanceDoctypeSpace(data []byte, index *int) bool {
	start := *index
	for *index < len(data) && isInstanceXMLSpace(data[*index]) {
		(*index)++
	}
	return *index != start
}

func consumeInstanceDoctypeLiteral(data []byte, index *int) bool {
	if *index >= len(data) || (data[*index] != '\'' && data[*index] != '"') {
		return false
	}
	quote := data[*index]
	*index++
	for *index < len(data) && data[*index] != quote {
		(*index)++
	}
	if *index == len(data) {
		return false
	}
	(*index)++
	return true
}

func validInstanceXMLName(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	first := true
	for _, character := range value {
		if first {
			if !validNCNameStart(character) {
				return false
			}
			first = false
			continue
		}
		if character != ':' && !validNCNameChar(character) {
			return false
		}
	}
	return !first
}

func invalidInstanceDoctype(loc Loc, message string) error {
	return newInstanceInvalid(
		InvalidInstanceXMLCode,
		loc,
		message,
		instanceXMLWellFormedSpecRef,
		nil,
	)
}

func (parser *instanceDecoder) startElement(token xml.StartElement, loc Loc, rawToken []byte) error {
	if parser.rootClosed || (parser.root != nil && len(parser.stack) == 0) {
		return newInstanceInvalid(
			InvalidInstanceRootCode,
			loc,
			"instance document has more than one root element",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	rawAttributes, rawAttributesOK := instanceRawStartTagAttributes(rawToken, loc)
	scope, err := instanceChildScope(parser.currentScope(), token.Attr, rawAttributes, loc)
	if err != nil {
		return err
	}
	name, err := resolveInstanceName(token.Name, scope, true, loc)
	if err != nil {
		return err
	}
	attrs, err := instanceAttributes(token.Attr, scope, rawAttributes, rawAttributesOK, loc)
	if err != nil {
		return err
	}
	element := &instanceElement{
		name:     name,
		loc:      loc,
		attrs:    attrs,
		children: make([]instanceNode, 0),
		scope:    scope,
	}
	if parser.root == nil {
		parser.root = element
	}
	if parser.root != element {
		parent := parser.stack[len(parser.stack)-1].element
		parent.children = append(parent.children, element)
	}
	parser.stack = append(parser.stack, instanceFrame{
		element: element,
		name:    name,
		scope:   scope,
		loc:     loc,
	})
	parser.seenToken = true
	return nil
}

func (parser *instanceDecoder) endElement(token xml.EndElement, loc Loc) error {
	if len(parser.stack) == 0 {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			fmt.Sprintf("unexpected end element </%s>", token.Name.Local),
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	frame := parser.stack[len(parser.stack)-1]
	name, err := resolveInstanceName(token.Name, frame.scope, true, loc)
	if err != nil {
		return err
	}
	if name != frame.name {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			fmt.Sprintf("end element </%s> does not match <%s>", renderSyntaxName(name), renderSyntaxName(frame.name)),
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	parser.stack = parser.stack[:len(parser.stack)-1]
	if len(parser.stack) == 0 {
		parser.rootClosed = true
	}
	return nil
}

func (parser *instanceDecoder) characterData(data xml.CharData, loc Loc, rawToken []byte) error {
	if len(parser.stack) == 0 {
		return parser.characterDataOutside(data, rawToken, loc)
	}
	return parser.characterDataInside(data, loc)
}

func (parser *instanceDecoder) characterDataOutside(data xml.CharData, rawToken []byte, loc Loc) error {
	if parser.root == nil && !parser.seenToken {
		var bomOnly bool
		data, rawToken, bomOnly = stripInstanceLeadingBOM(data, rawToken)
		if bomOnly {
			return nil
		}
	}
	if instanceLiteralWhitespace(data, rawToken) {
		parser.seenToken = true
		return nil
	}
	if parser.rootClosed {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"character data follows the instance root",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	return newInstanceInvalid(
		InvalidInstanceXMLCode,
		loc,
		"character data is outside the instance root",
		instanceXMLWellFormedSpecRef,
		nil,
	)
}

func (parser *instanceDecoder) characterDataInside(data xml.CharData, loc Loc) error {
	if len(data) == 0 {
		return nil
	}
	frame := parser.stack[len(parser.stack)-1]
	frame.element.children = append(frame.element.children, instanceText{
		data: string(data),
		loc:  loc,
	})
	parser.seenToken = true
	return nil
}

func stripInstanceLeadingBOM(data xml.CharData, rawToken []byte) (xml.CharData, []byte, bool) {
	if !bytes.HasPrefix(data, instanceUTF8BOM) {
		return data, rawToken, false
	}
	data = bytes.TrimPrefix(data, instanceUTF8BOM)
	rawToken = bytes.TrimPrefix(rawToken, instanceUTF8BOM)
	return data, rawToken, len(data) == 0
}

func instanceLiteralWhitespace(data xml.CharData, rawToken []byte) bool {
	return xmlWhitespace(data) && len(rawToken) > 0 && xmlWhitespace(rawToken)
}

func (parser *instanceDecoder) processingInstruction(token xml.ProcInst, loc Loc) error {
	if !validInstanceXMLCharacters(token.Inst) {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"invalid processing instruction",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	if strings.EqualFold(token.Target, "xml") {
		if token.Target != "xml" || parser.seenToken || parser.seenXML {
			return newInstanceInvalid(
				InvalidInstanceXMLCode,
				loc,
				"XML declaration must be the first document token",
				instanceXMLWellFormedSpecRef,
				nil,
			)
		}
		if err := validateInstanceXMLDeclaration(token.Inst, loc); err != nil {
			return err
		}
		parser.seenXML = true
	}
	parser.seenToken = true
	return nil
}

func (parser *instanceDecoder) currentScope() *syntaxScope {
	if len(parser.stack) == 0 {
		return nil
	}
	return parser.stack[len(parser.stack)-1].scope
}

func instanceChildScope(parent *syntaxScope, attrs []xml.Attr, rawAttributes []instanceRawAttribute, loc Loc) (*syntaxScope, error) {
	for attrIndex, attr := range attrs {
		if attr.Name.Space != "" && !validNCName(attr.Name.Space) || !validNCName(attr.Name.Local) {
			attributeLoc := loc
			if attrIndex < len(rawAttributes) {
				attributeLoc = rawAttributes[attrIndex].loc
			}
			return nil, newInstanceInvalid(
				InvalidInstanceNamespaceCode,
				attributeLoc,
				fmt.Sprintf("invalid XML namespace name %q", instanceLexicalName(attr.Name)),
				instanceXMLNamespacesSpecRef,
				nil,
			)
		}
	}
	normalized := normalizeInstanceXMLAttributes(attrs, rawAttributes)
	scope, err := childSyntaxScope(parent, normalized, loc)
	if err == nil {
		return scope, nil
	}
	return nil, instanceNamespaceError(err, loc)
}

func resolveInstanceName(name xml.Name, scope *syntaxScope, element bool, loc Loc) (syntaxName, error) {
	if name.Space != "" && !validNCName(name.Space) || !validNCName(name.Local) {
		return syntaxName{}, newInstanceInvalid(
			InvalidInstanceNamespaceCode,
			loc,
			fmt.Sprintf("invalid XML namespace name %q", instanceLexicalName(name)),
			instanceXMLNamespacesSpecRef,
			nil,
		)
	}
	prefix := name.Space
	if prefix == "xmlns" {
		return syntaxName{}, newInstanceInvalid(
			InvalidInstanceNamespaceCode,
			loc,
			"the xmlns prefix cannot name an element or attribute",
			instanceXMLNamespacesSpecRef,
			nil,
		)
	}
	if prefix == "" && !element {
		return syntaxName{local: name.Local}, nil
	}
	namespace, ok := scope.lookup(prefix)
	if prefix == "" && !ok {
		return syntaxName{local: name.Local}, nil
	}
	if !ok {
		return syntaxName{}, newInstanceInvalid(
			InvalidInstanceNamespaceCode,
			loc,
			fmt.Sprintf("unbound namespace prefix %q", prefix),
			instanceXMLNamespacesSpecRef,
			nil,
		)
	}
	return syntaxName{namespace: namespace, local: name.Local}, nil
}

func instanceLexicalName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

func instanceAttributes(attrs []xml.Attr, scope *syntaxScope, rawAttributes []instanceRawAttribute, rawAttributesOK bool, loc Loc) ([]instanceAttribute, error) {
	result := make([]instanceAttribute, 0, len(attrs))
	seen := make([]syntaxName, 0, len(attrs))
	for attrIndex, attr := range attrs {
		if _, ok := namespaceDeclaration(attr); ok {
			continue
		}
		attributeLoc := loc
		if rawAttributesOK && attrIndex < len(rawAttributes) {
			attributeLoc = rawAttributes[attrIndex].loc
		}
		name, err := resolveInstanceName(attr.Name, scope, false, attributeLoc)
		if err != nil {
			return nil, err
		}
		if err := duplicateInstanceAttribute(seen, name, attributeLoc); err != nil {
			return nil, err
		}
		seen = append(seen, name)
		result = append(result, instanceAttribute{
			name:  name,
			value: instanceAttributeValue(attr.Value, rawAttributes, rawAttributesOK, attrIndex, len(attrs)),
			loc:   attributeLoc,
		})
	}
	return result, nil
}

func duplicateInstanceAttribute(seen []syntaxName, name syntaxName, loc Loc) error {
	for _, previous := range seen {
		if previous == name {
			return newInstanceInvalid(
				InvalidInstanceXMLCode,
				loc,
				fmt.Sprintf("duplicate attribute %q", renderSyntaxName(name)),
				instanceXMLStartTagsSpecRef,
				nil,
			)
		}
	}
	return nil
}

func instanceAttributeValue(value string, rawAttributes []instanceRawAttribute, rawAttributesOK bool, index, attributeCount int) string {
	normalized := normalizeInstanceXMLAttributeValue(value)
	if !rawAttributesOK || len(rawAttributes) != attributeCount {
		return normalized
	}
	lexical, ok := normalizeInstanceXMLAttributeLexeme(rawAttributes[index].value)
	if !ok {
		return normalized
	}
	return lexical
}

func normalizeInstanceXMLAttributes(attrs []xml.Attr, rawAttributes []instanceRawAttribute) []xml.Attr {
	result := make([]xml.Attr, len(attrs))
	copy(result, attrs)
	for index := range result {
		value := normalizeInstanceXMLAttributeValue(result[index].Value)
		if len(rawAttributes) == len(attrs) {
			if normalized, ok := normalizeInstanceXMLAttributeLexeme(rawAttributes[index].value); ok {
				value = normalized
			}
		}
		result[index].Value = value
	}
	return result
}

func normalizeInstanceXMLAttributeValue(value string) string {
	return strings.Map(func(character rune) rune {
		switch character {
		case '\t', '\n', '\r':
			return ' '
		default:
			return character
		}
	}, value)
}

func instanceRawStartTagAttributes(raw []byte, start Loc) ([]instanceRawAttribute, bool) {
	if len(raw) < 2 || raw[0] != '<' {
		return nil, false
	}
	index := 1
	index = skipInstanceRawStartName(raw, index)
	values := make([]instanceRawAttribute, 0)
	for {
		value, done, ok := readInstanceRawAttribute(raw, &index, start)
		if !ok {
			return nil, false
		}
		if done {
			return values, true
		}
		values = append(values, value)
	}
}

func skipInstanceRawStartName(raw []byte, index int) int {
	for index < len(raw) && !isInstanceXMLSpace(raw[index]) && raw[index] != '/' && raw[index] != '>' {
		index++
	}
	return index
}

func readInstanceRawAttribute(raw []byte, index *int, start Loc) (instanceRawAttribute, bool, bool) {
	skipInstanceRawSpace(raw, index)
	if *index >= len(raw) || raw[*index] == '>' {
		return instanceRawAttribute{}, true, true
	}
	if raw[*index] == '/' {
		if *index+1 < len(raw) && raw[*index+1] == '>' {
			return instanceRawAttribute{}, true, true
		}
		return instanceRawAttribute{}, false, false
	}
	nameStart := *index
	skipInstanceRawName(raw, index)
	skipInstanceRawSpace(raw, index)
	if *index >= len(raw) || raw[*index] != '=' {
		return instanceRawAttribute{}, false, false
	}
	(*index)++
	skipInstanceRawSpace(raw, index)
	if *index >= len(raw) || (raw[*index] != '\'' && raw[*index] != '"') {
		return instanceRawAttribute{}, false, false
	}
	quote := raw[*index]
	(*index)++
	valueStart := *index
	for *index < len(raw) && raw[*index] != quote {
		(*index)++
	}
	if *index >= len(raw) {
		return instanceRawAttribute{}, false, false
	}
	value := string(raw[valueStart:*index])
	(*index)++
	return instanceRawAttribute{
		value: value,
		loc:   instanceRawAttributeLoc(start, raw, nameStart),
	}, false, true
}

func instanceRawAttributeLoc(start Loc, raw []byte, offset int) Loc {
	if start.IsZero() || offset < 0 || offset > len(raw) {
		return start
	}
	line := start.line
	column := start.column
	previousCR := false
	for len(raw) > 0 && offset > 0 {
		value := raw[0]
		if value == '\r' {
			raw = raw[1:]
			offset--
			line++
			column = 1
			previousCR = true
			continue
		}
		if value == '\n' {
			raw = raw[1:]
			offset--
			if previousCR {
				previousCR = false
				continue
			}
			line++
			column = 1
			continue
		}
		previousCR = false
		_, size := utf8.DecodeRune(raw)
		if size > offset {
			size = 1
		}
		raw = raw[size:]
		offset -= size
		column++
	}
	return Loc{source: start.source, line: line, column: column}
}

func skipInstanceRawSpace(raw []byte, index *int) {
	for *index < len(raw) && isInstanceXMLSpace(raw[*index]) {
		(*index)++
	}
}

func skipInstanceRawName(raw []byte, index *int) {
	for *index < len(raw) && !isInstanceXMLSpace(raw[*index]) && raw[*index] != '=' && raw[*index] != '>' {
		(*index)++
	}
}

func normalizeInstanceXMLAttributeLexeme(value string) (string, bool) {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\t', '\n':
			result.WriteByte(' ')
		case '\r':
			if index+1 < len(value) && value[index+1] == '\n' {
				index++
			}
			result.WriteByte(' ')
		case '&':
			end := strings.IndexByte(value[index+1:], ';')
			if end < 0 {
				return "", false
			}
			end += index + 1
			decoded, ok := decodeInstanceXMLReference(value[index+1 : end])
			if !ok {
				return "", false
			}
			result.WriteString(decoded)
			index = end
		default:
			result.WriteByte(value[index])
		}
	}
	return result.String(), true
}

func decodeInstanceXMLReference(reference string) (string, bool) {
	switch reference {
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "amp":
		return "&", true
	case "apos":
		return "'", true
	case "quot":
		return `"`, true
	}
	base := 10
	digits := reference
	if strings.HasPrefix(reference, "#x") {
		base = 16
		digits = reference[2:]
	}
	if strings.HasPrefix(reference, "#") && base == 10 {
		digits = reference[1:]
	}
	if digits == "" || (!strings.HasPrefix(reference, "#") && !strings.HasPrefix(reference, "#x")) {
		return "", false
	}
	codePoint, err := strconv.ParseUint(digits, base, 32)
	if err != nil || codePoint > 0x10ffff || !validInstanceXMLCharacter(rune(codePoint)) {
		return "", false
	}
	return string(rune(codePoint)), true
}

func instanceNamespaceError(err error, loc Loc) error {
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		return err
	}
	diagnostic.code = InvalidInstanceNamespaceCode
	diagnostic.loc = loc
	diagnostic.specRef = instanceXMLNamespacesSpecRef
	return diagnostic
}

func newInstanceInvalid(code string, loc Loc, message, specRef string, cause error) Diagnostic {
	diagnostic := newDiagnostic(FailureInvalid, code, loc, message, cause)
	diagnostic.specRef = specRef
	return diagnostic
}

func newInstanceUnsupported(loc Loc, message string, cause error) error {
	feature, ok := LookupUnsupportedFeature(FeatureInstanceSyntax)
	if !ok {
		return newDiagnostic(
			FailureInternal,
			diagnosticInstanceFeatureCode,
			loc,
			"instance syntax feature is not registered",
			nil,
		)
	}
	diagnostic := newUnsupported(feature, UnsupportedInstanceSyntaxCode, loc, message)
	diagnostic.cause = cause
	return diagnostic
}

func validateInstanceComment(data xml.Comment, loc Loc) error {
	if bytes.Contains(data, []byte("--")) || bytes.HasSuffix(data, []byte{'-'}) || !validInstanceXMLCharacters(data) {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"invalid XML comment",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	return nil
}

func validInstanceXMLCharacters(data []byte) bool {
	for len(data) > 0 {
		character, size := utf8.DecodeRune(data)
		if character == utf8.RuneError && size == 1 {
			return false
		}
		if !validInstanceXMLCharacter(character) {
			return false
		}
		data = data[size:]
	}
	return true
}

func validInstanceXMLCharacter(character rune) bool {
	return character == 0x09 ||
		character == 0x0a ||
		character == 0x0d ||
		character >= 0x20 && character <= 0xd7ff ||
		character >= 0xe000 && character <= 0xfffd ||
		character >= 0x10000 && character <= 0x10ffff
}

func validateInstanceXMLDeclaration(data []byte, loc Loc) error {
	if len(data) == 0 {
		return invalidInstanceXMLDeclaration(loc, "invalid XML declaration field")
	}
	index := 0
	field := 0
	for {
		if field > 0 {
			if !consumeInstanceXMLSpace(data, &index) {
				return invalidInstanceXMLDeclaration(loc, "invalid XML declaration spacing")
			}
			if index == len(data) {
				return nil
			}
		}
		if err := readInstanceXMLDeclarationField(data, &index, field, loc); err != nil {
			return err
		}
		field++
		if index == len(data) {
			return nil
		}
	}
}

func instanceXMLDeclarationData(rawToken []byte) ([]byte, bool) {
	const prefix = "<?xml"
	if len(rawToken) < len(prefix)+2 || !bytes.HasPrefix(rawToken, []byte(prefix)) || !bytes.HasSuffix(rawToken, []byte("?>")) {
		return nil, false
	}
	data := rawToken[len(prefix) : len(rawToken)-2]
	if len(data) == 0 || !isInstanceXMLSpace(data[0]) {
		return nil, false
	}
	for len(data) > 0 && isInstanceXMLSpace(data[0]) {
		data = data[1:]
	}
	return data, true
}

func readInstanceXMLDeclarationField(data []byte, index *int, field int, loc Loc) error {
	nameStart := *index
	for *index < len(data) && isInstanceXMLDeclarationNameByte(data[*index]) {
		(*index)++
	}
	if nameStart == *index {
		return invalidInstanceXMLDeclaration(loc, "invalid XML declaration field")
	}
	name := string(data[nameStart:*index])
	if !instanceXMLDeclarationFieldAllowed(name, field) {
		return invalidInstanceXMLDeclaration(loc, "invalid XML declaration field order")
	}
	value, ok := readInstanceXMLDeclarationValue(data, index)
	if !ok {
		return invalidInstanceXMLDeclaration(loc, "invalid XML declaration value")
	}
	if !instanceXMLDeclarationValueAllowed(name, value) {
		return invalidInstanceXMLDeclaration(loc, "invalid XML declaration value")
	}
	return nil
}

func invalidInstanceXMLDeclaration(loc Loc, message string) error {
	return newInstanceInvalid(
		InvalidInstanceXMLCode,
		loc,
		message,
		instanceXMLWellFormedSpecRef,
		nil,
	)
}

func consumeInstanceXMLSpace(data []byte, index *int) bool {
	start := *index
	for *index < len(data) && isInstanceXMLSpace(data[*index]) {
		(*index)++
	}
	return *index > start
}

func isInstanceXMLSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func isInstanceXMLDeclarationNameByte(value byte) bool {
	return value >= 'a' && value <= 'z'
}

func instanceXMLDeclarationFieldAllowed(name string, field int) bool {
	switch field {
	case 0:
		return name == "version"
	case 1:
		return name == "encoding"
	case 2:
		return name == "standalone"
	default:
		return false
	}
}

func readInstanceXMLDeclarationValue(data []byte, index *int) (string, bool) {
	for *index < len(data) && isInstanceXMLSpace(data[*index]) {
		(*index)++
	}
	if *index >= len(data) || data[*index] != '=' {
		return "", false
	}
	(*index)++
	for *index < len(data) && isInstanceXMLSpace(data[*index]) {
		(*index)++
	}
	if *index >= len(data) || (data[*index] != '\'' && data[*index] != '"') {
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

func instanceXMLDeclarationValueAllowed(name, value string) bool {
	switch name {
	case "version":
		return value == "1.0"
	case "encoding":
		return validInstanceEncodingName(value)
	case "standalone":
		return value == "yes" || value == "no"
	default:
		return false
	}
}

func validInstanceEncodingName(value string) bool {
	if value == "" || !isInstanceEncodingNameStart(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isInstanceEncodingNameCharacter(character) {
			return false
		}
	}
	return true
}

func isInstanceEncodingNameStart(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isInstanceEncodingNameCharacter(value byte) bool {
	return isInstanceEncodingNameStart(value) || value >= '0' && value <= '9' ||
		value == '.' || value == '_' || value == '-'
}
