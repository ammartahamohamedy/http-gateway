package fields

import (
	"errors"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ErrInvalidRetryAfter reports a repeated or malformed Retry-After value.
var ErrInvalidRetryAfter = errors.New("http1: Retry-After requires one HTTP-date or delay in seconds")

// ValidateRetryAfter checks that a supplied Retry-After field holds exactly one
// HTTP-date or delay-seconds value, 1*DIGIT (RFC 9110, Section 10.2.3). Values
// must have surrounding SP/HTAB already removed; absence is valid. Errors match
// ErrInvalidRetryAfter.
func ValidateRetryAfter(values []string) error {
	if len(values) == 0 {
		return nil
	}

	if len(values) != 1 || (!grammar.IsDigits(values[0]) && !isHTTPDate(values[0])) {
		return ErrInvalidRetryAfter
	}

	return nil
}
