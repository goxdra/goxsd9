package workflowctl

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

type documentHeadingLink struct {
	source   string
	line     int
	target   string
	fragment string
}

type documentHeadingScan struct {
	anchors map[string]bool
	links   []documentHeadingLink
}

type documentHeadingParser struct {
	scan         documentHeadingScan
	counts       map[string]int
	fenceChar    byte
	fenceLength  int
	fenceContext markdownFenceContext
	htmlComment  bool
}

// checkDocumentHeadingLinks checks the bounded inline-link and ATX-heading subset
// documented for workflowctl docs check. Rules supply both traversal and target scope.
func checkDocumentHeadingLinks(root string, rules []documentRule) error {
	scans := make([]documentHeadingScan, 0, len(rules))
	byPath := make(map[string]int, len(rules))
	for _, rule := range rules {
		// #nosec G304 -- rules are the fixed, validated managed-document registry.
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rule.path)))
		if err != nil {
			return fmt.Errorf("read heading links in %s: %w", rule.path, err)
		}
		byPath[rule.path] = len(scans)
		scans = append(scans, scanDocumentHeadingLinks(rule.path, string(contents)))
	}
	for _, scan := range scans {
		for _, link := range scan.links {
			targetIndex, ok := byPath[link.target]
			if !ok {
				continue
			}
			fragment, err := url.PathUnescape(link.fragment)
			if err != nil {
				return fmt.Errorf("DOCS002 %s:%d: invalid heading fragment %q in %s: %w", link.source, link.line, "#"+link.fragment, link.target, err)
			}
			if scans[targetIndex].anchors[fragment] {
				continue
			}
			return fmt.Errorf("DOCS001 %s:%d: heading fragment %q not found in %s", link.source, link.line, "#"+link.fragment, link.target)
		}
	}
	return nil
}

func scanDocumentHeadingLinks(source, contents string) documentHeadingScan {
	parser := documentHeadingParser{
		scan:   documentHeadingScan{anchors: make(map[string]bool)},
		counts: make(map[string]int),
	}
	for index, line := range strings.Split(contents, "\n") {
		visible, ok := parser.visibleLine(strings.TrimSuffix(line, "\r"))
		if !ok {
			continue
		}
		if heading, ok := documentATXHeading(visible); ok {
			parser.addHeading(heading)
		}
		visible = maskDocumentCodeSpans(visible)
		for _, destination := range documentInlineDestinations(visible) {
			target, fragment, ok := documentHeadingDestination(source, destination)
			if !ok {
				continue
			}
			parser.scan.links = append(parser.scan.links, documentHeadingLink{source: source, line: index + 1, target: target, fragment: fragment})
		}
	}
	return parser.scan
}

func (parser *documentHeadingParser) visibleLine(line string) (string, bool) {
	if parser.fenceChar != 0 {
		if isClosingFenceInContext(line, parser.fenceChar, parser.fenceLength, parser.fenceContext) {
			parser.fenceChar = 0
			parser.fenceLength = 0
			parser.fenceContext = markdownFenceContext{}
		}
		return "", false
	}
	if parser.htmlComment {
		visible := maskDocumentComments(line, &parser.htmlComment)
		if isIndentedCode(line) {
			return "", false
		}
		return visible, true
	}
	if isIndentedCode(line) {
		return "", false
	}
	if char, length, ok := openingFence(line); ok {
		parser.fenceChar = char
		parser.fenceLength = length
		_, parser.fenceContext, _ = markdownFencePrefix(line)
		return "", false
	}
	return maskDocumentComments(line, &parser.htmlComment), true
}

func (parser *documentHeadingParser) addHeading(heading string) {
	base := documentHeadingSlug(heading)
	if base == "" {
		return
	}
	anchor := base
	for parser.scan.anchors[anchor] {
		parser.counts[base]++
		anchor = fmt.Sprintf("%s-%d", base, parser.counts[base])
	}
	parser.scan.anchors[anchor] = true
}

func documentATXHeading(line string) (string, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return "", false
	}
	line = line[indent:]
	marks := 0
	for marks < len(line) && line[marks] == '#' {
		marks++
	}
	if marks == 0 || marks > 6 {
		return "", false
	}
	if marks < len(line) && line[marks] != ' ' && line[marks] != '\t' {
		return "", false
	}
	text := strings.TrimSpace(line[marks:])
	// Closing hashes require whitespace before the run.
	withoutClosing := strings.TrimRight(text, "#")
	if withoutClosing != text && strings.TrimSpace(withoutClosing) != withoutClosing {
		text = strings.TrimSpace(withoutClosing)
	}
	return text, true
}

