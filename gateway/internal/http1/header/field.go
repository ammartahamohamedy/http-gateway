package header

import (
	"errors"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// parseField parses one field line without CRLF. It preserves name casing and
// value bytes, removing only surrounding SP/HTAB from the value. It checks
// generic syntax, not field-specific semantics, and does not enforce size limits.
// On error, both returned strings are empty.
func parseField(rawLine string) (name, value string, err error) {
	name, value, found := strings.Cut(rawLine, ":")
	if !found {
		return "", "", errors.New("http1: header field is missing a colon")
	}

	if !grammar.IsToken(name) {
		return "", "", errors.New("http1: invalid header field name")
	}

	if !grammar.IsFieldValue(value) {
		return "", "", errors.New("http1: invalid header field value")
	}

	return name, strings.Trim(value, " \t"), nil
}
