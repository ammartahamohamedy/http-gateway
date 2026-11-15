package request

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

var (
	// ErrLineTooLong is returned by Read when request-line
	// content, excluding CRLF, exceeds maxLineBytes.
	ErrLineTooLong = errors.New("http1: request line exceeds 8192 bytes")

	// ErrTooManyLeadingEmptyLines is returned by Read when more than
	// maxLeadingEmptyLines empty lines precede the request-line.
	ErrTooManyLeadingEmptyLines = errors.New("http1: too many empty lines before request line")
)

// Line contains the method, request-target, and HTTP version number
// from a request's start-line, excluding its CRLF terminator.
type Line struct {
	Method        string
	RequestTarget string
	// Version is always "1.1".
	Version string
}

// readLine skips up to maxLeadingEmptyLines empty CRLF lines, reads a
// bounded request-line, and parses it with parseLine. Read errors are
// wrapped; parsing errors are returned unchanged.
func readLine(reader *bufio.Reader) (Line, error) {
	for range maxLeadingEmptyLines + 1 {
		rawLine, err := http1.ReadCRLFLine(reader, maxLineBytes)
		if errors.Is(err, http1.ErrLineTooLong) {
			return Line{}, ErrLineTooLong
		}
		if err != nil {
			return Line{}, fmt.Errorf("http1: reading request line: %w", err)
		}
		if rawLine == "" {
			continue
		}
		return parseLine(rawLine)
	}

	return Line{}, ErrTooManyLeadingEmptyLines
}

// parseLine parses line contents without a CRLF terminator. It requires
// exactly three nonempty components separated by single spaces and a version
// that http1.ParseVersion processes as 1.1; HTTP/1.0 is rejected as local policy.
// Method case and request-target encoding are preserved. It does not check
// whether a method is implemented by the server. This function does not enforce
// a size limit; Read applies the input limit before calling it. Direct
// callers are responsible for bounding input when a size limit is required.
func parseLine(rawLine string) (Line, error) {
	parts := strings.Split(rawLine, " ")
	if len(parts) != 3 {
		return Line{}, errors.New(
			"http1: expected exactly three space-separated components",
		)
	}

	method, requestTarget, version := parts[0], parts[1], parts[2]

	if method == "" || requestTarget == "" || version == "" {
		return Line{}, errors.New(
			"http1: request-line components must not be empty",
		)
	}

	if !grammar.IsToken(method) {
		return Line{}, fmt.Errorf("http1: invalid method %q", method)
	}

	if err := validateTarget(method, requestTarget); err != nil {
		return Line{}, err
	}

	versionNumber, err := http1.ParseVersion(version)
	if err != nil {
		return Line{}, err
	}

	if versionNumber != "1.1" {
		return Line{}, fmt.Errorf("http1: unsupported HTTP version %q", version)
	}

	return Line{
		Method:        method,
		RequestTarget: requestTarget,
		Version:       versionNumber,
	}, nil
}
