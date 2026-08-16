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

// Generate downloads, verifies, and converts one manifest entry.
func Generate(ctx context.Context, client *http.Client, entry Entry) (Document, error) {
	raw, err := Fetch(ctx, client, entry)
	if err != nil {
		return Document{}, err
	}
	converted, err := convert(entry, raw)
	if err != nil {
		return Document{}, err
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
	case "xml":
		return append([]byte(nil), raw...), nil
	case "html-cdata-pre":
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
	case manifestHTMLRepresentation:
		return append([]byte(nil), raw...), nil
	default:
		return nil, corpusError("specs.conversion.representation", entry.ID, entry.URL,
			fmt.Errorf("unsupported representation %q", entry.Representation))
	}
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
