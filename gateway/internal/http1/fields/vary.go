package fields

import (
	"errors"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ErrInvalidVary reports a Vary list member that is neither "*" nor a field
// name.
var ErrInvalidVary = errors.New(`http1: Vary requires "*" or a list of field names`)

// ParseVary returns the members of Vary in received order, lowercased because
// field names are case-insensitive, or nil when the field is absent or empty
// (RFC 9110, Section 12.5.5). Members are split by grammar.SplitTokenList; "*"
// is a token, so it needs no special case. Errors match ErrInvalidVary and
// return nil. The input is not changed.
func ParseVary(values []string) ([]string, error) {
	members, ok := grammar.SplitTokenList(values)
	if !ok {
		return nil, ErrInvalidVary
	}

	for index := range members {
		members[index] = strings.ToLower(members[index])
	}

	return members, nil
}
