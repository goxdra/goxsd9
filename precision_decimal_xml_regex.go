package goxsd9

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
	"unicode/utf8"
)

type precisionDecimalXMLRegex struct {
	branches []precisionDecimalXMLRegexBranch
}

type precisionDecimalXMLRegexBranch struct {
	pieces []precisionDecimalXMLRegexPiece
}

type precisionDecimalXMLRegexPiece struct {
	atom precisionDecimalXMLRegexAtom
	min  *big.Int
	max  *big.Int
}

type precisionDecimalXMLRegexAtom struct {
	kind    precisionDecimalXMLRegexAtomKind
	literal rune
	set     precisionDecimalXMLCharSet
	group   *precisionDecimalXMLRegex
}

type precisionDecimalXMLRegexAtomKind uint8

const (
	precisionDecimalXMLRegexLiteralAtom precisionDecimalXMLRegexAtomKind = iota + 1
	precisionDecimalXMLRegexSetAtom
	precisionDecimalXMLRegexGroupAtom
)

type precisionDecimalXMLCharSet struct {
	contains func(rune) bool
}

type precisionDecimalXMLRegexParser struct {
	input []rune
	pos   int
}

type precisionDecimalXMLRegexMatchState struct {
	piece int
	pos   int
}

type precisionDecimalXMLBlock struct {
	name string
	low  rune
	high rune
}

func parsePrecisionDecimalXMLRegex(source string) (*precisionDecimalXMLRegex, error) {
	if !utf8.ValidString(source) {
		return nil, errors.New("pattern is not valid UTF-8")
	}
	parser := precisionDecimalXMLRegexParser{input: []rune(source)}
	regex, err := parser.parseRegex(false)
	if err != nil {
		return nil, err
	}
	if parser.pos != len(parser.input) {
		return nil, fmt.Errorf("unexpected pattern character at position %d", parser.pos)
	}
	return regex, nil
}

func (parser *precisionDecimalXMLRegexParser) parseRegex(inGroup bool) (*precisionDecimalXMLRegex, error) {
	branches := make([]precisionDecimalXMLRegexBranch, 0, 1)
	for {
		branch, err := parser.parseBranch()
		if err != nil {
			return nil, err
		}
		branches = append(branches, branch)
		if parser.atEnd() {
			if inGroup {
				return nil, errors.New("unterminated group")
			}
			return &precisionDecimalXMLRegex{branches: branches}, nil
		}
		if parser.peek() == '|' {
			parser.pos++
			continue
		}
		if parser.peek() == ')' {
			if !inGroup {
				return nil, errors.New("unmatched closing parenthesis")
			}
			return &precisionDecimalXMLRegex{branches: branches}, nil
		}
		return nil, fmt.Errorf("unexpected pattern character at position %d", parser.pos)
	}
}

func (parser *precisionDecimalXMLRegexParser) parseBranch() (precisionDecimalXMLRegexBranch, error) {
	branch := precisionDecimalXMLRegexBranch{}
	for !parser.atEnd() {
		next := parser.peek()
		if next == '|' || next == ')' {
			break
		}
		piece, err := parser.parsePiece()
		if err != nil {
			return precisionDecimalXMLRegexBranch{}, err
		}
		branch.pieces = append(branch.pieces, piece)
	}
	return branch, nil
}

func (parser *precisionDecimalXMLRegexParser) parsePiece() (precisionDecimalXMLRegexPiece, error) {
	atom, err := parser.parseAtom()
	if err != nil {
		return precisionDecimalXMLRegexPiece{}, err
	}
	piece := precisionDecimalXMLRegexPiece{atom: atom, min: big.NewInt(1), max: big.NewInt(1)}
	if parser.atEnd() {
		return piece, nil
	}
	switch parser.peek() {
	case '?':
		parser.pos++
		piece.min = big.NewInt(0)
		piece.max = big.NewInt(1)
	case '*':
		parser.pos++
		piece.min = big.NewInt(0)
		piece.max = nil
	case '+':
		parser.pos++
		piece.min = big.NewInt(1)
		piece.max = nil
	case '{':
		if err := parser.parseBracedQuantifier(&piece); err != nil {
			return precisionDecimalXMLRegexPiece{}, err
		}
	}
	return piece, nil
}

func (parser *precisionDecimalXMLRegexParser) parseAtom() (precisionDecimalXMLRegexAtom, error) {
	if parser.atEnd() {
		return precisionDecimalXMLRegexAtom{}, errors.New("missing pattern atom")
	}
	next := parser.peek()
	parser.pos++
	switch next {
	case '(':
		group, err := parser.parseRegex(true)
		if err != nil {
			return precisionDecimalXMLRegexAtom{}, err
		}
		if parser.atEnd() || parser.peek() != ')' {
			return precisionDecimalXMLRegexAtom{}, errors.New("unterminated group")
		}
		parser.pos++
		return precisionDecimalXMLRegexAtom{kind: precisionDecimalXMLRegexGroupAtom, group: group}, nil
	case '[':
		parser.pos--
		set, err := parser.parseCharacterClass()
		if err != nil {
			return precisionDecimalXMLRegexAtom{}, err
		}
		return precisionDecimalXMLRegexAtom{kind: precisionDecimalXMLRegexSetAtom, set: set}, nil
	case '\\':
		return parser.parseEscape()
	case '.':
		return precisionDecimalXMLRegexAtom{
			kind: precisionDecimalXMLRegexSetAtom,
			set:  precisionDecimalXMLCharSet{contains: precisionDecimalXMLWildcard},
		}, nil
	case '|', ')', '?', '*', '+', '{', '}', ']':
		return precisionDecimalXMLRegexAtom{}, fmt.Errorf("unexpected metacharacter %q", next)
	default:
		if !precisionDecimalXMLChar(next) {
			return precisionDecimalXMLRegexAtom{}, fmt.Errorf("pattern literal %U is not an XML character", next)
		}
		return precisionDecimalXMLRegexAtom{kind: precisionDecimalXMLRegexLiteralAtom, literal: next}, nil
	}
}

func (parser *precisionDecimalXMLRegexParser) parseEscape() (precisionDecimalXMLRegexAtom, error) {
	set, literal, single, err := parser.parseEscapeSet()
	if err != nil {
		return precisionDecimalXMLRegexAtom{}, err
	}
	if single {
		return precisionDecimalXMLRegexAtom{kind: precisionDecimalXMLRegexLiteralAtom, literal: literal}, nil
	}
	return precisionDecimalXMLRegexAtom{kind: precisionDecimalXMLRegexSetAtom, set: set}, nil
}

