package request

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Write writes message to writer as an HTTP/1.1 request: the request-line with
// the method and request-target as given, the header section, and the body.
// The request-line, the Host field, the header section, and the trailers are
// validated before the first byte is written, so an invalid message writes
// nothing.
//
// The writer frames the body itself with fields.WithFraming, never copying the
// received framing fields. A message with Trailers, or with transfer codings in
// Fields, is written chunked, and its trailers follow the body, announced by a
// Trailer field. Otherwise a message whose Fields declare a Content-Length is
// written with exactly that many bytes, and one without has no body. A nil Body
// reads as empty. body.Write enforces the framing.
//
// Output is buffered and flushed once the body ends. Write neither closes nor
// drains Body; errors from writer and Body are returned.
func Write(writer io.Writer, message *Message) error {
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

	if err := header.Write(io.Discard, message.Trailers); err != nil {
		return err
	}

	mode, length := body.None, int64(0)
	switch {
	case len(message.Trailers) > 0 || len(message.Fields.TransferCodings) > 0:
		mode = body.Chunked
	case message.Fields.ContentLengthPresent:
		mode, length = body.ContentLength, message.Fields.ContentLength
	}

	var head bytes.Buffer
	head.WriteString(line.Method)
	head.WriteString(" ")
	head.WriteString(line.RequestTarget)
	head.WriteString(" HTTP/1.1\r\n")

	trailerNames := slices.Sorted(maps.Keys(message.Trailers))
	if err := header.Write(&head, fields.WithFraming(message.Headers, mode, length, trailerNames)); err != nil {
		return err
	}

	var source io.Reader = strings.NewReader("")
	if message.Body != nil {
		source = message.Body
	}

	buffered := bufio.NewWriter(writer)
	if _, err := buffered.Write(head.Bytes()); err != nil {
		return err
	}

	if err := body.Write(buffered, source, mode, length, func() header.Section { return message.Trailers }); err != nil {
		return err
	}

	return buffered.Flush()
}
