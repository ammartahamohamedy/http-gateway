package request

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name    string
		message *Message
		want    string
	}{
		{"no body", &Message{
			Line:    Line{Method: "GET", RequestTarget: "/search?q=go", Version: "1.1"},
			Headers: header.Section{"host": {"backend:8080"}, "accept": {"application/json"}},
		}, "GET /search?q=go HTTP/1.1\r\naccept: application/json\r\nhost: backend:8080\r\n\r\n"},
		{"Content-Length body", &Message{
			Line:    Line{Method: "POST", RequestTarget: "/documents", Version: "1.1"},
			Headers: header.Section{"host": {"backend"}, "content-type": {"text/plain"}},
			Fields:  Fields{ContentLength: 5, ContentLengthPresent: true},
			Body:    io.NopCloser(strings.NewReader("hello")),
		}, "POST /documents HTTP/1.1\r\ncontent-length: 5\r\ncontent-type: text/plain\r\nhost: backend\r\n\r\nhello"},
		{"Content-Length zero", &Message{
			Line:    Line{Method: "POST", RequestTarget: "/documents", Version: "1.1"},
			Headers: header.Section{"host": {"backend"}},
			Fields:  Fields{ContentLengthPresent: true},
		}, "POST /documents HTTP/1.1\r\ncontent-length: 0\r\nhost: backend\r\n\r\n"},
		{"unknown length is chunked", &Message{
			Line:    Line{Method: "POST", RequestTarget: "/documents", Version: "1.1"},
			Headers: header.Section{"host": {"backend"}},
			Fields:  Fields{TransferCodings: []string{"gzip", "chunked"}},
			Body:    io.NopCloser(strings.NewReader("hello")),
		}, "POST /documents HTTP/1.1\r\nhost: backend\r\ntransfer-encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"},
		{"trailers force chunked", &Message{
			Line:     Line{Method: "POST", RequestTarget: "/documents", Version: "1.1"},
			Headers:  header.Section{"host": {"backend"}},
			Fields:   Fields{ContentLength: 5, ContentLengthPresent: true},
			Body:     io.NopCloser(strings.NewReader("hello")),
			Trailers: header.Section{"x-checksum": {"abc"}, "server-timing": {"db;dur=5"}},
		}, "POST /documents HTTP/1.1\r\nhost: backend\r\ntrailer: server-timing, x-checksum\r\ntransfer-encoding: chunked\r\n\r\n" +
			"5\r\nhello\r\n0\r\nserver-timing: db;dur=5\r\nx-checksum: abc\r\n\r\n"},
		{"received framing fields replaced", &Message{
			Line: Line{Method: "POST", RequestTarget: "/documents", Version: "1.1"},
			Headers: header.Section{"host": {"backend"}, "content-length": {"99"},
				"transfer-encoding": {"gzip, chunked"}, "trailer": {"x-old"}},
			Fields: Fields{ContentLength: 2, ContentLengthPresent: true},
			Body:   io.NopCloser(strings.NewReader("hi")),
		}, "POST /documents HTTP/1.1\r\ncontent-length: 2\r\nhost: backend\r\n\r\nhi"},
		{"absolute-form target as given", &Message{
			Line:    Line{Method: "GET", RequestTarget: "http://example.com/a%2Fb?x=1", Version: "1.1"},
			Headers: header.Section{"host": {"example.com"}},
		}, "GET http://example.com/a%2Fb?x=1 HTTP/1.1\r\nhost: example.com\r\n\r\n"},
		{"asterisk-form", &Message{
			Line:    Line{Method: "OPTIONS", RequestTarget: "*", Version: "1.1"},
			Headers: header.Section{"host": {"backend"}},
		}, "OPTIONS * HTTP/1.1\r\nhost: backend\r\n\r\n"},
		{"repeated values on separate lines", &Message{
			Line:    Line{Method: "GET", RequestTarget: "/", Version: "1.1"},
			Headers: header.Section{"host": {"backend"}, "accept": {"text/html", "application/json"}},
		}, "GET / HTTP/1.1\r\naccept: text/html\r\naccept: application/json\r\nhost: backend\r\n\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, Write(&output, test.message))
			require.Equal(t, test.want, output.String())
		})
	}
}