func (parser *precisionDecimalXMLRegexParser) parseEscapeSet() (precisionDecimalXMLCharSet, rune, bool, error) {
	if parser.atEnd() {
		return precisionDecimalXMLCharSet{}, 0, false, errors.New("pattern ends after escape")
	}
	next := parser.peek()
	parser.pos++
	if set, literal, single, ok := precisionDecimalXMLSimpleEscape(next); ok {
		return set, literal, single, nil
	}
	if next == 'p' || next == 'P' {
		return parser.parseUnicodePropertyEscape(next)
	}
	return precisionDecimalXMLCharSet{}, 0, false, fmt.Errorf("invalid pattern escape \\%c", next)
}

func precisionDecimalXMLSimpleEscape(next rune) (precisionDecimalXMLCharSet, rune, bool, bool) {
	switch next {
	case 'n':
		return precisionDecimalXMLLiteralSet('\n'), '\n', true, true
	case 'r':
		return precisionDecimalXMLLiteralSet('\r'), '\r', true, true
	case 't':
		return precisionDecimalXMLLiteralSet('\t'), '\t', true, true
	case '\\', '.', '?', '*', '+', '{', '}', '(', ')', '|', '[', ']', '-', '^':
		return precisionDecimalXMLLiteralSet(next), next, true, true
	case 's':
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLWhitespace}, 0, false, true
	case 'S':
		return precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && !precisionDecimalXMLWhitespace(value)
		}}, 0, false, true
	case 'i':
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLNameStartChar}, 0, false, true
	case 'I':
		return precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && !precisionDecimalXMLNameStartChar(value)
		}}, 0, false, true
	case 'c':
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLNameChar}, 0, false, true
	case 'C':
		return precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && !precisionDecimalXMLNameChar(value)
		}}, 0, false, true
	case 'd':
		return precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && unicode.Is(unicode.Nd, value)
		}}, 0, false, true
	case 'D':
		return precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && !unicode.Is(unicode.Nd, value)
		}}, 0, false, true
	case 'w':
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLWord}, 0, false, true
	case 'W':
		return precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && !precisionDecimalXMLWord(value)
		}}, 0, false, true
	default:
		return precisionDecimalXMLCharSet{}, 0, false, false
	}
}

func (parser *precisionDecimalXMLRegexParser) parseUnicodePropertyEscape(prefix rune) (precisionDecimalXMLCharSet, rune, bool, error) {
	if parser.atEnd() || parser.peek() != '{' {
		return precisionDecimalXMLCharSet{}, 0, false, errors.New("unicode property escape lacks a braced name")
	}
	parser.pos++
	start := parser.pos
	for !parser.atEnd() && parser.peek() != '}' {
		parser.pos++
	}
	if parser.atEnd() {
		return precisionDecimalXMLCharSet{}, 0, false, errors.New("unterminated Unicode property escape")
	}
	name := string(parser.input[start:parser.pos])
	parser.pos++
	set, ok := precisionDecimalXMLPropertySet(name)
	if !ok {
		return precisionDecimalXMLCharSet{}, 0, false, fmt.Errorf("unknown Unicode property %q", name)
	}
	if prefix == 'P' {
		base := set
		set = precisionDecimalXMLCharSet{contains: func(value rune) bool {
			return precisionDecimalXMLChar(value) && !base.contains(value)
		}}
	}
	return set, 0, false, nil
}

func (parser *precisionDecimalXMLRegexParser) parseBracedQuantifier(piece *precisionDecimalXMLRegexPiece) error {
	parser.pos++
	start := parser.pos
	for !parser.atEnd() && parser.peek() >= '0' && parser.peek() <= '9' {
		parser.pos++
	}
	if start == parser.pos {
		return errors.New("braced quantifier has no lower bound")
	}
	minimum, ok := new(big.Int).SetString(string(parser.input[start:parser.pos]), 10)
	if !ok {
		return errors.New("invalid braced quantifier lower bound")
	}
	if parser.atEnd() {
		return errors.New("unterminated braced quantifier")
	}
	if parser.peek() == '}' {
		parser.pos++
		piece.min = minimum
		piece.max = new(big.Int).Set(minimum)
		return nil
	}
	if parser.peek() != ',' {
		return errors.New("invalid braced quantifier separator")
	}
	parser.pos++
	if parser.atEnd() {
		return errors.New("unterminated braced quantifier")
	}
	if parser.peek() == '}' {
		parser.pos++
		piece.min = minimum
		piece.max = nil
		return nil
	}
	maxStart := parser.pos
	for !parser.atEnd() && parser.peek() >= '0' && parser.peek() <= '9' {
		parser.pos++
	}
	if maxStart == parser.pos || parser.atEnd() || parser.peek() != '}' {
		return errors.New("invalid braced quantifier upper bound")
	}
	maximum, ok := new(big.Int).SetString(string(parser.input[maxStart:parser.pos]), 10)
	if !ok || maximum.Cmp(minimum) < 0 {
		return errors.New("braced quantifier upper bound is below its lower bound")
	}
	parser.pos++
	piece.min = minimum
	piece.max = maximum
	return nil
}

func (parser *precisionDecimalXMLRegexParser) parseCharacterClass() (precisionDecimalXMLCharSet, error) {
	if parser.atEnd() || parser.peek() != '[' {
		return precisionDecimalXMLCharSet{}, errors.New("character class must start with [")
	}
	parser.pos++
	negative := parser.consumeCharacterClassNegation()
	parts := make([]precisionDecimalXMLCharSet, 0, 1)
	for {
		if parser.atEnd() {
			return precisionDecimalXMLCharSet{}, errors.New("unterminated character class")
		}
		if parser.peek() == ']' {
			if len(parts) == 0 {
				return precisionDecimalXMLCharSet{}, errors.New("empty character class")
			}
			parser.pos++
			return precisionDecimalXMLClassSet(parts, negative), nil
		}
		if precisionDecimalXMLCharacterClassSubtractionStart(parser, parts) {
			return parser.parseCharacterClassSubtraction(parts, negative)
		}
		part, err := parser.parseCharacterClassMember()
		if err != nil {
			return precisionDecimalXMLCharSet{}, err
		}
		parts = append(parts, part)
	}
}

func (parser *precisionDecimalXMLRegexParser) consumeCharacterClassNegation() bool {
	if parser.atEnd() || parser.peek() != '^' {
		return false
	}
	parser.pos++
	return true
}

func precisionDecimalXMLCharacterClassSubtractionStart(parser *precisionDecimalXMLRegexParser, parts []precisionDecimalXMLCharSet) bool {
	return len(parts) != 0 && parser.peek() == '-' && parser.pos+1 < len(parser.input) && parser.input[parser.pos+1] == '['
}

