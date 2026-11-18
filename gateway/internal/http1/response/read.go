// Package response reads and validates HTTP/1.1 response messages.
package response

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

var (
	// errSwitchingProtocols reports a 101 (Switching Protocols) response, which
	// the gateway never requests since it sends no Upgrade field.
	errSwitchingProtocols = errors.New("http1: 101 (Switching Protocols) response without an upgrade request")

	// errTooManyInterimResponses reports more than maxInterimResponses interim
	// responses before the final response.
	errTooManyInterimResponses = errors.New("http1: too many interim responses before the final response")
)

// Read reads the final response to a request sent with requestMethod. Each
// interim (1xx) response before it, a status-line and header section without a
// body (RFC 9110, Section 15.2), is passed to onInterim as its own Message; a nil
// onInterim discards them. At most maxInterimResponses are accepted, and a 101
// (Switching Protocols) response is rejected.
//
// For the final response, Read validates the supported fields into Fields and
// attaches Body, which reads lazily from reader with the framing body.Framing
// selects for requestMethod and the status code, so bytes after the body stay
// there. Trailers holds the allowed trailer fields once Body returns io.EOF.
//
// Every read and validation error is a *http1.StageError naming the failed
// stage; an error from onInterim stops Read and is returned unchanged. Size
// limits are local: maxLineBytes and headerLimits. EOF before the first
// status-line matches io.EOF; EOF anywhere later, including before a final
// response that follows interim ones, matches io.ErrUnexpectedEOF. All errors
// return a nil Message.
//
// The caller must reuse the same buffered reader for subsequent bytes. On error,
// the reader may be positioned partway through a line. The input is never closed.
func Read(reader *bufio.Reader, requestMethod string, onInterim func(*Message) error) (*Message, error) {
	var message Message
	var err error

	for interimCount := 0; ; interimCount++ {
		// Status-line stage: read one status-line, interim or final.
		if message.Line, err = readLine(reader); err != nil {
			if interimCount > 0 && errors.Is(err, io.EOF) {
				err = fmt.Errorf("http1: reading final status line: %w", io.ErrUnexpectedEOF)
			}
			return nil, &http1.StageError{Stage: http1.StageStatusLine, Err: err}
		}

		statusCode := message.Line.StatusCode

		if statusCode == 101 {
			return nil, &http1.StageError{Stage: http1.StageStatusLine, Err: errSwitchingProtocols}
		}

		if statusCode < 200 && interimCount == maxInterimResponses {
			return nil, &http1.StageError{Stage: http1.StageStatusLine, Err: errTooManyInterimResponses}
		}

		// Header stage: read, unfold, and parse the complete header section.
		if message.Headers, err = header.Read(reader, headerLimits); err != nil {
			return nil, &http1.StageError{Stage: http1.StageHeaders, Err: err}
		}

		if statusCode >= 200 {
			break
		}

		if onInterim != nil {
			interim := Message{Line: message.Line, Headers: message.Headers}
			if err := onInterim(&interim); err != nil {
				return nil, err
			}
		}
	}

	return &message, nil
}
