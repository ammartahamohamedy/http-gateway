package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
	"github.com/stretchr/testify/require"
)

// readRequest parses raw, which must be valid, and buffers its body.
func readRequest(t *testing.T, raw string) (*request.Message, []byte) {
	t.Helper()

	message, err := request.Read(bufio.NewReader(strings.NewReader(raw)))
	require.NoError(t, err)

	content, err := io.ReadAll(message.Body)
	require.NoError(t, err)

	return message, content
}

func TestPrepareRequest(t *testing.T) {
	message, content := readRequest(t, "POST /documents HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n"+
		"Connection: keep-alive, x-secret\r\nKeep-Alive: timeout=5\r\nX-Secret: s\r\nProxy-Connection: keep-alive\r\n"+
		"TE: trailers\r\nUpgrade: websocket\r\nExpect: 100-continue\r\nForwarded: for=192.0.2.1\r\n"+
		"X-Forwarded-For: 192.0.2.1\r\nX-Forwarded-Host: a\r\nX-Forwarded-Proto: https\r\nVia: 1.0 edge\r\n"+
		"Proxy-Authorization: Basic cHJveHk6c2VjcmV0\r\nAuthorization: Bearer origin-token\r\n"+
		"Accept: application/json\r\n\r\nhello")
	received := message.Headers

	prepareRequest(message, content, "backend:8081")

	require.Equal(t, header.Section{
		"host":           {"backend:8081"},
		"content-length": {"5"},
		"accept":         {"application/json"},
		"authorization":  {"Bearer origin-token"},
		"via":            {"1.0 edge", gatewayVia},
		"connection":     {"close"},
	}, message.Headers)
	require.Equal(t, []string{"1.0 edge"}, received.Values("Via"), "the received headers are not changed")
	require.Equal(t, request.Fields{
		Authority:            "example.com",
		ContentLength:        5,
		ContentLengthPresent: true,
		ConnectionOptions:    []string{"keep-alive", "x-secret"},
		Expectations:         []string{"100-continue"},
	}, message.Fields)

	body, err := io.ReadAll(message.Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(body))
}

func TestPrepareRequestTargets(t *testing.T) {
	for raw, want := range map[string]string{
		"GET /search?q=go HTTP/1.1\r\nHost: a\r\n\r\n":           "/search?q=go",
		"GET http://example.com/a?b HTTP/1.1\r\nHost: a\r\n\r\n": "/a?b",
		"GET http://example.com HTTP/1.1\r\nHost: a\r\n\r\n":     "/",
		"OPTIONS * HTTP/1.1\r\nHost: a\r\n\r\n":                  "*",
	} {
		message, content := readRequest(t, raw)
		prepareRequest(message, content, "backend:8081")
		require.Equal(t, want, message.Line.RequestTarget)
	}
}

func TestPrepareRequestFraming(t *testing.T) {
	message, content := readRequest(t, "GET / HTTP/1.1\r\nHost: a\r\n\r\n")
	prepareRequest(message, content, "backend:8081")
	require.False(t, message.Fields.ContentLengthPresent, "a request without a body declares none")

	var encoded bytes.Buffer
	gzipWriter := gzip.NewWriter(&encoded)
	_, err := gzipWriter.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	message, content = readRequest(t, "POST / HTTP/1.1\r\nHost: a\r\nTransfer-Encoding: gzip, chunked\r\n\r\n"+
		strconv.FormatInt(int64(encoded.Len()), 16)+"\r\n"+encoded.String()+"\r\n0\r\n\r\n")
	prepareRequest(message, content, "backend:8081")
	require.Nil(t, message.Fields.TransferCodings, "decoded codings are never applied again")
	require.True(t, message.Fields.ContentLengthPresent)
	require.Equal(t, int64(5), message.Fields.ContentLength)

	message, content = readRequest(t, "POST / HTTP/1.1\r\nHost: a\r\nTransfer-Encoding: chunked\r\n\r\n"+
		"2\r\nhi\r\n0\r\nX-Checksum: abc\r\n\r\n")
	prepareRequest(message, content, "backend:8081")
	require.Equal(t, header.Section{"x-checksum": {"abc"}}, message.Trailers, "trailers stay for request.Write")
}

func TestPrepareResponse(t *testing.T) {
	message := &response.Message{
		Line: response.Line{Version: "1.1", StatusCode: 200},
		Headers: header.Section{
			"connection":   {"keep-alive, x-secret"},
			"keep-alive":   {"timeout=5"},
			"x-secret":     {"s"},
			"upgrade":      {"h2c"},
			"content-type": {"application/json"},
			"set-cookie":   {"a=1", "b=2"},
		},
	}

	require.NoError(t, prepareResponse(message))
	require.NoError(t, fields.ValidateDate(message.Headers.Values("Date")))

	delete(message.Headers, "date")
	require.Equal(t, header.Section{
		"content-type": {"application/json"},
		"set-cookie":   {"a=1", "b=2"},
		"via":          {gatewayVia},
		"connection":   {"close"},
	}, message.Headers)
}

func TestPrepareResponseKeepsTheBackendDate(t *testing.T) {
	message := &response.Message{
		Line:    response.Line{Version: "1.1", StatusCode: 204},
		Headers: header.Section{"date": {"Sun, 06 Nov 1994 08:49:37 GMT"}, "via": {"1.1 backend"}},
	}

	require.NoError(t, prepareResponse(message))
	require.Equal(t, []string{"Sun, 06 Nov 1994 08:49:37 GMT"}, message.Headers.Values("Date"))
	require.Equal(t, []string{"1.1 backend", gatewayVia}, message.Headers.Values("Via"))
}

func TestPrepareResponseInterim(t *testing.T) {
	message := &response.Message{
		Line:    response.Line{Version: "1.1", StatusCode: 103},
		Headers: header.Section{"link": {"</a.css>"}, "connection": {"x-hint"}, "x-hint": {"1"}},
	}

	require.NoError(t, prepareResponse(message))
	require.Equal(t, header.Section{"link": {"</a.css>"}, "via": {gatewayVia}}, message.Headers,
		"an interim response gets neither Date nor Connection: close")
}

func TestPrepareResponseRefusesAnInvalidConnectionList(t *testing.T) {
	headers := header.Section{"connection": {"a/b"}}
	message := &response.Message{Line: response.Line{Version: "1.1", StatusCode: 103}, Headers: headers}

	require.ErrorIs(t, prepareResponse(message), fields.ErrInvalidConnection)
	require.Equal(t, header.Section{"connection": {"a/b"}}, message.Headers)
}
