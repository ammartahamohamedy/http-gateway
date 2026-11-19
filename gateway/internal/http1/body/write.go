package body

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// ErrBodyLength reports a body source that yields fewer or more bytes than its
// declared Content-Length.
var ErrBodyLength = errors.New("http1: body length does not match its Content-Length")

// chunkBufferBytes caps the data in one written chunk.
const chunkBufferBytes = 32 * 1024

// Write writes source to writer as a body framed by mode. None writes nothing
// and does not read source. ContentLength writes exactly length bytes; a source
// that ends early or holds more returns ErrBodyLength, so the declared length
// never disagrees with the bytes on the wire (RFC 9112, Section 11.2), and a
// negative length is treated as zero. Chunked writes each read from source as
// one chunk (RFC 9112, Section 7.1) and, once source ends, the last chunk and
// the trailer section that trailers returns, or an empty one when trailers is
// nil. The trailer section is validated by header.Write before the last chunk
// is written, so a failed chunked body never looks complete. Other modes return
// ErrUnsupportedFraming without writing.
//
// Errors from source and writer are returned. Write neither closes nor drains
// source.
func Write(writer io.Writer, source io.Reader, mode Mode, length int64, trailers func() header.Section) error {
	switch mode {
	case None:
		return nil
	case ContentLength:
		return writeFixed(writer, source, max(length, 0))
	case Chunked:
		return writeChunked(writer, source, trailers)
	default:
		return fmt.Errorf("%w: cannot write mode %d", ErrUnsupportedFraming, mode)
	}
}

// writeFixed copies exactly length bytes, then checks that source has ended.
func writeFixed(writer io.Writer, source io.Reader, length int64) error {
	written, err := io.CopyN(writer, source, length)
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: source ended after %d of %d bytes", ErrBodyLength, written, length)
	}
	if err != nil {
		return err
	}

	count, err := source.Read(make([]byte, 1))
	if count > 0 {
		return fmt.Errorf("%w: source holds more than %d bytes", ErrBodyLength, length)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}

// writeChunked writes one chunk per read from source, its size in hexadecimal
// before the data, then the last chunk and the trailer section.
func writeChunked(writer io.Writer, source io.Reader, trailers func() header.Section) error {
	buffer := make([]byte, chunkBufferBytes)

	for {
		count, err := source.Read(buffer)
		if count > 0 {
			size := strconv.AppendInt(nil, int64(count), 16)
			for part := range slices.Values([][]byte{append(size, "\r\n"...), buffer[:count], []byte("\r\n")}) {
				if _, err := writer.Write(part); err != nil {
					return err
				}
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}

	var section header.Section
	if trailers != nil {
		section = trailers()
	}

	var trailer bytes.Buffer
	if err := header.Write(&trailer, section); err != nil {
		return err
	}

	_, err := writer.Write(append([]byte("0\r\n"), trailer.Bytes()...))
	return err
}
