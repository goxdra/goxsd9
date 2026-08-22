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
	contains     func(rune) bool
	anyCharacter bool
}

type precisionDecimalXMLRegexParser struct {
	input      []rune
	pos        int
	depth      int
	classDepth int
	pieces     int
	classParts int
}

type precisionDecimalXMLRegexMatchState struct {
	piece int
	pos   int
}

type precisionDecimalXMLRegexMatchBudget struct {
	remaining int
}

type precisionDecimalXMLBlock struct {
	name string
	low  rune
	high rune
}

const (
	precisionDecimalXMLRegexMaxSourceRunes = 16384
	precisionDecimalXMLRegexMaxSourceBytes = precisionDecimalXMLRegexMaxSourceRunes * utf8.UTFMax
	precisionDecimalXMLRegexMaxDepth       = 128
	precisionDecimalXMLRegexMaxPieces      = 2048
	precisionDecimalXMLRegexMaxClassParts  = 512
	precisionDecimalXMLRegexMaxMatchWork   = 250000
)

var errPrecisionDecimalXMLRegexResourceLimit = errors.New("precisionDecimal XML Schema pattern exceeds resource limits")
var errPrecisionDecimalXMLRegexMatchResourceLimit = errors.New("precisionDecimal XML Schema pattern matching exceeds resource limits")

func parsePrecisionDecimalXMLRegex(source string) (*precisionDecimalXMLRegex, error) {
	if len(source) > precisionDecimalXMLRegexMaxSourceBytes {
		return nil, errPrecisionDecimalXMLRegexResourceLimit
	}
	if !utf8.ValidString(source) {
		return nil, errors.New("pattern is not valid UTF-8")
	}
	input := []rune(source)
	if len(input) > precisionDecimalXMLRegexMaxSourceRunes {
		return nil, errPrecisionDecimalXMLRegexResourceLimit
	}
	parser := precisionDecimalXMLRegexParser{input: input}
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
	parser.pieces++
	if parser.pieces > precisionDecimalXMLRegexMaxPieces {
		return precisionDecimalXMLRegexPiece{}, errPrecisionDecimalXMLRegexResourceLimit
	}
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
		parser.depth++
		if parser.depth > precisionDecimalXMLRegexMaxDepth {
			parser.depth--
			return precisionDecimalXMLRegexAtom{}, errPrecisionDecimalXMLRegexResourceLimit
		}
		group, err := parser.parseRegex(true)
		parser.depth--
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
	if prefix == 'P' && !set.anyCharacter {
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
	parser.classDepth++
	if parser.classDepth > precisionDecimalXMLRegexMaxDepth {
		parser.classDepth--
		return precisionDecimalXMLCharSet{}, errPrecisionDecimalXMLRegexResourceLimit
	}
	defer func() { parser.classDepth-- }()
	if parser.atEnd() || parser.peek() != '[' {
		return precisionDecimalXMLCharSet{}, errors.New("character class must start with [")
	}
	parser.pos++
	return parser.parseCharacterClassGroup(parser.consumeCharacterClassNegation())
}

func (parser *precisionDecimalXMLRegexParser) parseCharacterClassGroup(negative bool) (precisionDecimalXMLCharSet, error) {
	parts := make([]precisionDecimalXMLCharSet, 0, 1)
	previousSingle := false
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
		if precisionDecimalXMLCharacterClassHasInvalidHyphen(parser, previousSingle) {
			return precisionDecimalXMLCharSet{}, errors.New("unescaped hyphen is not at a character class edge")
		}
		part, single, err := parser.parseCharacterClassMember()
		if err != nil {
			return precisionDecimalXMLCharSet{}, err
		}
		parts = append(parts, part)
		previousSingle = single
	}
}

