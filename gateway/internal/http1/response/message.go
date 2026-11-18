// Package response reads and validates HTTP/1.1 response messages.
package response

import (
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Message holds an HTTP/1.x status-line, header fields, and an optional body
// stream. The type itself does not parse or validate these fields.
type Message struct {
	Line    Line
	Headers header.Section
	Body    io.ReadCloser
}
