package fields

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

var (
	// ErrInvalidTrailer reports a Trailer list member that is not a field name.
	ErrInvalidTrailer = errors.New("http1: Trailer requires a list of field names")

	// ErrForbiddenTrailerField reports a Trailer field announcing a field that
	// must not be sent as a trailer.
	ErrForbiddenTrailerField = errors.New("http1: Trailer must not announce a forbidden trailer field")
)

// forbiddenTrailerFields lists the lowercase fields that must not be processed
// as trailers: those needed for framing, routing, connection management,
// request modifiers, authentication, response control, or content format (RFC
// 9110, Section 6.5.1). The list is local policy; unlisted fields are allowed.
var forbiddenTrailerFields = []string{
	"content-length", "transfer-encoding", "trailer",
	"host",
	"connection", "keep-alive", "proxy-connection", "te", "upgrade",
	"cache-control", "expect", "max-forwards", "pragma", "range",
	"if-match", "if-none-match", "if-modified-since", "if-unmodified-since", "if-range",
	"authorization", "proxy-authorization", "www-authenticate", "proxy-authenticate",
	"content-encoding", "content-range", "content-type",
}

// ParseTrailer returns the field names a message announces as trailers, in
// received order and lowercased, or nil when the field is absent or empty (RFC
// 9110, Section 6.6.2). Values must have surrounding SP/HTAB already removed.
// Repeated field lines form one list, and empty members are ignored (RFC 9110,
// Section 5.6.1). Errors match ErrInvalidTrailer and return nil. The input is
// not changed.
func ParseTrailer(values []string) ([]string, error) {
	var names []string
	for value := range slices.Values(values) {
		members, ok := grammar.SplitList(value)
		if !ok {
			return nil, ErrInvalidTrailer
		}
		for member := range slices.Values(members) {
			if !grammar.IsToken(member) {
				return nil, ErrInvalidTrailer
			}
			names = append(names, strings.ToLower(member))
		}
	}
	return names, nil
}

// ValidateTrailerNames rejects names from ParseTrailer that IsAllowedTrailer
// forbids. Errors match ErrForbiddenTrailerField.
func ValidateTrailerNames(names []string) error {
	for name := range slices.Values(names) {
		if !IsAllowedTrailer(name) {
			return fmt.Errorf("%w: %q", ErrForbiddenTrailerField, name)
		}
	}
	return nil
}

// IsAllowedTrailer reports whether a field may be processed as a trailer,
// comparing name without regard to case.
func IsAllowedTrailer(name string) bool {
	return !slices.Contains(forbiddenTrailerFields, strings.ToLower(name))
}