func TestWriteRejectsInvalidMessagesWithoutWriting(t *testing.T) {
	valid := func() *Message {
		return &Message{Line: Line{Method: "GET", RequestTarget: "/", Version: "1.1"}, Headers: header.Section{"host": {"backend"}}}
	}

	tests := []struct {
		name   string
		modify func(*Message)
		want   error
	}{
		{"method not a token", func(message *Message) { message.Line.Method = "GE T" }, nil},
		{"empty method", func(message *Message) { message.Line.Method = "" }, nil},
		{"target with CRLF", func(message *Message) { message.Line.RequestTarget = "/a\r\nhost: evil" }, nil},
		{"target with space", func(message *Message) { message.Line.RequestTarget = "/a b" }, nil},
		{"relative target", func(message *Message) { message.Line.RequestTarget = "a" }, nil},
		{"missing Host", func(message *Message) { message.Headers = header.Section{} }, fields.ErrInvalidHost},
		{"repeated Host", func(message *Message) { message.Headers["host"] = []string{"a", "b"} }, fields.ErrInvalidHost},
		{"invalid field value", func(message *Message) { message.Headers["x-split"] = []string{"a\r\nb"} }, header.ErrInvalidField},
		{"invalid trailer", func(message *Message) { message.Trailers = header.Section{"x-bad": {"a\nb"}} }, header.ErrInvalidField},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message := valid()
			test.modify(message)

			var output bytes.Buffer
			err := Write(&output, message)
			require.Error(t, err)
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
			}
			require.Empty(t, output.String())
		})
	}
}

func TestWriteEnforcesDeclaredLength(t *testing.T) {
	tests := []struct {
		name   string
		source io.ReadCloser
	}{
		{"shorter body", io.NopCloser(strings.NewReader("hel"))},
		{"longer body", io.NopCloser(strings.NewReader("hello!"))},
		{"nil body", nil},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message := &Message{
				Line:    Line{Method: "POST", RequestTarget: "/documents", Version: "1.1"},
				Headers: header.Section{"host": {"backend"}},
				Fields:  Fields{ContentLength: 5, ContentLengthPresent: true},
				Body:    test.source,
			}

			require.ErrorIs(t, Write(io.Discard, message), body.ErrBodyLength)
		})
	}
}

// failingWriter returns err from every Write.
type failingWriter struct {
	err error
}

func (failing failingWriter) Write([]byte) (int, error) { return 0, failing.err }

func TestWritePassesWriterErrors(t *testing.T) {
	writeErr := errors.New("backend reset")
	message := &Message{Line: Line{Method: "GET", RequestTarget: "/", Version: "1.1"}, Headers: header.Section{"host": {"backend"}}}

	require.ErrorIs(t, Write(failingWriter{err: writeErr}, message), writeErr)

	message.Headers["x-large"] = []string{strings.Repeat("a", 8*1024)}
	require.ErrorIs(t, Write(failingWriter{err: writeErr}, message), writeErr)

	delete(message.Headers, "x-large")
	message.Fields = Fields{ContentLength: 40000, ContentLengthPresent: true}
	message.Body = io.NopCloser(strings.NewReader(strings.Repeat("a", 40000)))
	require.ErrorIs(t, Write(failingWriter{err: writeErr}, message), writeErr)
}

func TestWriteRoundTripsThroughRead(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"no body", "GET /search?q=go HTTP/1.1\r\nHost: backend\r\nAccept: application/json\r\n\r\n"},
		{"Content-Length body", "POST /documents HTTP/1.1\r\nHost: backend\r\nContent-Length: 5\r\n\r\nhello"},
		{"chunked body with trailers", "POST /documents HTTP/1.1\r\nHost: backend\r\nTransfer-Encoding: chunked\r\n" +
			"Trailer: X-Checksum\r\n\r\n5\r\nhello\r\n0\r\nX-Checksum: abc\r\n\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			received, err := Read(bufio.NewReader(strings.NewReader(test.raw)))
			require.NoError(t, err)

			content, err := io.ReadAll(received.Body)
			require.NoError(t, err)
			received.Body = io.NopCloser(bytes.NewReader(content))

			var output bytes.Buffer
			require.NoError(t, Write(&output, received))

			forwarded, err := Read(bufio.NewReader(&output))
			require.NoError(t, err)

			forwardedContent, err := io.ReadAll(forwarded.Body)
			require.NoError(t, err)
			require.Equal(t, received.Line, forwarded.Line)
			require.Equal(t, string(content), string(forwardedContent))
			require.Equal(t, received.Trailers, forwarded.Trailers)
			require.Equal(t, received.Headers.Values("Host"), forwarded.Headers.Values("Host"))
		})
	}
}
