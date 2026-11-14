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

// maxRequestLineBytes caps content read by ReadRequest at 8 KiB, excluding
// CRLF, to bound buffering before parsing. This is a local
// implementation limit: RFC 9112, Section 3 recommends supporting at least
// 8000 octets but defines no universal maximum.
// https://datatracker.ietf.org/doc/html/rfc9112#section-3
const maxRequestLineBytes = 8 * 1024

// maxLeadingEmptyLines caps the empty lines skipped before a request-line, so a
// client cannot hold a connection by sending CRLFs forever. This is a local
// limit: RFC 9112, Section 2.2 asks servers to ignore at least one.
const maxLeadingEmptyLines = 4

// requestHeaderLimits are local limits for a request header section: 8 KiB per
// physical line, 32 KiB in total including CRLFs, and at most 100 fields.
var requestHeaderLimits = header.Limits{
	LineBytes:    8 * 1024,
	SectionBytes: 32 * 1024,
	Fields:       100,
}

// ErrRequestLineTooLong is returned by ReadRequest when request-line content
// exceeds 8192 bytes, excluding its terminating CRLF.
var ErrRequestLineTooLong = errors.New("http1: request line exceeds 8192 bytes")

// ErrTooManyLeadingEmptyLines is returned by ReadRequest when more than 4 empty
// lines precede the request-line.
var ErrTooManyLeadingEmptyLines = errors.New("http1: too many empty lines before request line")

// RequestLine contains the method, request-target, and HTTP version number
// from a request's start-line, excluding its CRLF terminator.
type RequestLine struct {
	Method        string
	RequestTarget string
	// Version is the protocol version the request is processed as, always
	// "1.1": higher HTTP/1 minor versions are processed as 1.1.
	Version string
}

// Request holds an HTTP/1.x request-line, header fields, parsed metadata from
// the supported fields, and an optional body stream. ReadRequest currently
// populates RequestLine, Headers, and Fields, leaving Body nil and body bytes
// available through the same buffered reader.
type Request struct {
	RequestLine RequestLine
	Headers     header.Headers
	Fields      RequestFields
	Body        io.ReadCloser
}

// processRequestLine skips up to maxLeadingEmptyLines empty CRLF lines, reads a
// bounded request-line, and calls parseRequestLine to validate and parse its
// method, target, and version. Oversized lines return ErrRequestLineTooLong, and
// further empty lines return ErrTooManyLeadingEmptyLines. Read errors are wrapped;
// parsing errors are returned unchanged.
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
// exactly three nonempty components separated by single spaces. It supports
// HTTP/1.1 and processes higher HTTP/1 minor versions as 1.1; HTTP/1.0 is
// rejected as local policy. Method case and request-target encoding are
// preserved, and Version contains "1.1". It does not check whether a method is
// implemented by the server. This function does not enforce a size limit;
// ReadRequest applies the input limit before calling it. Direct callers
// are responsible for bounding input when a size limit is required.
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