func precisionDecimalXMLCharacterClassHasInvalidHyphen(parser *precisionDecimalXMLRegexParser, previousSingle bool) bool {
	if !previousSingle || parser.peek() != '-' || parser.pos+1 >= len(parser.input) {
		return false
	}
	if parser.input[parser.pos+1] == ']' {
		return false
	}
	return !precisionDecimalXMLCharacterClassDoubleHyphenSubtraction(parser)
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

func precisionDecimalXMLCharacterClassDoubleHyphenSubtraction(parser *precisionDecimalXMLRegexParser) bool {
	return parser.pos+2 < len(parser.input) && parser.input[parser.pos] == '-' && parser.input[parser.pos+1] == '-' && parser.input[parser.pos+2] == '['
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

func (parser *precisionDecimalXMLRegexParser) parseCharacterClassMember() (precisionDecimalXMLCharSet, bool, error) {
	part, literal, single, escaped, err := parser.parseCharacterClassPart()
	if err != nil {
		return precisionDecimalXMLCharSet{}, false, err
	}
	if !single || !precisionDecimalXMLCharacterClassRangeStart(parser) {
		return part, single, nil
	}
	if literal == '-' && !escaped {
		return precisionDecimalXMLCharSet{}, false, errors.New("unescaped hyphen cannot start a character class range")
	}
	parser.pos++
	_, endpointLiteral, endpointSingle, endpointEscaped, err := parser.parseCharacterClassPart()
	if err != nil {
		return precisionDecimalXMLCharSet{}, false, err
	}
	if !endpointSingle {
		return precisionDecimalXMLCharSet{}, false, errors.New("character class range endpoint is not a single character")
	}
	if endpointLiteral == '-' && !endpointEscaped {
		return precisionDecimalXMLCharSet{}, false, errors.New("unescaped hyphen cannot end a character class range")
	}
	if literal > endpointLiteral {
		return precisionDecimalXMLCharSet{}, false, errors.New("character class range is reversed")
	}
	return precisionDecimalXMLRangeSet(literal, endpointLiteral), false, nil
}

func precisionDecimalXMLCharacterClassRangeStart(parser *precisionDecimalXMLRegexParser) bool {
	if parser.atEnd() || parser.peek() != '-' || parser.pos+1 >= len(parser.input) {
		return false
	}
	next := parser.input[parser.pos+1]
	if next == ']' || next == '[' || next == '-' {
		return false
	}
	return true
}

func (parser *precisionDecimalXMLRegexParser) parseCharacterClassPart() (precisionDecimalXMLCharSet, rune, bool, bool, error) {
	parser.classParts++
	if parser.classParts > precisionDecimalXMLRegexMaxClassParts {
		return precisionDecimalXMLCharSet{}, 0, false, false, errPrecisionDecimalXMLRegexResourceLimit
	}
	if parser.atEnd() {
		return precisionDecimalXMLCharSet{}, 0, false, false, errors.New("missing character class member")
	}
	next := parser.peek()
	parser.pos++
	if next == '\\' {
		set, literal, single, err := parser.parseEscapeSet()
		return set, literal, single, single, err
	}
	if next == '[' || next == ']' {
		return precisionDecimalXMLCharSet{}, 0, false, false, fmt.Errorf("unescaped %c is not allowed in a character class", next)
	}
	if !precisionDecimalXMLChar(next) {
		return precisionDecimalXMLCharSet{}, 0, false, false, fmt.Errorf("character class literal %U is not an XML character", next)
	}
	return precisionDecimalXMLLiteralSet(next), next, true, false, nil
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
		if !precisionDecimalXMLIsBlockSyntax(name) {
			return precisionDecimalXMLCharSet{}, false
		}
		block, found := precisionDecimalXMLBlockSet(name[2:])
		if found {
			return block, true
		}
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLChar, anyCharacter: true}, true
	}
	if !precisionDecimalXMLCategoryCode(name) {
		return precisionDecimalXMLCharSet{}, false
	}
	if name == "Cn" {
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLUnassigned}, true
	}
	if name == "C" {
		return precisionDecimalXMLCharSet{contains: precisionDecimalXMLCategoryC}, true
	}
	table, ok := unicode.Categories[name]
	if !ok {
		return precisionDecimalXMLCharSet{}, false
	}
	return precisionDecimalXMLCategorySet(table), true
}

func precisionDecimalXMLIsBlockSyntax(name string) bool {
	if len(name) <= 2 || !strings.HasPrefix(name, "Is") {
		return false
	}
	for _, value := range name[2:] {
		if value >= 'A' && value <= 'Z' {
			continue
		}
		if value >= 'a' && value <= 'z' {
			continue
		}
		if value >= '0' && value <= '9' {
			continue
		}
		if value == '-' {
			continue
		}
		return false
	}
	return true
}

func precisionDecimalXMLCategoryCode(name string) bool {
	switch name {
	case "L", "Lu", "Ll", "Lt", "Lm", "Lo",
		"M", "Mn", "Mc", "Me",
		"N", "Nd", "Nl", "No",
		"P", "Pc", "Pd", "Ps", "Pe", "Pi", "Pf", "Po",
		"Z", "Zs", "Zl", "Zp",
		"S", "Sm", "Sc", "Sk", "So",
		"C", "Cc", "Cf", "Co", "Cn":
		return true
	default:
		return false
	}
}

func precisionDecimalXMLCategorySet(table *unicode.RangeTable) precisionDecimalXMLCharSet {
	return precisionDecimalXMLCharSet{contains: func(value rune) bool {
		return precisionDecimalXMLChar(value) && unicode.Is(table, value)
	}}
}

func precisionDecimalXMLBlockSet(name string) (precisionDecimalXMLCharSet, bool) {
	normalized := precisionDecimalXMLNormalizeBlockName(name)
	switch normalized {
	case "HighSurrogates", "HighPrivateUseSurrogates", "LowSurrogates":
		return precisionDecimalXMLCharSet{contains: func(rune) bool { return false }}, true
	}
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
		if value == '_' || unicode.IsSpace(value) {
			continue
		}
		builder.WriteRune(value)
	}
	return builder.String()
}

