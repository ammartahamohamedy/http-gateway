package http1

import "io"

// StatusLine contains the HTTP version number, numeric status code, and
// optional reason phrase from a response's start-line.
type StatusLine struct {
	// Version is the numeric protocol version, such as "1.1".
	Version      string
	StatusCode   int
	ReasonPhrase string
}

// Response holds an HTTP/1.x status-line, header fields, and an optional body
// stream. The type itself does not parse or validate these fields.
type Response struct {
	StatusLine StatusLine
	Headers    []HeaderField
	Body       io.ReadCloser
}
