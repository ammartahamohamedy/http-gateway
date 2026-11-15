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

// closeGuard holds a body's closed state. Readers embed it, check it first in
// Read, and gain its Close, which stops further reads without closing or
// draining the source.
type closeGuard struct {
	closed bool
}

// checkOpen returns ErrBodyClosed after Close.
func (guard *closeGuard) checkOpen() error {
	if guard.closed {
		return ErrBodyClosed
	}
	return nil
}

func (guard *closeGuard) Close() error {
	guard.closed = true
	return nil
}
