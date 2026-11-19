package proxy

import (
	"bufio"
	"io"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
	"github.com/stretchr/testify/require"
)

// received is one request the fake upstream read, with its body.
type received struct {
	message *request.Message
	content []byte
}

// fakeUpstream starts a backend on a loopback listener. For each connection it
// reads one request, reports it on the returned channel, and lets respond write
// the reply before closing. It returns the backend's address.
func fakeUpstream(t *testing.T, respond func(connection net.Conn)) (string, <-chan received) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	requests := make(chan received, 4)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}

			go func() {
				defer func() { _ = connection.Close() }()

				message, err := request.Read(bufio.NewReader(connection))
				if err != nil {
					return
				}

				content, err := io.ReadAll(message.Body)
				if err != nil {
					return
				}

				requests <- received{message: message, content: content}
				respond(connection)
			}()
		}
	}()

	return listener.Addr().String(), requests
}

// replyWith returns a respond function for fakeUpstream that writes raw.
func replyWith(raw string) func(net.Conn) {
	return func(connection net.Conn) { _, _ = io.WriteString(connection, raw) }
}

// unusedAddress returns a loopback address with nothing listening on it.
func unusedAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

// runForward reads raw as a client request, buffers its body, and runs forward
// against upstream. It returns what the client received and forward's results.
func runForward(t *testing.T, raw, upstream string) (string, bool, error) {
	t.Helper()

	message, err := request.Read(bufio.NewReader(strings.NewReader(raw)))
	require.NoError(t, err)

	content, err := io.ReadAll(message.Body)
	require.NoError(t, err)

	client, gatewaySide := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	output := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(client)
		output <- string(data)
	}()

	committed, err := forward(gatewaySide, message, content, Config{Upstream: upstream})
	_ = gatewaySide.Close()

	return <-output, committed, err
}

func TestForwardRelaysTheExchange(t *testing.T) {
	upstream, requests := fakeUpstream(t, replyWith("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n"+
		"Content-Length: 2\r\nConnection: keep-alive\r\nKeep-Alive: timeout=5\r\n\r\n{}"))

	raw := "POST http://example.com/documents?x=1 HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n" +
		"Expect: 100-continue\r\nConnection: x-secret\r\nX-Secret: s\r\nX-Forwarded-For: 192.0.2.1\r\n" +
		"TE: trailers\r\nVia: 1.0 edge\r\nContent-Type: text/plain\r\n\r\nhello"

	output, committed, err := runForward(t, raw, upstream)
	require.NoError(t, err)
	require.True(t, committed)

	sent := <-requests
	require.Equal(t, request.Line{Method: "POST", RequestTarget: "/documents?x=1", Version: "1.1"}, sent.message.Line)
	require.Equal(t, "hello", string(sent.content))
	require.Equal(t, header.Section{
		"host":           {upstream},
		"content-length": {"5"},
		"content-type":   {"text/plain"},
		"via":            {"1.0 edge", gatewayVia},
		"connection":     {"close"},
	}, sent.message.Headers)

	relayed, err := response.Read(bufio.NewReader(strings.NewReader(output)), "POST", nil)
	require.NoError(t, err)
	require.Equal(t, 200, relayed.Line.StatusCode)
	require.Equal(t, []string{"close"}, relayed.Headers.Values("Connection"))
	require.Equal(t, []string{gatewayVia}, relayed.Headers.Values("Via"))
	require.Equal(t, []string{"application/json"}, relayed.Headers.Values("Content-Type"))
	require.Nil(t, relayed.Headers.Values("Keep-Alive"))
	require.NoError(t, fields.ValidateDate(relayed.Headers.Values("Date")))

	content, err := io.ReadAll(relayed.Body)
	require.NoError(t, err)
	require.Equal(t, "{}", string(content))
}

