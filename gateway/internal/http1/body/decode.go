package body

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
)

var (
	// errInvalidCoding reports encoded data that its transfer coding cannot
	// decode, including a stream cut short within a complete body and data after
	// the end of the stream.
	errInvalidCoding = errors.New("http1: transfer-coded data is malformed")

	// errUnsupportedCoding reports a transfer coding with no decoder.
	errUnsupportedCoding = errors.New("http1: transfer coding has no decoder")
)

// decode returns framed with codings removed. Codings are listed in the order
// they were applied, as in Transfer-Encoding without its final chunked, so they
// are decoded in reverse. Each decoder is created on the first Read, so decode
// reads nothing. Decoded data over maxBytes returns ErrBodyTooLarge; a negative
// maxBytes is treated as zero. Malformed, truncated, or trailing encoded data
// within a complete body returns errInvalidCoding, a coding without a decoder
// returns errUnsupportedCoding, and errors from framed, such as
// io.ErrUnexpectedEOF for an incomplete body, pass through. Errors are sticky.
// Close closes framed. Without codings, framed is returned unchanged.
func decode(framed io.ReadCloser, codings []string, maxBytes int64) io.ReadCloser {
	if len(codings) == 0 {
		return framed
	}

	var reader io.Reader = framed
	for _, coding := range slices.Backward(codings) {
		input := &endTracker{source: reader}
		reader = &decodedReader{coding: coding, input: input, source: bufio.NewReader(input)}
	}

	return newLimitedBody(reader, framed, maxBytes)
}

// decodedReader decodes one coding. Its source is buffered so the decoder,
// which then reads byte by byte, leaves any data after the stream in source.
type decodedReader struct {
	coding  string
	input   *endTracker
	source  *bufio.Reader
	entry   codingDecoder // looked up on the first Read
	decoder io.Reader     // created on the first Read
	err     error         // sticky
}

func (decoded *decodedReader) Read(buffer []byte) (int, error) {
	if decoded.err != nil {
		return 0, decoded.err
	}

	if decoded.decoder == nil {
		entry, found := decoders[decoded.coding]
		if !found {
			decoded.err = fmt.Errorf("%w: %q", errUnsupportedCoding, decoded.coding)
			return 0, decoded.err
		}
		decoded.entry = entry
		decoder, err := entry.open(decoded.source)
		if errors.Is(err, io.EOF) {
			err = fmt.Errorf("%w: %s: empty stream", errInvalidCoding, decoded.coding)
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
		return fmt.Errorf("%w: %s: data after the end of the stream", errInvalidCoding, decoded.coding)
	case errors.Is(err, io.EOF):
		return io.EOF
	}

	return err
}

// classify marks the decoder's format errors as errInvalidCoding, as well as
// io.ErrUnexpectedEOF when the input ended cleanly, since the stream was then
// cut short inside a complete body. io.EOF and other errors pass through.
func (decoded *decodedReader) classify(err error) error {
	truncated := errors.Is(err, io.ErrUnexpectedEOF) && decoded.input.ended
	if decoded.entry.isFormatError(err) || truncated {
		// The cause is formatted, not wrapped, so the error matches only
		// errInvalidCoding and never reads as an incomplete body.
		return fmt.Errorf("%w: %s: %v", errInvalidCoding, decoded.coding, err)
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
