package body

import (
	"errors"
	"io"
)

// ErrBodyClosed reports a Read after Close.
var ErrBodyClosed = errors.New("http1: read on closed body")

// NewFixedReader returns a body of exactly length bytes read from source. It
// never reads past the body, so later bytes stay in source. The final bytes are
// returned with io.EOF; an earlier end of source returns io.ErrUnexpectedEOF
// (RFC 9112, Section 8). A negative length is treated as zero. Close stops
// further reads without closing or draining source.
func NewFixedReader(source io.Reader, length int64) io.ReadCloser {
	return &fixedReader{source: source, remaining: max(length, 0)}
}

type fixedReader struct {
	source    io.Reader
	remaining int64
	closed    bool
}

func (fixed *fixedReader) Read(buffer []byte) (int, error) {
	if fixed.closed {
		return 0, ErrBodyClosed
	}
	if fixed.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(buffer)) > fixed.remaining {
		buffer = buffer[:fixed.remaining]
	}
	count, err := fixed.source.Read(buffer)
	fixed.remaining -= int64(count)
	switch {
	case fixed.remaining == 0:
		return count, io.EOF
	case errors.Is(err, io.EOF):
		return count, io.ErrUnexpectedEOF
	}
	return count, err
}

func (fixed *fixedReader) Close() error {
	fixed.closed = true
	return nil
}
