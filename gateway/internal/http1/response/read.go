// Package response reads and validates HTTP/1.1 response messages.
package response

import "bufio"

// Read reads the final response to a request sent with requestMethod, after the
// interim (1xx) responses that readFinalHead passes to onInterim. It validates
// the supported fields into Fields with processFields and attaches Body, which
// reads lazily from reader with the framing body.Framing selects for
// requestMethod and the status code, so bytes after the body stay there.
// Trailers holds the allowed trailer fields once Body returns io.EOF.
//
// Every read and validation error is a *http1.StageError naming the failed
// stage; an error from onInterim is returned unchanged. Size limits are local:
// maxLineBytes, headerLimits, and maxBodyBytes. EOF before the first
// status-line matches io.EOF; any later EOF matches io.ErrUnexpectedEOF. All
// errors return a nil Message.
//
// The caller must reuse the same buffered reader for subsequent bytes. On error,
// the reader may be positioned partway through a line. The input is never closed.
func Read(reader *bufio.Reader, requestMethod string, onInterim func(*Message) error) (*Message, error) {
	var message Message
	var err error

	// Head stages: read the final status-line and header section, passing
	// interim responses to onInterim.
	if message.Line, message.Headers, err = readFinalHead(reader, onInterim); err != nil {
		return nil, err
	}

	return &message, nil
}
