package goxsd9

import (
	"net/url"
	"strings"
)

// legacyURIReference checks the XLink escaping and URI-reference lexical
// boundary used by XSD 1.0 anyURI and XML Base. The datatype caller applies
// its whitespace collapse first; XML Base uses the XML attribute value.
func legacyURIReference(value string) bool { //nolint:gocognit // URI phases remain in lexical order.
	escaped := xlinkEscapeURI(value)
	for index := 0; index < len(escaped); index++ {
		if escaped[index] != '%' {
			continue
		}
		if index+2 >= len(escaped) || !isHexDigit(escaped[index+1]) || !isHexDigit(escaped[index+2]) {
			return false
		}
		index += 2
	}
	if strings.Count(escaped, "#") > 1 {
		return false
	}
	if strings.HasPrefix(escaped, "?") {
		return false
	}
	parsed, err := url.Parse(escaped)
	if err != nil && legacyRegisteredNameAuthority(escaped) {
		parsed, err = url.Parse(replaceLegacyAuthority(escaped))
	}
	if err != nil {
		return false
	}
	if parsed.Scheme != "" && legacyEmptySchemePayload(escaped, parsed.Scheme) {
		return false
	}
	if legacyIPv6Zone(escaped, parsed.Scheme) {
		return false
	}
	if parsed.Opaque != "" {
		return !strings.ContainsAny(escaped[len(parsed.Scheme)+1:len(parsed.Scheme)+2], "[]")
	}
	path := escaped
	if fragment := strings.IndexByte(path, '#'); fragment >= 0 {
		path = path[:fragment]
	}
	if query := strings.IndexByte(path, '?'); query >= 0 {
		path = path[:query]
	}
	if parsed.Scheme != "" {
		path = path[len(parsed.Scheme)+1:]
	}
	if strings.HasPrefix(path, "//") {
		authority := path[2:]
		path = ""
		if slash := strings.IndexByte(authority, '/'); slash >= 0 {
			path = authority[slash:]
		}
	}
	return !strings.ContainsAny(path, "[]")
}

func legacyEmptySchemePayload(value, scheme string) bool {
	part := value[len(scheme)+1:]
	if separator := strings.IndexByte(part, '#'); separator >= 0 {
		part = part[:separator]
	}
	return part == ""
}

// RFC 2732 IPv6 references do not admit the later zone identifier syntax.
func legacyIPv6Zone(value, scheme string) bool {
	start := 0
	if scheme != "" {
		start = len(scheme) + 1
	}
	if !strings.HasPrefix(value[start:], "//") {
		return false
	}
	start += 2
	end := start + strings.IndexAny(value[start:], "/?#")
	if end < start {
		end = len(value)
	}
	open := strings.IndexByte(value[start:end], '[')
	closingBracket := strings.IndexByte(value[start:end], ']')
	return open >= 0 && closingBracket > open && strings.Contains(value[start+open:start+closingBracket], "%")
}

// RFC 2396 permits an authority to be a registered name even when Go's URL
// parser interprets the same text as an invalid host or port.
func legacyRegisteredNameAuthority(value string) bool {
	start := 0
	if scheme := strings.IndexByte(value, ':'); scheme >= 0 && !strings.HasPrefix(value, "//") {
		start = scheme + 1
	}
	if !strings.HasPrefix(value[start:], "//") {
		return false
	}
	start += 2
	end := start + strings.IndexAny(value[start:], "/?#")
	if end < start {
		end = len(value)
	}
	if end == start {
		return false
	}
	for index := start; index < end; index++ {
		character := value[index]
		if character == '%' {
			index += 2 // legacyURIReference already checked the triplet.
			continue
		}
		if isLegacyUnreserved(character) || strings.ContainsRune("$,;:@&=+", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func replaceLegacyAuthority(value string) string {
	start := 0
	if scheme := strings.IndexByte(value, ':'); scheme >= 0 && !strings.HasPrefix(value, "//") {
		start = scheme + 1
	}
	start += 2
	end := start + strings.IndexAny(value[start:], "/?#")
	if end < start {
		end = len(value)
	}
	return value[:start] + "example.invalid" + value[end:]
}

func xlinkEscapeURI(value string) string {
	const hex = "0123456789ABCDEF"
	var escaped strings.Builder
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isLegacyURILiteral(character) {
			escaped.WriteByte(character)
			continue
		}
		escaped.WriteByte('%')
		escaped.WriteByte(hex[character>>4])
		escaped.WriteByte(hex[character&0x0f])
	}
	return escaped.String()
}

func isLegacyURILiteral(character byte) bool {
	return isLegacyUnreserved(character) || strings.ContainsRune("();/?:@&=+$,#%[]", rune(character))
}

func isLegacyUnreserved(character byte) bool {
	if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
		return true
	}
	return strings.ContainsRune("-_.!~*'", rune(character))
}