func documentHeadingSlug(heading string) string {
	var slug strings.Builder
	for _, value := range strings.TrimSpace(heading) {
		switch {
		case unicode.IsLetter(value), unicode.IsDigit(value), unicode.IsMark(value):
			slug.WriteRune(unicode.ToLower(value))
		case value == '-' || value == '_':
			slug.WriteRune(value)
		case unicode.IsSpace(value):
			slug.WriteByte('-')
		}
	}
	return slug.String()
}

func documentHeadingDestination(source, destination string) (string, string, bool) {
	before, fragment, found := strings.Cut(destination, "#")
	if !found || fragment == "" || strings.ContainsAny(destination, " \t<>\n") {
		return "", "", false
	}
	if before == "" {
		return source, fragment, true
	}
	if path.IsAbs(before) || strings.ContainsAny(before, "?:%") || !strings.EqualFold(path.Ext(before), ".md") {
		return "", "", false
	}
	return path.Clean(path.Join(path.Dir(source), before)), fragment, true
}

func documentInlineDestinations(line string) []string {
	var destinations []string
	for index := 0; index < len(line); index++ {
		if line[index] != '[' || documentEscaped(line, index) || (index > 0 && line[index-1] == '!') {
			continue
		}
		destination, end, ok := documentInlineDestinationAt(line, index)
		if !ok {
			continue
		}
		destinations = append(destinations, destination)
		index = end
	}
	return destinations
}

func documentInlineDestinationAt(line string, start int) (string, int, bool) {
	closeLabel := start + 1
	for closeLabel < len(line) && line[closeLabel] != ']' && line[closeLabel] != '[' {
		closeLabel++
	}
	if closeLabel+1 >= len(line) || line[closeLabel] != ']' || line[closeLabel+1] != '(' {
		return "", 0, false
	}
	closeTarget := closeLabel + 2
	for closeTarget < len(line) && line[closeTarget] != ')' && line[closeTarget] != '(' {
		closeTarget++
	}
	if closeTarget >= len(line) || line[closeTarget] != ')' {
		return "", 0, false
	}
	return line[closeLabel+2 : closeTarget], closeTarget, true
}

func documentEscaped(line string, index int) bool {
	slashes := 0
	for index > 0 && line[index-1] == '\\' {
		slashes++
		index--
	}
	return slashes%2 != 0
}

func maskDocumentCodeSpans(line string) string {
	masked := []byte(line)
	for index := 0; index < len(line); {
		if line[index] != '`' || documentEscaped(line, index) {
			index++
			continue
		}
		openingEnd := documentBacktickRunEnd(line, index)
		closing := documentCodeSpanEnd(line, openingEnd, openingEnd-index)
		if closing == -1 {
			index = openingEnd
			continue
		}
		for offset := index; offset < closing; offset++ {
			masked[offset] = ' '
		}
		index = closing
	}
	return string(masked)
}

func documentCodeSpanEnd(line string, start, length int) int {
	for candidate := start; candidate < len(line); candidate++ {
		if line[candidate] != '`' || documentEscaped(line, candidate) {
			continue
		}
		after := documentBacktickRunEnd(line, candidate)
		if after-candidate == length {
			return after
		}
		candidate = after - 1
	}
	return -1
}

func documentBacktickRunEnd(line string, start int) int {
	for start < len(line) && line[start] == '`' {
		start++
	}
	return start
}

func maskDocumentComments(line string, inComment *bool) string {
	masked := []byte(line)
	search := maskDocumentCodeSpans(line)
	for index := 0; index < len(line); {
		if *inComment {
			end := strings.Index(line[index:], "-->")
			if end < 0 {
				for offset := index; offset < len(line); offset++ {
					masked[offset] = ' '
				}
				break
			}
			end += index + 3
			for offset := index; offset < end; offset++ {
				masked[offset] = ' '
			}
			*inComment = false
			index = end
			continue
		}
		start := strings.Index(search[index:], "<!--")
		if start < 0 {
			break
		}
		index += start
		*inComment = true
	}
	return string(masked)
}