var precisionDecimalXMLBlocks = []precisionDecimalXMLBlock{
	{name: "BasicLatin", low: 0x0000, high: 0x007F},
	{name: "Latin-1Supplement", low: 0x0080, high: 0x00FF},
	{name: "LatinExtended-A", low: 0x0100, high: 0x017F},
	{name: "LatinExtended-B", low: 0x0180, high: 0x024F},
	{name: "IPAExtensions", low: 0x0250, high: 0x02AF},
	{name: "SpacingModifierLetters", low: 0x02B0, high: 0x02FF},
	{name: "CombiningDiacriticalMarks", low: 0x0300, high: 0x036F},
	{name: "GreekandCoptic", low: 0x0370, high: 0x03FF},
	{name: "Cyrillic", low: 0x0400, high: 0x04FF},
	{name: "CyrillicSupplement", low: 0x0500, high: 0x052F},
	{name: "Armenian", low: 0x0530, high: 0x058F},
	{name: "Hebrew", low: 0x0590, high: 0x05FF},
	{name: "Arabic", low: 0x0600, high: 0x06FF},
	{name: "Syriac", low: 0x0700, high: 0x074F},
	{name: "ArabicSupplement", low: 0x0750, high: 0x077F},
	{name: "Thaana", low: 0x0780, high: 0x07BF},
	{name: "NKo", low: 0x07C0, high: 0x07FF},
	{name: "Samaritan", low: 0x0800, high: 0x083F},
	{name: "Mandaic", low: 0x0840, high: 0x085F},
	{name: "SyriacSupplement", low: 0x0860, high: 0x086F},
	{name: "ArabicExtended-B", low: 0x0870, high: 0x089F},
	{name: "ArabicExtended-A", low: 0x08A0, high: 0x08FF},
	{name: "Devanagari", low: 0x0900, high: 0x097F},
	{name: "Bengali", low: 0x0980, high: 0x09FF},
	{name: "Gurmukhi", low: 0x0A00, high: 0x0A7F},
	{name: "Gujarati", low: 0x0A80, high: 0x0AFF},
	{name: "Oriya", low: 0x0B00, high: 0x0B7F},
	{name: "Tamil", low: 0x0B80, high: 0x0BFF},
	{name: "Telugu", low: 0x0C00, high: 0x0C7F},
	{name: "Kannada", low: 0x0C80, high: 0x0CFF},
	{name: "Malayalam", low: 0x0D00, high: 0x0D7F},
	{name: "Sinhala", low: 0x0D80, high: 0x0DFF},
	{name: "Thai", low: 0x0E00, high: 0x0E7F},
	{name: "Lao", low: 0x0E80, high: 0x0EFF},
	{name: "Tibetan", low: 0x0F00, high: 0x0FFF},
	{name: "Myanmar", low: 0x1000, high: 0x109F},
	{name: "Georgian", low: 0x10A0, high: 0x10FF},
	{name: "HangulJamo", low: 0x1100, high: 0x11FF},
	{name: "Ethiopic", low: 0x1200, high: 0x137F},
	{name: "EthiopicSupplement", low: 0x1380, high: 0x139F},
	{name: "Cherokee", low: 0x13A0, high: 0x13FF},
	{name: "UnifiedCanadianAboriginalSyllabics", low: 0x1400, high: 0x167F},
	{name: "Ogham", low: 0x1680, high: 0x169F},
	{name: "Runic", low: 0x16A0, high: 0x16FF},
	{name: "Tagalog", low: 0x1700, high: 0x171F},
	{name: "Hanunoo", low: 0x1720, high: 0x173F},
	{name: "Buhid", low: 0x1740, high: 0x175F},
	{name: "Tagbanwa", low: 0x1760, high: 0x177F},
	{name: "Khmer", low: 0x1780, high: 0x17FF},
	{name: "Mongolian", low: 0x1800, high: 0x18AF},
	{name: "UnifiedCanadianAboriginalSyllabicsExtended", low: 0x18B0, high: 0x18FF},
	{name: "Limbu", low: 0x1900, high: 0x194F},
	{name: "TaiLe", low: 0x1950, high: 0x197F},
	{name: "NewTaiLue", low: 0x1980, high: 0x19DF},
	{name: "KhmerSymbols", low: 0x19E0, high: 0x19FF},
	{name: "Buginese", low: 0x1A00, high: 0x1A1F},
	{name: "TaiTham", low: 0x1A20, high: 0x1AAF},
	{name: "CombiningDiacriticalMarksExtended", low: 0x1AB0, high: 0x1AFF},
	{name: "Balinese", low: 0x1B00, high: 0x1B7F},
	{name: "Sundanese", low: 0x1B80, high: 0x1BBF},
	{name: "Batak", low: 0x1BC0, high: 0x1BFF},
	{name: "Lepcha", low: 0x1C00, high: 0x1C4F},
	{name: "OlChiki", low: 0x1C50, high: 0x1C7F},
	{name: "CyrillicExtended-C", low: 0x1C80, high: 0x1C8F},
	{name: "GeorgianExtended", low: 0x1C90, high: 0x1CBF},
	{name: "SundaneseSupplement", low: 0x1CC0, high: 0x1CCF},
	{name: "VedicExtensions", low: 0x1CD0, high: 0x1CFF},
	{name: "PhoneticExtensions", low: 0x1D00, high: 0x1D7F},
	{name: "PhoneticExtensionsSupplement", low: 0x1D80, high: 0x1DBF},
	{name: "CombiningDiacriticalMarksSupplement", low: 0x1DC0, high: 0x1DFF},
	{name: "LatinExtendedAdditional", low: 0x1E00, high: 0x1EFF},
	{name: "GreekExtended", low: 0x1F00, high: 0x1FFF},
	{name: "GeneralPunctuation", low: 0x2000, high: 0x206F},
	{name: "SuperscriptsandSubscripts", low: 0x2070, high: 0x209F},
	{name: "CurrencySymbols", low: 0x20A0, high: 0x20CF},
	{name: "CombiningDiacriticalMarksforSymbols", low: 0x20D0, high: 0x20FF},
	{name: "LetterlikeSymbols", low: 0x2100, high: 0x214F},
	{name: "NumberForms", low: 0x2150, high: 0x218F},
	{name: "Arrows", low: 0x2190, high: 0x21FF},
	{name: "MathematicalOperators", low: 0x2200, high: 0x22FF},
	{name: "MiscellaneousTechnical", low: 0x2300, high: 0x23FF},
	{name: "ControlPictures", low: 0x2400, high: 0x243F},
	{name: "OpticalCharacterRecognition", low: 0x2440, high: 0x245F},
	{name: "EnclosedAlphanumerics", low: 0x2460, high: 0x24FF},
	{name: "BoxDrawing", low: 0x2500, high: 0x257F},
	{name: "BlockElements", low: 0x2580, high: 0x259F},
	{name: "GeometricShapes", low: 0x25A0, high: 0x25FF},
	{name: "MiscellaneousSymbols", low: 0x2600, high: 0x26FF},
	{name: "Dingbats", low: 0x2700, high: 0x27BF},
	{name: "MiscellaneousMathematicalSymbols-A", low: 0x27C0, high: 0x27EF},
	{name: "SupplementalArrows-A", low: 0x27F0, high: 0x27FF},
	{name: "BraillePatterns", low: 0x2800, high: 0x28FF},
	{name: "SupplementalArrows-B", low: 0x2900, high: 0x297F},
	{name: "MiscellaneousMathematicalSymbols-B", low: 0x2980, high: 0x29FF},
	{name: "SupplementalMathematicalOperators", low: 0x2A00, high: 0x2AFF},
	{name: "MiscellaneousSymbolsandArrows", low: 0x2B00, high: 0x2BFF},
	{name: "Glagolitic", low: 0x2C00, high: 0x2C5F},
	{name: "LatinExtended-C", low: 0x2C60, high: 0x2C7F},
	{name: "Coptic", low: 0x2C80, high: 0x2CFF},
	{name: "GeorgianSupplement", low: 0x2D00, high: 0x2D2F},
	{name: "Tifinagh", low: 0x2D30, high: 0x2D7F},
	{name: "EthiopicExtended", low: 0x2D80, high: 0x2DDF},
	{name: "CyrillicExtended-A", low: 0x2DE0, high: 0x2DFF},
	{name: "SupplementalPunctuation", low: 0x2E00, high: 0x2E7F},
	{name: "CJKRadicalsSupplement", low: 0x2E80, high: 0x2EFF},
	{name: "KangxiRadicals", low: 0x2F00, high: 0x2FDF},
	{name: "IdeographicDescriptionCharacters", low: 0x2FF0, high: 0x2FFF},
	{name: "CJKSymbolsandPunctuation", low: 0x3000, high: 0x303F},
	{name: "Hiragana", low: 0x3040, high: 0x309F},
	{name: "Katakana", low: 0x30A0, high: 0x30FF},
	{name: "Bopomofo", low: 0x3100, high: 0x312F},
	{name: "HangulCompatibilityJamo", low: 0x3130, high: 0x318F},
	{name: "Kanbun", low: 0x3190, high: 0x319F},
	{name: "BopomofoExtended", low: 0x31A0, high: 0x31BF},
	{name: "CJKStrokes", low: 0x31C0, high: 0x31EF},
	{name: "KatakanaPhoneticExtensions", low: 0x31F0, high: 0x31FF},
	{name: "EnclosedCJKLettersandMonths", low: 0x3200, high: 0x32FF},
	{name: "CJKCompatibility", low: 0x3300, high: 0x33FF},
	{name: "CJKUnifiedIdeographsExtensionA", low: 0x3400, high: 0x4DBF},
	{name: "YijingHexagramSymbols", low: 0x4DC0, high: 0x4DFF},
	{name: "CJKUnifiedIdeographs", low: 0x4E00, high: 0x9FFF},
	{name: "YiSyllables", low: 0xA000, high: 0xA48F},
	{name: "YiRadicals", low: 0xA490, high: 0xA4CF},
	{name: "Lisu", low: 0xA4D0, high: 0xA4FF},
	{name: "Vai", low: 0xA500, high: 0xA63F},
	{name: "CyrillicExtended-B", low: 0xA640, high: 0xA69F},
	{name: "Bamum", low: 0xA6A0, high: 0xA6FF},
	{name: "ModifierToneLetters", low: 0xA700, high: 0xA71F},
	{name: "LatinExtended-D", low: 0xA720, high: 0xA7FF},
	{name: "SylotiNagri", low: 0xA800, high: 0xA82F},
	{name: "CommonIndicNumberForms", low: 0xA830, high: 0xA83F},
	{name: "Phags-pa", low: 0xA840, high: 0xA87F},
	{name: "Saurashtra", low: 0xA880, high: 0xA8DF},
	{name: "DevanagariExtended", low: 0xA8E0, high: 0xA8FF},
	{name: "KayahLi", low: 0xA900, high: 0xA92F},
	{name: "Rejang", low: 0xA930, high: 0xA95F},
	{name: "HangulJamoExtended-A", low: 0xA960, high: 0xA97F},
	{name: "Javanese", low: 0xA980, high: 0xA9DF},
	{name: "MyanmarExtended-B", low: 0xA9E0, high: 0xA9FF},
	{name: "Cham", low: 0xAA00, high: 0xAA5F},
	{name: "MyanmarExtended-A", low: 0xAA60, high: 0xAA7F},
	{name: "TaiViet", low: 0xAA80, high: 0xAADF},
	{name: "MeeteiMayekExtensions", low: 0xAAE0, high: 0xAAFF},
	{name: "EthiopicExtended-A", low: 0xAB00, high: 0xAB2F},
	{name: "LatinExtended-E", low: 0xAB30, high: 0xAB6F},
	{name: "CherokeeSupplement", low: 0xAB70, high: 0xABBF},
	{name: "MeeteiMayek", low: 0xABC0, high: 0xABFF},
	{name: "HangulSyllables", low: 0xAC00, high: 0xD7AF},
	{name: "HangulJamoExtended-B", low: 0xD7B0, high: 0xD7FF},
	{name: "HighSurrogates", low: 0xD800, high: 0xDB7F},
	{name: "HighPrivateUseSurrogates", low: 0xDB80, high: 0xDBFF},
	{name: "LowSurrogates", low: 0xDC00, high: 0xDFFF},
	{name: "PrivateUseArea", low: 0xE000, high: 0xF8FF},
	{name: "CJKCompatibilityIdeographs", low: 0xF900, high: 0xFAFF},
	{name: "AlphabeticPresentationForms", low: 0xFB00, high: 0xFB4F},
	{name: "ArabicPresentationForms-A", low: 0xFB50, high: 0xFDFF},
	{name: "VariationSelectors", low: 0xFE00, high: 0xFE0F},
	{name: "VerticalForms", low: 0xFE10, high: 0xFE1F},
	{name: "CombiningHalfMarks", low: 0xFE20, high: 0xFE2F},
	{name: "CJKCompatibilityForms", low: 0xFE30, high: 0xFE4F},
	{name: "SmallFormVariants", low: 0xFE50, high: 0xFE6F},
	{name: "ArabicPresentationForms-B", low: 0xFE70, high: 0xFEFF},
	{name: "HalfwidthandFullwidthForms", low: 0xFF00, high: 0xFFEF},
	{name: "Specials", low: 0xFFF0, high: 0xFFFF},
	{name: "LinearBSyllabary", low: 0x10000, high: 0x1007F},
	{name: "LinearBIdeograms", low: 0x10080, high: 0x100FF},
	{name: "AegeanNumbers", low: 0x10100, high: 0x1013F},
	{name: "AncientGreekNumbers", low: 0x10140, high: 0x1018F},
	{name: "AncientSymbols", low: 0x10190, high: 0x101CF},
	{name: "PhaistosDisc", low: 0x101D0, high: 0x101FF},
	{name: "Lycian", low: 0x10280, high: 0x1029F},
	{name: "Carian", low: 0x102A0, high: 0x102DF},
	{name: "CopticEpactNumbers", low: 0x102E0, high: 0x102FF},
	{name: "OldItalic", low: 0x10300, high: 0x1032F},
	{name: "Gothic", low: 0x10330, high: 0x1034F},
	{name: "OldPermic", low: 0x10350, high: 0x1037F},
	{name: "Ugaritic", low: 0x10380, high: 0x1039F},
	{name: "OldPersian", low: 0x103A0, high: 0x103DF},
	{name: "Deseret", low: 0x10400, high: 0x1044F},
	{name: "Shavian", low: 0x10450, high: 0x1047F},
	{name: "Osmanya", low: 0x10480, high: 0x104AF},
	{name: "Osage", low: 0x104B0, high: 0x104FF},
	{name: "Elbasan", low: 0x10500, high: 0x1052F},
	{name: "CaucasianAlbanian", low: 0x10530, high: 0x1056F},
	{name: "Vithkuqi", low: 0x10570, high: 0x105BF},
	{name: "LinearA", low: 0x10600, high: 0x1077F},
	{name: "LatinExtended-F", low: 0x10780, high: 0x107BF},
	{name: "CypriotSyllabary", low: 0x10800, high: 0x1083F},
	{name: "ImperialAramaic", low: 0x10840, high: 0x1085F},
	{name: "Palmyrene", low: 0x10860, high: 0x1087F},
	{name: "Nabataean", low: 0x10880, high: 0x108AF},
	{name: "Hatran", low: 0x108E0, high: 0x108FF},
	{name: "Phoenician", low: 0x10900, high: 0x1091F},
	{name: "Lydian", low: 0x10920, high: 0x1093F},
	{name: "MeroiticHieroglyphs", low: 0x10980, high: 0x1099F},
	{name: "MeroiticCursive", low: 0x109A0, high: 0x109FF},
	{name: "Kharoshthi", low: 0x10A00, high: 0x10A5F},
	{name: "OldSouthArabian", low: 0x10A60, high: 0x10A7F},
	{name: "OldNorthArabian", low: 0x10A80, high: 0x10A9F},
	{name: "Manichaean", low: 0x10AC0, high: 0x10AFF},
	{name: "Avestan", low: 0x10B00, high: 0x10B3F},
	{name: "InscriptionalParthian", low: 0x10B40, high: 0x10B5F},
	{name: "InscriptionalPahlavi", low: 0x10B60, high: 0x10B7F},
	{name: "PsalterPahlavi", low: 0x10B80, high: 0x10BAF},
	{name: "OldTurkic", low: 0x10C00, high: 0x10C4F},
	{name: "OldHungarian", low: 0x10C80, high: 0x10CFF},
	{name: "HanifiRohingya", low: 0x10D00, high: 0x10D3F},
	{name: "RumiNumeralSymbols", low: 0x10E60, high: 0x10E7F},
	{name: "Yezidi", low: 0x10E80, high: 0x10EBF},
	{name: "ArabicExtended-C", low: 0x10EC0, high: 0x10EFF},
	{name: "OldSogdian", low: 0x10F00, high: 0x10F2F},
	{name: "Sogdian", low: 0x10F30, high: 0x10F6F},
	{name: "OldUyghur", low: 0x10F70, high: 0x10FAF},
	{name: "Chorasmian", low: 0x10FB0, high: 0x10FDF},
	{name: "Elymaic", low: 0x10FE0, high: 0x10FFF},
	{name: "Brahmi", low: 0x11000, high: 0x1107F},
	{name: "Kaithi", low: 0x11080, high: 0x110CF},
	{name: "SoraSompeng", low: 0x110D0, high: 0x110FF},
	{name: "Chakma", low: 0x11100, high: 0x1114F},
	{name: "Mahajani", low: 0x11150, high: 0x1117F},
	{name: "Sharada", low: 0x11180, high: 0x111DF},
	{name: "SinhalaArchaicNumbers", low: 0x111E0, high: 0x111FF},
	{name: "Khojki", low: 0x11200, high: 0x1124F},
	{name: "Multani", low: 0x11280, high: 0x112AF},
	{name: "Khudawadi", low: 0x112B0, high: 0x112FF},
	{name: "Grantha", low: 0x11300, high: 0x1137F},
	{name: "Newa", low: 0x11400, high: 0x1147F},
	{name: "Tirhuta", low: 0x11480, high: 0x114DF},
	{name: "Siddham", low: 0x11580, high: 0x115FF},
	{name: "Modi", low: 0x11600, high: 0x1165F},
	{name: "MongolianSupplement", low: 0x11660, high: 0x1167F},
	{name: "Takri", low: 0x11680, high: 0x116CF},
	{name: "Ahom", low: 0x11700, high: 0x1174F},
	{name: "Dogra", low: 0x11800, high: 0x1184F},
	{name: "WarangCiti", low: 0x118A0, high: 0x118FF},
	{name: "DivesAkuru", low: 0x11900, high: 0x1195F},
	{name: "Nandinagari", low: 0x119A0, high: 0x119FF},
	{name: "ZanabazarSquare", low: 0x11A00, high: 0x11A4F},
	{name: "Soyombo", low: 0x11A50, high: 0x11AAF},
	{name: "UnifiedCanadianAboriginalSyllabicsExtended-A", low: 0x11AB0, high: 0x11ABF},
	{name: "PauCinHau", low: 0x11AC0, high: 0x11AFF},
	{name: "DevanagariExtended-A", low: 0x11B00, high: 0x11B5F},
	{name: "Bhaiksuki", low: 0x11C00, high: 0x11C6F},
	{name: "Marchen", low: 0x11C70, high: 0x11CBF},
	{name: "MasaramGondi", low: 0x11D00, high: 0x11D5F},
	{name: "GunjalaGondi", low: 0x11D60, high: 0x11DAF},
	{name: "Makasar", low: 0x11EE0, high: 0x11EFF},
	{name: "Kawi", low: 0x11F00, high: 0x11F5F},
	{name: "LisuSupplement", low: 0x11FB0, high: 0x11FBF},
	{name: "TamilSupplement", low: 0x11FC0, high: 0x11FFF},
	{name: "Cuneiform", low: 0x12000, high: 0x123FF},
	{name: "CuneiformNumbersandPunctuation", low: 0x12400, high: 0x1247F},
	{name: "EarlyDynasticCuneiform", low: 0x12480, high: 0x1254F},
	{name: "Cypro-Minoan", low: 0x12F90, high: 0x12FFF},
	{name: "EgyptianHieroglyphs", low: 0x13000, high: 0x1342F},
	{name: "EgyptianHieroglyphFormatControls", low: 0x13430, high: 0x1345F},
	{name: "AnatolianHieroglyphs", low: 0x14400, high: 0x1467F},
	{name: "BamumSupplement", low: 0x16800, high: 0x16A3F},
	{name: "Mro", low: 0x16A40, high: 0x16A6F},
	{name: "Tangsa", low: 0x16A70, high: 0x16ACF},
	{name: "BassaVah", low: 0x16AD0, high: 0x16AFF},
	{name: "PahawhHmong", low: 0x16B00, high: 0x16B8F},
	{name: "Medefaidrin", low: 0x16E40, high: 0x16E9F},
	{name: "Miao", low: 0x16F00, high: 0x16F9F},
	{name: "IdeographicSymbolsandPunctuation", low: 0x16FE0, high: 0x16FFF},
	{name: "Tangut", low: 0x17000, high: 0x187FF},
	{name: "TangutComponents", low: 0x18800, high: 0x18AFF},
	{name: "KhitanSmallScript", low: 0x18B00, high: 0x18CFF},
	{name: "TangutSupplement", low: 0x18D00, high: 0x18D7F},
	{name: "KanaExtended-B", low: 0x1AFF0, high: 0x1AFFF},
	{name: "KanaSupplement", low: 0x1B000, high: 0x1B0FF},
	{name: "KanaExtended-A", low: 0x1B100, high: 0x1B12F},
	{name: "SmallKanaExtension", low: 0x1B130, high: 0x1B16F},
	{name: "Nushu", low: 0x1B170, high: 0x1B2FF},
	{name: "Duployan", low: 0x1BC00, high: 0x1BC9F},
	{name: "ShorthandFormatControls", low: 0x1BCA0, high: 0x1BCAF},
	{name: "ZnamennyMusicalNotation", low: 0x1CF00, high: 0x1CFCF},
	{name: "ByzantineMusicalSymbols", low: 0x1D000, high: 0x1D0FF},
	{name: "MusicalSymbols", low: 0x1D100, high: 0x1D1FF},
	{name: "AncientGreekMusicalNotation", low: 0x1D200, high: 0x1D24F},
	{name: "KaktovikNumerals", low: 0x1D2C0, high: 0x1D2DF},
	{name: "MayanNumerals", low: 0x1D2E0, high: 0x1D2FF},
	{name: "TaiXuanJingSymbols", low: 0x1D300, high: 0x1D35F},
	{name: "CountingRodNumerals", low: 0x1D360, high: 0x1D37F},
	{name: "MathematicalAlphanumericSymbols", low: 0x1D400, high: 0x1D7FF},
	{name: "SuttonSignWriting", low: 0x1D800, high: 0x1DAAF},
	{name: "LatinExtended-G", low: 0x1DF00, high: 0x1DFFF},
	{name: "GlagoliticSupplement", low: 0x1E000, high: 0x1E02F},
	{name: "CyrillicExtended-D", low: 0x1E030, high: 0x1E08F},
	{name: "NyiakengPuachueHmong", low: 0x1E100, high: 0x1E14F},
	{name: "Toto", low: 0x1E290, high: 0x1E2BF},
	{name: "Wancho", low: 0x1E2C0, high: 0x1E2FF},
	{name: "NagMundari", low: 0x1E4D0, high: 0x1E4FF},
	{name: "EthiopicExtended-B", low: 0x1E7E0, high: 0x1E7FF},
	{name: "MendeKikakui", low: 0x1E800, high: 0x1E8DF},
	{name: "Adlam", low: 0x1E900, high: 0x1E95F},
	{name: "IndicSiyaqNumbers", low: 0x1EC70, high: 0x1ECBF},
	{name: "OttomanSiyaqNumbers", low: 0x1ED00, high: 0x1ED4F},
	{name: "ArabicMathematicalAlphabeticSymbols", low: 0x1EE00, high: 0x1EEFF},
	{name: "MahjongTiles", low: 0x1F000, high: 0x1F02F},
	{name: "DominoTiles", low: 0x1F030, high: 0x1F09F},
	{name: "PlayingCards", low: 0x1F0A0, high: 0x1F0FF},
	{name: "EnclosedAlphanumericSupplement", low: 0x1F100, high: 0x1F1FF},
	{name: "EnclosedIdeographicSupplement", low: 0x1F200, high: 0x1F2FF},
	{name: "MiscellaneousSymbolsandPictographs", low: 0x1F300, high: 0x1F5FF},
	{name: "Emoticons", low: 0x1F600, high: 0x1F64F},
	{name: "OrnamentalDingbats", low: 0x1F650, high: 0x1F67F},
	{name: "TransportandMapSymbols", low: 0x1F680, high: 0x1F6FF},
	{name: "AlchemicalSymbols", low: 0x1F700, high: 0x1F77F},
	{name: "GeometricShapesExtended", low: 0x1F780, high: 0x1F7FF},
	{name: "SupplementalArrows-C", low: 0x1F800, high: 0x1F8FF},
	{name: "SupplementalSymbolsandPictographs", low: 0x1F900, high: 0x1F9FF},
	{name: "ChessSymbols", low: 0x1FA00, high: 0x1FA6F},
	{name: "SymbolsandPictographsExtended-A", low: 0x1FA70, high: 0x1FAFF},
	{name: "SymbolsforLegacyComputing", low: 0x1FB00, high: 0x1FBFF},
	{name: "CJKUnifiedIdeographsExtensionB", low: 0x20000, high: 0x2A6DF},
	{name: "CJKUnifiedIdeographsExtensionC", low: 0x2A700, high: 0x2B73F},
	{name: "CJKUnifiedIdeographsExtensionD", low: 0x2B740, high: 0x2B81F},
	{name: "CJKUnifiedIdeographsExtensionE", low: 0x2B820, high: 0x2CEAF},
	{name: "CJKUnifiedIdeographsExtensionF", low: 0x2CEB0, high: 0x2EBEF},
	{name: "CJKCompatibilityIdeographsSupplement", low: 0x2F800, high: 0x2FA1F},
	{name: "CJKUnifiedIdeographsExtensionG", low: 0x30000, high: 0x3134F},
	{name: "CJKUnifiedIdeographsExtensionH", low: 0x31350, high: 0x323AF},
	{name: "Tags", low: 0xE0000, high: 0xE007F},
	{name: "VariationSelectorsSupplement", low: 0xE0100, high: 0xE01EF},
	{name: "SupplementaryPrivateUseArea-A", low: 0xF0000, high: 0xFFFFF},
	{name: "SupplementaryPrivateUseArea-B", low: 0x100000, high: 0x10FFFF},
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
	return !unicode.Is(unicode.P, value) && !unicode.Is(unicode.Z, value) && !precisionDecimalXMLCategoryC(value)
}

