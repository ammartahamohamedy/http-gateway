package response

import "github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"

// Local response limits. They bound buffering and how long a server can hold an
// exchange; RFC 9112 sets no universal maximum for any of them.
const (
	// maxLineBytes caps status-line content, excluding CRLF, at 8 KiB.
	maxLineBytes = 8 * 1024

	// maxInterimResponses caps the interim (1xx) responses accepted before the
	// final response, so a server cannot delay it indefinitely.
	maxInterimResponses = 5

	// maxBodyBytes caps a response body at 1 MiB: the declared Content-Length,
	// or the data read or decoded.
	maxBodyBytes = 1 << 20
)

// headerLimits are the local limits for each response header section, interim
// and final.
var headerLimits = header.Limits{
	LineBytes:    8 * 1024,
	SectionBytes: 32 * 1024,
	Fields:       100,
}
