package http1

import (
	"bufio"
	"fmt"
	"io"
	"maps"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// requestBody returns the body reader for request's framing, reading from
// reader. A chunked body sets request.Trailers when it ends.
func requestBody(reader *bufio.Reader, request *Request) (io.ReadCloser, error) {
	parsed := request.Fields
	mode, err := body.Framing(body.Message{
		Version:              request.RequestLine.Version,
		ContentLength:        parsed.ContentLength,
		ContentLengthPresent: parsed.ContentLengthPresent,
		TransferCodings:      parsed.TransferCodings,
	})
	if err != nil {
		return nil, err
	}
	switch mode {
	case body.None:
		return body.NewFixedReader(reader, 0), nil
	case body.ContentLength:
		if parsed.ContentLength > maxRequestBodyBytes {
			return nil, fmt.Errorf("%w: Content-Length %d exceeds %d bytes",
				body.ErrBodyTooLarge, parsed.ContentLength, maxRequestBodyBytes)
		}
		return body.NewFixedReader(reader, parsed.ContentLength), nil
	case body.Chunked:
		return body.NewChunkedReader(reader, maxRequestBodyBytes, func(trailers header.Headers) {
			request.Trailers = allowedTrailers(trailers)
		}), nil
	}
	return nil, fmt.Errorf("http1: unexpected request body framing %d", mode)
}

// allowedTrailers returns the trailer fields that fields.IsAllowedTrailer
// permits, or nil when none remain. The input is not changed.
func allowedTrailers(trailers header.Headers) header.Headers {
	allowed := maps.Clone(trailers)
	maps.DeleteFunc(allowed, func(name string, _ []string) bool {
		return !fields.IsAllowedTrailer(name)
	})
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}