func precisionDecimalXMLCategoryC(value rune) bool {
	return unicode.Is(unicode.C, value) || precisionDecimalXMLUnassigned(value)
}

func precisionDecimalXMLUnassigned(value rune) bool {
	if !precisionDecimalXMLChar(value) {
		return false
	}
	if table, ok := unicode.Categories["Cn"]; ok {
		return unicode.Is(table, value)
	}
	return !unicode.Is(unicode.L, value) &&
		!unicode.Is(unicode.M, value) &&
		!unicode.Is(unicode.N, value) &&
		!unicode.Is(unicode.P, value) &&
		!unicode.Is(unicode.Z, value) &&
		!unicode.Is(unicode.S, value) &&
		!unicode.Is(unicode.C, value)
}

func (regex *precisionDecimalXMLRegex) match(source string) (bool, error) {
	if len(source) > precisionDecimalXMLRegexMaxSourceBytes {
		return false, errPrecisionDecimalXMLRegexMatchResourceLimit
	}
	if !utf8.ValidString(source) {
		return false, nil
	}
	input := []rune(source)
	if len(input) > precisionDecimalXMLRegexMaxSourceRunes {
		return false, errPrecisionDecimalXMLRegexMatchResourceLimit
	}
	for _, value := range input {
		if precisionDecimalXMLChar(value) {
			continue
		}
		return false, nil
	}
	budget := precisionDecimalXMLRegexMatchBudget{remaining: precisionDecimalXMLRegexMaxMatchWork}
	for _, branch := range regex.branches {
		positions, err := precisionDecimalXMLRegexBranchPositionsWithBudget(branch, input, 0, &budget)
		if err != nil {
			return false, err
		}
		for _, position := range positions {
			if position == len(input) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (regex *precisionDecimalXMLRegex) matches(source string) bool {
	matched, err := regex.match(source)
	if err != nil {
		return false
	}
	return matched
}

func precisionDecimalXMLRegexBranchPositionsWithBudget(branch precisionDecimalXMLRegexBranch, input []rune, start int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	memo := make(map[precisionDecimalXMLRegexMatchState][]int)
	return precisionDecimalXMLRegexBranchPositionsFrom(branch, input, 0, start, memo, budget)
}

func precisionDecimalXMLRegexBranchPositionsFrom(branch precisionDecimalXMLRegexBranch, input []rune, pieceIndex, position int, memo map[precisionDecimalXMLRegexMatchState][]int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	if err := budget.consume(); err != nil {
		return nil, err
	}
	key := precisionDecimalXMLRegexMatchState{piece: pieceIndex, pos: position}
	if result, ok := memo[key]; ok {
		return result, nil
	}
	if pieceIndex == len(branch.pieces) {
		result := []int{position}
		memo[key] = result
		return result, nil
	}
	piece := branch.pieces[pieceIndex]
	ends, err := precisionDecimalXMLRepeatPositions(piece.atom, piece.min, piece.max, input, position, budget)
	if err != nil {
		return nil, err
	}
	result := make([]int, 0)
	for _, end := range ends {
		suffix, err := precisionDecimalXMLRegexBranchPositionsFrom(branch, input, pieceIndex+1, end, memo, budget)
		if err != nil {
			return nil, err
		}
		for _, candidate := range suffix {
			if err := precisionDecimalXMLAppendUnique(&result, candidate, budget); err != nil {
				return nil, err
			}
		}
	}
	memo[key] = result
	return result, nil
}

func precisionDecimalXMLRepeatPositions(atom precisionDecimalXMLRegexAtom, minimumCount, maximumCount *big.Int, input []rune, start int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	nullable := precisionDecimalXMLAtomNullable(atom)
	limit := len(input)
	if nullable {
		limit++
	}
	minimum, maximum, ok := precisionDecimalXMLRepeatBounds(minimumCount, maximumCount, limit, nullable)
	if !ok {
		return nil, nil
	}
	return precisionDecimalXMLRepeatPositionsWithinBounds(atom, minimum, maximum, input, start, budget)
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

func precisionDecimalXMLRepeatPositionsWithinBounds(atom precisionDecimalXMLRegexAtom, minimum, maximum int, input []rune, start int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	positions := []int{start}
	result := make([]int, 0)
	if minimum == 0 {
		if err := precisionDecimalXMLAppendUnique(&result, start, budget); err != nil {
			return nil, err
		}
	}
	for count := 1; count <= maximum; count++ {
		next, err := precisionDecimalXMLNextPositions(atom, input, positions, budget)
		if err != nil {
			return nil, err
		}
		if len(next) == 0 {
			break
		}
		positions = next
		if count < minimum {
			continue
		}
		if err := precisionDecimalXMLAppendPositions(&result, positions, budget); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func precisionDecimalXMLAppendPositions(result *[]int, positions []int, budget *precisionDecimalXMLRegexMatchBudget) error {
	for _, position := range positions {
		if err := precisionDecimalXMLAppendUnique(result, position, budget); err != nil {
			return err
		}
	}
	return nil
}

func precisionDecimalXMLNextPositions(atom precisionDecimalXMLRegexAtom, input []rune, positions []int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	next := make([]int, 0)
	for _, position := range positions {
		ends, err := precisionDecimalXMLAtomPositions(atom, input, position, budget)
		if err != nil {
			return nil, err
		}
		for _, end := range ends {
			if err := precisionDecimalXMLAppendUnique(&next, end, budget); err != nil {
				return nil, err
			}
		}
	}
	return next, nil
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

func precisionDecimalXMLAtomPositions(atom precisionDecimalXMLRegexAtom, input []rune, position int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	if err := budget.consume(); err != nil {
		return nil, err
	}
	switch atom.kind {
	case precisionDecimalXMLRegexLiteralAtom:
		return precisionDecimalXMLLiteralPositions(atom, input, position)
	case precisionDecimalXMLRegexSetAtom:
		return precisionDecimalXMLSetPositions(atom, input, position)
	case precisionDecimalXMLRegexGroupAtom:
		return precisionDecimalXMLGroupPositions(atom, input, position, budget)
	default:
		panic("precisionDecimal XML regex: invalid atom kind")
	}
}

func precisionDecimalXMLLiteralPositions(atom precisionDecimalXMLRegexAtom, input []rune, position int) ([]int, error) {
	if position < len(input) && input[position] == atom.literal {
		return []int{position + 1}, nil
	}
	return nil, nil
}

func precisionDecimalXMLSetPositions(atom precisionDecimalXMLRegexAtom, input []rune, position int) ([]int, error) {
	if position < len(input) && atom.set.contains(input[position]) {
		return []int{position + 1}, nil
	}
	return nil, nil
}

func precisionDecimalXMLGroupPositions(atom precisionDecimalXMLRegexAtom, input []rune, position int, budget *precisionDecimalXMLRegexMatchBudget) ([]int, error) {
	result := []int{}
	for _, branch := range atom.group.branches {
		positions, err := precisionDecimalXMLRegexBranchPositionsWithBudget(branch, input, position, budget)
		if err != nil {
			return nil, err
		}
		if err := precisionDecimalXMLAppendPositions(&result, positions, budget); err != nil {
			return nil, err
		}
	}
	return result, nil
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

func precisionDecimalXMLAppendUnique(values *[]int, candidate int, budget *precisionDecimalXMLRegexMatchBudget) error {
	for _, value := range *values {
		if err := budget.consume(); err != nil {
			return err
		}
		if value == candidate {
			return nil
		}
	}
	if err := budget.consume(); err != nil {
		return err
	}
	*values = append(*values, candidate)
	return nil
}

func (budget *precisionDecimalXMLRegexMatchBudget) consume() error {
	if budget.remaining <= 0 {
		return errPrecisionDecimalXMLRegexMatchResourceLimit
	}
	budget.remaining--
	return nil
}
