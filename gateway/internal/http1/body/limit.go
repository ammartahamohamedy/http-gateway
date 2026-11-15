package body

import (
	"fmt"
	"io"
)

// newLimitedBody returns source capped at limit bytes; a negative limit is
// treated as zero. More data returns ErrBodyTooLarge, withholding the excess.
// Errors are sticky. Close closes closer.
func newLimitedBody(source io.Reader, closer io.Closer, limit int64) io.ReadCloser {
	limit = max(limit, 0)
	return &limitedBody{limited: io.LimitedReader{R: source, N: limit + 1}, closer: closer, limit: limit}
}

// limitedBody reads one byte past its limit: reading that byte proves the data
// is too large.
type limitedBody struct {
	closeGuard
	limited io.LimitedReader // N starts at limit + 1
	closer  io.Closer
	limit   int64
	err     error // sticky
}

func (limited *limitedBody) Read(buffer []byte) (int, error) {
	if err := limited.checkOpen(); err != nil {
		return 0, err
	}

	if limited.err != nil {
		return 0, limited.err
	}

	count, err := limited.limited.Read(buffer)
	if limited.limited.N == 0 {
		// The last byte read is the one past the limit, so it is withheld.
		limited.err = fmt.Errorf("%w: data exceeds %d bytes", ErrBodyTooLarge, limited.limit)
		return count - 1, limited.err
	}

	limited.err = err
	return count, err
}

func (limited *limitedBody) Close() error {
	_ = limited.closeGuard.Close()
	return limited.closer.Close()
}