func (parser *precisionDecimalXMLRegexParser) parseCharacterClassSubtraction(parts []precisionDecimalXMLCharSet, negative bool) (precisionDecimalXMLCharSet, error) {
	parser.pos++
	subtraction, err := parser.parseCharacterClass()
	if err != nil {
		return precisionDecimalXMLCharSet{}, err
	}
	if parser.atEnd() || parser.peek() != ']' {
		return precisionDecimalXMLCharSet{}, errors.New("character class subtraction must be last")
	}
	parser.pos++
	base := precisionDecimalXMLClassSet(parts, negative)
	return precisionDecimalXMLCharSet{contains: func(value rune) bool {
		return base.contains(value) && !subtraction.contains(value)
	}}, nil
}

func (parser *precisionDecimalXMLRegexParser) parseCharacterClassMember() (precisionDecimalXMLCharSet, error) {
	part, literal, single, err := parser.parseCharacterClassPart()
	if err != nil {
		return precisionDecimalXMLCharSet{}, err
	}
	if !single || !precisionDecimalXMLCharacterClassRangeStart(parser) {
		return part, nil
	}
	parser.pos++
	_, endpointLiteral, endpointSingle, err := parser.parseCharacterClassPart()
	if err != nil {
		return precisionDecimalXMLCharSet{}, err
	}
	if !endpointSingle {
		return precisionDecimalXMLCharSet{}, errors.New("character class range endpoint is not a single character")
	}
	if literal > endpointLiteral {
		return precisionDecimalXMLCharSet{}, errors.New("character class range is reversed")
	}
	return precisionDecimalXMLRangeSet(literal, endpointLiteral), nil
}

func precisionDecimalXMLCharacterClassRangeStart(parser *precisionDecimalXMLRegexParser) bool {
	if parser.atEnd() || parser.peek() != '-' || parser.pos+1 >= len(parser.input) {
		return false
	}
	next := parser.input[parser.pos+1]
	return next != ']' && next != '['
}

func (parser *precisionDecimalXMLRegexParser) parseCharacterClassPart() (precisionDecimalXMLCharSet, rune, bool, error) {
	if parser.atEnd() {
		return precisionDecimalXMLCharSet{}, 0, false, errors.New("missing character class member")
	}
	next := parser.peek()
	parser.pos++
	if next == '\\' {
		return parser.parseEscapeSet()
	}
	if next == '[' || next == ']' {
		return precisionDecimalXMLCharSet{}, 0, false, fmt.Errorf("unescaped %c is not allowed in a character class", next)
	}
	if !precisionDecimalXMLChar(next) {
		return precisionDecimalXMLCharSet{}, 0, false, fmt.Errorf("character class literal %U is not an XML character", next)
	}
	return precisionDecimalXMLLiteralSet(next), next, true, nil
}

func (parser *precisionDecimalXMLRegexParser) atEnd() bool {
	return parser.pos >= len(parser.input)
}

func (parser *precisionDecimalXMLRegexParser) peek() rune {
	return parser.input[parser.pos]
}

func precisionDecimalXMLLiteralSet(literal rune) precisionDecimalXMLCharSet {
	return precisionDecimalXMLCharSet{contains: func(value rune) bool {
		return value == literal
	}}
}

func precisionDecimalXMLRangeSet(low, high rune) precisionDecimalXMLCharSet {
	return precisionDecimalXMLCharSet{contains: func(value rune) bool {
		return value >= low && value <= high
	}}
}

func precisionDecimalXMLClassSet(parts []precisionDecimalXMLCharSet, negative bool) precisionDecimalXMLCharSet {
	return precisionDecimalXMLCharSet{contains: func(value rune) bool {
		if !precisionDecimalXMLChar(value) {
			return false
		}
		matched := false
		for _, part := range parts {
			if part.contains(value) {
				matched = true
				break
			}
		}
		if negative {
			return !matched
		}
		return matched
	}}
}

func precisionDecimalXMLPropertySet(name string) (precisionDecimalXMLCharSet, bool) {
	if strings.HasPrefix(name, "Is") {
		block, found := precisionDecimalXMLBlockSet(name[2:])
		if found {
			return block, true
		}
		return precisionDecimalXMLCharSet{}, false
	}
	if table, ok := unicode.Categories[name]; ok {
		return precisionDecimalXMLCategorySet(table), true
	}
	if alias, ok := unicode.CategoryAliases[name]; ok {
		table, found := unicode.Categories[alias]
		if found {
			return precisionDecimalXMLCategorySet(table), true
		}
	}
	return precisionDecimalXMLCharSet{}, false
}

func precisionDecimalXMLCategorySet(table *unicode.RangeTable) precisionDecimalXMLCharSet {
	return precisionDecimalXMLCharSet{contains: func(value rune) bool {
		return precisionDecimalXMLChar(value) && unicode.Is(table, value)
	}}
}

func precisionDecimalXMLBlockSet(name string) (precisionDecimalXMLCharSet, bool) {
	normalized := precisionDecimalXMLNormalizeBlockName(name)
	for _, block := range precisionDecimalXMLBlocks {
		if precisionDecimalXMLNormalizeBlockName(block.name) != normalized {
			continue
		}
		return precisionDecimalXMLRangeSet(block.low, block.high), true
	}
	return precisionDecimalXMLCharSet{}, false
}

func precisionDecimalXMLNormalizeBlockName(name string) string {
	var builder strings.Builder
	for _, value := range name {
		if value == '_' || value == '-' || value == ' ' {
			continue
		}
		builder.WriteRune(value)
	}
	return strings.ToLower(builder.String())
}

