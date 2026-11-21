package response

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name          string
		requestMethod string
		message       *Message
		want          string
	}{
		{"Content-Length body", "GET", &Message{
			Line:    Line{Version: "1.1", StatusCode: 200, ReasonPhrase: "OK"},
			Headers: header.Section{"content-type": {"application/json"}},
			Fields:  Fields{ContentLength: 2, ContentLengthPresent: true},
			Body:    io.NopCloser(strings.NewReader("{}")),
		}, "HTTP/1.1 200 OK\r\ncontent-length: 2\r\ncontent-type: application/json\r\n\r\n{}"},
		{"unknown length is chunked", "GET", &Message{
			Line: Line{Version: "1.1", StatusCode: 200, ReasonPhrase: "OK"},
			Body: io.NopCloser(strings.NewReader("hello")),
		}, "HTTP/1.1 200 OK\r\ntransfer-encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"},
		{"chunked with trailers announced from allowed names", "GET", &Message{
			Line:     Line{Version: "1.1", StatusCode: 200, ReasonPhrase: "OK"},
			Fields:   Fields{TransferCodings: []string{"chunked"}, TrailerNames: []string{"server-timing", "content-length"}},
			Body:     io.NopCloser(strings.NewReader("hi")),
			Trailers: header.Section{"server-timing": {"db;dur=5"}},
		}, "HTTP/1.1 200 OK\r\ntrailer: server-timing\r\ntransfer-encoding: chunked\r\n\r\n2\r\nhi\r\n0\r\nserver-timing: db;dur=5\r\n\r\n"},
		{"registry phrase replaces received phrase", "GET", &Message{
			Line:   Line{Version: "1.1", StatusCode: 404, ReasonPhrase: "Nope"},
			Fields: Fields{ContentLengthPresent: true},
		}, "HTTP/1.1 404 Not Found\r\ncontent-length: 0\r\n\r\n"},
		{"unregistered code keeps received phrase", "GET", &Message{
			Line:   Line{Version: "1.1", StatusCode: 299, ReasonPhrase: "Custom"},
			Fields: Fields{ContentLengthPresent: true},
		}, "HTTP/1.1 299 Custom\r\ncontent-length: 0\r\n\r\n"},
		{"unregistered code with empty phrase", "GET", &Message{
			Line:   Line{Version: "1.1", StatusCode: 299},
			Fields: Fields{ContentLengthPresent: true},
		}, "HTTP/1.1 299 \r\ncontent-length: 0\r\n\r\n"},
		{"204 carries no framing fields", "DELETE", &Message{
			Line:    Line{Version: "1.1", StatusCode: 204},
			Headers: header.Section{"content-length": {"5"}, "date": {"Sun, 06 Nov 1994 08:49:37 GMT"}},
			Fields:  Fields{ContentLength: 5, ContentLengthPresent: true},
			Body:    io.NopCloser(strings.NewReader("ignored")),
		}, "HTTP/1.1 204 No Content\r\ndate: Sun, 06 Nov 1994 08:49:37 GMT\r\n\r\n"},
		{"304 declares Content-Length without a body", "GET", &Message{
			Line:   Line{Version: "1.1", StatusCode: 304},
			Fields: Fields{ContentLength: 42, ContentLengthPresent: true},
			Body:   io.NopCloser(strings.NewReader("ignored")),
		}, "HTTP/1.1 304 Not Modified\r\ncontent-length: 42\r\n\r\n"},
		{"HEAD declares Content-Length without a body", "HEAD", &Message{
			Line:   Line{Version: "1.1", StatusCode: 200},
			Fields: Fields{ContentLength: 42, ContentLengthPresent: true},
		}, "HTTP/1.1 200 OK\r\ncontent-length: 42\r\n\r\n"},
		{"HEAD with unknown length declares nothing", "HEAD", &Message{
			Line:   Line{Version: "1.1", StatusCode: 200},
			Fields: Fields{TransferCodings: []string{"chunked"}},
		}, "HTTP/1.1 200 OK\r\n\r\n"},
		{"interim response", "GET", &Message{
			Line:    Line{Version: "1.1", StatusCode: 103, ReasonPhrase: "Early Hints"},
			Headers: header.Section{"link": {"</style.css>; rel=preload"}},
		}, "HTTP/1.1 103 Early Hints\r\nlink: </style.css>; rel=preload\r\n\r\n"},
		{"100 Continue", "POST", &Message{Line: Line{Version: "1.1", StatusCode: 100}}, "HTTP/1.1 100 Continue\r\n\r\n"},
		{"unregistered interim code", "GET", &Message{Line: Line{Version: "1.1", StatusCode: 199}}, "HTTP/1.1 199 \r\n\r\n"},
		{"received framing fields replaced", "GET", &Message{
			Line:    Line{Version: "1.1", StatusCode: 200},
			Headers: header.Section{"content-length": {"99"}, "transfer-encoding": {"gzip, chunked"}, "trailer": {"x-old"}},
			Fields:  Fields{ContentLength: 2, ContentLengthPresent: true},
			Body:    io.NopCloser(strings.NewReader("hi")),
		}, "HTTP/1.1 200 OK\r\ncontent-length: 2\r\n\r\nhi"},
		{"repeated Set-Cookie on separate lines", "GET", &Message{
			Line:    Line{Version: "1.1", StatusCode: 200},
			Headers: header.Section{"set-cookie": {"a=1; Expires=Wed, 21 Oct 2026 07:28:00 GMT", "b=2"}},
			Fields:  Fields{ContentLengthPresent: true},
		}, "HTTP/1.1 200 OK\r\ncontent-length: 0\r\nset-cookie: a=1; Expires=Wed, 21 Oct 2026 07:28:00 GMT\r\nset-cookie: b=2\r\n\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, Write(&output, test.message, test.requestMethod))
			require.Equal(t, test.want, output.String())
		})
	}
}

