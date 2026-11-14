package http1

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/lineio"
)

// Local request limits. They bound buffering and connection time; RFC 9112
// sets no universal maximum for any of them.
const (
	// maxRequestLineBytes caps request-line content, excluding CRLF, at 8 KiB.
	// RFC 9112, Section 3 recommends supporting at least 8000 octets.
	maxRequestLineBytes = 8 * 1024

	// maxLeadingEmptyLines caps the empty lines skipped before a request-line,
	// so a client cannot hold a connection by sending CRLFs forever. RFC 9112,
	// Section 2.2 asks servers to ignore at least one.
	maxLeadingEmptyLines = 4

	// maxRequestBodyBytes caps a Content-Length request body at 1 MiB.
	maxRequestBodyBytes = 1 << 20
)

// requestHeaderLimits are the local limits for a request header section.
var requestHeaderLimits = header.Limits{
	LineBytes:    8 * 1024,
	SectionBytes: 32 * 1024,
	Fields:       100,
}

var (
	// ErrRequestLineTooLong is returned by ReadRequest when request-line
	// content, excluding CRLF, exceeds maxRequestLineBytes.
	ErrRequestLineTooLong = errors.New("http1: request line exceeds 8192 bytes")

	// ErrTooManyLeadingEmptyLines is returned by ReadRequest when more than
	// maxLeadingEmptyLines empty lines precede the request-line.
	ErrTooManyLeadingEmptyLines = errors.New("http1: too many empty lines before request line")

	// ErrRequestBodyTooLarge is returned by ReadRequest when Content-Length
	// exceeds maxRequestBodyBytes.
	ErrRequestBodyTooLarge = errors.New("http1: request body exceeds 1048576 bytes")
)

// RequestLine contains the method, request-target, and HTTP version number
// from a request's start-line, excluding its CRLF terminator.
type RequestLine struct {
	Method        string
	RequestTarget string
	// Version is always "1.1".
	Version string
}

// Request holds an HTTP/1.x request-line, header fields, parsed metadata from
// the supported fields, and the body. Body is empty without framing fields,
// exactly Content-Length bytes with one, and nil for chunked bodies until their
// decoder exists.
type Request struct {
	RequestLine RequestLine
	Headers     header.Headers
	Fields      RequestFields
	Body        io.ReadCloser
}

// processRequestLine skips up to maxLeadingEmptyLines empty CRLF lines, reads a
// bounded request-line, and parses it with parseRequestLine. Read errors are
// wrapped; parsing errors are returned unchanged.
func processRequestLine(reader *bufio.Reader) (RequestLine, error) {
	for range maxLeadingEmptyLines + 1 {
		rawLine, err := lineio.ReadCRLFLine(reader, maxRequestLineBytes)
		if errors.Is(err, lineio.ErrLineTooLong) {
			return RequestLine{}, ErrRequestLineTooLong
		}
		if err != nil {
			return RequestLine{}, fmt.Errorf("http1: reading request line: %w", err)
		}
		if rawLine == "" {
			continue
		}
		return parseRequestLine(rawLine)
	}
	return RequestLine{}, ErrTooManyLeadingEmptyLines
}

// parseRequestLine parses line contents without a CRLF terminator. It requires
// exactly three nonempty components separated by single spaces and a version
// that parseHTTPVersion processes as 1.1; HTTP/1.0 is rejected as local policy.
// Method case and request-target encoding are preserved. It does not check
// whether a method is implemented by the server. This function does not enforce
// a size limit; ReadRequest applies the input limit before calling it. Direct
// callers are responsible for bounding input when a size limit is required.
func parseRequestLine(rawLine string) (RequestLine, error) {
	parts := strings.Split(rawLine, " ")
	if len(parts) != 3 {
		return RequestLine{}, errors.New(
			"http1: expected exactly three space-separated components",
		)
	}

	method, requestTarget, version := parts[0], parts[1], parts[2]

	if method == "" || requestTarget == "" || version == "" {
		return RequestLine{}, errors.New(
			"http1: request-line components must not be empty",
		)
	}

	if !grammar.IsToken(method) {
		return RequestLine{}, fmt.Errorf("http1: invalid method %q", method)
	}
	if err := validateRequestTarget(method, requestTarget); err != nil {
		return RequestLine{}, err
	}
	versionNumber, err := parseHTTPVersion(version)
	if err != nil {
		return RequestLine{}, err
	}
	if versionNumber != "1.1" {
		return RequestLine{}, fmt.Errorf("http1: unsupported HTTP version %q", version)
	}

	return RequestLine{
		Method:        method,
		RequestTarget: requestTarget,
		Version:       versionNumber,
	}, nil
}
