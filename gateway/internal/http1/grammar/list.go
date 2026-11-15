package grammar

import (
	"slices"
	"strings"
)

// SplitList splits one field-line value into its comma-separated list members
// (RFC 9110, Section 5.6.1). Commas inside quoted-strings do not separate
// members. Members have surrounding SP/HTAB removed, and empty members are
// dropped, as recipients must accept them. It reports false for an unterminated
// quoted-string. Member syntax is left to the caller.
func SplitList(value string) ([]string, bool) {
	parts, ok := SplitUnquoted(value, ',')
	if !ok {
		return nil, false
	}

	members := make([]string, 0, len(parts))
	for part := range slices.Values(parts) {
		if member := TrimOWS(part); member != "" {
			members = append(members, member)
		}
	}

	return members, true
}

// SplitUnquoted splits text at every sep outside a quoted-string, keeping empty
// parts and all other bytes. A backslash escapes the next byte only inside a
// quoted-string. It reports false for an unterminated quoted-string.
func SplitUnquoted(text string, sep byte) ([]string, bool) {
	var parts []string
	start := 0
	quoted, escaped := false, false

	for index, octet := range []byte(text) {
		switch {
		case escaped:
			escaped = false
		case quoted && octet == '\\':
			escaped = true
		case octet == '"':
			quoted = !quoted
		case !quoted && octet == sep:
			parts = append(parts, text[start:index])
			start = index + 1
		}
	}

	if quoted {
		return nil, false
	}

	return append(parts, text[start:]), true
}

// TrimOWS removes surrounding optional whitespace: SP and HTAB only.
func TrimOWS(text string) string {
	return strings.Trim(text, " \t")
}

// IsQuotedString reports whether text is exactly one RFC 9110 quoted-string,
// including its surrounding quotes (Section 5.6.4). Bytes are checked directly
// so obs-text is accepted whether or not it is valid UTF-8.
func IsQuotedString(text string) bool {
	if len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return false
	}

	escaped := false
	for index := 1; index < len(text)-1; index++ {
		octet := text[index]
		switch {
		case (octet < 0x20 && octet != '\t') || octet == 0x7f:
			return false
		case escaped:
			escaped = false
		case octet == '\\':
			escaped = true
		case octet == '"':
			return false
		}
	}

	return !escaped
}

// IsParameter reports whether text is exactly one RFC 9110 parameter:
// token "=" ( token / quoted-string ), with no whitespace around "=" (Section
// 5.6.6). Fields that allow bad whitespace there check it themselves.
func IsParameter(text string) bool {
	name, value, found := strings.Cut(text, "=")
	return found && IsToken(name) && (IsToken(value) || IsQuotedString(value))
}

// IsChunkExtension reports whether text is one chunk extension without its ";":
// chunk-ext-name [ BWS "=" BWS chunk-ext-val ], where the value is a token or
// quoted-string (RFC 9112, Section 7.1.1). Whitespace around text is the
// caller's.
func IsChunkExtension(text string) bool {
	name, value, hasValue := strings.Cut(text, "=")
	if !hasValue {
		return IsToken(name)
	}

	value = strings.TrimLeft(value, " \t")
	return IsToken(strings.TrimRight(name, " \t")) && (IsToken(value) || IsQuotedString(value))
}
