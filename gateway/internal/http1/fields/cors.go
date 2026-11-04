package fields

import (
	"errors"
	"regexp"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

var (
	// ErrInvalidOrigin reports a repeated or malformed Origin field.
	ErrInvalidOrigin = errors.New("http1: Origin requires one origin or null")

	// ErrInvalidAccessControlRequestMethod reports a repeated or malformed
	// Access-Control-Request-Method field.
	ErrInvalidAccessControlRequestMethod = errors.New("http1: Access-Control-Request-Method requires one method")

	// ErrInvalidAccessControlRequestHeaders reports an empty or malformed
	// Access-Control-Request-Headers list.
	ErrInvalidAccessControlRequestHeaders = errors.New("http1: Access-Control-Request-Headers requires a list of field names")
)

// originSchemePattern matches a URI scheme (RFC 3986, Section 3.1).
var originSchemePattern = regexp.MustCompile(`\A[A-Za-z][A-Za-z0-9+.-]*\z`)

// ParseOrigin returns the Origin value as received, or "" when the field is
// absent. Values must have surrounding SP/HTAB already removed. The field holds
// exactly one scheme "://" host [ ":" port ] or the case-sensitive word null
// (WHATWG Fetch), so paths, queries, user information, and lists are rejected. A
// host is required, which is stricter than URI syntax because browsers send null
// for hostless origins. Port ranges are not checked. Whether an origin is allowed
// is policy for the caller. Errors match ErrInvalidOrigin and return "".
func ParseOrigin(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if len(values) != 1 {
		return "", ErrInvalidOrigin
	}
	origin := values[0]
	if origin == "null" {
		return origin, nil
	}
	scheme, authority, found := strings.Cut(origin, "://")
	if !found || !originSchemePattern.MatchString(scheme) {
		return "", ErrInvalidOrigin
	}
	if authority == "" || authority[0] == ':' || validateHostValue(authority) != nil {
		return "", ErrInvalidOrigin
	}
	return origin, nil
}

// ParseAccessControlRequestMethod returns the one requested method as received,
// or "" when the field is absent. Values must have surrounding SP/HTAB already
// removed. Methods are case-sensitive tokens. Whether a method is allowed is
// policy for the caller. Errors match ErrInvalidAccessControlRequestMethod and
// return "".
func ParseAccessControlRequestMethod(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if len(values) != 1 || !grammar.IsToken(values[0]) {
		return "", ErrInvalidAccessControlRequestMethod
	}
	return values[0], nil
}

// ParseAccessControlRequestHeaders returns the requested field names in received
// order, lowercased because field names are case-insensitive, or nil when the
// field is absent. Values must have surrounding SP/HTAB already removed. Repeated
// field lines form one list and empty members are ignored, but a supplied field
// must name at least one field (1#field-name in WHATWG Fetch). Whether a field is
// allowed is policy for the caller. Errors match
// ErrInvalidAccessControlRequestHeaders and return nil.
func ParseAccessControlRequestHeaders(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var names []string
	for _, value := range values {
		members, ok := grammar.SplitList(value)
		if !ok {
			return nil, ErrInvalidAccessControlRequestHeaders
		}
		for _, member := range members {
			if !grammar.IsToken(member) {
				return nil, ErrInvalidAccessControlRequestHeaders
			}
			names = append(names, strings.ToLower(member))
		}
	}
	if len(names) == 0 {
		return nil, ErrInvalidAccessControlRequestHeaders
	}
	return names, nil
}
