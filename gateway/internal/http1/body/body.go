// Package body handles HTTP/1.1 message bodies from plain framing inputs,
// independent of the parent http1 message types.
package body

import "errors"

var (
	// ErrBodyClosed reports a Read after Close.
	ErrBodyClosed = errors.New("http1: read on closed body")

	// ErrBodyTooLarge reports a body over its byte limit.
	ErrBodyTooLarge = errors.New("http1: body exceeds its byte limit")
)