var precisionDecimalXMLBlocks = []precisionDecimalXMLBlock{
	{name: "basiclatin", low: 0x0000, high: 0x007F},
	{name: "latin-1supplement", low: 0x0080, high: 0x00FF},
	{name: "latinextended-a", low: 0x0100, high: 0x017F},
	{name: "latinextended-b", low: 0x0180, high: 0x024F},
	{name: "ipaextensions", low: 0x0250, high: 0x02AF},
	{name: "spacingmodifierletters", low: 0x02B0, high: 0x02FF},
	{name: "combiningdiacriticalmarks", low: 0x0300, high: 0x036F},
	{name: "greekandcoptic", low: 0x0370, high: 0x03FF},
	{name: "cyrillic", low: 0x0400, high: 0x04FF},
	{name: "cyrillicsupplement", low: 0x0500, high: 0x052F},
	{name: "armenian", low: 0x0530, high: 0x058F},
	{name: "hebrew", low: 0x0590, high: 0x05FF},
	{name: "arabic", low: 0x0600, high: 0x06FF},
	{name: "syriac", low: 0x0700, high: 0x074F},
	{name: "arabicsupplement", low: 0x0750, high: 0x077F},
	{name: "thaana", low: 0x0780, high: 0x07BF},
	{name: "nko", low: 0x07C0, high: 0x07FF},
	{name: "samaritan", low: 0x0800, high: 0x083F},
	{name: "mandaic", low: 0x0840, high: 0x085F},
	{name: "syriacsupplement", low: 0x0860, high: 0x086F},
	{name: "arabicextended-b", low: 0x0870, high: 0x089F},
	{name: "arabicextended-a", low: 0x08A0, high: 0x08FF},
	{name: "devanagari", low: 0x0900, high: 0x097F},
	{name: "bengali", low: 0x0980, high: 0x09FF},
	{name: "gurmukhi", low: 0x0A00, high: 0x0A7F},
	{name: "gujarati", low: 0x0A80, high: 0x0AFF},
	{name: "oriya", low: 0x0B00, high: 0x0B7F},
	{name: "tamil", low: 0x0B80, high: 0x0BFF},
	{name: "telugu", low: 0x0C00, high: 0x0C7F},
	{name: "kannada", low: 0x0C80, high: 0x0CFF},
	{name: "malayalam", low: 0x0D00, high: 0x0D7F},
	{name: "sinhala", low: 0x0D80, high: 0x0DFF},
	{name: "thai", low: 0x0E00, high: 0x0E7F},
	{name: "lao", low: 0x0E80, high: 0x0EFF},
	{name: "tibetan", low: 0x0F00, high: 0x0FFF},
	{name: "myanmar", low: 0x1000, high: 0x109F},
	{name: "georgian", low: 0x10A0, high: 0x10FF},
	{name: "hanguljamo", low: 0x1100, high: 0x11FF},
	{name: "ethiopic", low: 0x1200, high: 0x137F},
	{name: "ethiopicsupplement", low: 0x1380, high: 0x139F},
	{name: "cherokee", low: 0x13A0, high: 0x13FF},
	{name: "unifiedcanadianaboriginalsyllabics", low: 0x1400, high: 0x167F},
	{name: "ogham", low: 0x1680, high: 0x169F},
	{name: "runic", low: 0x16A0, high: 0x16FF},
	{name: "tagalog", low: 0x1700, high: 0x171F},
	{name: "hanunoo", low: 0x1720, high: 0x173F},
	{name: "buhid", low: 0x1740, high: 0x175F},
	{name: "tagbanwa", low: 0x1760, high: 0x177F},
	{name: "khmer", low: 0x1780, high: 0x17FF},
	{name: "mongolian", low: 0x1800, high: 0x18AF},
	{name: "unifiedcanadianaboriginalsyllabicsextended", low: 0x18B0, high: 0x18FF},
	{name: "limbu", low: 0x1900, high: 0x194F},
	{name: "taile", low: 0x1950, high: 0x197F},
	{name: "newtailue", low: 0x1980, high: 0x19DF},
	{name: "khmersymbols", low: 0x19E0, high: 0x19FF},
	{name: "buginese", low: 0x1A00, high: 0x1A1F},
	{name: "taitham", low: 0x1A20, high: 0x1AAF},
	{name: "combiningdiacriticalmarksextended", low: 0x1AB0, high: 0x1AFF},
	{name: "balinese", low: 0x1B00, high: 0x1B7F},
	{name: "sundanese", low: 0x1B80, high: 0x1BBF},
	{name: "batak", low: 0x1BC0, high: 0x1BFF},
	{name: "lepcha", low: 0x1C00, high: 0x1C4F},
	{name: "olchiki", low: 0x1C50, high: 0x1C7F},
	{name: "cyrillicextended-c", low: 0x1C80, high: 0x1C8F},
	{name: "georgianextended", low: 0x1C90, high: 0x1CBF},
	{name: "sundanesesupplement", low: 0x1CC0, high: 0x1CCF},
	{name: "vedicextensions", low: 0x1CD0, high: 0x1CFF},
	{name: "phoneticextensions", low: 0x1D00, high: 0x1D7F},
	{name: "phoneticextensionssupplement", low: 0x1D80, high: 0x1DBF},
	{name: "combiningdiacriticalmarkssupplement", low: 0x1DC0, high: 0x1DFF},
	{name: "latinextendedadditional", low: 0x1E00, high: 0x1EFF},
	{name: "greekextended", low: 0x1F00, high: 0x1FFF},
	{name: "generalpunctuation", low: 0x2000, high: 0x206F},
	{name: "superscriptsandsubscripts", low: 0x2070, high: 0x209F},
	{name: "currencysymbols", low: 0x20A0, high: 0x20CF},
	{name: "combiningdiacriticalmarksforsymbols", low: 0x20D0, high: 0x20FF},
	{name: "letterlikesymbols", low: 0x2100, high: 0x214F},
	{name: "numberforms", low: 0x2150, high: 0x218F},
	{name: "arrows", low: 0x2190, high: 0x21FF},
	{name: "mathematicaloperators", low: 0x2200, high: 0x22FF},
	{name: "miscellaneoustechnical", low: 0x2300, high: 0x23FF},
	{name: "controlpictures", low: 0x2400, high: 0x243F},
	{name: "opticalcharacterrecognition", low: 0x2440, high: 0x245F},
	{name: "enclosedalphanumerics", low: 0x2460, high: 0x24FF},
	{name: "boxdrawing", low: 0x2500, high: 0x257F},
	{name: "blockelements", low: 0x2580, high: 0x259F},
	{name: "geometricshapes", low: 0x25A0, high: 0x25FF},
	{name: "miscellaneoussymbols", low: 0x2600, high: 0x26FF},
	{name: "dingbats", low: 0x2700, high: 0x27BF},
	{name: "miscellaneousmathematicalsymbols-a", low: 0x27C0, high: 0x27EF},
	{name: "supplementalarrows-a", low: 0x27F0, high: 0x27FF},
	{name: "braillepatterns", low: 0x2800, high: 0x28FF},
	{name: "supplementalarrows-b", low: 0x2900, high: 0x297F},
	{name: "miscellaneousmathematicalsymbols-b", low: 0x2980, high: 0x29FF},
	{name: "supplementalmathematicaloperators", low: 0x2A00, high: 0x2AFF},
	{name: "miscellaneoussymbolsandarrows", low: 0x2B00, high: 0x2BFF},
	{name: "glagolitic", low: 0x2C00, high: 0x2C5F},
	{name: "latinextended-c", low: 0x2C60, high: 0x2C7F},
	{name: "coptic", low: 0x2C80, high: 0x2CFF},
	{name: "georgiansupplement", low: 0x2D00, high: 0x2D2F},
	{name: "tifinagh", low: 0x2D30, high: 0x2D7F},
	{name: "ethiopicextended", low: 0x2D80, high: 0x2DDF},
	{name: "cyrillicextended-a", low: 0x2DE0, high: 0x2DFF},
	{name: "supplementalpunctuation", low: 0x2E00, high: 0x2E7F},
	{name: "cjkradicalssupplement", low: 0x2E80, high: 0x2EFF},
	{name: "kangxiradicals", low: 0x2F00, high: 0x2FDF},
	{name: "ideographicdescriptioncharacters", low: 0x2FF0, high: 0x2FFF},
	{name: "cjksymbolsandpunctuation", low: 0x3000, high: 0x303F},
	{name: "hiragana", low: 0x3040, high: 0x309F},
	{name: "katakana", low: 0x30A0, high: 0x30FF},
	{name: "bopomofo", low: 0x3100, high: 0x312F},
	{name: "hangulcompatibilityjamo", low: 0x3130, high: 0x318F},
	{name: "kanbun", low: 0x3190, high: 0x319F},
	{name: "bopomofoextended", low: 0x31A0, high: 0x31BF},
	{name: "cjkstrokes", low: 0x31C0, high: 0x31EF},
	{name: "katakanaphoneticextensions", low: 0x31F0, high: 0x31FF},
	{name: "enclosedcjklettersandmonths", low: 0x3200, high: 0x32FF},
	{name: "cjkcompatibility", low: 0x3300, high: 0x33FF},
	{name: "cjkunifiedideographsextensiona", low: 0x3400, high: 0x4DBF},
	{name: "yijinghexagramsymbols", low: 0x4DC0, high: 0x4DFF},
	{name: "cjkunifiedideographs", low: 0x4E00, high: 0x9FFF},
	{name: "yisyllables", low: 0xA000, high: 0xA48F},
	{name: "yiradicals", low: 0xA490, high: 0xA4CF},
	{name: "lisu", low: 0xA4D0, high: 0xA4FF},
	{name: "vai", low: 0xA500, high: 0xA63F},
	{name: "cyrillicextended-b", low: 0xA640, high: 0xA69F},
	{name: "bamum", low: 0xA6A0, high: 0xA6FF},
	{name: "modifiertoneletters", low: 0xA700, high: 0xA71F},
	{name: "latinextended-d", low: 0xA720, high: 0xA7FF},
	{name: "sylotinagri", low: 0xA800, high: 0xA82F},
	{name: "commonindicnumberforms", low: 0xA830, high: 0xA83F},
	{name: "phags-pa", low: 0xA840, high: 0xA87F},
	{name: "saurashtra", low: 0xA880, high: 0xA8DF},
	{name: "devanagariextended", low: 0xA8E0, high: 0xA8FF},
	{name: "kayahli", low: 0xA900, high: 0xA92F},
	{name: "rejang", low: 0xA930, high: 0xA95F},
	{name: "hanguljamoextended-a", low: 0xA960, high: 0xA97F},
	{name: "javanese", low: 0xA980, high: 0xA9DF},
	{name: "myanmarextended-b", low: 0xA9E0, high: 0xA9FF},
	{name: "cham", low: 0xAA00, high: 0xAA5F},
	{name: "myanmarextended-a", low: 0xAA60, high: 0xAA7F},
	{name: "taiviet", low: 0xAA80, high: 0xAADF},
	{name: "meeteimayekextensions", low: 0xAAE0, high: 0xAAFF},
	{name: "ethiopicextended-a", low: 0xAB00, high: 0xAB2F},
	{name: "latinextended-e", low: 0xAB30, high: 0xAB6F},
	{name: "cherokeesupplement", low: 0xAB70, high: 0xABBF},
	{name: "meeteimayek", low: 0xABC0, high: 0xABFF},
	{name: "hangulsyllables", low: 0xAC00, high: 0xD7AF},
	{name: "hanguljamoextended-b", low: 0xD7B0, high: 0xD7FF},
	{name: "highsurrogates", low: 0xD800, high: 0xDB7F},
	{name: "highprivateusesurrogates", low: 0xDB80, high: 0xDBFF},
	{name: "lowsurrogates", low: 0xDC00, high: 0xDFFF},
	{name: "privateusearea", low: 0xE000, high: 0xF8FF},
	{name: "cjkcompatibilityideographs", low: 0xF900, high: 0xFAFF},
	{name: "alphabeticpresentationforms", low: 0xFB00, high: 0xFB4F},
	{name: "arabicpresentationforms-a", low: 0xFB50, high: 0xFDFF},
	{name: "variationselectors", low: 0xFE00, high: 0xFE0F},
	{name: "verticalforms", low: 0xFE10, high: 0xFE1F},
	{name: "combininghalfmarks", low: 0xFE20, high: 0xFE2F},
	{name: "cjkcompatibilityforms", low: 0xFE30, high: 0xFE4F},
	{name: "smallformvariants", low: 0xFE50, high: 0xFE6F},
	{name: "arabicpresentationforms-b", low: 0xFE70, high: 0xFEFF},
	{name: "halfwidthandfullwidthforms", low: 0xFF00, high: 0xFFEF},
	{name: "specials", low: 0xFFF0, high: 0xFFFF},
	{name: "linearbsyllabary", low: 0x10000, high: 0x1007F},
	{name: "linearbideograms", low: 0x10080, high: 0x100FF},
	{name: "aegeannumbers", low: 0x10100, high: 0x1013F},
	{name: "ancientgreeknumbers", low: 0x10140, high: 0x1018F},
	{name: "ancientsymbols", low: 0x10190, high: 0x101CF},
	{name: "phaistosdisc", low: 0x101D0, high: 0x101FF},
	{name: "lycian", low: 0x10280, high: 0x1029F},
	{name: "carian", low: 0x102A0, high: 0x102DF},
	{name: "copticepactnumbers", low: 0x102E0, high: 0x102FF},
	{name: "olditalic", low: 0x10300, high: 0x1032F},
	{name: "gothic", low: 0x10330, high: 0x1034F},
	{name: "oldpermic", low: 0x10350, high: 0x1037F},
	{name: "ugaritic", low: 0x10380, high: 0x1039F},
	{name: "oldpersian", low: 0x103A0, high: 0x103DF},
	{name: "deseret", low: 0x10400, high: 0x1044F},
	{name: "shavian", low: 0x10450, high: 0x1047F},
	{name: "osmanya", low: 0x10480, high: 0x104AF},
	{name: "osage", low: 0x104B0, high: 0x104FF},
	{name: "elbasan", low: 0x10500, high: 0x1052F},
	{name: "caucasianalbanian", low: 0x10530, high: 0x1056F},
	{name: "vithkuqi", low: 0x10570, high: 0x105BF},
	{name: "lineara", low: 0x10600, high: 0x1077F},
	{name: "latinextended-f", low: 0x10780, high: 0x107BF},
	{name: "cypriotsyllabary", low: 0x10800, high: 0x1083F},
	{name: "imperialaramaic", low: 0x10840, high: 0x1085F},
	{name: "palmyrene", low: 0x10860, high: 0x1087F},
	{name: "nabataean", low: 0x10880, high: 0x108AF},
	{name: "hatran", low: 0x108E0, high: 0x108FF},
	{name: "phoenician", low: 0x10900, high: 0x1091F},
	{name: "lydian", low: 0x10920, high: 0x1093F},
	{name: "meroitichieroglyphs", low: 0x10980, high: 0x1099F},
	{name: "meroiticcursive", low: 0x109A0, high: 0x109FF},
	{name: "kharoshthi", low: 0x10A00, high: 0x10A5F},
	{name: "oldsoutharabian", low: 0x10A60, high: 0x10A7F},
	{name: "oldnortharabian", low: 0x10A80, high: 0x10A9F},
	{name: "manichaean", low: 0x10AC0, high: 0x10AFF},
	{name: "avestan", low: 0x10B00, high: 0x10B3F},
	{name: "inscriptionalparthian", low: 0x10B40, high: 0x10B5F},
	{name: "inscriptionalpahlavi", low: 0x10B60, high: 0x10B7F},
	{name: "psalterpahlavi", low: 0x10B80, high: 0x10BAF},
	{name: "oldturkic", low: 0x10C00, high: 0x10C4F},
	{name: "oldhungarian", low: 0x10C80, high: 0x10CFF},
	{name: "hanifirohingya", low: 0x10D00, high: 0x10D3F},
	{name: "ruminumeralsymbols", low: 0x10E60, high: 0x10E7F},
	{name: "yezidi", low: 0x10E80, high: 0x10EBF},
	{name: "arabicextended-c", low: 0x10EC0, high: 0x10EFF},
	{name: "oldsogdian", low: 0x10F00, high: 0x10F2F},
	{name: "sogdian", low: 0x10F30, high: 0x10F6F},
	{name: "olduyghur", low: 0x10F70, high: 0x10FAF},
	{name: "chorasmian", low: 0x10FB0, high: 0x10FDF},
	{name: "elymaic", low: 0x10FE0, high: 0x10FFF},
	{name: "brahmi", low: 0x11000, high: 0x1107F},
	{name: "kaithi", low: 0x11080, high: 0x110CF},
	{name: "sorasompeng", low: 0x110D0, high: 0x110FF},
	{name: "chakma", low: 0x11100, high: 0x1114F},
	{name: "mahajani", low: 0x11150, high: 0x1117F},
	{name: "sharada", low: 0x11180, high: 0x111DF},
	{name: "sinhalaarchaicnumbers", low: 0x111E0, high: 0x111FF},
	{name: "khojki", low: 0x11200, high: 0x1124F},
	{name: "multani", low: 0x11280, high: 0x112AF},
	{name: "khudawadi", low: 0x112B0, high: 0x112FF},
	{name: "grantha", low: 0x11300, high: 0x1137F},
	{name: "newa", low: 0x11400, high: 0x1147F},
	{name: "tirhuta", low: 0x11480, high: 0x114DF},
	{name: "siddham", low: 0x11580, high: 0x115FF},
	{name: "modi", low: 0x11600, high: 0x1165F},
	{name: "mongoliansupplement", low: 0x11660, high: 0x1167F},
	{name: "takri", low: 0x11680, high: 0x116CF},
	{name: "ahom", low: 0x11700, high: 0x1174F},
	{name: "dogra", low: 0x11800, high: 0x1184F},
	{name: "warangciti", low: 0x118A0, high: 0x118FF},
	{name: "divesakuru", low: 0x11900, high: 0x1195F},
	{name: "nandinagari", low: 0x119A0, high: 0x119FF},
	{name: "zanabazarsquare", low: 0x11A00, high: 0x11A4F},
	{name: "soyombo", low: 0x11A50, high: 0x11AAF},
	{name: "unifiedcanadianaboriginalsyllabicsextended-a", low: 0x11AB0, high: 0x11ABF},
	{name: "paucinhau", low: 0x11AC0, high: 0x11AFF},
	{name: "devanagariextended-a", low: 0x11B00, high: 0x11B5F},
	{name: "bhaiksuki", low: 0x11C00, high: 0x11C6F},
	{name: "marchen", low: 0x11C70, high: 0x11CBF},
	{name: "masaramgondi", low: 0x11D00, high: 0x11D5F},
	{name: "gunjalagondi", low: 0x11D60, high: 0x11DAF},
	{name: "makasar", low: 0x11EE0, high: 0x11EFF},
	{name: "kawi", low: 0x11F00, high: 0x11F5F},
	{name: "lisusupplement", low: 0x11FB0, high: 0x11FBF},
	{name: "tamilsupplement", low: 0x11FC0, high: 0x11FFF},
	{name: "cuneiform", low: 0x12000, high: 0x123FF},
	{name: "cuneiformnumbersandpunctuation", low: 0x12400, high: 0x1247F},
	{name: "earlydynasticcuneiform", low: 0x12480, high: 0x1254F},
	{name: "cypro-minoan", low: 0x12F90, high: 0x12FFF},
	{name: "egyptianhieroglyphs", low: 0x13000, high: 0x1342F},
	{name: "egyptianhieroglyphformatcontrols", low: 0x13430, high: 0x1345F},
	{name: "anatolianhieroglyphs", low: 0x14400, high: 0x1467F},
	{name: "bamumsupplement", low: 0x16800, high: 0x16A3F},
	{name: "mro", low: 0x16A40, high: 0x16A6F},
	{name: "tangsa", low: 0x16A70, high: 0x16ACF},
	{name: "bassavah", low: 0x16AD0, high: 0x16AFF},
	{name: "pahawhhmong", low: 0x16B00, high: 0x16B8F},
	{name: "medefaidrin", low: 0x16E40, high: 0x16E9F},
	{name: "miao", low: 0x16F00, high: 0x16F9F},
	{name: "ideographicsymbolsandpunctuation", low: 0x16FE0, high: 0x16FFF},
	{name: "tangut", low: 0x17000, high: 0x187FF},
	{name: "tangutcomponents", low: 0x18800, high: 0x18AFF},
	{name: "khitansmallscript", low: 0x18B00, high: 0x18CFF},
	{name: "tangutsupplement", low: 0x18D00, high: 0x18D7F},
	{name: "kanaextended-b", low: 0x1AFF0, high: 0x1AFFF},
	{name: "kanasupplement", low: 0x1B000, high: 0x1B0FF},
	{name: "kanaextended-a", low: 0x1B100, high: 0x1B12F},
	{name: "smallkanaextension", low: 0x1B130, high: 0x1B16F},
	{name: "nushu", low: 0x1B170, high: 0x1B2FF},
	{name: "duployan", low: 0x1BC00, high: 0x1BC9F},
	{name: "shorthandformatcontrols", low: 0x1BCA0, high: 0x1BCAF},
	{name: "znamennymusicalnotation", low: 0x1CF00, high: 0x1CFCF},
	{name: "byzantinemusicalsymbols", low: 0x1D000, high: 0x1D0FF},
	{name: "musicalsymbols", low: 0x1D100, high: 0x1D1FF},
	{name: "ancientgreekmusicalnotation", low: 0x1D200, high: 0x1D24F},
	{name: "kaktoviknumerals", low: 0x1D2C0, high: 0x1D2DF},
	{name: "mayannumerals", low: 0x1D2E0, high: 0x1D2FF},
	{name: "taixuanjingsymbols", low: 0x1D300, high: 0x1D35F},
	{name: "countingrodnumerals", low: 0x1D360, high: 0x1D37F},
	{name: "mathematicalalphanumericsymbols", low: 0x1D400, high: 0x1D7FF},
	{name: "suttonsignwriting", low: 0x1D800, high: 0x1DAAF},
	{name: "latinextended-g", low: 0x1DF00, high: 0x1DFFF},
	{name: "glagoliticsupplement", low: 0x1E000, high: 0x1E02F},
	{name: "cyrillicextended-d", low: 0x1E030, high: 0x1E08F},
	{name: "nyiakengpuachuehmong", low: 0x1E100, high: 0x1E14F},
	{name: "toto", low: 0x1E290, high: 0x1E2BF},
	{name: "wancho", low: 0x1E2C0, high: 0x1E2FF},
	{name: "nagmundari", low: 0x1E4D0, high: 0x1E4FF},
	{name: "ethiopicextended-b", low: 0x1E7E0, high: 0x1E7FF},
	{name: "mendekikakui", low: 0x1E800, high: 0x1E8DF},
	{name: "adlam", low: 0x1E900, high: 0x1E95F},
	{name: "indicsiyaqnumbers", low: 0x1EC70, high: 0x1ECBF},
	{name: "ottomansiyaqnumbers", low: 0x1ED00, high: 0x1ED4F},
	{name: "arabicmathematicalalphabeticsymbols", low: 0x1EE00, high: 0x1EEFF},
	{name: "mahjongtiles", low: 0x1F000, high: 0x1F02F},
	{name: "dominotiles", low: 0x1F030, high: 0x1F09F},
	{name: "playingcards", low: 0x1F0A0, high: 0x1F0FF},
	{name: "enclosedalphanumericsupplement", low: 0x1F100, high: 0x1F1FF},
	{name: "enclosedideographicsupplement", low: 0x1F200, high: 0x1F2FF},
	{name: "miscellaneoussymbolsandpictographs", low: 0x1F300, high: 0x1F5FF},
	{name: "emoticons", low: 0x1F600, high: 0x1F64F},
	{name: "ornamentaldingbats", low: 0x1F650, high: 0x1F67F},
	{name: "transportandmapsymbols", low: 0x1F680, high: 0x1F6FF},
	{name: "alchemicalsymbols", low: 0x1F700, high: 0x1F77F},
	{name: "geometricshapesextended", low: 0x1F780, high: 0x1F7FF},
	{name: "supplementalarrows-c", low: 0x1F800, high: 0x1F8FF},
	{name: "supplementalsymbolsandpictographs", low: 0x1F900, high: 0x1F9FF},
	{name: "chesssymbols", low: 0x1FA00, high: 0x1FA6F},
	{name: "symbolsandpictographsextended-a", low: 0x1FA70, high: 0x1FAFF},
	{name: "symbolsforlegacycomputing", low: 0x1FB00, high: 0x1FBFF},
	{name: "cjkunifiedideographsextensionb", low: 0x20000, high: 0x2A6DF},
	{name: "cjkunifiedideographsextensionc", low: 0x2A700, high: 0x2B73F},
	{name: "cjkunifiedideographsextensiond", low: 0x2B740, high: 0x2B81F},
	{name: "cjkunifiedideographsextensione", low: 0x2B820, high: 0x2CEAF},
	{name: "cjkunifiedideographsextensionf", low: 0x2CEB0, high: 0x2EBEF},
	{name: "cjkcompatibilityideographssupplement", low: 0x2F800, high: 0x2FA1F},
	{name: "cjkunifiedideographsextensiong", low: 0x30000, high: 0x3134F},
	{name: "cjkunifiedideographsextensionh", low: 0x31350, high: 0x323AF},
	{name: "tags", low: 0xE0000, high: 0xE007F},
	{name: "variationselectorssupplement", low: 0xE0100, high: 0xE01EF},
	{name: "supplementaryprivateusearea-a", low: 0xF0000, high: 0xFFFFF},
	{name: "supplementaryprivateusearea-b", low: 0x100000, high: 0x10FFFF},
}

