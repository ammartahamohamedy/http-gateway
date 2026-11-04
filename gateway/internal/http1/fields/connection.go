package fields

import (
	"errors"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

var (
	// ErrInvalidConnection reports a Connection list member that is not a token.
	ErrInvalidConnection = errors.New("http1: Connection requires a list of option tokens")

	// ErrProtectedConnectionOption reports a Connection option naming a routing
	// or framing field. Proxies remove every field Connection nominates, so
	// honoring one would strip a field that the next hop depends on.
	ErrProtectedConnectionOption = errors.New("http1: Connection must not nominate routing or framing fields")
)

// protectedConnectionOptions lists the lowercase names of the routing and
// framing fields that a request must not nominate.
var protectedConnectionOptions = []string{"host", "content-length", "transfer-encoding"}

// ParseConnection returns the request's connection options in received order,
// lowercased because they are case-insensitive (RFC 9110, Section 7.6.1). Values
// must have surrounding SP/HTAB already removed. Repeated field lines form one
// list, and empty members are ignored (RFC 9110, Section 5.6.1), so an absent or
// empty field returns nil options. Every member must be a token; unfamiliar
// options are accepted. Errors match ErrInvalidConnection and return nil options.
// This function does not apply options, remove nominated fields, or decide
// connection persistence. The input is not changed.
func ParseConnection(values []string) ([]string, error) {
	var options []string
	for _, value := range values {
		members, ok := grammar.SplitList(value)
		if !ok {
			return nil, ErrInvalidConnection
		}
		for _, member := range members {
			if !grammar.IsToken(member) {
				return nil, ErrInvalidConnection
			}
			options = append(options, strings.ToLower(member))
		}
	}
	return options, nil
}

// ValidateConnectionOptions rejects options from ParseConnection that nominate a
// routing or framing field, whether or not the request carries that field. It
// expects lowercase options. Errors match ErrProtectedConnectionOption.
func ValidateConnectionOptions(options []string) error {
	for _, option := range options {
		if slices.Contains(protectedConnectionOptions, option) {
			return ErrProtectedConnectionOption
		}
	}
	return nil
}