func TestWriteRejectsInvalidMessagesWithoutWriting(t *testing.T) {
	tests := []struct {
		name    string
		message *Message
		want    error
	}{
		{"status code below range", &Message{Line: Line{StatusCode: 99}}, nil},
		{"status code above range", &Message{Line: Line{StatusCode: 600}}, nil},
		{"zero status code", &Message{}, nil},
		{"reason phrase with CRLF", &Message{Line: Line{StatusCode: 299, ReasonPhrase: "x\r\nset-cookie: evil=1"}}, nil},
		{"reason phrase with NUL", &Message{Line: Line{StatusCode: 299, ReasonPhrase: "x\x00"}}, nil},
		{"invalid field value", &Message{
			Line:    Line{StatusCode: 200},
			Headers: header.Section{"location": {"/a\r\nset-cookie: evil=1"}},
		}, header.ErrInvalidField},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := Write(&output, test.message, "GET")
			require.Error(t, err)
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
			}
			require.Empty(t, output.String())
		})
	}
}

func TestWriteEnforcesDeclaredLength(t *testing.T) {
	for source := range slices.Values([]io.ReadCloser{
		io.NopCloser(strings.NewReader("hel")),
		io.NopCloser(strings.NewReader("hello!")),
		nil,
	}) {
		message := &Message{
			Line:   Line{StatusCode: 200},
			Fields: Fields{ContentLength: 5, ContentLengthPresent: true},
			Body:   source,
		}

		require.ErrorIs(t, Write(io.Discard, message, "GET"), body.ErrBodyLength)
	}
}

func TestWriteNeverCompletesABodyWithInvalidTrailers(t *testing.T) {
	message := &Message{
		Line:     Line{StatusCode: 200},
		Fields:   Fields{TrailerNames: []string{"x-bad"}},
		Body:     io.NopCloser(strings.NewReader("hi")),
		Trailers: header.Section{"x-bad": {"a\nb"}},
	}

	var output bytes.Buffer
	require.ErrorIs(t, Write(&output, message, "GET"), header.ErrInvalidField)
	require.NotContains(t, output.String(), "0\r\n")
}

// failingWriter returns err from every Write.
type failingWriter struct {
	err error
}

func (failing failingWriter) Write([]byte) (int, error) { return 0, failing.err }

