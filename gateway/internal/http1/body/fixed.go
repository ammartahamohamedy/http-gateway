package body

import (
	"errors"
	"io"
)

// NewFixedReader returns a body of exactly length bytes read from source. It
// never reads past the body, so later bytes stay in source. The final bytes are
// returned with io.EOF; an earlier end of source returns io.ErrUnexpectedEOF
// (RFC 9112, Section 8). A negative length is treated as zero. Close stops
// further reads without closing or draining source.
func NewFixedReader(source io.Reader, length int64) io.ReadCloser {
	return &fixedReader{limited: io.LimitedReader{R: source, N: max(length, 0)}}
}

type fixedReader struct {
	closeGuard
	limited io.LimitedReader
}

func (fixed *fixedReader) Read(buffer []byte) (int, error) {
	if err := fixed.checkOpen(); err != nil {
		return 0, err
	}

	count, err := fixed.limited.Read(buffer)
	switch {
	case fixed.limited.N == 0:
		return count, io.EOF
	case errors.Is(err, io.EOF):
		return count, io.ErrUnexpectedEOF
	}

	return count, err
}
