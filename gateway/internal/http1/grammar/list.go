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

// SplitUnquoted splits s at every sep outside a quoted-string, keeping empty
// parts and all other bytes. A backslash escapes the next byte only inside a
// quoted-string. It reports false for an unterminated quoted-string.
func SplitUnquoted(s string, sep byte) ([]string, bool) {
	var parts []string
	start := 0
	quoted, escaped := false, false
	for i, b := range []byte(s) {
		switch {
		case escaped:
			escaped = false
		case quoted && b == '\\':
			escaped = true
		case b == '"':
			quoted = !quoted
		case !quoted && b == sep:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if quoted {
		return nil, false
	}
	return append(parts, s[start:]), true
}

// TrimOWS removes surrounding optional whitespace: SP and HTAB only.
func TrimOWS(s string) string {
	return strings.Trim(s, " \t")
}

// IsQuotedString reports whether s is exactly one RFC 9110 quoted-string,
// including its surrounding quotes (Section 5.6.4). Bytes are checked directly
// so obs-text is accepted whether or not it is valid UTF-8.
func IsQuotedString(s string) bool {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return false
	}
	escaped := false
	for i := 1; i < len(s)-1; i++ {
		b := s[i]
		switch {
		case (b < 0x20 && b != '\t') || b == 0x7f:
			return false
		case escaped:
			escaped = false
		case b == '\\':
			escaped = true
		case b == '"':
			return false
		}
	}
	return !escaped
}

// IsParameter reports whether s is exactly one RFC 9110 parameter:
// token "=" ( token / quoted-string ), with no whitespace around "=" (Section
// 5.6.6). Fields that allow bad whitespace there check it themselves.
func IsParameter(s string) bool {
	name, value, found := strings.Cut(s, "=")
	return found && IsToken(name) && (IsToken(value) || IsQuotedString(value))
}
