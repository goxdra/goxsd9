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
	instanceXMLDTDSpecRef          = "xml10#sec-prolog-dtd"
	instanceXMLStartTagsSpecRef    = "xml10#sec-starttags"
	instanceXMLNamespacesSpecRef   = "xml-names#scoping-defaulting"
	instanceXMLEncodingSpecRef     = "xml10#charencoding"
	diagnosticInstanceNoReaderCode = "XML3004"
	diagnosticInstanceFeatureCode  = "XML3006"
)

var instanceUTF8BOM = []byte{0xef, 0xbb, 0xbf}

var errInstanceUnsupportedEncoding = errors.New("non-UTF-8 instance encoding")

// instanceDocument is the completed, ordered representation consumed by the
// first validator phase. It retains no decoder, reader, or source bytes.
type instanceDocument struct {
	source   SourceID
	root     *instanceElement
	streamed bool
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
	rawName string
	scope   *syntaxScope
	loc     Loc
}

type instanceDecoder struct {
	source    SourceID
	decoder   *xml.Decoder
	positions *syntaxPositionReader
	lexical   *instanceLexicalReader
	observer  instanceDecoderObserver

	root            *instanceElement
	stack           []instanceFrame
	rootSeen        bool
	rootClosed      bool
	seenToken       bool
	seenXML         bool
	unsupported     error
	semantic        error
	observerChosen  bool
	streaming       bool
	allowUnboundDTD bool
}

type instanceDecodeConfig struct {
	sourceID SourceID
	observer instanceDecoderObserver
}

type instanceDecoderObserver interface {
	startElement(name syntaxName, loc Loc, attrs []instanceAttribute) (retainTree bool, err error)
	endElement(name syntaxName, loc Loc) error
	characterData(data []byte, loc Loc) error
}

type instanceLexicalReader struct {
	positions *syntaxPositionReader
	lexical   *xmlLexicalReader

	doctype                   bool
	doctypeDepth              int
	doctypeQuote              byte
	doctypeComment            bool
	doctypePI                 bool
	doctypePIPreviousQuestion bool
	doctypePrevious1          byte
	doctypePrevious2          byte
	doctypePrevious3          byte
	doctypeProbe              int
}

func newInstanceLexicalReader(positions *syntaxPositionReader) *instanceLexicalReader {
	return &instanceLexicalReader{
		positions: positions,
		lexical:   newXMLLexicalReader(positions),
	}
}

func (reader *instanceLexicalReader) base() *xmlLexicalReader {
	if reader.lexical == nil {
		reader.lexical = newXMLLexicalReader(reader.positions)
	}
	return reader.lexical
}

func (reader *instanceLexicalReader) ReadByte() (byte, error) {
	value, err := reader.base().ReadByte()
	if err != nil {
		return 0, err
	}
	return reader.transformByte(value), nil
}

func (reader *instanceLexicalReader) Read(buffer []byte) (int, error) {
	position := reader.base()
	n, err := position.Read(buffer)
	for index := 0; index < n; index++ {
		buffer[index] = reader.transformByte(buffer[index])
	}
	return n, err
}

func (reader *instanceLexicalReader) transformByte(value byte) byte {
	if reader.doctype {
		return reader.transformDoctypeByte(value)
	}
	reader.observeDoctypePrefix(value)
	return value
}

func (reader *instanceLexicalReader) observeDoctypePrefix(value byte) {
	const prefix = "<!DOCTYPE"
	if reader.doctypeProbe < 0 {
		return
	}
	if reader.doctypeProbe == 0 {
		if value == '<' {
			reader.doctypeProbe = 1
			return
		}
		reader.doctypeProbe = -1
		return
	}
	if reader.doctypeProbe < len(prefix) && value == prefix[reader.doctypeProbe] {
		reader.doctypeProbe++
		if reader.doctypeProbe == len(prefix) {
			reader.doctype = true
			reader.doctypeDepth = 1
			reader.doctypeProbe = 0
			reader.resetDoctypeHistory()
		}
		return
	}
	reader.doctypeProbe = -1
}

func (reader *instanceLexicalReader) transformDoctypeByte(value byte) byte {
	if reader.doctypePI {
		return reader.transformDoctypePIByte(value)
	}
	if reader.doctypeComment {
		return reader.transformDoctypeCommentByte(value)
	}
	if reader.doctypeQuote != 0 {
		return reader.transformDoctypeQuoteByte(value)
	}
	if reader.doctypePrevious1 == '<' && value == '?' {
		reader.doctypePI = true
		reader.doctypePIPreviousQuestion = false
		reader.rememberDoctypeByte(value)
		return value
	}
	return reader.transformDoctypeMarkupByte(value)
}

func (reader *instanceLexicalReader) transformDoctypePIByte(value byte) byte {
	if reader.doctypePIPreviousQuestion && value == '>' {
		reader.doctypePI = false
		reader.doctypePIPreviousQuestion = false
		reader.doctypeDepth--
		reader.resetDoctypeHistory()
		return value
	}
	reader.doctypePIPreviousQuestion = value == '?'
	if value == '<' || value == '>' || value == '\'' || value == '"' {
		return ' '
	}
	return value
}

func (reader *instanceLexicalReader) transformDoctypeCommentByte(value byte) byte {
	if reader.doctypePrevious2 == '-' && reader.doctypePrevious1 == '-' && value == '>' {
		reader.doctypeComment = false
	}
	reader.rememberDoctypeByte(value)
	return value
}

func (reader *instanceLexicalReader) transformDoctypeQuoteByte(value byte) byte {
	if value == reader.doctypeQuote {
		reader.doctypeQuote = 0
	}
	reader.rememberDoctypeByte(value)
	return value
}

func (reader *instanceLexicalReader) transformDoctypeMarkupByte(value byte) byte {
	if reader.doctypePrevious3 == '<' && reader.doctypePrevious2 == '!' && reader.doctypePrevious1 == '-' && value == '-' {
		reader.doctypeComment = true
		reader.doctypeDepth--
		reader.rememberDoctypeByte(value)
		return value
	}
	switch value {
	case '\'', '"':
		reader.doctypeQuote = value
	case '<':
		reader.doctypeDepth++
	case '>':
		reader.doctypeDepth--
		if reader.doctypeDepth == 0 {
			reader.doctype = false
			reader.resetDoctypeHistory()
			return value
		}
	}
	reader.rememberDoctypeByte(value)
	return value
}

