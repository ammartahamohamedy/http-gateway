package fields

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ianaTransferCodingNames lists the HTTP Transfer Coding Registry entries,
// including its deprecated, withdrawn, and reserved names.
// https://www.iana.org/assignments/http-parameters/http-parameters.xhtml#transfer-coding
var ianaTransferCodingNames = []string{
	"chunked",
	"compress",
	"deflate",
	"gzip",
	"identity",   // withdrawn
	"trailers",   // reserved for TE
	"x-compress", // deprecated alias for compress
	"x-gzip",     // deprecated alias for gzip
}

// supportedTransferCodings lists the transfer codings the gateway accepts.
// "deflate" is the zlib format, not raw deflate (RFC 9110, Section 8.4.1.2).
var supportedTransferCodings = []string{"chunked", "gzip", "deflate"}

var (
	// ErrInvalidTransferEncoding reports a malformed Transfer-Encoding list, or
	// one whose codings do not end in exactly one parameterless chunked.
	ErrInvalidTransferEncoding = errors.New("http1: Transfer-Encoding requires a valid coding list ending in one chunked coding")

	// ErrUnsupportedTransferCoding reports a registry name the gateway cannot decode.
	ErrUnsupportedTransferCoding = errors.New("http1: unsupported transfer coding")

	// ErrUnknownTransferCoding reports a name absent from the transfer coding registry.
	ErrUnknownTransferCoding = errors.New("http1: unknown transfer coding")

	// ErrFramingConflict reports a request carrying both Transfer-Encoding and
	// Content-Length, whose disagreement enables request smuggling.
	ErrFramingConflict = errors.New("http1: request must not combine Transfer-Encoding and Content-Length")
)

// ParseTransferEncoding returns the request's transfer-coding names in the order
// they were applied, lowercased, or nil when the field is absent. Values must
// have surrounding SP/HTAB already removed. Repeated field lines form one list,
// and empty list members are ignored (RFC 9110, Section 5.6.1).
//
// A request's final coding must be chunked, or its length cannot be determined
// (RFC 9112, Section 6.3), and chunked must be applied once and take no
// parameters (RFC 9112, Section 7). Other codings need only be syntactically
// valid, and their parameters are checked but not returned. Decoder support is
// checked separately by CheckTransferCodingSupport. This function does not check
// Content-Length conflicts. Errors match ErrInvalidTransferEncoding and return
// nil codings. The input is not changed.
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
			if !ok {
				return nil, ErrInvalidTransferEncoding
			}
			codings = append(codings, name)
		}
	}
	if len(codings) == 0 {
		return nil, ErrInvalidTransferEncoding
	}
	last := len(codings) - 1
	if codings[last] != "chunked" || slices.Contains(codings[:last], "chunked") {
		return nil, ErrInvalidTransferEncoding
	}
	return codings, nil
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

// parseTransferCoding validates one list member, a token followed by optional
// ";"-separated parameters, and returns its lowercase name. The token check
// comes first so lowercasing cannot fold a non-ASCII rune into "chunked".
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
