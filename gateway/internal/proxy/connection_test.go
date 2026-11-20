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
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
	"github.com/stretchr/testify/require"
)

// serve runs HandleConnection with config on one end of a pipe and returns the
// client end and a channel closed when the handler returns.
func serve(t *testing.T, config Config) (net.Conn, <-chan struct{}) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleConnection(server, config)
	}()

	return client, done
}

func waitForHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "handler did not return")
	}
}

// exchange sends raw to HandleConnection with config, without waiting for it
// to be read in full, and returns the response the client receives with its
// body.
func exchange(t *testing.T, raw string, config Config) (*response.Message, string) {
	t.Helper()
	client, done := serve(t, config)

	go func() { _, _ = io.WriteString(client, raw) }()

	reader := bufio.NewReader(client)
	message, err := response.Read(reader, "GET", nil)
	require.NoError(t, err)

	content, err := io.ReadAll(message.Body)
	require.NoError(t, err)

	waitForHandler(t, done)
	return message, string(content)
}

func TestHandleConnectionForwardsRequests(t *testing.T) {
	upstream, requests := fakeUpstream(t, replyWith("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))

	tests := []struct {
		name    string
		raw     string
		content string
	}{
		{"no body", "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", ""},
		{"Content-Length body", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello", "hello"},
		{"chunked body", "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n", "hello"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, content := exchange(t, test.raw, Config{Upstream: upstream})
			require.Equal(t, 200, message.Line.StatusCode)
			require.Equal(t, "ok", content)
			require.Equal(t, test.content, string((<-requests).content))
		})
	}
}

func TestHandleConnectionAnswersContinueLocally(t *testing.T) {
	upstream, requests := fakeUpstream(t, replyWith("HTTP/1.1 201 Created\r\nContent-Length: 0\r\n\r\n"))
	client, done := serve(t, Config{Upstream: upstream})

	go func() {
		_, _ = io.WriteString(client, "POST /documents HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nExpect: 100-continue\r\n\r\n")
	}()

	reader := bufio.NewReader(client)
	interim := make([]byte, len(continueResponse))
	_, err := io.ReadFull(reader, interim)
	require.NoError(t, err)
	require.Equal(t, continueResponse, string(interim))

	go func() { _, _ = io.WriteString(client, "hello") }()

	final, err := response.Read(reader, "POST", nil)
	require.NoError(t, err)
	require.Equal(t, 201, final.Line.StatusCode)
	waitForHandler(t, done)

	sent := <-requests
	require.Equal(t, "hello", string(sent.content))
	require.Nil(t, sent.message.Headers.Values("Expect"), "the backend never sees the expectation")
}

func TestHandleConnectionAnswersRefusedRequests(t *testing.T) {
	upstream, _ := fakeUpstream(t, replyWith("HTTP/1.1 204 No Content\r\n\r\n"))

	tests := []struct {
		name   string
		raw    string
		status int
	}{
		{"malformed request-line", "GET  / HTTP/1.1\r\nHost: example.com\r\n\r\n", 400},
		{"HTTP/1.0", "GET / HTTP/1.0\r\nHost: example.com\r\n\r\n", 505},
		{"another major version", "GET / HTTP/2.0\r\nHost: example.com\r\n\r\n", 505},
		{"HTTP/2 connection preface", "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n", 505},
		{"missing Host", "GET / HTTP/1.1\r\n\r\n", 400},
		{"framing conflict", "POST / HTTP/1.1\r\nHost: a\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n", 400},
		{"ambiguous path", "GET /a/../b HTTP/1.1\r\nHost: example.com\r\n\r\n", 400},
		{"request-line too long", "GET /" + strings.Repeat("a", 9000) + " HTTP/1.1\r\nHost: a\r\n\r\n", 414},
		{"header section too large", "GET / HTTP/1.1\r\nHost: a\r\n" + strings.Repeat("X-Pad: "+strings.Repeat("a", 4000)+"\r\n", 9) + "\r\n", 431},
		{"declared body too large", "POST / HTTP/1.1\r\nHost: a\r\nContent-Length: 2000000\r\n\r\n", 413},
		{"unsupported expectation", "POST / HTTP/1.1\r\nHost: a\r\nContent-Length: 5\r\nExpect: x-custom\r\n\r\n", 417},
		{"unsupported transfer coding", "POST / HTTP/1.1\r\nHost: a\r\nTransfer-Encoding: compress, chunked\r\n\r\n", 501},
		{"CONNECT", "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n", 501},
		{"TRACE", "TRACE / HTTP/1.1\r\nHost: example.com\r\n\r\n", 501},
		{"TRACE with Max-Forwards 0", "TRACE / HTTP/1.1\r\nHost: example.com\r\nMax-Forwards: 0\r\n\r\n", 501},
		{"invalid Max-Forwards on OPTIONS", "OPTIONS / HTTP/1.1\r\nHost: example.com\r\nMax-Forwards: x\r\n\r\n", 400},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, content := exchange(t, test.raw, Config{Upstream: upstream})
			require.Equal(t, test.status, message.Line.StatusCode)
			require.Equal(t, []string{"close"}, message.Headers.Values("Connection"))
			require.Empty(t, content)
		})
	}
}

