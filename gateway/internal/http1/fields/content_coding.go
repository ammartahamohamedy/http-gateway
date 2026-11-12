package fields

import (
	"errors"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

var (
	// ErrInvalidContentEncoding reports a malformed Content-Encoding list.
	ErrInvalidContentEncoding = errors.New("http1: Content-Encoding requires a list of content codings")

	// ErrInvalidAcceptEncoding reports a malformed Accept-Encoding list.
	ErrInvalidAcceptEncoding = errors.New("http1: Accept-Encoding requires a list of codings with optional weights")
)

// ValidateContentEncoding checks a list of content-coding tokens (RFC 9110,
// Section 8.4). Values must have surrounding SP/HTAB already removed. Absence,
// an empty value, repeated field lines, and empty list members are valid.
// Unfamiliar codings are accepted: this checks syntax only and neither decodes
// the body nor claims the gateway supports a coding. Errors match
// ErrInvalidContentEncoding.
func ValidateContentEncoding(values []string) error {
	for value := range slices.Values(values) {
		members, ok := grammar.SplitList(value)
		if !ok {
			return ErrInvalidContentEncoding
		}
		for member := range slices.Values(members) {
			if !grammar.IsToken(member) {
				return ErrInvalidContentEncoding
			}
		}
	}
	return nil
}

// ValidateAcceptEncoding checks a list of codings, each with an optional weight
// (RFC 9110, Section 12.5.3). Values must have surrounding SP/HTAB already
// removed. "identity" and "*" need no special case because both are tokens.
// Absence, an empty value, repeated field lines, and empty list members are
// valid. The weight is the only parameter allowed. This selects no coding, and
// the gateway compresses nothing. Errors match ErrInvalidAcceptEncoding.
func ValidateAcceptEncoding(values []string) error {
	for value := range slices.Values(values) {
		members, ok := grammar.SplitList(value)
		if !ok {
			return ErrInvalidAcceptEncoding
		}
		for member := range slices.Values(members) {
			if !isAcceptEncodingMember(member) {
				return ErrInvalidAcceptEncoding
			}
		}
	}
	return nil
}

// isAcceptEncodingMember reports whether member is a coding token, optionally
// followed by one ";" and a weight.
func isAcceptEncodingMember(member string) bool {
	coding, weight, weighted := strings.Cut(member, ";")
	if !grammar.IsToken(grammar.TrimOWS(coding)) {
		return false
	}
	if !weighted {
		return true
	}
	name, qvalue, found := strings.Cut(grammar.TrimOWS(weight), "=")
	return found && (name == "q" || name == "Q") && grammar.IsQValue(qvalue)
}
