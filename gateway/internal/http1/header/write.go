package header

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ErrInvalidField reports a field that Write refuses to send: a name that is
// not a token, or a value with bytes outside field-value or with surrounding
// SP or HTAB.
var ErrInvalidField = errors.New("http1: field cannot be written")

// Write writes section as field lines followed by the empty line that ends a
// header or trailer section (RFC 9112, Section 2.1). Each value becomes its own
// field line, so repeated fields such as Set-Cookie are never combined (RFC
// 9110, Section 5.3). Names are written as stored and sorted, so output is
// deterministic; each name's values keep their order. An empty or nil section
// writes only the empty line.
//
// The whole section is validated before anything is written, so an invalid
// field, which could split the message (RFC 9112, Section 11.1), returns
// ErrInvalidField and writes nothing. Valid output is passed to writer in one
// Write call, and its error is returned. The input is not changed.
func Write(writer io.Writer, section Section) error {
	var output []byte

	for name := range slices.Values(slices.Sorted(maps.Keys(section))) {
		if !grammar.IsToken(name) {
			return fmt.Errorf("%w: invalid name %q", ErrInvalidField, name)
		}

		for value := range slices.Values(section[name]) {
			if !grammar.IsFieldValue(value) || grammar.TrimOWS(value) != value {
				return fmt.Errorf("%w: invalid %s value %q", ErrInvalidField, name, value)
			}

			output = append(output, name...)
			output = append(output, ':')
			if value != "" {
				output = append(output, ' ')
				output = append(output, value...)
			}
			output = append(output, "\r\n"...)
		}
	}

	output = append(output, "\r\n"...)

	_, err := writer.Write(output)
	return err
}
