package request

import (
	"bufio"
	"fmt"
	"io"
	"maps"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// messageBody returns the body reader for request's framing, reading from
// reader. A chunked body has its other transfer codings decoded and sets
// message.Trailers when it ends.
func messageBody(reader *bufio.Reader, message *Message) (io.ReadCloser, error) {
	parsed := message.Fields
	mode, err := body.Framing(body.Message{
		Version:              message.Line.Version,
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
		if parsed.ContentLength > maxBodyBytes {
			return nil, fmt.Errorf("%w: Content-Length %d exceeds %d bytes",
				body.ErrBodyTooLarge, parsed.ContentLength, maxBodyBytes)
		}
		return body.NewFixedReader(reader, parsed.ContentLength), nil
	case body.Chunked:
		chunked := body.NewChunkedReader(reader, maxBodyBytes, func(trailers header.Section) {
			message.Trailers = allowedTrailers(trailers)
		})
		codings := parsed.TransferCodings[:len(parsed.TransferCodings)-1]
		return body.Decode(chunked, codings, maxBodyBytes), nil
	}

	return nil, fmt.Errorf("http1: unexpected request body framing %d", mode)
}

// allowedTrailers returns the trailer fields that fields.IsAllowedTrailer
// permits, or nil when none remain. The input is not changed.
func allowedTrailers(trailers header.Section) header.Section {
	allowed := maps.Clone(trailers)
	maps.DeleteFunc(allowed, func(name string, _ []string) bool {
		return !fields.IsAllowedTrailer(name)
	})

	if len(allowed) == 0 {
		return nil
	}

	return allowed
}
