package body

import "io"

// newUntilCloseReader returns a body that ends when source reports io.EOF, as
// for a response delimited by the server closing the connection (RFC 9112,
// Section 6.3). newLimitedBody enforces maxBytes. An early end of source cannot
// be told apart from the end of the body (RFC 9112, Section 8). Close stops
// further reads without closing or draining source.
func newUntilCloseReader(source io.Reader, maxBytes int64) io.ReadCloser {
	return newLimitedBody(source, io.NopCloser(nil), maxBytes)
}
