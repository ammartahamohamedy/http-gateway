package request

import (
	"bufio"
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// messageBody returns the body.NewReader body for message, reading from reader
// within maxBodyBytes. A chunked body sets message.Trailers to its allowed
// trailer fields when it ends.
func messageBody(reader *bufio.Reader, message *Message) (io.ReadCloser, error) {
	parsed := message.Fields

	return body.NewReader(reader, body.Message{
		Version:              message.Line.Version,
		ContentLength:        parsed.ContentLength,
		ContentLengthPresent: parsed.ContentLengthPresent,
		TransferCodings:      parsed.TransferCodings,
	}, maxBodyBytes, func(trailers header.Section) {
		message.Trailers = fields.AllowedTrailers(trailers)
	})
}
