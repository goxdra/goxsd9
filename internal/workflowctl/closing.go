package workflowctl

import (
	"strconv"
	"strings"
	"unicode"
)

const closingReferenceRepository = repositoryKey

type closingReferenceParser struct {
	fenceChar        byte
	fenceLength      int
	fenceContext     markdownFenceContext
	inlineCodeLength int
	htmlComment      bool
	htmlBlockTags    []string
	htmlRawTag       string
}

type markdownFenceContext struct {
	blockQuoteDepth int
	listDepth       int
	listIndent      int
	indent          int
}

func closingIssueNumbers(body string) []int {
	parser := closingReferenceParser{}
	var numbers []int
	for _, line := range strings.Split(body, "\n") {
		for _, number := range parser.parseLine(line) {
			if !containsNumber(numbers, number) {
				numbers = append(numbers, number)
			}
		}
	}
	return numbers
}

func (parser *closingReferenceParser) parseLine(rawLine string) []int {
	line := strings.TrimSuffix(rawLine, "\r")
	if len(parser.htmlBlockTags) != 0 {
		parser.consumeHTMLBlock(line)
		return nil
	}
	if parser.fenceChar != 0 {
		if isClosingFenceInContext(line, parser.fenceChar, parser.fenceLength, parser.fenceContext) {
			parser.fenceChar = 0
			parser.fenceLength = 0
			parser.fenceContext = markdownFenceContext{}
		}
		return nil
	}
	if isIndentedCode(line) {
		return nil
	}
	if fenceChar, fenceLength, ok := openingFence(line); ok {
		parser.fenceChar = fenceChar
		parser.fenceLength = fenceLength
		_, parser.fenceContext, _ = markdownFencePrefix(line)
		return nil
	}
	visible := parser.maskExcluded(line)
	if hasHTMLTag(visible) {
		parser.updateHTMLBlock(visible)
		return nil
	}
	return parseClosingReferenceLine(visible)
}

func isIndentedCode(line string) bool {
	spaces := 0
	for _, value := range line {
		if value == '\t' {
			return true
		}
		if value != ' ' {
			break
		}
		spaces++
	}
	return spaces >= 4
}

func openingFence(line string) (byte, int, bool) {
	offset, _, ok := markdownFencePrefix(line)
	if !ok || offset == len(line) {
		return 0, 0, false
	}
	value := line[offset]
	if value != '`' && value != '~' {
		return 0, 0, false
	}
	length := 0
	for offset+length < len(line) && line[offset+length] == value {
		length++
	}
	if length < 3 {
		return 0, 0, false
	}
	return value, length, true
}

func isClosingFenceInContext(line string, fenceChar byte, fenceLength int, opening markdownFenceContext) bool {
	offset, candidate, ok := markdownClosingFencePrefix(line, opening)
	if !ok || offset == len(line) || line[offset] != fenceChar {
		return false
	}
	if candidate.blockQuoteDepth != opening.blockQuoteDepth {
		return false
	}
	if opening.listDepth == 0 && candidate.listDepth != 0 {
		return false
	}
	if opening.listDepth != 0 && (candidate.listDepth != 0 || candidate.indent < opening.listIndent) {
		return false
	}
	length := 0
	for offset+length < len(line) && line[offset+length] == fenceChar {
		length++
	}
	if length < fenceLength {
		return false
	}
	return strings.TrimSpace(line[offset+length:]) == ""
}

func markdownClosingFencePrefix(line string, opening markdownFenceContext) (int, markdownFenceContext, bool) {
	offset, context, ok := markdownFencePrefix(line)
	if ok {
		return offset, context, true
	}
	if opening.listDepth == 0 {
		return 0, markdownFenceContext{}, false
	}
	return markdownListContinuationFencePrefix(line, opening)
}

func markdownListContinuationFencePrefix(line string, opening markdownFenceContext) (int, markdownFenceContext, bool) {
	context := markdownFenceContext{}
	offset := 0
	for offset < len(line) {
		spaces := leadingSpacesFrom(line, offset)
		if spaces > 3 {
			return markdownClosingFenceIndent(offset, spaces, context, opening)
		}
		offset += spaces
		if offset == len(line) {
			context.indent = spaces
			return offset, context, true
		}
		next, consumed := consumeMarkdownFenceContainer(line, offset, spaces, &context)
		if consumed {
			offset = next
			continue
		}
		context.indent = spaces
		return offset, context, true
	}
	return offset, context, true
}

