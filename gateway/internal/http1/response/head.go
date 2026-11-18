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

// readFinalHead reads status-lines and header sections until a final (non-1xx)
// response and returns its head. Each interim response, which has no body (RFC
// 9110, Section 15.2), is passed to onInterim as its own Message, or discarded
// when onInterim is nil. More than maxInterimResponses interim responses and a
// 101 (Switching Protocols) response are rejected, and EOF where a final
// status-line must follow interim responses becomes io.ErrUnexpectedEOF.
//
// The loop spans the status-line and header stages, so it tags its own errors
// with http1.StageStatusLine or http1.StageHeaders.
func readFinalHead(reader *bufio.Reader, onInterim func(*Message) error) (Line, header.Section, error) {
	for interimCount := 0; ; interimCount++ {
		line, err := readLine(reader)
		if err != nil {
			if interimCount > 0 && errors.Is(err, io.EOF) {
				err = fmt.Errorf("http1: reading final status line: %w", io.ErrUnexpectedEOF)
			}
			return Line{}, nil, &http1.StageError{Stage: http1.StageStatusLine, Err: err}
		}

		if line.StatusCode == 101 {
			return Line{}, nil, &http1.StageError{Stage: http1.StageStatusLine, Err: errSwitchingProtocols}
		}

		if line.StatusCode < 200 && interimCount == maxInterimResponses {
			return Line{}, nil, &http1.StageError{Stage: http1.StageStatusLine, Err: errTooManyInterimResponses}
		}

		headers, err := header.Read(reader, headerLimits)
		if err != nil {
			return Line{}, nil, &http1.StageError{Stage: http1.StageHeaders, Err: err}
		}

		if line.StatusCode >= 200 {
			return line, headers, nil
		}

		if onInterim != nil {
			if err := onInterim(&Message{Line: line, Headers: headers}); err != nil {
				return Line{}, nil, err
			}
		}
	}
}
