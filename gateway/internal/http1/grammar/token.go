// Package grammar checks shared HTTP syntax rules from RFC 9110 and RFC 9112.
// It performs no I/O and knows nothing about specific fields or messages.
package grammar

import "regexp"

// Patterns are compiled once. The \A and \z anchors require a whole-input match.
var (
	// tokenPattern matches a nonempty token: ASCII letters, digits, and the
	// punctuation permitted by tchar (RFC 9110, Section 5.6.2).
	tokenPattern = regexp.MustCompile("\\A[A-Za-z0-9!#$%&'*+.^_`|~-]+\\z")

	// digitsPattern matches 1*DIGIT: ASCII decimal digits, excluding signs.
	digitsPattern = regexp.MustCompile(`\A[0-9]+\z`)

	// hexDigitsPattern matches 1*HEXDIG: ASCII hexadecimal digits in either case.
	hexDigitsPattern = regexp.MustCompile(`\A[0-9A-Fa-f]+\z`)

	// qvaluePattern matches a weight's value from 0 to 1 with at most three
	// decimal places (RFC 9110, Section 12.4.2).
	qvaluePattern = regexp.MustCompile(`\A(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)\z`)
)

// IsToken reports whether text is a nonempty RFC 9110 token. Methods and field
// names use this grammar; case is not interpreted.
func IsToken(text string) bool {
	return tokenPattern.MatchString(text)
}

// IsDigits reports whether text is one or more ASCII decimal digits. It checks
// syntax only, not numeric range.
func IsDigits(text string) bool {
	return digitsPattern.MatchString(text)
}

// IsHexDigits reports whether text is one or more ASCII hexadecimal digits. It
// checks syntax only, not numeric range.
func IsHexDigits(text string) bool {
	return hexDigitsPattern.MatchString(text)
}

// IsQValue reports whether text is a valid weight value, such as "0.5" or
// "1.000".
func IsQValue(text string) bool {
	return qvaluePattern.MatchString(text)
}

// IsFieldValue reports whether text contains only field-value bytes: visible
// ASCII, SP, HTAB, and obs-text (0x80-0xFF). It rejects other controls and DEL.
// Surrounding whitespace is permitted; callers trim it as their grammar requires.
func IsFieldValue(text string) bool {
	for index := range len(text) {
		octet := text[index]
		if (octet < 0x20 && octet != '\t') || octet == 0x7f {
			return false
		}
	}

	return true
}