func (reader *instanceLexicalReader) rememberDoctypeByte(value byte) {
	reader.doctypePrevious3 = reader.doctypePrevious2
	reader.doctypePrevious2 = reader.doctypePrevious1
	reader.doctypePrevious1 = value
}

func (reader *instanceLexicalReader) resetDoctypeHistory() {
	reader.doctypePrevious1 = 0
	reader.doctypePrevious2 = 0
	reader.doctypePrevious3 = 0
}

func (reader *instanceLexicalReader) beginCapture(logicalOffset int64) {
	reader.base().beginCapture(logicalOffset)
	reader.doctype = false
	reader.doctypeDepth = 0
	reader.doctypeQuote = 0
	reader.doctypeComment = false
	reader.doctypePI = false
	reader.doctypePIPreviousQuestion = false
	reader.doctypeProbe = 0
	reader.resetDoctypeHistory()
	missing := reader.positions.offset - logicalOffset
	if missing == 1 {
		reader.doctypeProbe = 1
	}
}

func (reader *instanceLexicalReader) endCapture(logicalOffset int64) []byte {
	return reader.base().endCapture(logicalOffset)
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
	lexical := newInstanceLexicalReader(positions)
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
		observer:  config.observer,
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
			document, tokenErr := parser.handleTokenError(err, loc, rawToken)
			return document, parser.withRecordedSemantic(tokenErr)
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
			return nil, parser.withRecordedSemantic(err)
		}
	}
}

func (parser *instanceDecoder) recordSemantic(err error) {
	if err == nil || parser.semantic != nil {
		return
	}
	parser.semantic = err
}

func (parser *instanceDecoder) withRecordedSemantic(err error) error {
	if parser.semantic == nil || err == nil {
		if err != nil {
			return err
		}
		return parser.semantic
	}

	for _, diagnostic := range syntaxDiagnostics(err) {
		if instanceDiagnosticsMatch(diagnostic, parser.semantic) {
			return err
		}
	}
	return combineInstanceErrors(parser.semantic, err)
}

