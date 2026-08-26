package goxsd9

import (
	"io"
	"unicode/utf8"
)

// xmlLexicalReader captures only the bytes consumed for one XML token. The
// position reader remains the source of truth for offsets and locations; this
// reader does not retain source data between captures.
type xmlLexicalReader struct {
	positions    *syntaxPositionReader
	capture      []byte
	captureStart int64
}

func newXMLLexicalReader(positions *syntaxPositionReader) *xmlLexicalReader {
	return &xmlLexicalReader{positions: positions}
}

func (reader *xmlLexicalReader) ReadByte() (byte, error) {
	value, err := reader.positions.ReadByte()
	if err != nil {
		return 0, err
	}
	reader.captureByte(value)
	return value, nil
}

func (reader *xmlLexicalReader) Read(buffer []byte) (int, error) {
	n, err := reader.positions.Read(buffer)
	for index := 0; index < n; index++ {
		reader.captureByte(buffer[index])
	}
	return n, err
}

func (reader *xmlLexicalReader) beginCapture(logicalOffset int64) {
	reader.capture = reader.capture[:0]
	reader.captureStart = logicalOffset
	missing := reader.positions.offset - logicalOffset
	if missing == 1 {
		// encoding/xml can read the '<' that begins the next token while
		// returning the preceding character-data token. Reconstruct that
		// single lookahead byte in the bounded token capture.
		reader.capture = append(reader.capture, '<')
	}
}

func (reader *xmlLexicalReader) captureByte(value byte) {
	reader.capture = append(reader.capture, value)
}

func (reader *xmlLexicalReader) endCapture(logicalOffset int64) []byte {
	defer func() {
		reader.capture = nil
	}()
	length := logicalOffset - reader.captureStart
	if length < 0 || length > int64(len(reader.capture)) {
		return nil
	}
	return append([]byte(nil), reader.capture[:length]...)
}

type rawXMLAttribute struct {
	value string
	loc   Loc
}

// rawXMLStartTagAttributes scans one already-decoded start-tag token. It
// returns lexical attribute values and name locations in encoding/xml order,
// including namespace declarations so callers can keep attribute indexes
// aligned with xml.StartElement.Attr.
func rawXMLStartTagAttributes(raw []byte, start Loc) ([]rawXMLAttribute, bool) {
	if len(raw) < 2 || raw[0] != '<' {
		return nil, false
	}
	index := rawXMLStartNameEnd(raw, 1)
	attributes := make([]rawXMLAttribute, 0)
	for {
		attribute, done, ok := readRawXMLAttribute(raw, &index, start)
		if !ok {
			return nil, false
		}
		if done {
			return attributes, true
		}
		attributes = append(attributes, attribute)
	}
}

func rawXMLStartTagAttributeLocs(raw []byte, start Loc) ([]Loc, bool) {
	attributes, ok := rawXMLStartTagAttributes(raw, start)
	if !ok {
		return nil, false
	}
	locations := make([]Loc, len(attributes))
	for index, attribute := range attributes {
		locations[index] = attribute.loc
	}
	return locations, true
}

func rawXMLStartNameEnd(raw []byte, index int) int {
	for index < len(raw) && !rawXMLSpace(raw[index]) && raw[index] != '/' && raw[index] != '>' {
		index++
	}
	return index
}

func readRawXMLAttribute(raw []byte, index *int, start Loc) (rawXMLAttribute, bool, bool) {
	hadSpace := skipRawXMLSpace(raw, index)
	if *index >= len(raw) || raw[*index] == '>' {
		return rawXMLAttribute{}, true, true
	}
	if raw[*index] == '/' {
		if *index+1 < len(raw) && raw[*index+1] == '>' {
			return rawXMLAttribute{}, true, true
		}
		return rawXMLAttribute{}, false, false
	}
	if !hadSpace {
		return rawXMLAttribute{}, false, false
	}
	nameStart := *index
	skipRawXMLName(raw, index)
	skipRawXMLSpace(raw, index)
	if *index >= len(raw) || raw[*index] != '=' {
		return rawXMLAttribute{}, false, false
	}
	(*index)++
	skipRawXMLSpace(raw, index)
	if *index >= len(raw) || raw[*index] != '\'' && raw[*index] != '"' {
		return rawXMLAttribute{}, false, false
	}
	quote := raw[*index]
	(*index)++
	valueStart := *index
	for *index < len(raw) && raw[*index] != quote {
		(*index)++
	}
	if *index >= len(raw) {
		return rawXMLAttribute{}, false, false
	}
	value := string(raw[valueStart:*index])
	(*index)++
	return rawXMLAttribute{
		value: value,
		loc:   rawXMLAttributeLoc(start, raw, nameStart),
	}, false, true
}

func rawXMLAttributeLoc(start Loc, raw []byte, offset int) Loc {
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

func skipRawXMLSpace(raw []byte, index *int) bool {
	start := *index
	for *index < len(raw) && rawXMLSpace(raw[*index]) {
		(*index)++
	}
	return *index != start
}

func skipRawXMLName(raw []byte, index *int) {
	for *index < len(raw) && !rawXMLSpace(raw[*index]) && raw[*index] != '=' && raw[*index] != '>' {
		(*index)++
	}
}

func rawXMLSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

var _ io.ByteReader = (*xmlLexicalReader)(nil)
