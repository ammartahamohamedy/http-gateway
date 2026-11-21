package response

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Write writes message to writer as an HTTP/1.1 response to a request sent with
// requestMethod: the status-line, the header section, and the body. The head is
// checked by statusLine and buildHead before the first byte is written, so an
// invalid message writes nothing; the trailers, known only once Body ends, are
// validated before the last chunk, and only those announcedTrailers allows
// follow the body. Interim responses that Read passes to onInterim are written
// the same way. A nil Body reads as empty. Write neither closes nor drains
// Body; errors from writer and Body are returned.
func Write(writer io.Writer, message *Message, requestMethod string) error {
	reasonPhrase, err := statusLine(message.Line)
	if err != nil {
		return err
	}

	frame := bodyFraming(message, requestMethod)

	head, err := buildHead(message, reasonPhrase, frame)
	if err != nil {
		return err
	}

	return body.WriteMessage(writer, head, message.Body, frame.sent, frame.length, func() header.Section {
		return announcedTrailers(message.Trailers, frame.trailerNames)
	})
}

// framing is how a response is framed: what its head declares and what its
// body sends, which differ for a response that declares a length but carries
// no content.
type framing struct {
	declared, sent body.Mode
	length         int64
	trailerNames   []string // the allowed fields the Trailer field announces
}

// statusLine checks line's status code, 100-599, and returns its reason
// phrase: the reasonPhrases entry for a registered code, or the received
// phrase, possibly empty, for an unregistered one. A phrase that is not a
// field value, so could split the message, is rejected.
func statusLine(line Line) (string, error) {
	if line.StatusCode < 100 || line.StatusCode > 599 {
		return "", fmt.Errorf("http1: status code %d is outside 100-599", line.StatusCode)
	}

	reasonPhrase, known := reasonPhrases[line.StatusCode]
	if !known {
		reasonPhrase = line.ReasonPhrase
	}

	if !grammar.IsFieldValue(reasonPhrase) {
		return "", fmt.Errorf("http1: invalid reason phrase %q", reasonPhrase)
	}

	return reasonPhrase, nil
}

// bodyFraming chooses how message, a response to requestMethod, is framed. A
// 1xx or 204 response carries no framing fields. A 205 response declares
// Content-Length: 0 and sends no content, whatever it received (RFC 9110,
// Section 15.3.6). A 304 response, and any response to HEAD, declares the
// Content-Length in Fields when present but has no body. Otherwise a declared
// Content-Length is sent with exactly that many bytes, and a response without
// one is sent chunked, announcing the allowed fields from Fields.TrailerNames.
func bodyFraming(message *Message, requestMethod string) framing {
	parsed := message.Fields

	switch {
	case message.Line.StatusCode < 200 || message.Line.StatusCode == 204:
		return framing{declared: body.None, sent: body.None}
	case message.Line.StatusCode == 205:
		return framing{declared: body.ContentLength, sent: body.None}
	case message.Line.StatusCode == 304 || requestMethod == "HEAD":
		if parsed.ContentLengthPresent {
			return framing{declared: body.ContentLength, sent: body.None, length: parsed.ContentLength}
		}
		return framing{declared: body.None, sent: body.None}
	case parsed.ContentLengthPresent:
		return framing{declared: body.ContentLength, sent: body.ContentLength, length: parsed.ContentLength}
	}

	trailerNames := slices.DeleteFunc(slices.Clone(parsed.TrailerNames), func(name string) bool {
		return !fields.IsAllowedTrailer(name)
	})
	return framing{declared: body.Chunked, sent: body.Chunked, trailerNames: trailerNames}
}

// buildHead returns the status-line, with reasonPhrase, and the header section
// with the framing fields frame declares (fields.WithFraming), never the
// received ones. The header section is validated as it is built.
func buildHead(message *Message, reasonPhrase string, frame framing) ([]byte, error) {
	var head bytes.Buffer
	head.WriteString("HTTP/1.1 ")
	head.WriteString(strconv.Itoa(message.Line.StatusCode))
	head.WriteString(" ")
	head.WriteString(reasonPhrase)
	head.WriteString("\r\n")

	if err := header.Write(&head, fields.WithFraming(message.Headers, frame.declared, frame.length, frame.trailerNames)); err != nil {
		return nil, err
	}

	return head.Bytes(), nil
}

// announcedTrailers returns the trailers named in names, those the Trailer
// field announced, so no other trailer follows the body.
func announcedTrailers(trailers header.Section, names []string) header.Section {
	announced := maps.Clone(trailers)
	maps.DeleteFunc(announced, func(name string, _ []string) bool {
		return !slices.Contains(names, name)
	})

	return announced
}
