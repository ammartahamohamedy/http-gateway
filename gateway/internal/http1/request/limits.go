package request

import (
	"errors"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Local request limits. They bound buffering and connection time; RFC 9112
// sets no universal maximum for any of them.
const (
	// maxLineBytes caps request-line content, excluding CRLF, at 8 KiB.
	// RFC 9112, Section 3 recommends supporting at least 8000 octets.
	maxLineBytes = 8 * 1024

	// maxLeadingEmptyLines caps the empty lines skipped before a request-line,
	// so a client cannot hold a connection by sending CRLFs forever. RFC 9112,
	// Section 2.2 asks servers to ignore at least one.
	maxLeadingEmptyLines = 4

	// maxBodyBytes caps a request body at 1 MiB: the declared
	// Content-Length, or the decoded chunked data.
	maxBodyBytes = 1 << 20
)

// headerLimits are the local limits for a request header section.
var headerLimits = header.Limits{
	LineBytes:    8 * 1024,
	SectionBytes: 32 * 1024,
	Fields:       100,
}

var (
	// ErrLineTooLong is returned by Read when request-line
	// content, excluding CRLF, exceeds maxLineBytes.
	ErrLineTooLong = errors.New("http1: request line exceeds 8192 bytes")

	// ErrTooManyLeadingEmptyLines is returned by Read when more than
	// maxLeadingEmptyLines empty lines precede the request-line.
	ErrTooManyLeadingEmptyLines = errors.New("http1: too many empty lines before request line")
)