func TestWritePassesWriterErrors(t *testing.T) {
	writeErr := errors.New("client reset")

	small := &Message{Line: Line{StatusCode: 200}, Fields: Fields{ContentLengthPresent: true}}
	require.ErrorIs(t, Write(failingWriter{err: writeErr}, small, "GET"), writeErr)

	largeHead := &Message{
		Line:    Line{StatusCode: 200},
		Headers: header.Section{"x-large": {strings.Repeat("a", 8*1024)}},
		Fields:  Fields{ContentLengthPresent: true},
	}
	require.ErrorIs(t, Write(failingWriter{err: writeErr}, largeHead, "GET"), writeErr)

	largeBody := &Message{
		Line:   Line{StatusCode: 200},
		Fields: Fields{ContentLength: 40000, ContentLengthPresent: true},
		Body:   io.NopCloser(strings.NewReader(strings.Repeat("a", 40000))),
	}
	require.ErrorIs(t, Write(failingWriter{err: writeErr}, largeBody, "GET"), writeErr)
}

func TestWriteRoundTripsThroughRead(t *testing.T) {
	var encoded bytes.Buffer
	gzipWriter := gzip.NewWriter(&encoded)
	_, err := gzipWriter.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	tests := []struct {
		name          string
		requestMethod string
		raw           string
		wantFields    Fields
	}{
		{"Content-Length", "GET", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello",
			Fields{ContentLength: 5, ContentLengthPresent: true}},
		{"chunked with trailers", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nTrailer: Server-Timing\r\n\r\n" +
			"5\r\nhello\r\n0\r\nServer-Timing: db;dur=5\r\n\r\n",
			Fields{TransferCodings: []string{"chunked"}, TrailerNames: []string{"server-timing"}}},
		{"close-delimited becomes chunked", "GET", "HTTP/1.1 200 OK\r\n\r\nhello",
			Fields{TransferCodings: []string{"chunked"}}},
		{"decoded gzip becomes plain chunked", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip, chunked\r\n\r\n" +
			strconv.FormatInt(int64(encoded.Len()), 16) + "\r\n" + encoded.String() + "\r\n0\r\n\r\n",
			Fields{TransferCodings: []string{"chunked"}}},
		{"HEAD keeps the declared length", "HEAD", "HTTP/1.1 200 OK\r\nContent-Length: 42\r\n\r\n",
			Fields{ContentLength: 42, ContentLengthPresent: true}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			received, err := Read(bufio.NewReader(strings.NewReader(test.raw)), test.requestMethod, nil)
			require.NoError(t, err)

			var output bytes.Buffer
			require.NoError(t, Write(&output, received, test.requestMethod))

			forwarded, err := Read(bufio.NewReader(&output), test.requestMethod, nil)
			require.NoError(t, err)
			require.Equal(t, test.wantFields, forwarded.Fields)

			content, err := io.ReadAll(forwarded.Body)
			require.NoError(t, err)
			if test.requestMethod != "HEAD" {
				require.NotEmpty(t, content)
			}
			require.Equal(t, received.Trailers, forwarded.Trailers)
		})
	}
}

func TestWriteForwardsInterimResponses(t *testing.T) {
	const raw = "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 103 Early Hints\r\nLink: </a.css>\r\n\r\n" +
		"HTTP/1.1 201 Created\r\nLocation: /documents/1\r\nContent-Length: 0\r\n\r\n"

	var output bytes.Buffer
	final, err := Read(bufio.NewReader(strings.NewReader(raw)), "POST", func(interim *Message) error {
		return Write(&output, interim, "POST")
	})
	require.NoError(t, err)
	require.NoError(t, Write(&output, final, "POST"))

	require.Equal(t, "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 103 Early Hints\r\nlink: </a.css>\r\n\r\n"+
		"HTTP/1.1 201 Created\r\ncontent-length: 0\r\nlocation: /documents/1\r\n\r\n", output.String())
}

func TestWriteSendsOnlyAnnouncedTrailers(t *testing.T) {
	message := &Message{
		Line:     Line{StatusCode: 200},
		Fields:   Fields{TrailerNames: []string{"server-timing"}},
		Body:     io.NopCloser(strings.NewReader("hi")),
		Trailers: header.Section{"server-timing": {"db;dur=5"}, "x-unannounced": {"1"}},
	}

	var output bytes.Buffer
	require.NoError(t, Write(&output, message, "GET"))
	require.Equal(t, "HTTP/1.1 200 OK\r\ntrailer: server-timing\r\ntransfer-encoding: chunked\r\n\r\n"+
		"2\r\nhi\r\n0\r\nserver-timing: db;dur=5\r\n\r\n", output.String())
}
