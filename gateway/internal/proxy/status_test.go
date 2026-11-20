package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"syscall"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
	"github.com/stretchr/testify/require"
)

func TestRequestErrorStatus(t *testing.T) {
	staged := func(stage http1.Stage, err error) error { return &http1.StageError{Stage: stage, Err: err} }

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nothing sent", staged(http1.StageRequestLine, fmt.Errorf("reading: %w", io.EOF)), 0},
		{"client left mid-request", staged(http1.StageHeaders, io.ErrUnexpectedEOF), 0},
		{"connection reset", staged(http1.StageHeaders, fmt.Errorf("reading: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET})), 0},
		{"connection reset mid-body", fmt.Errorf("reading body: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}), 0},
		{"head timeout", staged(http1.StageHeaders, fmt.Errorf("reading: %w", os.ErrDeadlineExceeded)), 408},
		{"body timeout", fmt.Errorf("reading body: %w", os.ErrDeadlineExceeded), 408},
		{"declared body too large", staged(http1.StageBody, body.ErrBodyTooLarge), 413},
		{"decoded body too large", body.ErrBodyTooLarge, 413},
		{"request-line too long", staged(http1.StageRequestLine, request.ErrLineTooLong), 414},
		{"header line too long", staged(http1.StageHeaders, header.ErrLineTooLong), 431},
		{"header section too large", staged(http1.StageHeaders, header.ErrSectionTooLarge), 431},
		{"too many header fields", staged(http1.StageHeaders, header.ErrTooManyFields), 431},
		{"unserved host", errUnservedHost, 421},
		{"refused expectation", fields.ErrUnsupportedExpectation, 417},
		{"unsupported transfer coding", fields.ErrUnsupportedTransferCoding, 501},
		{"unknown transfer coding", fields.ErrUnknownTransferCoding, 501},
		{"CONNECT", errUnsupportedTargetForm, 501},
		{"TRACE", errUnsupportedMethod, 501},
		{"unsupported HTTP version", staged(http1.StageRequestLine, fmt.Errorf("%w %q", http1.ErrUnsupportedVersion, "HTTP/2.0")), 505},
		{"malformed HTTP version", staged(http1.StageRequestLine, errors.New("http1: invalid HTTP version")), 400},
		{"malformed request-line", staged(http1.StageRequestLine, errors.New("http1: invalid method")), 400},
		{"invalid field", staged(http1.StageFields, &http1.FieldError{Name: "Host", Err: fields.ErrInvalidHost}), 400},
		{"framing conflict", staged(http1.StageFields, &http1.FieldError{Name: "Transfer-Encoding", Err: fields.ErrFramingConflict}), 400},
		{"malformed chunked body", body.ErrInvalidFraming, 400},
		{"ambiguous path", errAmbiguousPath, 400},
		{"unsupported scheme", errUnsupportedScheme, 400},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, requestErrorStatus(test.err))
		})
	}
}

func TestUpstreamErrorStatus(t *testing.T) {
	require.Equal(t, 504, upstreamErrorStatus(fmt.Errorf("reading response upstream: %w", os.ErrDeadlineExceeded)))
	require.Equal(t, 502, upstreamErrorStatus(fmt.Errorf("dialing upstream: %w", syscall.ECONNREFUSED)))
	require.Equal(t, 502, upstreamErrorStatus(fields.ErrUnsupportedTransferCoding))
	require.Equal(t, 502, upstreamErrorStatus(io.ErrUnexpectedEOF))
}

func TestWriteGatewayResponse(t *testing.T) {
	for method := range slices.Values([]string{"", "GET", "HEAD"}) {
		client, server := net.Pipe()

		go func() {
			writeGatewayResponse(server, 502, method)
			_ = server.Close()
		}()

		written, err := io.ReadAll(client)
		require.NoError(t, err)

		relayed, err := response.Read(bufio.NewReader(bytes.NewReader(written)), method, nil)
		require.NoError(t, err)
		require.Equal(t, response.Line{Version: "1.1", StatusCode: 502, ReasonPhrase: "Bad Gateway"}, relayed.Line)
		require.True(t, relayed.Fields.ContentLengthPresent)
		require.Zero(t, relayed.Fields.ContentLength)
		require.Equal(t, []string{"close"}, relayed.Headers.Values("Connection"))
		require.NoError(t, fields.ValidateDate(relayed.Headers.Values("Date")))
		_ = client.Close()
	}
}

func TestWriteGatewayResponseToAClosedClient(t *testing.T) {
	client, server := net.Pipe()
	require.NoError(t, client.Close())
	require.NoError(t, server.Close())

	writeGatewayResponse(server, 400, "GET")
}
