package request

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Write writes message to writer as an HTTP/1.1 request: the request-line, the
// header section, and the body. The message is checked by validateMessage and
// buildHead before the first byte is written, so an invalid message writes
// nothing. A nil Body reads as empty. Write neither closes nor drains Body;
// errors from writer and Body are returned.
func Write(writer io.Writer, message *Message) error {
	if err := validateMessage(message); err != nil {
		return err
	}

	mode, length := bodyFraming(message)

	head, err := buildHead(message, mode, length)
	if err != nil {
		return err
	}

	return body.WriteMessage(writer, head, message.Body, mode, length, func() header.Section { return message.Trailers })
}

// validateMessage checks what the header section's own validation does not:
// the method, the request-target (validateTarget), exactly one valid Host, and
// the trailers, which are known before writing because request bodies are
// buffered.
func validateMessage(message *Message) error {
	line := message.Line

	if !grammar.IsToken(line.Method) {
		return fmt.Errorf("http1: invalid method %q", line.Method)
	}

	if err := validateTarget(line.Method, line.RequestTarget); err != nil {
		return err
	}

	if err := fields.ValidateRequestHost(message.Headers.Values("Host"), line.Method, line.RequestTarget); err != nil {
		return err
	}

	return header.Write(io.Discard, message.Trailers)
}

// bodyFraming chooses how message's body is framed. A message with Trailers, or
// with transfer codings in Fields, is written chunked, its trailers following
// the body. Otherwise a message whose Fields declare a Content-Length is
// written with exactly that many bytes, and one without has no body.
func bodyFraming(message *Message) (body.Mode, int64) {
	switch {
	case len(message.Trailers) > 0 || len(message.Fields.TransferCodings) > 0:
		return body.Chunked, 0
	case message.Fields.ContentLengthPresent:
		return body.ContentLength, message.Fields.ContentLength
	}

	return body.None, 0
}

// buildHead returns the request-line, with the method and request-target as
// given, and the header section with the framing fields for mode and length
// (fields.WithFraming), never the received ones. A chunked message's Trailer
// field names its trailers in sorted order. The header section is validated as
// it is built.
func buildHead(message *Message, mode body.Mode, length int64) ([]byte, error) {
	var head bytes.Buffer
	head.WriteString(message.Line.Method)
	head.WriteString(" ")
	head.WriteString(message.Line.RequestTarget)
	head.WriteString(" HTTP/1.1\r\n")

	trailerNames := slices.Sorted(maps.Keys(message.Trailers))
	if err := header.Write(&head, fields.WithFraming(message.Headers, mode, length, trailerNames)); err != nil {
		return nil, err
	}

	return head.Bytes(), nil
}
