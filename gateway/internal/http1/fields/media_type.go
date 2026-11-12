package fields

import (
	"errors"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

var (
	// ErrInvalidContentType reports a repeated or malformed Content-Type field.
	ErrInvalidContentType = errors.New("http1: Content-Type requires one valid media type")

	// ErrInvalidAccept reports a malformed Accept list.
	ErrInvalidAccept = errors.New("http1: Accept requires a valid media range list")
)

// ValidateContentType requires at most one Content-Type field line, holding one
// type "/" subtype and optional parameters (RFC 9110, Sections 8.3.1 and 5.6.6).
// Values must have surrounding SP/HTAB already removed. Absence is valid. The
// field is not a list, so a comma outside a quoted-string is an error. Wildcards
// are not special here, since "*" is a token. It does not judge whether the media
// type is registered or supported. Errors match ErrInvalidContentType.
func ValidateContentType(values []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) != 1 {
		return ErrInvalidContentType
	}
	parts, ok := grammar.SplitUnquoted(values[0], ';')
	if !ok {
		return ErrInvalidContentType
	}
	mediaType, subtype, found := strings.Cut(grammar.TrimOWS(parts[0]), "/")
	if !found || !grammar.IsToken(mediaType) || !grammar.IsToken(subtype) {
		return ErrInvalidContentType
	}
	for parameter := range slices.Values(parts[1:]) {
		parameter = grammar.TrimOWS(parameter)
		if parameter != "" && !grammar.IsParameter(parameter) {
			return ErrInvalidContentType
		}
	}
	return nil
}

// ValidateAccept checks a list of media ranges with optional parameters and a
// weight (RFC 9110, Section 12.5.1). Values must have surrounding SP/HTAB already
// removed. Absence, an empty value, repeated field lines, and empty list members
// are valid. It selects no representation. Errors match ErrInvalidAccept.
func ValidateAccept(values []string) error {
	for value := range slices.Values(values) {
		members, ok := grammar.SplitList(value)
		if !ok {
			return ErrInvalidAccept
		}
		for member := range slices.Values(members) {
			if !isAcceptMember(member) {
				return ErrInvalidAccept
			}
		}
	}
	return nil
}

// isAcceptMember reports whether member is a media range with optional
// parameters. A parameter named "q", in either case, is the weight: it must be a
// valid qvalue and come last, ignoring empty parameters.
func isAcceptMember(member string) bool {
	parts, ok := grammar.SplitUnquoted(member, ';')
	if !ok || !isMediaRange(grammar.TrimOWS(parts[0])) {
		return false
	}
	weighted := false
	for parameter := range slices.Values(parts[1:]) {
		parameter = grammar.TrimOWS(parameter)
		if parameter == "" {
			continue
		}
		if weighted {
			return false
		}
		name, value, _ := strings.Cut(parameter, "=")
		if name == "q" || name == "Q" {
			if !grammar.IsQValue(value) {
				return false
			}
			weighted = true
		} else if !grammar.IsParameter(parameter) {
			return false
		}
	}
	return true
}

// isMediaRange reports whether s is */*, type/*, or type/subtype. A "*" is a
// valid token, so */subtype needs its own rejection.
func isMediaRange(s string) bool {
	mediaType, subtype, found := strings.Cut(s, "/")
	if !found || !grammar.IsToken(mediaType) || !grammar.IsToken(subtype) {
		return false
	}
	return mediaType != "*" || subtype == "*"
}
