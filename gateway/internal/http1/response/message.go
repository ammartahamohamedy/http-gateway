package response

import (
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Message holds an HTTP/1.x status-line, header fields, parsed metadata from the
// supported fields, the body, and its trailers. An interim response carries only
// its status-line and header fields.
type Message struct {
	Line    Line
	Headers header.Section
	Fields  Fields
	Body    io.ReadCloser

	// Trailers holds the allowed trailer fields, set once Body returns io.EOF.
	Trailers header.Section
}
