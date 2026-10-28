package http1

import (
	"bufio"
	"errors"
	"fmt"
)

// maxRequestLineBytes caps content read by ReadRequest at 8 KiB, excluding
// CRLF, to bound buffering before parsing. This is a local
// implementation limit: RFC 9112, Section 3 recommends supporting at least
// 8000 octets but defines no universal maximum.
// https://datatracker.ietf.org/doc/html/rfc9112#section-3
const maxRequestLineBytes = 8 * 1024

// ErrRequestLineTooLong is returned by ReadRequest when request-line content
// exceeds 8192 bytes, excluding its terminating CRLF.
var ErrRequestLineTooLong = errors.New("http1: request line exceeds 8192 bytes")

// ReadRequest currently reads and parses only a request's request-line. It skips
// leading empty CRLF lines and returns a Request with RequestLine populated;
// Headers and Body remain nil. Header and body bytes remain unread in reader.
// A successful return does not mean that a complete HTTP request was received.
//
// Each line is limited to 8192 content bytes, excluding CRLF. Bare LF endings
// and CR or LF within line contents are rejected. Oversized lines return
// ErrRequestLineTooLong before EOF or framing checks. Otherwise, EOF before a
// nonempty line matches io.EOF, and EOF within a line matches io.ErrUnexpectedEOF.
// Read and framing errors are wrapped; ParseRequestLine errors are returned
// unchanged. All errors return a nil Request.
//
// The caller must reuse the same buffered reader for subsequent bytes. On error,
// the reader may be positioned partway through a line. The input is never closed.
func ReadRequest(reader *bufio.Reader) (*Request, error) {
	for {
		rawLine, err := readCRLFLine(reader, maxRequestLineBytes)
		if errors.Is(err, errLineTooLong) {
			return nil, ErrRequestLineTooLong
		}
		if err != nil {
			return nil, fmt.Errorf("http1: reading request line: %w", err)
		}
		if rawLine == "" {
			continue
		}
		requestLine, err := ParseRequestLine(rawLine)
		if err != nil {
			return nil, err
		}
		return &Request{RequestLine: requestLine}, nil
	}
}