func TestForwardRelaysInterimResponsesExceptContinue(t *testing.T) {
	upstream, _ := fakeUpstream(t, replyWith("HTTP/1.1 100 Continue\r\n\r\n"+
		"HTTP/1.1 103 Early Hints\r\nLink: </a.css>; rel=preload\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n"))

	output, committed, err := runForward(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", upstream)
	require.NoError(t, err)
	require.True(t, committed)
	require.NotContains(t, output, "100 Continue")

	var interims []*response.Message
	final, err := response.Read(bufio.NewReader(strings.NewReader(output)), "GET", func(interim *response.Message) error {
		interims = append(interims, interim)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, interims, 1)
	require.Equal(t, 103, interims[0].Line.StatusCode)
	require.Equal(t, []string{"</a.css>; rel=preload"}, interims[0].Headers.Values("Link"))
	require.Equal(t, []string{gatewayVia}, interims[0].Headers.Values("Via"))
	require.Equal(t, 204, final.Line.StatusCode)
}

func TestForwardDropsInterimResponsesWithInvalidConnection(t *testing.T) {
	upstream, _ := fakeUpstream(t, replyWith("HTTP/1.1 103 Early Hints\r\nConnection: a/b\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n"))

	output, _, err := runForward(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", upstream)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(output, "HTTP/1.1 204 No Content\r\n"))
}

func TestForwardChunksABodyOfUnknownLength(t *testing.T) {
	upstream, _ := fakeUpstream(t, replyWith("HTTP/1.1 200 OK\r\n\r\nhello"))

	output, _, err := runForward(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", upstream)
	require.NoError(t, err)
	require.Contains(t, output, "transfer-encoding: chunked\r\n")

	relayed, err := response.Read(bufio.NewReader(strings.NewReader(output)), "GET", nil)
	require.NoError(t, err)

	content, err := io.ReadAll(relayed.Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
}

func TestForwardHEAD(t *testing.T) {
	upstream, requests := fakeUpstream(t, replyWith("HTTP/1.1 200 OK\r\nContent-Length: 42\r\n\r\n"))

	output, _, err := runForward(t, "HEAD / HTTP/1.1\r\nHost: example.com\r\n\r\n", upstream)
	require.NoError(t, err)
	require.Nil(t, (<-requests).message.Headers.Values("Content-Length"), "a request without a body declares none")

	relayed, err := response.Read(bufio.NewReader(strings.NewReader(output)), "HEAD", nil)
	require.NoError(t, err)
	require.Equal(t, int64(42), relayed.Fields.ContentLength)
	require.True(t, relayed.Fields.ContentLengthPresent)
	require.True(t, strings.HasSuffix(output, "\r\n\r\n"), "a HEAD response ends with its head")
}

func TestForwardKeepsRequestTrailers(t *testing.T) {
	upstream, requests := fakeUpstream(t, replyWith("HTTP/1.1 204 No Content\r\n\r\n"))

	raw := "POST /documents HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\nTrailer: X-Checksum\r\n\r\n" +
		"5\r\nhello\r\n0\r\nX-Checksum: abc\r\n\r\n"

	_, _, err := runForward(t, raw, upstream)
	require.NoError(t, err)

	sent := <-requests
	require.Equal(t, "hello", string(sent.content))
	require.Equal(t, header.Section{"x-checksum": {"abc"}}, sent.message.Trailers)
}

func TestForwardFailsBeforeCommit(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })

	saved := upstreamHeadTimeout
	t.Cleanup(func() { upstreamHeadTimeout = saved })
	upstreamHeadTimeout = 200 * time.Millisecond

	tests := []struct {
		name     string
		upstream func(t *testing.T) string
		status   int
	}{
		{"unreachable backend", unusedAddress, 502},
		{"closed without a response", func(t *testing.T) string {
			address, _ := fakeUpstream(t, func(net.Conn) {})
			return address
		}, 502},
		{"HTTP/1.0 response", func(t *testing.T) string {
			address, _ := fakeUpstream(t, replyWith("HTTP/1.0 200 OK\r\n\r\n"))
			return address
		}, 502},
		{"undecodable transfer coding", func(t *testing.T) string {
			address, _ := fakeUpstream(t, replyWith("HTTP/1.1 200 OK\r\nTransfer-Encoding: compress, chunked\r\n\r\n"))
			return address
		}, 502},
		{"no answer within the head timeout", func(t *testing.T) string {
			address, _ := fakeUpstream(t, func(net.Conn) { <-stop })
			return address
		}, 504},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			output, committed, err := runForward(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", test.upstream(t))
			require.Error(t, err)
			require.False(t, committed)
			require.Empty(t, output)
			require.Equal(t, test.status, upstreamErrorStatus(err))
		})
	}
}

func TestForwardTruncatedBodyAfterCommit(t *testing.T) {
	upstream, _ := fakeUpstream(t, replyWith("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nhello"))

	_, committed, err := runForward(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", upstream)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.True(t, committed, "the final response has started, so no gateway error may follow")
}