func TestHandleConnectionAnswersUpstreamFailures(t *testing.T) {
	message, _ := exchange(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", Config{Upstream: unusedAddress(t)})
	require.Equal(t, 502, message.Line.StatusCode)
}

func TestHandleConnectionTimesOutSlowRequestHead(t *testing.T) {
	saved := requestHeadTimeout
	t.Cleanup(func() { requestHeadTimeout = saved })
	requestHeadTimeout = 50 * time.Millisecond

	message, _ := exchange(t, "GET / HTTP/1.1\r\nHost: exam", Config{Upstream: unusedAddress(t)})
	require.Equal(t, 408, message.Line.StatusCode)
}

func TestHandleConnectionTimesOutSlowBody(t *testing.T) {
	setBodyTimeouts(t, 50*time.Millisecond, time.Minute)

	message, _ := exchange(t, "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhe", Config{Upstream: unusedAddress(t)})
	require.Equal(t, 408, message.Line.StatusCode)
}

func TestHandleConnectionClosesAfterTruncatedBody(t *testing.T) {
	setBodyTimeouts(t, time.Minute, time.Minute)
	client, done := serve(t, Config{Upstream: unusedAddress(t)})

	_, err := client.Write([]byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhe"))
	require.NoError(t, err)
	require.NoError(t, client.Close())

	// With minute-long timeouts, only the early end of the body can end the handler.
	waitForHandler(t, done)
}

func TestHandleConnectionClosesSilentlyWithoutARequest(t *testing.T) {
	client, done := serve(t, Config{Upstream: unusedAddress(t)})
	require.NoError(t, client.Close())
	waitForHandler(t, done)
}

func TestAdmitRequest(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"admitted", "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", nil},
		{"TRACE", "TRACE / HTTP/1.1\r\nHost: example.com\r\n\r\n", errUnsupportedMethod},
		{"undecodable transfer coding", "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: compress, chunked\r\n\r\n0\r\n\r\n", fields.ErrUnsupportedTransferCoding},
		{"CONNECT tunnel", "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n", ErrUnsupportedTargetForm},
		{"ambiguous path", "GET /a/../b HTTP/1.1\r\nHost: example.com\r\n\r\n", ErrAmbiguousPath},
		{"unsupported expectation", "GET / HTTP/1.1\r\nHost: example.com\r\nExpect: something\r\n\r\n", fields.ErrUnsupportedExpectation},
		{"first refusal wins", "POST /a/../b HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: compress, chunked\r\nExpect: something\r\n\r\n0\r\n\r\n", fields.ErrUnsupportedTransferCoding},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, err := request.Read(bufio.NewReader(strings.NewReader(test.raw)))
			require.NoError(t, err)

			err = admitRequest(message)
			if test.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestHandleConnectionAnswersExhaustedMaxForwardsItself(t *testing.T) {
	message, content := exchange(t, "OPTIONS * HTTP/1.1\r\nHost: example.com\r\nMax-Forwards: 0\r\n\r\n",
		Config{Upstream: unusedAddress(t)})
	require.Equal(t, 200, message.Line.StatusCode, "answered as the final recipient, never forwarded")
	require.Equal(t, []string{"0"}, message.Headers.Values("Content-Length"))
	require.Equal(t, []string{"close"}, message.Headers.Values("Connection"))
	require.Empty(t, content)
}

func TestHandleConnectionForwardsDecrementedMaxForwards(t *testing.T) {
	upstream, requests := fakeUpstream(t, replyWith("HTTP/1.1 204 No Content\r\n\r\n"))
	message, _ := exchange(t, "OPTIONS / HTTP/1.1\r\nHost: example.com\r\nMax-Forwards: 3\r\n\r\n", Config{Upstream: upstream})
	require.Equal(t, 204, message.Line.StatusCode)

	sent := <-requests
	require.Equal(t, []string{"2"}, sent.message.Headers.Values("Max-Forwards"))
}
