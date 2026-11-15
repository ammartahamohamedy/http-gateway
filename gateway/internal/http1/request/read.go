// Package request reads and validates HTTP/1.1 request messages.
package request

import (
	"bufio"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Read reads one request from reader: the request-line, after skipping up to
// maxLeadingEmptyLines empty lines, then the header section and the supported
// fields, validated by processFields. Every error is a *http1.StageError naming
// the failed stage; field failures wrap a *FieldError. Gateway policy remains
// separate.
//
// Body reads lazily from reader, so bytes after the body stay there; Message
// describes its framing.
//
// Size limits are local: maxLineBytes, headerLimits, and maxBodyBytes. EOF
// before a request-line matches io.EOF; EOF before headers finish matches
// io.ErrUnexpectedEOF. All errors return a nil Message.
//
// The caller must reuse the same buffered reader for subsequent bytes. On error,
// the reader may be positioned partway through a line. The input is never closed.
func Read(reader *bufio.Reader) (*Message, error) {
	var message Message
	var err error

	// Request-line stage: read and parse one request-line.
	if message.Line, err = readLine(reader); err != nil {
		return nil, &http1.StageError{Stage: http1.StageRequestLine, Err: err}
	}

	// Header stage: read, unfold, and parse the complete header section.
	if message.Headers, err = header.Read(reader, headerLimits); err != nil {
		return nil, &http1.StageError{Stage: http1.StageHeaders, Err: err}
	}

	// Field stage: validate supported fields and collect their metadata.
	if message.Fields, err = processFields(message.Line, message.Headers); err != nil {
		return nil, &http1.StageError{Stage: http1.StageFields, Err: err}
	}

	// Body stage: decide framing and attach a body reader over the same reader.
	if message.Body, err = messageBody(reader, &message); err != nil {
		return nil, &http1.StageError{Stage: http1.StageBody, Err: err}
	}

	return &message, nil
}
