package fields

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ianaTransferCodingNames lists the HTTP Transfer Coding Registry names, except
// the deprecated aliases that ParseTransferEncoding replaces.
// https://www.iana.org/assignments/http-parameters/http-parameters.xhtml#transfer-coding
var ianaTransferCodingNames = []string{
	"chunked",
	"compress",
	"deflate",
	"gzip",
	"identity", // withdrawn
	"trailers", // reserved for TE
}

// transferCodingAliases maps deprecated names to their equivalents (RFC 9112,
// Section 7.2).
var transferCodingAliases = map[string]string{
	"x-compress": "compress",
	"x-gzip":     "gzip",
}

// supportedTransferCodings lists the accepted transfer codings. "deflate" is the
// zlib format (RFC 9110, Section 8.4.1.2).
var supportedTransferCodings = []string{"chunked", "gzip", "deflate"}

var (
	// ErrInvalidTransferEncoding reports a malformed list, or chunked with
	// parameters or applied more than once.
	ErrInvalidTransferEncoding = errors.New("http1: Transfer-Encoding requires a valid coding list applying chunked at most once")

	// ErrChunkedNotFinal reports request codings not ending in chunked, which leave
	// the length undetermined (RFC 9112, Section 6.3).
	ErrChunkedNotFinal = errors.New("http1: request Transfer-Encoding must end in chunked")

	// ErrUnsupportedTransferCoding reports a registry name the gateway cannot decode.
	ErrUnsupportedTransferCoding = errors.New("http1: unsupported transfer coding")

	// ErrUnknownTransferCoding reports a name absent from the transfer coding registry.
	ErrUnknownTransferCoding = errors.New("http1: unknown transfer coding")

	// ErrFramingConflict reports a message with both Transfer-Encoding and
	// Content-Length, a smuggling and splitting vector (RFC 9112, Section 6.3).
	ErrFramingConflict = errors.New("http1: message must not combine Transfer-Encoding and Content-Length")
)

// ParseTransferEncoding returns a message's transfer-coding names in applied
// order, lowercased, with x-gzip and x-compress replaced by gzip and compress (RFC
// 9112, Section 7.2); nil when absent. Values must have surrounding SP/HTAB
// already removed. Repeated field lines form one list, and empty members are
// ignored (RFC 9110, Section 5.6.1).
//
// Chunked may appear at most once, without parameters (RFC 9112, Sections 6.1 and
// 7); callers check its position, as ValidateRequestTransferCodings does. Other
// codings need only valid syntax; their parameters are checked, not returned.
// Support and Content-Length conflicts are checked elsewhere. Errors match
// ErrInvalidTransferEncoding and return nil codings. The input is not changed.
func ParseTransferEncoding(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var codings []string
	for value := range slices.Values(values) {
		members, ok := grammar.SplitList(value)
		if !ok {
			return nil, ErrInvalidTransferEncoding
		}
		for member := range slices.Values(members) {
			name, ok := parseTransferCoding(member)
			if !ok || (name == "chunked" && slices.Contains(codings, "chunked")) {
				return nil, ErrInvalidTransferEncoding
			}
			codings = append(codings, name)
		}
	}
	if len(codings) == 0 {
		return nil, ErrInvalidTransferEncoding
	}
	return codings, nil
}

// ValidateRequestTransferCodings requires request codings to end in chunked
// (RFC 9112, Section 6.3). Absence is valid. Errors match ErrChunkedNotFinal.
func ValidateRequestTransferCodings(codings []string) error {
	if len(codings) > 0 && codings[len(codings)-1] != "chunked" {
		return ErrChunkedNotFinal
	}
	return nil
}

// CheckTransferCodingSupport checks the lowercase names from ParseTransferEncoding
// after framing validation. The gateway accepts chunked, gzip, and deflate. The
// first other name returns ErrUnsupportedTransferCoding if listed in the IANA
// registry, or ErrUnknownTransferCoding otherwise. It reads no body data.
func CheckTransferCodingSupport(codings []string) error {
	for coding := range slices.Values(codings) {
		if slices.Contains(supportedTransferCodings, coding) {
			continue
		}
		if slices.Contains(ianaTransferCodingNames, coding) {
			return fmt.Errorf("%w: %q", ErrUnsupportedTransferCoding, coding)
		}
		return fmt.Errorf("%w: %q", ErrUnknownTransferCoding, coding)
	}
	return nil
}

// parseTransferCoding validates one list member, a token with optional
// ";"-separated parameters, and returns its lowercase name, aliases replaced. The
// token check comes first so lowercasing cannot fold a non-ASCII rune into
// "chunked".
func parseTransferCoding(member string) (string, bool) {
	parts, ok := grammar.SplitUnquoted(member, ';')
	if !ok {
		return "", false
	}
	name := grammar.TrimOWS(parts[0])
	if !grammar.IsToken(name) {
		return "", false
	}
	name = strings.ToLower(name)
	if canonical, found := transferCodingAliases[name]; found {
		name = canonical
	}
	parameters := parts[1:]
	if name == "chunked" && len(parameters) > 0 {
		return "", false
	}
	for parameter := range slices.Values(parameters) {
		if !isTransferParameter(parameter) {
			return "", false
		}
	}
	return name, true
}

// isTransferParameter reports whether s is OWS transfer-parameter OWS, where
// transfer-parameter is token BWS "=" BWS ( token / quoted-string ) (RFC 9110,
// Section 10.1.4).
func isTransferParameter(s string) bool {
	name, value, found := strings.Cut(s, "=")
	if !found || !grammar.IsToken(grammar.TrimOWS(name)) {
		return false
	}
	value = grammar.TrimOWS(value)
	return grammar.IsToken(value) || grammar.IsQuotedString(value)
}