func markdownClosingFenceIndent(offset, spaces int, context, opening markdownFenceContext) (int, markdownFenceContext, bool) {
	if context.blockQuoteDepth != opening.blockQuoteDepth {
		return 0, markdownFenceContext{}, false
	}
	if context.listDepth != 0 {
		return 0, markdownFenceContext{}, false
	}
	if spaces < opening.listIndent || spaces > opening.listIndent+3 {
		return 0, markdownFenceContext{}, false
	}
	context.indent = spaces
	return offset + spaces, context, true
}

func consumeMarkdownFenceContainer(line string, offset, spaces int, context *markdownFenceContext) (int, bool) {
	if line[offset] == '>' {
		context.blockQuoteDepth++
		offset++
		if offset < len(line) && isSpaceByte(line[offset]) {
			offset++
		}
		return offset, true
	}
	markerLength := markdownListMarkerLength(line, offset)
	if markerLength == 0 {
		return offset, false
	}
	context.listDepth++
	context.listIndent += spaces + markerLength
	return offset + markerLength, true
}

func markdownFencePrefix(line string) (int, markdownFenceContext, bool) {
	var context markdownFenceContext
	offset := 0
	for offset < len(line) {
		spaces := leadingSpacesFrom(line, offset)
		if spaces > 3 {
			return 0, markdownFenceContext{}, false
		}
		offset += spaces
		if offset == len(line) {
			context.indent = spaces
			return offset, context, true
		}
		if line[offset] == '>' {
			context.blockQuoteDepth++
			offset++
			if offset < len(line) && isSpaceByte(line[offset]) {
				offset++
			}
			continue
		}
		markerLength := markdownListMarkerLength(line, offset)
		if markerLength == 0 {
			context.indent = spaces
			return offset, context, true
		}
		context.listDepth++
		context.listIndent += spaces + markerLength
		offset += markerLength
	}
	return offset, context, true
}

func leadingSpacesFrom(line string, start int) int {
	index := start
	for index < len(line) && line[index] == ' ' {
		index++
	}
	return index - start
}

func markdownListMarkerLength(line string, offset int) int {
	if offset >= len(line) {
		return 0
	}
	if (line[offset] == '-' || line[offset] == '+' || line[offset] == '*') &&
		offset+1 < len(line) && isSpaceByte(line[offset+1]) {
		return 2
	}
	digitCount := 0
	for offset+digitCount < len(line) && line[offset+digitCount] >= '0' && line[offset+digitCount] <= '9' {
		digitCount++
	}
	if digitCount == 0 || digitCount > 9 || offset+digitCount+1 >= len(line) {
		return 0
	}
	if (line[offset+digitCount] != '.' && line[offset+digitCount] != ')') ||
		!isSpaceByte(line[offset+digitCount+1]) {
		return 0
	}
	return digitCount + 2
}

func (parser *closingReferenceParser) maskExcluded(line string) string {
	masked := []byte(line)
	for index := 0; index < len(line); {
		next, ok := parser.maskExcludedAt(line, masked, index)
		if !ok {
			return string(masked)
		}
		index = next
	}
	return string(masked)
}

func (parser *closingReferenceParser) maskExcludedAt(line string, masked []byte, index int) (int, bool) {
	if parser.inlineCodeLength != 0 {
		return parser.maskInlineCode(line, masked, index, parser.inlineCodeLength)
	}
	if parser.htmlComment {
		return parser.maskHTMLComment(line, masked, index)
	}
	if strings.HasPrefix(line[index:], "<!--") {
		return parser.maskHTMLCommentStart(line, masked, index)
	}
	if line[index] != '`' {
		return index + 1, true
	}
	return parser.maskInlineCodeStart(line, masked, index)
}

func (parser *closingReferenceParser) maskInlineCode(line string, masked []byte, index, length int) (int, bool) {
	end, ok := findBacktickRun(line, index, length)
	if !ok {
		maskBytes(masked, index, len(line))
		return len(line), false
	}
	end += length
	maskBytes(masked, index, end)
	parser.inlineCodeLength = 0
	return end, true
}

