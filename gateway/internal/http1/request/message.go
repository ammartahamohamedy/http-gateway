package request

import (
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Message holds an HTTP/1.x request-line, header fields, parsed metadata from
// the supported fields, the body, and its trailers. Body is empty without
// framing fields, exactly Content-Length bytes with one, and the decoded data
// for chunked framing.
type Message struct {
	Line    Line
	Headers header.Headers
	Fields  Fields
	Body    io.ReadCloser

	// Trailers holds the allowed trailer fields, set once Body returns io.EOF.
	Trailers header.Headers
}
