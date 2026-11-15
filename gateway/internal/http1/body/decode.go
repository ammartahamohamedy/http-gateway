package body

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidCoding reports encoded data that its transfer coding cannot
	// decode, including a stream cut short within a complete body and data after
	// the end of the stream.
	ErrInvalidCoding = errors.New("http1: transfer-coded data is malformed")

	// ErrUnsupportedCoding reports a transfer coding with no decoder.
	ErrUnsupportedCoding = errors.New("http1: transfer coding has no decoder")
)

// decoders maps a transfer coding to a function wrapping an encoded stream with
// its decoder. The function may read the stream's header. HTTP's deflate is the
// zlib format, not raw deflate (RFC 9110, Section 8.4.1.2).
var decoders = map[string]func(io.Reader) (io.Reader, error){
	"gzip": func(source io.Reader) (io.Reader, error) {
		reader, err := gzip.NewReader(source)
		if err != nil {
			return nil, err
		}
		return reader, nil
	},
	"deflate": func(source io.Reader) (io.Reader, error) {
		reader, err := zlib.NewReader(source)
		if err != nil {
			return nil, err
		}
		return reader, nil
	},
}

// CanDecode reports whether coding, a lowercase name from
// fields.ParseTransferEncoding, has a decoder.
func CanDecode(coding string) bool {
	_, found := decoders[coding]
	return found
}

// Decode returns framed with codings removed. Codings are listed in the order
// they were applied, as in Transfer-Encoding without its final chunked, so they
// are decoded in reverse. Each decoder is created on the first Read, so Decode
// reads nothing. Decoded data over maxBytes returns ErrBodyTooLarge; a negative
// maxBytes is treated as zero. Malformed, truncated, or trailing encoded data
// within a complete body returns ErrInvalidCoding, a coding without a decoder
// returns ErrUnsupportedCoding, and errors from framed, such as
// io.ErrUnexpectedEOF for an incomplete body, pass through. Errors are sticky. Close closes framed.
// Without codings, framed is returned unchanged.
func Decode(framed io.ReadCloser, codings []string, maxBytes int64) io.ReadCloser {
	if len(codings) == 0 {
		return framed
	}
	var reader io.Reader = framed
	for index := len(codings) - 1; index >= 0; index-- {
		input := &endTracker{source: reader}
		reader = &decodedReader{coding: codings[index], input: input, source: bufio.NewReader(input)}
	}
	limit := max(maxBytes, 0)
	return &limitedBody{limited: io.LimitedReader{R: reader, N: limit + 1}, framed: framed, limit: limit}
}

// decodedReader decodes one coding. Its source is buffered so the decoder,
// which then reads byte by byte, leaves any data after the stream in source.
type decodedReader struct {
	coding  string
	input   *endTracker
	source  *bufio.Reader
	decoder io.Reader // created on the first Read
	err     error     // sticky
}

func (decoded *decodedReader) Read(buffer []byte) (int, error) {
	if decoded.err != nil {
		return 0, decoded.err
	}
	if decoded.decoder == nil {
		create, found := decoders[decoded.coding]
		if !found {
			decoded.err = fmt.Errorf("%w: %q", ErrUnsupportedCoding, decoded.coding)
			return 0, decoded.err
		}
		decoder, err := create(decoded.source)
		if errors.Is(err, io.EOF) {
			err = fmt.Errorf("%w: %s: empty stream", ErrInvalidCoding, decoded.coding)
		}
		if err != nil {
			decoded.err = decoded.classify(err)
			return 0, decoded.err
		}
		decoded.decoder = decoder
	}
	count, err := decoded.decoder.Read(buffer)
	if errors.Is(err, io.EOF) {
		err = decoded.checkStreamEnd()
	}
	if err != nil {
		decoded.err = decoded.classify(err)
	}
	return count, decoded.err
}

// checkStreamEnd returns io.EOF when nothing follows the encoded stream.
func (decoded *decodedReader) checkStreamEnd() error {
	_, err := decoded.source.Peek(1)
	switch {
	case err == nil:
		return fmt.Errorf("%w: %s: data after the end of the stream", ErrInvalidCoding, decoded.coding)
	case errors.Is(err, io.EOF):
		return io.EOF
	}
	return err
}

// classify marks decoder format errors as ErrInvalidCoding, as well as
// io.ErrUnexpectedEOF when the input ended cleanly, since the stream was then
// cut short inside a complete body. io.EOF and other errors pass through.
func (decoded *decodedReader) classify(err error) error {
	var corrupt flate.CorruptInputError
	formatError := errors.Is(err, gzip.ErrHeader) || errors.Is(err, gzip.ErrChecksum) ||
		errors.Is(err, zlib.ErrHeader) || errors.Is(err, zlib.ErrChecksum) || errors.Is(err, zlib.ErrDictionary) ||
		errors.As(err, &corrupt)
	truncated := errors.Is(err, io.ErrUnexpectedEOF) && decoded.input.ended
	if formatError || truncated {
		// The cause is formatted, not wrapped, so the error matches only
		// ErrInvalidCoding and never reads as an incomplete body.
		return fmt.Errorf("%w: %s: %v", ErrInvalidCoding, decoded.coding, err)
	}
	return err
}

// endTracker records whether its source ended cleanly with io.EOF.
type endTracker struct {
	source io.Reader
	ended  bool
}

func (tracker *endTracker) Read(buffer []byte) (int, error) {
	count, err := tracker.source.Read(buffer)
	if errors.Is(err, io.EOF) {
		tracker.ended = true
	}
	return count, err
}

// limitedBody caps decoded data, so a small encoded body cannot expand without
// bound, and closes the framed body. Its reader allows one byte past limit:
// reading that byte proves the data is too large.
type limitedBody struct {
	closeGuard
	limited io.LimitedReader // N starts at limit + 1
	framed  io.Closer
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
		limited.err = fmt.Errorf("%w: decoded data exceeds %d bytes", ErrBodyTooLarge, limited.limit)
		return count - 1, limited.err
	}
	limited.err = err
	return count, err
}

func (limited *limitedBody) Close() error {
	_ = limited.closeGuard.Close()
	return limited.framed.Close()
}