func precisionDecimalXMLChar(value rune) bool {
	return value == 0x9 || value == 0xa || value == 0xd ||
		(value >= 0x20 && value <= 0xd7ff) ||
		(value >= 0xe000 && value <= 0xfffd) ||
		(value >= 0x10000 && value <= 0x10ffff)
}

func precisionDecimalXMLWildcard(value rune) bool {
	return precisionDecimalXMLChar(value) && value != '\n' && value != '\r'
}

func precisionDecimalXMLWhitespace(value rune) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func precisionDecimalXMLNameStartChar(value rune) bool {
	return value == ':' || value == '_' ||
		(value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z') ||
		(value >= 0xc0 && value <= 0xd6) ||
		(value >= 0xd8 && value <= 0xf6) ||
		(value >= 0xf8 && value <= 0x2ff) ||
		(value >= 0x370 && value <= 0x37d) ||
		(value >= 0x37f && value <= 0x1fff) ||
		(value >= 0x200c && value <= 0x200d) ||
		(value >= 0x2070 && value <= 0x218f) ||
		(value >= 0x2c00 && value <= 0x2fef) ||
		(value >= 0x3001 && value <= 0xd7ff) ||
		(value >= 0xf900 && value <= 0xfdcf) ||
		(value >= 0xfdf0 && value <= 0xfffd) ||
		(value >= 0x10000 && value <= 0xeffff)
}

func precisionDecimalXMLNameChar(value rune) bool {
	return precisionDecimalXMLNameStartChar(value) ||
		value == '-' || value == '.' ||
		(value >= '0' && value <= '9') ||
		value == 0xb7 ||
		(value >= 0x300 && value <= 0x36f) ||
		(value >= 0x203f && value <= 0x2040)
}

func precisionDecimalXMLWord(value rune) bool {
	if !precisionDecimalXMLChar(value) {
		return false
	}
	return unicode.Is(unicode.L, value) || unicode.Is(unicode.M, value) ||
		unicode.Is(unicode.N, value) || unicode.Is(unicode.Pc, value)
}

func (regex *precisionDecimalXMLRegex) matches(source string) bool {
	if !utf8.ValidString(source) {
		return false
	}
	input := []rune(source)
	for _, value := range input {
		if precisionDecimalXMLChar(value) {
			continue
		}
		return false
	}
	for _, branch := range regex.branches {
		positions := precisionDecimalXMLRegexBranchPositions(branch, input, 0)
		for _, position := range positions {
			if position == len(input) {
				return true
			}
		}
	}
	return false
}

func precisionDecimalXMLRegexBranchPositions(branch precisionDecimalXMLRegexBranch, input []rune, start int) []int {
	memo := make(map[precisionDecimalXMLRegexMatchState][]int)
	return precisionDecimalXMLRegexBranchPositionsFrom(branch, input, 0, start, memo)
}

func precisionDecimalXMLRegexBranchPositionsFrom(branch precisionDecimalXMLRegexBranch, input []rune, pieceIndex, position int, memo map[precisionDecimalXMLRegexMatchState][]int) []int {
	key := precisionDecimalXMLRegexMatchState{piece: pieceIndex, pos: position}
	if result, ok := memo[key]; ok {
		return result
	}
	if pieceIndex == len(branch.pieces) {
		result := []int{position}
		memo[key] = result
		return result
	}
	piece := branch.pieces[pieceIndex]
	ends := precisionDecimalXMLRepeatPositions(piece.atom, piece.min, piece.max, input, position)
	result := make([]int, 0, len(ends))
	for _, end := range ends {
		suffix := precisionDecimalXMLRegexBranchPositionsFrom(branch, input, pieceIndex+1, end, memo)
		for _, candidate := range suffix {
			precisionDecimalXMLAppendUnique(&result, candidate)
		}
	}
	memo[key] = result
	return result
}

func precisionDecimalXMLRepeatPositions(atom precisionDecimalXMLRegexAtom, minimumCount, maximumCount *big.Int, input []rune, start int) []int {
	nullable := precisionDecimalXMLAtomNullable(atom)
	limit := len(input)
	if nullable {
		limit++
	}
	minimum, maximum, ok := precisionDecimalXMLRepeatBounds(minimumCount, maximumCount, limit, nullable)
	if !ok {
		return nil
	}
	return precisionDecimalXMLRepeatPositionsWithinBounds(atom, minimum, maximum, input, start)
}

func precisionDecimalXMLRepeatBounds(minimumCount, maximumCount *big.Int, limit int, nullable bool) (int, int, bool) {
	minimum, ok := precisionDecimalXMLQuantifierLimit(minimumCount, limit, nullable)
	if !ok {
		return 0, 0, false
	}
	maximum := limit
	if maximumCount != nil {
		maximum, ok = precisionDecimalXMLQuantifierLimit(maximumCount, limit, nullable)
		if !ok {
			return 0, 0, false
		}
	}
	if maximum < minimum {
		return 0, 0, false
	}
	return minimum, maximum, true
}

func precisionDecimalXMLRepeatPositionsWithinBounds(atom precisionDecimalXMLRegexAtom, minimum, maximum int, input []rune, start int) []int {
	positions := []int{start}
	result := make([]int, 0, len(positions))
	if minimum == 0 {
		result = append(result, start)
	}
	for count := 1; count <= maximum; count++ {
		next := precisionDecimalXMLNextPositions(atom, input, positions)
		if len(next) == 0 {
			break
		}
		positions = next
		if count >= minimum {
			for _, position := range positions {
				precisionDecimalXMLAppendUnique(&result, position)
			}
		}
	}
	return result
}

func precisionDecimalXMLNextPositions(atom precisionDecimalXMLRegexAtom, input []rune, positions []int) []int {
	next := make([]int, 0, len(positions))
	for _, position := range positions {
		ends := precisionDecimalXMLAtomPositions(atom, input, position)
		for _, end := range ends {
			precisionDecimalXMLAppendUnique(&next, end)
		}
	}
	return next
}

func precisionDecimalXMLQuantifierLimit(value *big.Int, limit int, nullable bool) (int, bool) {
	limitValue := big.NewInt(int64(limit))
	if value.Cmp(limitValue) <= 0 {
		return int(value.Int64()), true
	}
	if nullable {
		return limit, true
	}
	return 0, false
}

func precisionDecimalXMLAtomPositions(atom precisionDecimalXMLRegexAtom, input []rune, position int) []int {
	switch atom.kind {
	case precisionDecimalXMLRegexLiteralAtom:
		if position < len(input) && input[position] == atom.literal {
			return []int{position + 1}
		}
		return nil
	case precisionDecimalXMLRegexSetAtom:
		if position < len(input) && atom.set.contains(input[position]) {
			return []int{position + 1}
		}
		return nil
	case precisionDecimalXMLRegexGroupAtom:
		result := []int{}
		for _, branch := range atom.group.branches {
			positions := precisionDecimalXMLRegexBranchPositions(branch, input, position)
			for _, end := range positions {
				precisionDecimalXMLAppendUnique(&result, end)
			}
		}
		return result
	default:
		panic("precisionDecimal XML regex: invalid atom kind")
	}
}

func precisionDecimalXMLAtomNullable(atom precisionDecimalXMLRegexAtom) bool {
	if atom.kind != precisionDecimalXMLRegexGroupAtom {
		return false
	}
	for _, branch := range atom.group.branches {
		if precisionDecimalXMLBranchNullable(branch) {
			return true
		}
	}
	return false
}

func precisionDecimalXMLBranchNullable(branch precisionDecimalXMLRegexBranch) bool {
	for _, piece := range branch.pieces {
		if piece.min.Sign() == 0 {
			continue
		}
		if !precisionDecimalXMLAtomNullable(piece.atom) {
			return false
		}
	}
	return true
}

func precisionDecimalXMLAppendUnique(values *[]int, candidate int) {
	for _, value := range *values {
		if value == candidate {
			return
		}
	}
	*values = append(*values, candidate)
}
