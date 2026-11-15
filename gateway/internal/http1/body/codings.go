package body

import (
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"slices"
)

// codingDecoder describes how to decode one transfer coding.
type codingDecoder struct {
	// open wraps an encoded stream with its decoder; it may read the header.
	open func(io.Reader) (io.Reader, error)

	// formatErrors are the decoder's errors for malformed data.
	formatErrors []error
}

// decoders maps each decodable transfer coding to its decoder. HTTP's deflate
// is the zlib format, not raw deflate (RFC 9110, Section 8.4.1.2).
var decoders = map[string]codingDecoder{
	"gzip": {
		open: func(source io.Reader) (io.Reader, error) {
			reader, err := gzip.NewReader(source)
			if err != nil {
				return nil, err
			}
			return reader, nil
		},
		formatErrors: []error{gzip.ErrHeader, gzip.ErrChecksum},
	},
	"deflate": {
		open: func(source io.Reader) (io.Reader, error) {
			reader, err := zlib.NewReader(source)
			if err != nil {
				return nil, err
			}
			return reader, nil
		},
		formatErrors: []error{zlib.ErrHeader, zlib.ErrChecksum, zlib.ErrDictionary},
	},
}

// CanDecode reports whether coding, a lowercase name from
// fields.ParseTransferEncoding, has a decoder.
func CanDecode(coding string) bool {
	_, found := decoders[coding]
	return found
}

// isFormatError reports whether err reports malformed data: one of the
// decoder's formatErrors, or a flate.CorruptInputError, which gzip and zlib
// both return.
func (decoder codingDecoder) isFormatError(err error) bool {
	var corrupt flate.CorruptInputError
	if errors.As(err, &corrupt) {
		return true
	}

	return slices.ContainsFunc(decoder.formatErrors, func(formatErr error) bool {
		return errors.Is(err, formatErr)
	})
}
