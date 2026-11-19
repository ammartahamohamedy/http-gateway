package proxy

import (
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
)

// errorResponseTimeout bounds writing the gateway's own error response.
var errorResponseTimeout = 10 * time.Second

// requestErrorStatus returns the status answering a request that failed to read
// or that gateway policy refused, or 0 when the client ended the connection and
// no response can be sent. Timeouts get 408, oversized bodies 413, an oversized
// request-line 414, an oversized header section 431, refused expectations 417,
// and unsupported transfer codings or CONNECT 501 (RFC 9110, Section 15.5);
// every other error is a malformed or refused request, 400.
func requestErrorStatus(err error) int {
	netErr, isNetErr := errors.AsType[net.Error](err)

	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return 0
	case isNetErr && netErr.Timeout():
		return 408
	case errors.Is(err, body.ErrBodyTooLarge):
		return 413
	case errors.Is(err, request.ErrLineTooLong):
		return 414
	case errors.Is(err, header.ErrLineTooLong), errors.Is(err, header.ErrSectionTooLarge),
		errors.Is(err, header.ErrTooManyFields):
		return 431
	case errors.Is(err, fields.ErrUnsupportedExpectation):
		return 417
	case errors.Is(err, fields.ErrUnsupportedTransferCoding), errors.Is(err, fields.ErrUnknownTransferCoding),
		errors.Is(err, ErrUnsupportedTargetForm):
		return 501
	default:
		return 400
	}
}

// upstreamErrorStatus returns 504 (Gateway Timeout) when the upstream exchange
// timed out and 502 (Bad Gateway) for every other upstream failure (RFC 9110,
// Sections 15.6.3 and 15.6.5).
func upstreamErrorStatus(err error) int {
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return 504
	}

	return 502
}

// writeErrorResponse sends the gateway's own response with status to client,
// answering a request sent with requestMethod, empty when unknown. It has no
// body, so it declares Content-Length: 0 rather than being chunked, and carries
// Connection: close, since the connection closes after it. A write failure is
// logged, since nothing else can be done with it.
func writeErrorResponse(client net.Conn, status int, requestMethod string) {
	message := &response.Message{
		Line: response.Line{Version: "1.1", StatusCode: status},
		Headers: header.Section{
			"connection": {"close"},
			"date":       {fields.FormatDate(time.Now())},
		},
		Fields: response.Fields{ContentLengthPresent: true},
	}

	if err := client.SetWriteDeadline(time.Now().Add(errorResponseTimeout)); err != nil {
		log.Printf("setting write deadline for %d response: %v", status, err)
		return
	}

	if err := response.Write(client, message, requestMethod); err != nil {
		log.Printf("writing %d response: %v", status, err)
	}
}