func (parser *instanceDecoder) handleTokenError(err error, loc Loc, rawToken []byte) (*instanceDocument, error) {
	if errors.Is(err, io.EOF) {
		return parser.finishAtEOF(err, loc)
	}
	if errors.Is(err, errInstanceUnsupportedEncoding) {
		if declarationErr := parser.validateUnsupportedEncodingDeclaration(rawToken, loc); declarationErr != nil {
			return nil, declarationErr
		}
		return nil, newInstanceEncodingUnsupported(
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
		return nil, newInstanceEncodingUnsupported(
			loc,
			"non-UTF-8 instance encodings are not supported",
			err,
		)
	}
	if diagnostic, ok := instanceRawMultiColonNameDiagnostic(rawToken, loc); ok {
		return nil, diagnostic
	}
	return nil, newInstanceInvalid(
		InvalidInstanceXMLCode,
		loc,
		"malformed XML in instance source",
		instanceXMLWellFormedSpecRef,
		err,
	)
}

func instanceRawMultiColonNameDiagnostic(raw []byte, start Loc) (Diagnostic, bool) {
	if len(raw) < 2 || raw[0] != '<' {
		return Diagnostic{}, false
	}
	if raw[1] == '/' {
		return instanceRawMultiColonNameAt(raw, 2, start)
	}
	if raw[1] == '?' || raw[1] == '!' {
		return Diagnostic{}, false
	}
	return instanceRawStartTagMultiColonName(raw, start)
}

func instanceRawStartTagMultiColonName(raw []byte, start Loc) (Diagnostic, bool) {
	nameEnd, ok := consumeInstanceRawXMLName(raw, 1)
	if !ok {
		return Diagnostic{}, false
	}
	if diagnostic, found := instanceRawMultiColonNameAt(raw, 1, start); found {
		return diagnostic, true
	}
	return instanceRawAttributeMultiColonName(raw, nameEnd, start)
}

func instanceRawAttributeMultiColonName(raw []byte, index int, start Loc) (Diagnostic, bool) {
	for {
		if !skipInstanceRawSpace(raw, &index) {
			return Diagnostic{}, false
		}
		if instanceRawTagEnd(raw, index) {
			return Diagnostic{}, false
		}
		nameStart := index
		nameEnd, ok := consumeInstanceRawXMLName(raw, index)
		if !ok {
			return Diagnostic{}, false
		}
		if diagnostic, found := instanceRawMultiColonNameAt(raw, nameStart, start); found {
			return diagnostic, true
		}
		index, ok = consumeInstanceRawAttributeValue(raw, nameEnd)
		if !ok {
			return Diagnostic{}, false
		}
	}
}

func instanceRawTagEnd(raw []byte, index int) bool {
	if index >= len(raw) || raw[index] == '>' {
		return true
	}
	return raw[index] == '/' && index+1 < len(raw) && raw[index+1] == '>'
}

func consumeInstanceRawAttributeValue(raw []byte, index int) (int, bool) {
	skipInstanceRawSpace(raw, &index)
	if index >= len(raw) || raw[index] != '=' {
		return 0, false
	}
	index++
	skipInstanceRawSpace(raw, &index)
	if index >= len(raw) || raw[index] != '\'' && raw[index] != '"' {
		return 0, false
	}
	quote := raw[index]
	index++
	for index < len(raw) && raw[index] != quote {
		index++
	}
	if index >= len(raw) {
		return 0, false
	}
	return index + 1, true
}

func instanceRawMultiColonNameAt(raw []byte, nameStart int, start Loc) (Diagnostic, bool) {
	nameEnd, ok := consumeInstanceRawXMLName(raw, nameStart)
	if !ok || bytes.Count(raw[nameStart:nameEnd], []byte{':'}) < 2 {
		return Diagnostic{}, false
	}
	return newInstanceInvalid(
		InvalidInstanceNamespaceCode,
		instanceRawAttributeLoc(start, raw, nameStart),
		fmt.Sprintf("invalid XML namespace name %q", string(raw[nameStart:nameEnd])),
		instanceXMLNamespacesSpecRef,
		nil,
	), true
}

func consumeInstanceRawXMLName(raw []byte, index int) (int, bool) {
	start := index
	for index < len(raw) {
		character, size := utf8.DecodeRune(raw[index:])
		if character == utf8.RuneError && size == 1 {
			return 0, false
		}
		if index == start {
			if character != ':' && !validNCNameStart(character) {
				return 0, false
			}
			index += size
			continue
		}
		if character != ':' && !validNCNameChar(character) {
			break
		}
		index += size
	}
	return index, index != start
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
		return nil, parser.withRecordedSemantic(newInstanceInvalid(
			InvalidInstanceXMLCode,
			open.loc,
			fmt.Sprintf("unexpected end of document; element <%s> is not closed", renderSyntaxName(open.name)),
			instanceXMLWellFormedSpecRef,
			cause,
		))
	}
	if !parser.rootSeen {
		return nil, newInstanceInvalid(
			InvalidInstanceRootCode,
			loc,
			"instance document has no root element",
			instanceXMLWellFormedSpecRef,
			cause,
		)
	}
	if parser.unsupported != nil {
		if parser.semantic == nil {
			return nil, parser.unsupported
		}
		return nil, combineInstanceErrors(parser.unsupported, parser.semantic)
	}
	if parser.semantic != nil {
		return nil, parser.semantic
	}
	if parser.streaming {
		return &instanceDocument{source: parser.source, streamed: true}, nil
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
			if parser.rootSeen || len(parser.stack) > 0 || parser.rootClosed {
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
			parser.allowUnboundDTD = instanceDoctypeMaySupplyNamespaces(rawToken)
			parser.decoder.Strict = false
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
	if !validInstanceXMLCharacters(raw) {
		return invalidInstanceDoctype(loc, "invalid character in DTD declaration")
	}
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
		consumeInstanceDoctypeSpace(body, &index)
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
	if !consumeInstanceDoctypeSpace(body, &index) || !consumeInstanceDoctypeLiteral(body, &index, keyword == "PUBLIC") {
		return 0, false
	}
	if keyword == "PUBLIC" && (!consumeInstanceDoctypeSpace(body, &index) || !consumeInstanceDoctypeLiteral(body, &index, false)) {
		return 0, false
	}
	return index, true
}

func validateInstanceDoctypeSubset(body []byte, index int, loc Loc) error {
	if index >= len(body) || body[index] != '[' {
		return invalidInstanceDoctype(loc, "invalid DTD internal subset")
	}
	index++
	for index < len(body) {
		if consumeInstanceDoctypeSpace(body, &index) {
			continue
		}
		if body[index] == ']' {
			index++
			consumeInstanceDoctypeSpace(body, &index)
			if index != len(body) {
				return invalidInstanceDoctype(loc, "invalid DTD internal subset")
			}
			return nil
		}
		var next int
		var ok bool
		switch {
		case bytes.HasPrefix(body[index:], []byte("<!--")):
			next, ok = consumeInstanceDoctypeComment(body, index)
		case bytes.HasPrefix(body[index:], []byte("<?")):
			next, ok = consumeInstanceDoctypePI(body, index)
		case body[index] == '%':
			next, ok = consumeInstanceDoctypePEReference(body, index)
		case bytes.HasPrefix(body[index:], []byte("<!")):
			next, ok = consumeInstanceDoctypeMarkup(body, index)
		}
		if !ok {
			return invalidInstanceDoctype(loc, "invalid DTD internal subset")
		}
		index = next
	}
	return invalidInstanceDoctype(loc, "unterminated DTD internal subset")
}

func instanceDoctypeMaySupplyNamespaces(raw []byte) bool {
	body, ok := instanceDoctypeBody(raw)
	if !ok {
		return false
	}
	index, ok := consumeInstanceDoctypeRoot(body)
	if !ok || index == len(body) {
		return false
	}
	if body[index] != '[' {
		return true
	}
	return instanceDoctypeSubsetMaySupplyNamespaces(body, index+1)
}

func instanceDoctypeSubsetMaySupplyNamespaces(body []byte, index int) bool {
	for index < len(body) {
		if consumeInstanceDoctypeSpace(body, &index) {
			continue
		}
		if body[index] == ']' {
			return false
		}
		next, namespace, ok := instanceDoctypeNamespaceItem(body, index)
		if !ok {
			return false
		}
		if namespace {
			return true
		}
		index = next
	}
	return false
}

func instanceDoctypeNamespaceItem(body []byte, index int) (int, bool, bool) {
	if body[index] == '%' {
		next, ok := consumeInstanceDoctypePEReference(body, index)
		return next, true, ok
	}
	if bytes.HasPrefix(body[index:], []byte("<!--")) {
		next, ok := consumeInstanceDoctypeComment(body, index)
		return next, false, ok
	}
	if bytes.HasPrefix(body[index:], []byte("<?")) {
		next, ok := consumeInstanceDoctypePI(body, index)
		return next, false, ok
	}
	if !bytes.HasPrefix(body[index:], []byte("<!")) {
		return 0, false, false
	}
	next, ok := consumeInstanceDoctypeMarkup(body, index)
	if !ok {
		return 0, false, false
	}
	if bytes.HasPrefix(body[index:], []byte("<!ATTLIST")) && instanceDoctypeAttlistHasNamespace(body[index:next]) {
		return next, true, true
	}
	return next, false, true
}

func instanceDoctypeAttlistHasNamespace(declaration []byte) bool {
	index := len("<!ATTLIST")
	if !consumeInstanceDoctypeSpace(declaration, &index) || !consumeInstanceDoctypeQName(declaration, &index) {
		return false
	}
	for {
		name, done, ok := instanceDoctypeAttlistAttribute(declaration, &index)
		if !ok {
			return false
		}
		if done {
			return false
		}
		if name != "" {
			return true
		}
	}
}

func instanceDoctypeAttlistAttribute(declaration []byte, index *int) (string, bool, bool) {
	if !consumeInstanceDoctypeSpace(declaration, index) {
		return "", false, false
	}
	if *index >= len(declaration) || declaration[*index] == '>' {
		return "", true, true
	}
	nameStart := *index
	if !consumeInstanceDoctypeQName(declaration, index) {
		return "", false, false
	}
	name := string(declaration[nameStart:*index])
	if !consumeInstanceDoctypeSpace(declaration, index) || !consumeInstanceDoctypeAttType(declaration, index) || !consumeInstanceDoctypeSpace(declaration, index) {
		return "", false, false
	}
	defaultStart := *index
	if !consumeInstanceDoctypeDefaultDecl(declaration, index) {
		return "", false, false
	}
	if (name != "xmlns" && !strings.HasPrefix(name, "xmlns:")) || !instanceDoctypeDefaultSuppliesValue(declaration[defaultStart:*index]) {
		return "", false, true
	}
	return name, false, true
}

func instanceDoctypeDefaultSuppliesValue(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	if value[0] == '\'' || value[0] == '"' {
		return true
	}
	return bytes.HasPrefix(value, []byte("#FIXED"))
}

func consumeInstanceDoctypeComment(body []byte, index int) (int, bool) {
	commentEnd := bytes.Index(body[index+4:], []byte("-->"))
	if commentEnd < 0 {
		return 0, false
	}
	comment := body[index+4 : index+4+commentEnd]
	if bytes.Contains(comment, []byte("--")) || bytes.HasSuffix(comment, []byte{'-'}) {
		return 0, false
	}
	return index + 4 + commentEnd + len("-->"), true
}

func consumeInstanceDoctypePI(body []byte, index int) (int, bool) {
	nameStart := index + len("<?")
	nameEnd := nameStart
	for nameEnd < len(body) && !isInstanceXMLSpace(body[nameEnd]) && body[nameEnd] != '?' && body[nameEnd] != '>' {
		nameEnd++
	}
	if nameStart == nameEnd || !validNCName(string(body[nameStart:nameEnd])) || strings.EqualFold(string(body[nameStart:nameEnd]), "xml") {
		return 0, false
	}
	if nameEnd == len(body) {
		return 0, false
	}
	if body[nameEnd] == '?' {
		if nameEnd+1 >= len(body) || body[nameEnd+1] != '>' {
			return 0, false
		}
		return nameEnd + 2, true
	}
	if body[nameEnd] == '>' {
		return 0, false
	}
	if !isInstanceXMLSpace(body[nameEnd]) {
		return 0, false
	}
	end := bytes.Index(body[nameEnd:], []byte("?>"))
	if end < 0 {
		return 0, false
	}
	return nameEnd + end + len("?>"), true
}

func consumeInstanceDoctypePEReference(body []byte, index int) (int, bool) {
	nameStart := index + 1
	nameEnd := bytes.IndexByte(body[nameStart:], ';')
	if nameEnd < 0 {
		return 0, false
	}
	nameEnd += nameStart
	if !validNCName(string(body[nameStart:nameEnd])) {
		return 0, false
	}
	return nameEnd + 1, true
}

func consumeInstanceDoctypeMarkup(body []byte, index int) (int, bool) {
	if !bytes.HasPrefix(body[index:], []byte("<!")) {
		return 0, false
	}
	index += len("<!")
	keywordStart := index
	for index < len(body) && body[index] >= 'A' && body[index] <= 'Z' {
		index++
	}
	keyword := string(body[keywordStart:index])
	if keyword == "" || !consumeInstanceDoctypeSpace(body, &index) {
		return 0, false
	}
	switch keyword {
	case "ELEMENT":
		return consumeInstanceDoctypeElement(body, index)
	case "ATTLIST":
		return consumeInstanceDoctypeAttlist(body, index)
	case "ENTITY":
		return consumeInstanceDoctypeEntity(body, index)
	case "NOTATION":
		return consumeInstanceDoctypeNotation(body, index)
	default:
		return 0, false
	}
}

func consumeInstanceDoctypeElement(body []byte, index int) (int, bool) {
	if !consumeInstanceDoctypeQName(body, &index) || !consumeInstanceDoctypeSpace(body, &index) {
		return 0, false
	}
	if !consumeInstanceDoctypeContentSpec(body, &index) {
		return 0, false
	}
	return consumeInstanceDoctypeMarkupEnd(body, index)
}

func consumeInstanceDoctypeContentSpec(body []byte, index *int) bool {
	for _, keyword := range []string{"EMPTY", "ANY"} {
		if bytes.HasPrefix(body[*index:], []byte(keyword)) {
			end := *index + len(keyword)
			if end == len(body) || isInstanceXMLSpace(body[end]) || body[end] == '>' {
				*index = end
				return true
			}
			return false
		}
	}
	if *index >= len(body) || body[*index] != '(' {
		return false
	}
	return consumeInstanceDoctypeModel(body, index)
}

func consumeInstanceDoctypeModel(body []byte, index *int) bool {
	(*index)++
	consumeInstanceDoctypeSpace(body, index)
	if bytes.HasPrefix(body[*index:], []byte("#PCDATA")) {
		return consumeInstanceDoctypeMixedModel(body, index)
	}
	return consumeInstanceDoctypeChildrenModel(body, index)
}

func consumeInstanceDoctypeMixedModel(body []byte, index *int) bool {
	end := *index + len("#PCDATA")
	if end < len(body) && !isInstanceDoctypeDelimiter(body[end]) {
		return false
	}
	*index = end
	consumeInstanceDoctypeSpace(body, index)
	nameCount := 0
	for *index < len(body) && body[*index] == '|' {
		(*index)++
		consumeInstanceDoctypeSpace(body, index)
		if !consumeInstanceDoctypeQName(body, index) {
			return false
		}
		nameCount++
		consumeInstanceDoctypeSpace(body, index)
	}
	if *index >= len(body) || body[*index] != ')' {
		return false
	}
	(*index)++
	if nameCount > 0 && (*index >= len(body) || body[*index] != '*') {
		return false
	}
	if *index < len(body) && body[*index] == '*' {
		(*index)++
	}
	return true
}

func consumeInstanceDoctypeChildrenModel(body []byte, index *int) bool {
	if !consumeInstanceDoctypeContentParticle(body, index) {
		return false
	}
	if !consumeInstanceDoctypeModelMembers(body, index) {
		return false
	}
	if *index >= len(body) || body[*index] != ')' {
		return false
	}
	(*index)++
	if *index < len(body) && (body[*index] == '?' || body[*index] == '*' || body[*index] == '+') {
		(*index)++
	}
	return true
}

func consumeInstanceDoctypeModelMembers(body []byte, index *int) bool {
	separator := byte(0)
	for {
		consumeInstanceDoctypeSpace(body, index)
		if *index >= len(body) || body[*index] == ')' {
			return true
		}
		if body[*index] != ',' && body[*index] != '|' {
			return false
		}
		if separator == 0 {
			separator = body[*index]
		}
		if body[*index] != separator {
			return false
		}
		(*index)++
		consumeInstanceDoctypeSpace(body, index)
		if !consumeInstanceDoctypeContentParticle(body, index) {
			return false
		}
	}
}

func consumeInstanceDoctypeContentParticle(body []byte, index *int) bool {
	if *index < len(body) && body[*index] == '(' {
		return consumeInstanceDoctypeModel(body, index)
	}
	if !consumeInstanceDoctypeQName(body, index) {
		return false
	}
	if *index < len(body) && (body[*index] == '?' || body[*index] == '*' || body[*index] == '+') {
		(*index)++
	}
	return true
}

func consumeInstanceDoctypeAttlist(body []byte, index int) (int, bool) {
	if !consumeInstanceDoctypeQName(body, &index) {
		return 0, false
	}
	for {
		if !consumeInstanceDoctypeSpace(body, &index) {
			return consumeInstanceDoctypeMarkupEnd(body, index)
		}
		if index >= len(body) || body[index] == '>' {
			return consumeInstanceDoctypeMarkupEnd(body, index)
		}
		if !consumeInstanceDoctypeQName(body, &index) || !consumeInstanceDoctypeSpace(body, &index) {
			return 0, false
		}
		if !consumeInstanceDoctypeAttType(body, &index) || !consumeInstanceDoctypeSpace(body, &index) {
			return 0, false
		}
		if !consumeInstanceDoctypeDefaultDecl(body, &index) {
			return 0, false
		}
	}
}

func consumeInstanceDoctypeAttType(body []byte, index *int) bool {
	for _, keyword := range []string{"CDATA", "ID", "IDREF", "IDREFS", "ENTITY", "ENTITIES", "NMTOKEN", "NMTOKENS"} {
		if bytes.HasPrefix(body[*index:], []byte(keyword)) {
			end := *index + len(keyword)
			if end == len(body) || isInstanceXMLSpace(body[end]) || body[end] == '>' {
				*index = end
				return true
			}
			continue
		}
	}
	if bytes.HasPrefix(body[*index:], []byte("NOTATION")) {
		*index += len("NOTATION")
		if !consumeInstanceDoctypeSpace(body, index) {
			return false
		}
		return consumeInstanceDoctypeNameList(body, index)
	}
	return consumeInstanceDoctypeEnumeration(body, index)
}

func consumeInstanceDoctypeNameList(body []byte, index *int) bool {
	if *index >= len(body) || body[*index] != '(' {
		return false
	}
	(*index)++
	consumeInstanceDoctypeSpace(body, index)
	if !consumeInstanceDoctypeName(body, index) {
		return false
	}
	for {
		consumeInstanceDoctypeSpace(body, index)
		if *index >= len(body) || body[*index] == ')' {
			break
		}
		if body[*index] != '|' {
			return false
		}
		(*index)++
		consumeInstanceDoctypeSpace(body, index)
		if !consumeInstanceDoctypeName(body, index) {
			return false
		}
	}
	if *index >= len(body) || body[*index] != ')' {
		return false
	}
	(*index)++
	return true
}

func consumeInstanceDoctypeEnumeration(body []byte, index *int) bool {
	if *index >= len(body) || body[*index] != '(' {
		return false
	}
	(*index)++
	consumeInstanceDoctypeSpace(body, index)
	if !consumeInstanceDoctypeNmtoken(body, index) {
		return false
	}
	for {
		consumeInstanceDoctypeSpace(body, index)
		if *index >= len(body) || body[*index] == ')' {
			break
		}
		if body[*index] != '|' {
			return false
		}
		(*index)++
		consumeInstanceDoctypeSpace(body, index)
		if !consumeInstanceDoctypeNmtoken(body, index) {
			return false
		}
	}
	if *index >= len(body) || body[*index] != ')' {
		return false
	}
	(*index)++
	return true
}

func consumeInstanceDoctypeDefaultDecl(body []byte, index *int) bool {
	if *index >= len(body) {
		return false
	}
	if body[*index] != '#' {
		return consumeInstanceDoctypeAttributeValue(body, index)
	}
	for _, keyword := range []string{"#REQUIRED", "#IMPLIED"} {
		if bytes.HasPrefix(body[*index:], []byte(keyword)) {
			end := *index + len(keyword)
			if end == len(body) || isInstanceXMLSpace(body[end]) || body[end] == '>' {
				*index = end
				return true
			}
			return false
		}
	}
	if !bytes.HasPrefix(body[*index:], []byte("#FIXED")) {
		return false
	}
	*index += len("#FIXED")
	if !consumeInstanceDoctypeSpace(body, index) {
		return false
	}
	return consumeInstanceDoctypeAttributeValue(body, index)
}

func consumeInstanceDoctypeEntity(body []byte, index int) (int, bool) {
	parameter := false
	if index < len(body) && body[index] == '%' {
		parameter = true
		index++
		if !consumeInstanceDoctypeSpace(body, &index) {
			return 0, false
		}
	}
	if !consumeInstanceDoctypeName(body, &index) || !consumeInstanceDoctypeSpace(body, &index) {
		return 0, false
	}
	literal := index < len(body) && (body[index] == '\'' || body[index] == '"')
	if literal {
		if !consumeInstanceDoctypeEntityValue(body, &index) {
			return 0, false
		}
		return consumeInstanceDoctypeMarkupEnd(body, index)
	}
	var ok bool
	index, ok = consumeInstanceDoctypeExternalID(body, index)
	if !ok {
		return 0, false
	}
	if parameter {
		return consumeInstanceDoctypeMarkupEnd(body, index)
	}
	return consumeInstanceDoctypeExternalEntityEnd(body, index)
}

func consumeInstanceDoctypeExternalEntityEnd(body []byte, index int) (int, bool) {
	declarationStart := index
	if !consumeInstanceDoctypeSpace(body, &index) {
		return consumeInstanceDoctypeMarkupEnd(body, declarationStart)
	}
	if !consumeInstanceDoctypeWord(body, &index, "NDATA") {
		return consumeInstanceDoctypeMarkupEnd(body, declarationStart)
	}
	if !consumeInstanceDoctypeSpace(body, &index) || !consumeInstanceDoctypeName(body, &index) {
		return 0, false
	}
	return consumeInstanceDoctypeMarkupEnd(body, index)
}

func consumeInstanceDoctypeNotation(body []byte, index int) (int, bool) {
	if !consumeInstanceDoctypeName(body, &index) || !consumeInstanceDoctypeSpace(body, &index) {
		return 0, false
	}
	if consumeInstanceDoctypeWord(body, &index, "PUBLIC") {
		if !consumeInstanceDoctypeSpace(body, &index) || !consumeInstanceDoctypeLiteral(body, &index, true) {
			return 0, false
		}
		publicEnd := index
		hasSpace := consumeInstanceDoctypeSpace(body, &index)
		if index >= len(body) || (body[index] != '\'' && body[index] != '"') {
			index = publicEnd
			return consumeInstanceDoctypeMarkupEnd(body, index)
		}
		if !hasSpace {
			return 0, false
		}
		if !consumeInstanceDoctypeLiteral(body, &index, false) {
			return 0, false
		}
		return consumeInstanceDoctypeMarkupEnd(body, index)
	}
	var ok bool
	index, ok = consumeInstanceDoctypeExternalID(body, index)
	if !ok {
		return 0, false
	}
	return consumeInstanceDoctypeMarkupEnd(body, index)
}

func consumeInstanceDoctypeMarkupEnd(body []byte, index int) (int, bool) {
	consumeInstanceDoctypeSpace(body, &index)
	if index >= len(body) || body[index] != '>' {
		return 0, false
	}
	return index + 1, true
}

func consumeInstanceDoctypeWord(body []byte, index *int, word string) bool {
	if !bytes.HasPrefix(body[*index:], []byte(word)) {
		return false
	}
	end := *index + len(word)
	if end < len(body) && !isInstanceDoctypeDelimiter(body[end]) {
		return false
	}
	*index = end
	return true
}

func isInstanceDoctypeDelimiter(value byte) bool {
	return isInstanceXMLSpace(value) || value == '>' || value == ')' || value == '|' || value == ',' || value == '*'
}

func consumeInstanceDoctypeName(body []byte, index *int) bool {
	value, ok := consumeInstanceDoctypeNameToken(body, index)
	return ok && validNCName(value)
}

func consumeInstanceDoctypeQName(body []byte, index *int) bool {
	value, ok := consumeInstanceDoctypeNameToken(body, index)
	return ok && validInstanceXMLName(value)
}

func consumeInstanceDoctypeNameToken(body []byte, index *int) (string, bool) {
	start := *index
	for *index < len(body) {
		character, size := utf8.DecodeRune(body[*index:])
		if character == utf8.RuneError && size == 1 {
			return "", false
		}
		if character != ':' && !validNCNameChar(character) {
			break
		}
		*index += size
	}
	if start == *index {
		return "", false
	}
	return string(body[start:*index]), true
}

func consumeInstanceDoctypeNmtoken(body []byte, index *int) bool {
	start := *index
	for *index < len(body) {
		character, size := utf8.DecodeRune(body[*index:])
		if character == utf8.RuneError && size == 1 {
			return false
		}
		if character != ':' && !validNCNameChar(character) {
			break
		}
		*index += size
	}
	return start != *index
}

func consumeInstanceDoctypeEntityValue(body []byte, index *int) bool {
	if *index >= len(body) || (body[*index] != '\'' && body[*index] != '"') {
		return false
	}
	quote := body[*index]
	(*index)++
	return consumeInstanceDoctypeEntityValueContent(body, index, quote)
}

func consumeInstanceDoctypeEntityValueContent(body []byte, index *int, quote byte) bool {
	for *index < len(body) {
		if body[*index] == quote {
			(*index)++
			return true
		}
		if body[*index] == '&' {
			if !consumeInstanceDoctypeReference(body, index) {
				return false
			}
			continue
		}
		if body[*index] == '%' {
			return false
		}
		(*index)++
	}
	return false
}

func consumeInstanceDoctypeAttributeValue(body []byte, index *int) bool {
	if *index >= len(body) || (body[*index] != '\'' && body[*index] != '"') {
		return false
	}
	quote := body[*index]
	(*index)++
	for *index < len(body) {
		if body[*index] == quote {
			(*index)++
			return true
		}
		if body[*index] == '<' || body[*index] == '&' && !consumeInstanceDoctypeReference(body, index) {
			return false
		}
		if body[*index] != '&' {
			(*index)++
		}
	}
	return false
}

func consumeInstanceDoctypeReference(body []byte, index *int) bool {
	if *index >= len(body) || body[*index] != '&' {
		return false
	}
	(*index)++
	if *index < len(body) && body[*index] == '#' {
		return consumeInstanceDoctypeCharReference(body, index)
	}
	if !consumeInstanceDoctypeName(body, index) || *index >= len(body) || body[*index] != ';' {
		return false
	}
	(*index)++
	return true
}

func consumeInstanceDoctypeCharReference(body []byte, index *int) bool {
	(*index)++
	hexadecimal := false
	if *index < len(body) && body[*index] == 'x' {
		hexadecimal = true
		(*index)++
	}
	digitStart := *index
	for *index < len(body) && isInstanceReferenceDigit(body[*index], hexadecimal) {
		(*index)++
	}
	if digitStart == *index || *index >= len(body) || body[*index] != ';' {
		return false
	}
	digits := string(body[digitStart:*index])
	base := 10
	if hexadecimal {
		base = 16
	}
	codePoint, err := strconv.ParseUint(digits, base, 32)
	if err != nil || codePoint > 0x10ffff {
		return false
	}
	if !validInstanceXMLCharacter(rune(codePoint)) {
		return false
	}
	(*index)++
	return true
}

func isInstanceReferenceDigit(value byte, hexadecimal bool) bool {
	if value >= '0' && value <= '9' {
		return true
	}
	if !hexadecimal {
		return false
	}
	return value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func validateInstanceDocumentReferences(data []byte, loc Loc) error {
	if !validInstanceXMLCharacters(data) {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"invalid XML character in instance source",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	if bytes.HasPrefix(data, []byte("<![CDATA[")) {
		return nil
	}
	for index := 0; index < len(data); index++ {
		if data[index] != '&' {
			continue
		}
		next := index
		if !consumeInstanceDoctypeReference(data, &next) {
			return newInstanceInvalid(
				InvalidInstanceXMLCode,
				loc,
				"invalid XML character reference",
				instanceXMLWellFormedSpecRef,
				nil,
			)
		}
		index = next - 1
	}
	return nil
}

func consumeInstanceDoctypeSpace(data []byte, index *int) bool {
	start := *index
	for *index < len(data) && isInstanceXMLSpace(data[*index]) {
		(*index)++
	}
	return *index != start
}

func consumeInstanceDoctypeLiteral(data []byte, index *int, pubid bool) bool {
	if *index >= len(data) || (data[*index] != '\'' && data[*index] != '"') {
		return false
	}
	quote := data[*index]
	valueStart := *index + 1
	*index++
	for *index < len(data) && data[*index] != quote {
		(*index)++
	}
	if *index == len(data) {
		return false
	}
	if pubid && !validInstancePubid(data[valueStart:*index]) {
		return false
	}
	(*index)++
	return true
}

func validInstanceXMLName(value string) bool {
	separator := strings.IndexByte(value, ':')
	if separator < 0 {
		return validNCName(value)
	}
	if separator == 0 || separator == len(value)-1 || strings.IndexByte(value[separator+1:], ':') >= 0 {
		return false
	}
	return validNCName(value[:separator]) && validNCName(value[separator+1:])
}

func validInstancePubid(value []byte) bool {
	for _, character := range string(value) {
		switch {
		case character == ' ' || character == '\r' || character == '\n':
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case strings.ContainsRune("-'()+,./:=?;!*#@$_%", character):
		default:
			return false
		}
	}
	return true
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
	if parser.rootClosed {
		return newInstanceInvalid(
			InvalidInstanceRootCode,
			loc,
			"instance document has more than one root element",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	rawAttributes, rawAttributesOK := instanceRawStartTagAttributes(rawToken, loc)
	if !rawAttributesOK {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			"malformed XML start tag",
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	if parser.unsupported != nil {
		if err := validateInstanceDocumentReferences(rawToken, loc); err != nil {
			return err
		}
	}
	scope, err := instanceChildScope(parser.currentScope(), token.Attr, rawAttributes, loc)
	if err != nil {
		return err
	}
	name, err := resolveInstanceName(token.Name, scope, true, parser.allowUnboundDTD, loc)
	if err != nil {
		return err
	}
	attrs, err := instanceAttributes(token.Attr, scope, rawAttributes, rawAttributesOK, loc, parser.allowUnboundDTD)
	if err != nil {
		return err
	}
	parser.rootSeen = true
	parser.observeStartElement(name, loc, attrs)
	if parser.streaming {
		parser.stack = append(parser.stack, instanceFrame{
			name:    name,
			rawName: instanceLexicalName(token.Name),
			scope:   scope,
			loc:     loc,
		})
		parser.seenToken = true
		return nil
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
		rawName: instanceLexicalName(token.Name),
		scope:   scope,
		loc:     loc,
	})
	parser.seenToken = true
	return nil
}

func (parser *instanceDecoder) observeStartElement(name syntaxName, loc Loc, attrs []instanceAttribute) {
	if parser.observer == nil {
		return
	}
	if parser.observerChosen {
		if parser.streaming {
			_, err := parser.observer.startElement(name, loc, attrs)
			parser.recordSemantic(err)
		}
		return
	}
	retainTree, err := parser.observer.startElement(name, loc, attrs)
	parser.recordSemantic(err)
	parser.observerChosen = true
	parser.streaming = !retainTree
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
	rawName := instanceLexicalName(token.Name)
	if rawName != frame.rawName {
		return newInstanceInvalid(
			InvalidInstanceXMLCode,
			loc,
			fmt.Sprintf("end element </%s> does not match <%s>", rawName, frame.rawName),
			instanceXMLWellFormedSpecRef,
			nil,
		)
	}
	name, err := resolveInstanceName(token.Name, frame.scope, true, parser.allowUnboundDTD, loc)
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
	if parser.streaming {
		parser.recordSemantic(parser.observer.endElement(name, loc))
	}
	if len(parser.stack) == 0 {
		parser.rootClosed = true
	}
	return nil
}

func (parser *instanceDecoder) characterData(data xml.CharData, loc Loc, rawToken []byte) error {
	if parser.unsupported != nil {
		if err := validateInstanceDocumentReferences(rawToken, loc); err != nil {
			return err
		}
	}
	if len(parser.stack) == 0 {
		return parser.characterDataOutside(data, rawToken, loc)
	}
	return parser.characterDataInside(data, loc)
}

func (parser *instanceDecoder) characterDataOutside(data xml.CharData, rawToken []byte, loc Loc) error {
	if !parser.rootSeen && !parser.seenToken {
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
	if parser.streaming {
		parser.recordSemantic(parser.observer.characterData([]byte(data), loc))
		parser.seenToken = true
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
	if !validNCName(token.Target) {
		return newInstanceInvalid(
			InvalidInstanceNamespaceCode,
			loc,
			fmt.Sprintf("invalid XML namespace name %q", token.Target),
			instanceXMLNamespacesSpecRef,
			nil,
		)
	}
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
		if encoding := instanceXMLDeclarationEncoding(token.Inst); encoding != "" && !strings.EqualFold(encoding, "UTF-8") {
			return newInstanceEncodingUnsupported(
				loc,
				"non-UTF-8 instance encodings are not supported",
				fmt.Errorf("%w: %s", errInstanceUnsupportedEncoding, encoding),
			)
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
	attributeLocs := make([]Loc, len(rawAttributes))
	for index := range rawAttributes {
		attributeLocs[index] = rawAttributes[index].loc
	}
	scope, err := childSyntaxScopeWithLocations(parent, normalized, loc, attributeLocs)
	if err == nil {
		return scope, nil
	}
	return nil, instanceNamespaceError(err, loc)
}

func resolveInstanceName(name xml.Name, scope *syntaxScope, element, allowUnbound bool, loc Loc) (syntaxName, error) {
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
	if prefix == "" && !ok && !allowUnbound {
		return syntaxName{local: name.Local}, nil
	}
	if !ok {
		if allowUnbound {
			// A DTD can provide a namespace declaration through a default
			// attribute, including from an external subset. Keep the unresolved
			// name structurally comparable until the deferred DTD diagnostic wins.
			return syntaxName{namespace: instanceUnresolvedNamespace(prefix), local: name.Local}, nil
		}
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

func instanceUnresolvedNamespace(prefix string) string {
	if prefix == "" {
		return "\x00goxsd9:instance-unresolved-default"
	}
	return "\x00goxsd9:instance-unresolved-prefix:" + prefix
}

func instanceLexicalName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

func instanceAttributes(attrs []xml.Attr, scope *syntaxScope, rawAttributes []instanceRawAttribute, rawAttributesOK bool, loc Loc, allowUnbound bool) ([]instanceAttribute, error) {
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
		name, err := resolveInstanceName(attr.Name, scope, false, allowUnbound, attributeLoc)
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
	attributes, ok := rawXMLStartTagAttributes(raw, start)
	if !ok {
		return nil, false
	}
	values := make([]instanceRawAttribute, len(attributes))
	for index, attribute := range attributes {
		values[index] = instanceRawAttribute(attribute)
	}
	return values, true
}

func instanceRawAttributeLoc(start Loc, raw []byte, offset int) Loc {
	return rawXMLAttributeLoc(start, raw, offset)
}

func skipInstanceRawSpace(raw []byte, index *int) bool {
	return skipRawXMLSpace(raw, index)
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
	if diagnostic.loc.IsZero() {
		diagnostic.loc = loc
	}
	diagnostic.specRef = instanceXMLNamespacesSpecRef
	return diagnostic
}

func newInstanceInvalid(code string, loc Loc, message, specRef string, cause error) Diagnostic {
	diagnostic := newDiagnostic(FailureInvalid, code, loc, message, cause)
	diagnostic.specRef = specRef
	return diagnostic
}

func newInstanceUnsupported(loc Loc, message string, cause error) error {
	return newInstanceUnsupportedWithSpecRef(loc, message, cause, "")
}

func newInstanceEncodingUnsupported(loc Loc, message string, cause error) error {
	return newInstanceUnsupportedWithSpecRef(loc, message, cause, instanceXMLEncodingSpecRef)
}

func newInstanceUnsupportedWithSpecRef(loc Loc, message string, cause error, specRef string) error {
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
	if specRef != "" && diagnostic.Class() == FailureUnsupported {
		diagnostic.specRef = specRef
	}
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
	encodingSeen := false
	standaloneSeen := false
	for {
		name, done, err := readNextInstanceXMLDeclarationField(data, &index, field, loc)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if err := validateInstanceXMLDeclarationFieldOrder(name, field, &encodingSeen, &standaloneSeen, loc); err != nil {
			return err
		}
		field++
		if index == len(data) {
			return nil
		}
	}
}

func readNextInstanceXMLDeclarationField(data []byte, index *int, field int, loc Loc) (string, bool, error) {
	if field == 0 {
		name, err := readInstanceXMLDeclarationField(data, index, field, loc)
		return name, false, err
	}
	if !consumeInstanceXMLSpace(data, index) {
		return "", false, invalidInstanceXMLDeclaration(loc, "invalid XML declaration spacing")
	}
	if *index == len(data) {
		return "", true, nil
	}
	name, err := readInstanceXMLDeclarationField(data, index, field, loc)
	return name, false, err
}

func validateInstanceXMLDeclarationFieldOrder(name string, field int, encodingSeen, standaloneSeen *bool, loc Loc) error {
	if name == "encoding" {
		*encodingSeen = true
		return nil
	}
	if name != "standalone" {
		return nil
	}
	if *standaloneSeen || field == 2 && !*encodingSeen {
		return invalidInstanceXMLDeclaration(loc, "invalid XML declaration field order")
	}
	*standaloneSeen = true
	return nil
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

func instanceXMLDeclarationEncoding(data []byte) string {
	index := 0
	field := 0
	for index < len(data) {
		if field > 0 {
			consumeInstanceXMLSpace(data, &index)
		}
		nameStart := index
		for index < len(data) && isInstanceXMLDeclarationNameByte(data[index]) {
			index++
		}
		if nameStart == index {
			return ""
		}
		name := string(data[nameStart:index])
		value, ok := readInstanceXMLDeclarationValue(data, &index)
		if !ok {
			return ""
		}
		if name == "encoding" {
			return value
		}
		field++
	}
	return ""
}

func readInstanceXMLDeclarationField(data []byte, index *int, field int, loc Loc) (string, error) {
	nameStart := *index
	for *index < len(data) && isInstanceXMLDeclarationNameByte(data[*index]) {
		(*index)++
	}
	if nameStart == *index {
		return "", invalidInstanceXMLDeclaration(loc, "invalid XML declaration field")
	}
	name := string(data[nameStart:*index])
	if !instanceXMLDeclarationFieldAllowed(name, field) {
		return "", invalidInstanceXMLDeclaration(loc, "invalid XML declaration field order")
	}
	value, ok := readInstanceXMLDeclarationValue(data, index)
	if !ok {
		return "", invalidInstanceXMLDeclaration(loc, "invalid XML declaration value")
	}
	if !instanceXMLDeclarationValueAllowed(name, value) {
		return "", invalidInstanceXMLDeclaration(loc, "invalid XML declaration value")
	}
	return name, nil
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
		return name == "encoding" || name == "standalone"
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
