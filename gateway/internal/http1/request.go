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

// RequestLine contains the method, request-target, and HTTP version number
// from a request's start-line, excluding its CRLF terminator.
type RequestLine struct {
	Method        string
	RequestTarget string
	// Version is the numeric protocol version, such as "1.1".
	Version string
}

// Request holds an HTTP/1.x request-line, header fields, parsed metadata from
// the supported fields, and an optional body stream. ReadRequest currently
// populates RequestLine, Headers, and Fields, leaving Body nil and body bytes
// available through the same buffered reader.
type Request struct {
	RequestLine RequestLine
	Headers     header.Section
	Fields      RequestFields
	Body        io.ReadCloser
}

// processRequestLine skips leading empty CRLF lines, reads a bounded request-line,
// and calls ParseRequestLine to validate and parse its method, target, and version.
// Oversized lines return ErrRequestLineTooLong. Read errors are wrapped;
// parsing errors are returned unchanged.
func processRequestLine(reader *bufio.Reader) (RequestLine, error) {
	for {
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
		return ParseRequestLine(rawLine)
	}
}

// ParseRequestLine parses line contents without a CRLF terminator. It requires
// exactly three nonempty components separated by single spaces and supports
// HTTP/1.1. Method case and request-target encoding are preserved, and Version
// contains the version number "1.1". It does not check whether a method is
// implemented by the server. This function does not enforce a size limit;
// ReadRequest applies the input limit before calling it. Direct callers
// are responsible for bounding input when a size limit is required.
func ParseRequestLine(rawLine string) (RequestLine, error) {
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

	return RequestLine{
		Method:        method,
		RequestTarget: requestTarget,
		Version:       versionNumber,
	}, nil
}
