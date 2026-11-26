package goxsd9

import (
	"net/netip"
	"net/url"
	"strconv"
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
	parseValue := escaped
	if start, end, present := legacyAuthorityBounds(escaped); present {
		if !legacyAuthority(escaped[start:end]) {
			return false
		}
		parseValue = escaped[:start] + "example.invalid" + escaped[end:]
	}
	parsed, err := url.Parse(parseValue)
	if err != nil {
		return false
	}
	if parsed.Scheme != "" && legacyEmptySchemePayload(escaped, parsed.Scheme) {
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

// RFC 2396 puts authority after a leading // or an absolute URI scheme.
func legacyAuthorityBounds(value string) (int, int, bool) {
	start := 2
	if !strings.HasPrefix(value, "//") {
		separator := strings.Index(value, "://")
		if separator < 0 || !legacyScheme(value[:separator]) {
			return 0, 0, false
		}
		start = separator + 3
	}
	end := start
	for end < len(value) && !strings.ContainsRune("/?#", rune(value[end])) {
		end++
	}
	return start, end, true
}

func legacyScheme(value string) bool {
	if value == "" || !isLegacyAlpha(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if isLegacyAlpha(character) || character >= '0' && character <= '9' || strings.ContainsRune("+.-", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func isLegacyAlpha(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

// RFC 2732 reserves literal brackets for an IPv6 host in server syntax.
func legacyAuthority(authority string) bool {
	if authority == "" {
		return true // RFC 2396 server is optional.
	}
	open := strings.IndexByte(authority, '[')
	if open < 0 {
		return legacyAuthorityCharacters(authority, "$,;:@&=+")
	}
	userinfo := authority[:open]
	if userinfo != "" {
		if userinfo[len(userinfo)-1] != '@' || !legacyAuthorityCharacters(userinfo[:len(userinfo)-1], ";:&=+$,") {
			return false
		}
	}
	hostport := authority[open:]
	closingBracket := strings.IndexByte(hostport, ']')
	if closingBracket < 0 || !legacyIPv6Address(hostport[1:closingBracket]) {
		return false
	}
	port := hostport[closingBracket+1:]
	if port == "" {
		return true
	}
	if port[0] != ':' {
		return false
	}
	for index := 1; index < len(port); index++ {
		if port[index] < '0' || port[index] > '9' {
			return false
		}
	}
	return true
}

func legacyAuthorityCharacters(value, reserved string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '%' {
			index += 2 // legacyURIReference already checked percent triplets.
			continue
		}
		if isLegacyUnreserved(character) || strings.ContainsRune(reserved, rune(character)) {
			continue
		}
		return false
	}
	return true
}

// RFC 2373 permits one to three decimal digits in each embedded IPv4 octet.
func legacyIPv6Address(value string) bool {
	if strings.Contains(value, "%") { // RFC 2732 predates IPv6 zone identifiers.
		return false
	}
	parseValue := value
	if strings.Contains(value, ".") {
		var valid bool
		parseValue, valid = normalizeLegacyIPv4Tail(value)
		if !valid {
			return false
		}
	}
	address, err := netip.ParseAddr(parseValue)
	return err == nil && address.Is6() && address.Zone() == ""
}

func normalizeLegacyIPv4Tail(value string) (string, bool) {
	separator := strings.LastIndexByte(value, ':')
	if separator < 0 {
		return "", false
	}
	parts := strings.Split(value[separator+1:], ".")
	if len(parts) != 4 {
		return "", false
	}
	for index, part := range parts {
		normalized, valid := normalizeLegacyIPv4Octet(part)
		if !valid {
			return "", false
		}
		parts[index] = normalized
	}
	return value[:separator+1] + strings.Join(parts, "."), true
}

func normalizeLegacyIPv4Octet(part string) (string, bool) {
	if len(part) < 1 || len(part) > 3 {
		return "", false
	}
	number := 0
	for position := 0; position < len(part); position++ {
		if part[position] < '0' || part[position] > '9' {
			return "", false
		}
		number = number*10 + int(part[position]-'0')
	}
	if number > 255 {
		return "", false
	}
	return strconv.Itoa(number), true
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
	return isLegacyUnreserved(character) || strings.ContainsRune(";/?:@&=+$,#%[]", rune(character))
}

func isLegacyUnreserved(character byte) bool {
	if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
		return true
	}
	return strings.ContainsRune("-_.!~*'()", rune(character))
}
