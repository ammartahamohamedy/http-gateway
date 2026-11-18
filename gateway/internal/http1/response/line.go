package response

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// errLineTooLong is returned by readLine when status-line content, excluding
// CRLF, exceeds maxLineBytes.
var errLineTooLong = errors.New("http1: status line exceeds 8192 bytes")

// Line contains the HTTP version number, status code, and reason phrase from a
// response's status-line, excluding its CRLF terminator.
type Line struct {
	Version    string
	StatusCode int

	// ReasonPhrase is kept as received and may be empty. Recipients should
	// ignore it (RFC 9112, Section 4).
	ReasonPhrase string
}

// readLine reads a bounded status-line and parses it with parseLine. Empty
// lines before it are not skipped, since RFC 9112, Section 2.2 permits that
// only to servers reading a request-line. Read errors are wrapped; parsing
// errors are returned unchanged.
func readLine(reader *bufio.Reader) (Line, error) {
	rawLine, err := http1.ReadCRLFLine(reader, maxLineBytes)
	if errors.Is(err, http1.ErrLineTooLong) {
		return Line{}, errLineTooLong
	}
	if err != nil {
		return Line{}, fmt.Errorf("http1: reading status line: %w", err)
	}

	return parseLine(rawLine)
}

// parseLine parses line contents without a CRLF terminator, following
// HTTP-version SP status-code SP [ reason-phrase ] (RFC 9112, Section 4). The
// second SP is required even when the reason phrase is empty. The version must
// be one that http1.ParseVersion processes as 1.1; HTTP/1.0 is rejected as
// local policy. parseStatusCode checks the status code. The reason phrase may
// contain SP and HTAB and is preserved as received. This function does not
// enforce a size limit; readLine applies it.
func parseLine(rawLine string) (Line, error) {
	parts := strings.SplitN(rawLine, " ", 3)
	if len(parts) != 3 {
		return Line{}, errors.New(
			"http1: status line requires a space after the HTTP version and after the status code",
		)
	}

	rawVersion, rawCode, reasonPhrase := parts[0], parts[1], parts[2]

	version, err := http1.ParseVersion(rawVersion)
	if err != nil {
		return Line{}, err
	}

	if version != "1.1" {
		return Line{}, fmt.Errorf("http1: unsupported HTTP version %q", rawVersion)
	}

	statusCode, err := parseStatusCode(rawCode)
	if err != nil {
		return Line{}, err
	}

	if !grammar.IsFieldValue(reasonPhrase) {
		return Line{}, fmt.Errorf("http1: invalid reason phrase %q", reasonPhrase)
	}

	return Line{
		Version:      version,
		StatusCode:   statusCode,
		ReasonPhrase: reasonPhrase,
	}, nil
}
