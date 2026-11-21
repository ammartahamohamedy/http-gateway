package response

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Write writes message to writer as an HTTP/1.1 response to a request sent with
// requestMethod: the status-line, the header section, and the body. A
// registered status code gets its reasonPhrases entry; an unregistered one
// keeps the received phrase, which may be empty. The status-line and the header
// section are validated before the first byte is written, so an invalid message
// writes nothing; the trailers, known only once Body ends, are validated before
// the last chunk.
//
// The writer frames the body itself with fields.WithFraming, never copying the
// received framing fields. A 1xx or 204 response carries no framing fields. A
// 205 response declares Content-Length: 0 and sends no content, whatever it
// received (RFC 9110, Section 15.3.6). A 304 response, and any response to HEAD, declares the Content-Length in Fields
// when present but has no body. Otherwise a declared Content-Length is written
// with exactly that many bytes, and a response without one is written chunked.
// Its Trailer field names the allowed fields from Fields.TrailerNames, and only
// those Trailers follow the body. A nil Body reads as empty. body.Write
// enforces the framing.
//
// Interim responses that Read passes to onInterim are written the same way.
// Output is buffered and flushed once the body ends. Write neither closes nor
// drains Body; errors from writer and Body are returned.
func Write(writer io.Writer, message *Message, requestMethod string) error {
	line := message.Line

	if line.StatusCode < 100 || line.StatusCode > 599 {
		return fmt.Errorf("http1: status code %d is outside 100-599", line.StatusCode)
	}

	reasonPhrase, known := reasonPhrases[line.StatusCode]
	if !known {
		reasonPhrase = line.ReasonPhrase
	}

	if !grammar.IsFieldValue(reasonPhrase) {
		return fmt.Errorf("http1: invalid reason phrase %q", reasonPhrase)
	}

	parsed := message.Fields
	declared, sent, length := body.None, body.None, int64(0)
	var trailerNames []string

	switch {
	case line.StatusCode < 200 || line.StatusCode == 204:
	case line.StatusCode == 205:
		declared = body.ContentLength
	case line.StatusCode == 304 || requestMethod == "HEAD":
		if parsed.ContentLengthPresent {
			declared, length = body.ContentLength, parsed.ContentLength
		}
	case parsed.ContentLengthPresent:
		declared, sent, length = body.ContentLength, body.ContentLength, parsed.ContentLength
	default:
		declared, sent = body.Chunked, body.Chunked
		trailerNames = slices.DeleteFunc(slices.Clone(parsed.TrailerNames), func(name string) bool {
			return !fields.IsAllowedTrailer(name)
		})
	}

	var head bytes.Buffer
	head.WriteString("HTTP/1.1 ")
	head.WriteString(strconv.Itoa(line.StatusCode))
	head.WriteString(" ")
	head.WriteString(reasonPhrase)
	head.WriteString("\r\n")

	if err := header.Write(&head, fields.WithFraming(message.Headers, declared, length, trailerNames)); err != nil {
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

	announcedTrailers := func() header.Section {
		trailers := maps.Clone(message.Trailers)
		maps.DeleteFunc(trailers, func(name string, _ []string) bool {
			return !slices.Contains(trailerNames, name)
		})
		return trailers
	}

	if err := body.Write(buffered, source, sent, length, announcedTrailers); err != nil {
		return err
	}

	return buffered.Flush()
}
