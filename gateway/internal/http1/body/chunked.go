package body

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/lineio"
)

// maxChunkLineBytes caps a chunk-size line, including extensions and excluding
// CRLF. It is a local limit.
const maxChunkLineBytes = 4 * 1024

// trailerLimits are the local limits for a trailer section.
var trailerLimits = header.Limits{
	LineBytes:    8 * 1024,
	SectionBytes: 8 * 1024,
	Fields:       100,
}

// NewChunkedReader returns the decoded data of a chunked body read from source
// (RFC 9112, Section 7.1). It never reads past the body, so later bytes stay in
// source. Chunk extensions are checked and ignored. The trailer section is read
// within trailerLimits and passed to onTrailers before io.EOF is returned, or
// discarded when onTrailers is nil (RFC 9110, Section 6.5.1). Decoded data over
// maxBytes returns ErrBodyTooLarge; a negative maxBytes is treated as zero. Malformed framing returns ErrInvalidFraming, and an early end of source
// returns io.ErrUnexpectedEOF. Errors are sticky. Close stops further reads
// without closing or draining source.
func NewChunkedReader(source *bufio.Reader, maxBytes int64, onTrailers func(header.Headers)) io.ReadCloser {
	return &chunkedReader{source: source, allowance: max(maxBytes, 0), onTrailers: onTrailers}
}

type chunkedReader struct {
	closeGuard
	source     *bufio.Reader
	onTrailers func(header.Headers)
	allowance  int64 // decoded bytes still permitted
	remaining  int64 // data bytes left in the current chunk
	err        error // sticky; io.EOF after the last chunk and trailers
}

func (chunked *chunkedReader) Read(buffer []byte) (int, error) {
	if err := chunked.checkOpen(); err != nil {
		return 0, err
	}
	if chunked.err != nil {
		return 0, chunked.err
	}
	if chunked.remaining == 0 {
		if chunked.err = chunked.nextChunk(); chunked.err != nil {
			return 0, chunked.err
		}
	}
	if int64(len(buffer)) > chunked.remaining {
		buffer = buffer[:chunked.remaining]
	}
	count, err := chunked.source.Read(buffer)
	chunked.remaining -= int64(count)
	switch {
	case chunked.remaining == 0:
		chunked.err = readDataTerminator(chunked.source)
	case errors.Is(err, io.EOF):
		chunked.err = io.ErrUnexpectedEOF
	case err != nil:
		chunked.err = err
	}
	return count, chunked.err
}

// nextChunk reads a chunk-size line. The last chunk is followed by the trailer
// section, after which it returns io.EOF.
func (chunked *chunkedReader) nextChunk() error {
	line, err := lineio.ReadCRLFLine(chunked.source, maxChunkLineBytes)
	if err != nil {
		return chunkLineError(err)
	}
	size, ok := parseChunkLine(line)
	if !ok {
		return fmt.Errorf("%w: malformed chunk-size line", ErrInvalidFraming)
	}
	if size == 0 {
		trailers, err := header.ReadHeadersSection(chunked.source, trailerLimits)
		if err != nil {
			return fmt.Errorf("http1: reading trailers: %w", err)
		}
		if chunked.onTrailers != nil {
			chunked.onTrailers(trailers)
		}
		return io.EOF
	}
	if size > chunked.allowance {
		return ErrBodyTooLarge
	}
	chunked.allowance -= size
	chunked.remaining = size
	return nil
}

// readDataTerminator reads the CRLF that ends chunk data. It reads exactly two
// bytes, so malformed input cannot make it wait for a line feed.
func readDataTerminator(source *bufio.Reader) error {
	var terminator [2]byte
	if _, err := io.ReadFull(source, terminator[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	if terminator != [2]byte{'\r', '\n'} {
		return fmt.Errorf("%w: chunk data not followed by CRLF", ErrInvalidFraming)
	}
	return nil
}

// chunkLineError classifies a ReadCRLFLine error for a chunk-size line.
func chunkLineError(err error) error {
	switch {
	case errors.Is(err, io.EOF):
		return io.ErrUnexpectedEOF
	case errors.Is(err, lineio.ErrLineTooLong), errors.Is(err, lineio.ErrInvalidLineEnding):
		return fmt.Errorf("%w: %w", ErrInvalidFraming, err)
	}
	return err
}

// parseChunkLine parses chunk-size [ chunk-ext ], where chunk-ext is
// *( BWS ";" BWS chunk-ext-name [ BWS "=" BWS chunk-ext-val ] ) (RFC 9112,
// Section 7.1.1). Whitespace is accepted only where BWS appears, so the line
// cannot end in whitespace. The size must fit an int64.
func parseChunkLine(line string) (int64, bool) {
	parts, ok := grammar.SplitUnquoted(line, ';')
	if !ok {
		return 0, false
	}
	sizeText := parts[0]
	if len(parts) > 1 {
		sizeText = strings.TrimRight(sizeText, " \t")
	}
	if !grammar.IsHexDigits(sizeText) {
		return 0, false
	}
	size, err := strconv.ParseInt(sizeText, 16, 64)
	if err != nil {
		return 0, false
	}
	extensions := parts[1:]
	for index, extension := range extensions {
		if index == len(extensions)-1 && extension != strings.TrimRight(extension, " \t") {
			return 0, false
		}
		if !grammar.IsChunkExtension(grammar.TrimOWS(extension)) {
			return 0, false
		}
	}
	return size, true
}
