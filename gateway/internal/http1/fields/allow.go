package fields

import (
	"errors"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ErrInvalidAllow reports an Allow list member that is not a method token.
var ErrInvalidAllow = errors.New("http1: Allow requires a list of methods")

// ValidateAllow checks the methods Allow advertises (RFC 9110, Section 10.2.1),
// split by grammar.SplitTokenList. Methods are case-sensitive and are not
// checked against any set. An empty list, meaning no allowed methods, is valid.
// Errors match ErrInvalidAllow.
func ValidateAllow(values []string) error {
	if _, ok := grammar.SplitTokenList(values); !ok {
		return ErrInvalidAllow
	}

	return nil
}