func (parser *closingReferenceParser) maskInlineCodeStart(line string, masked []byte, index int) (int, bool) {
	run := backtickRun(line, index)
	end, ok := findBacktickRun(line, index+run, run)
	if !ok {
		maskBytes(masked, index, len(line))
		parser.inlineCodeLength = run
		return len(line), false
	}
	end += run
	maskBytes(masked, index, end)
	return end, true
}

func (parser *closingReferenceParser) maskHTMLComment(line string, masked []byte, index int) (int, bool) {
	end := strings.Index(line[index:], "-->")
	if end < 0 {
		maskBytes(masked, index, len(line))
		return len(line), false
	}
	end += index + len("-->")
	maskBytes(masked, index, end)
	parser.htmlComment = false
	return end, true
}

func (parser *closingReferenceParser) maskHTMLCommentStart(line string, masked []byte, index int) (int, bool) {
	start := index + len("<!--")
	end := strings.Index(line[start:], "-->")
	if end < 0 {
		maskBytes(masked, index, len(line))
		parser.htmlComment = true
		return len(line), false
	}
	end += start + len("-->")
	maskBytes(masked, index, end)
	return end, true
}

func findBacktickRun(line string, start, length int) (int, bool) {
	for index := start; index < len(line); index++ {
		if line[index] != '`' {
			continue
		}
		if backtickRun(line, index) == length {
			return index, true
		}
	}
	return 0, false
}

func backtickRun(line string, start int) int {
	length := 0
	for start+length < len(line) && line[start+length] == '`' {
		length++
	}
	return length
}

func maskBytes(data []byte, start, end int) {
	for index := start; index < end; index++ {
		data[index] = ' '
	}
}

func (parser *closingReferenceParser) consumeHTMLBlock(line string) {
	parser.updateHTMLBlock(line)
}

func hasHTMLTag(line string) bool {
	for index := 0; index < len(line); index++ {
		if line[index] != '<' || index+1 >= len(line) {
			continue
		}
		next := line[index+1]
		if isASCIILetter(next) || next == '/' || next == '!' || next == '?' {
			if strings.IndexByte(line[index+1:], '>') >= 0 {
				return true
			}
		}
	}
	return false
}

func (parser *closingReferenceParser) updateHTMLBlock(line string) {
	for index := 0; index < len(line); {
		end, ok := nextHTMLTag(line, index)
		if !ok {
			return
		}
		if parser.htmlRawTag != "" {
			name, closing, _, tagOK := htmlTag(line[index:end])
			if tagOK && closing && name == parser.htmlRawTag {
				parser.closeHTMLTag(name)
			}
			index = end
			continue
		}
		parser.updateHTMLTag(line[index:end])
		index = end
	}
}

func nextHTMLTag(line string, start int) (int, bool) {
	index := strings.IndexByte(line[start:], '<')
	if index < 0 {
		return 0, false
	}
	index += start
	end := strings.IndexByte(line[index+1:], '>')
	if end < 0 {
		return 0, false
	}
	return index + end + 2, true
}

func (parser *closingReferenceParser) updateHTMLTag(text string) {
	name, closing, selfClosing, ok := htmlTag(text)
	if !ok {
		return
	}
	if closing {
		parser.closeHTMLTag(name)
		return
	}
	if selfClosing || isHTMLVoidTag(name) || !isHTMLBlockTag(name) {
		return
	}
	parser.htmlBlockTags = append(parser.htmlBlockTags, name)
	if isHTMLRawBlockTag(name) {
		parser.htmlRawTag = name
	}
}

func (parser *closingReferenceParser) closeHTMLTag(name string) {
	if len(parser.htmlBlockTags) == 0 || parser.htmlBlockTags[len(parser.htmlBlockTags)-1] != name {
		return
	}
	parser.htmlBlockTags = parser.htmlBlockTags[:len(parser.htmlBlockTags)-1]
	if parser.htmlRawTag == name {
		parser.htmlRawTag = ""
	}
}

