// Package body handles HTTP/1.1 message bodies from plain framing inputs,
// independent of the parent http1 message types.
package body

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

var (
	// errBodyClosed reports a Read after Close.
	errBodyClosed = errors.New("http1: read on closed body")

	// ErrBodyTooLarge reports a body over its byte limit.
	ErrBodyTooLarge = errors.New("http1: body exceeds its byte limit")
)

// NewReader returns the body of msg read from source, delimited as framing
// decides: empty for None, the declared length for ContentLength, the chunked
// data for Chunked, with its trailer section passed to onTrailers, and all data
// until source ends for UntilClose. The transfer codings other than chunked are
// then removed. maxBytes caps the body: a declared Content-Length over it
// returns ErrBodyTooLarge at once, and more data read or decoded returns
// ErrBodyTooLarge from Read. The body never reads past its end, so following
// bytes stay in source. Errors from framing are returned with a nil body.
func NewReader(source *bufio.Reader, msg Message, maxBytes int64, onTrailers func(header.Section)) (io.ReadCloser, error) {
	mode, err := framing(msg)
	if err != nil {
		return nil, err
	}

	switch mode {
	case None:
		return newFixedReader(source, 0), nil
	case ContentLength:
		if msg.ContentLength > maxBytes {
			return nil, fmt.Errorf("%w: Content-Length %d exceeds %d bytes",
				ErrBodyTooLarge, msg.ContentLength, maxBytes)
		}
		return newFixedReader(source, msg.ContentLength), nil
	case Chunked:
		codings := msg.TransferCodings[:len(msg.TransferCodings)-1]
		return decode(newChunkedReader(source, maxBytes, onTrailers), codings, maxBytes), nil
	default:
		return decode(newUntilCloseReader(source, maxBytes), msg.TransferCodings, maxBytes), nil
	}
}

// closeGuard holds a body's closed state. Readers embed it, check it first in
// Read, and gain its Close, which stops further reads without closing or
// draining the source.
type closeGuard struct {
	closed bool
}

// checkOpen returns errBodyClosed after Close.
func (guard *closeGuard) checkOpen() error {
	if guard.closed {
		return errBodyClosed
	}

	return nil
}

func (guard *closeGuard) Close() error {
	guard.closed = true
	return nil
}