func htmlTag(text string) (string, bool, bool, bool) {
	if len(text) < 3 || text[0] != '<' || text[len(text)-1] != '>' {
		return "", false, false, false
	}
	index := 1
	closing := false
	if text[index] == '/' {
		closing = true
		index++
	}
	for index < len(text) && isSpaceByte(text[index]) {
		index++
	}
	start := index
	for index < len(text) && (isASCIILetter(text[index]) || text[index] >= '0' && text[index] <= '9' || text[index] == '-') {
		index++
	}
	if start == index {
		return "", false, false, false
	}
	name := strings.ToLower(text[start:index])
	suffix := strings.TrimSpace(text[index : len(text)-1])
	selfClosing := strings.HasSuffix(suffix, "/")
	return name, closing, selfClosing, true
}

func isHTMLBlockTag(name string) bool {
	switch name {
	case "address", "article", "aside", "base", "basefont", "blockquote", "body", "caption", "center", "col",
		"colgroup", "dd", "details", "dialog", "dir", "div", "dl", "dt", "fieldset", "figcaption",
		"figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hr", "html",
		"iframe", "legend", "li", "link", "main", "menu", "menuitem", "nav", "ol", "p", "pre", "script",
		"search", "section", "style", "summary", "table", "tbody", "td", "textarea", "tfoot", "th", "thead",
		"title", "tr", "track", "ul", "xmp":
		return true
	}
	return false
}

func isHTMLRawBlockTag(name string) bool {
	switch name {
	case "pre", "script", "style", "textarea", "xmp":
		return true
	}
	return false
}

func isHTMLVoidTag(name string) bool {
	switch name {
	case "area", "base", "basefont", "br", "col", "embed", "hr", "img", "input", "link", "meta",
		"param", "source", "track", "wbr":
		return true
	}
	return false
}

func parseClosingReferenceLine(line string) []int {
	line = strings.TrimSpace(line)
	line = stripClosingListMarker(line)
	if line == "" {
		return nil
	}
	var numbers []int
	for {
		number, rest, ok := parseClosingReference(line)
		if !ok {
			return nil
		}
		numbers = append(numbers, number)
		line = strings.TrimSpace(rest)
		if line == "" || isClosingTrailingPunctuation(line) {
			return numbers
		}
		if line[0] != ',' {
			return nil
		}
		line = strings.TrimSpace(line[1:])
		if line == "" {
			return nil
		}
	}
}

func parseClosingReference(line string) (int, string, bool) {
	word, rest, ok := consumeClosingKeyword(line)
	if !ok || !isClosingKeyword(word) {
		return 0, "", false
	}
	beforeSpace := len(rest)
	rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	hadSpace := len(rest) != beforeSpace
	hadColon := false
	if strings.HasPrefix(rest, ":") {
		hadColon = true
		rest = strings.TrimLeftFunc(rest[1:], unicode.IsSpace)
	}
	if !hadSpace && !hadColon {
		return 0, "", false
	}
	return consumeClosingIssue(rest)
}

func stripClosingListMarker(line string) string {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && isSpaceByte(line[1]) {
		return strings.TrimSpace(line[2:])
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index != 0 && index+1 < len(line) && (line[index] == '.' || line[index] == ')') && isSpaceByte(line[index+1]) {
		return strings.TrimSpace(line[index+2:])
	}
	return line
}

func consumeClosingKeyword(line string) (string, string, bool) {
	index := 0
	for index < len(line) && isASCIILetter(line[index]) {
		index++
	}
	if index == 0 {
		return "", "", false
	}
	return strings.ToLower(line[:index]), line[index:], true
}

func isClosingKeyword(word string) bool {
	switch word {
	case "close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved":
		return true
	}
	return false
}

func consumeClosingIssue(line string) (int, string, bool) {
	if strings.HasPrefix(line, closingReferenceRepository+"#") {
		line = strings.TrimPrefix(line, closingReferenceRepository)
	}
	if len(line) == 0 || line[0] != '#' {
		return 0, "", false
	}
	index := 1
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == 1 {
		return 0, "", false
	}
	number, err := strconv.Atoi(line[1:index])
	if err != nil || number < 1 {
		return 0, "", false
	}
	return number, line[index:], true
}

func isClosingTrailingPunctuation(line string) bool {
	if line == "" {
		return false
	}
	for _, value := range line {
		if value != '.' && value != '!' && value != '?' {
			return false
		}
	}
	return true
}

func isSpaceByte(value byte) bool {
	return value == ' ' || value == '\t'
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
