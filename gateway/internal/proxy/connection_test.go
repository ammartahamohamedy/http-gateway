package proxy

import (
	"bufio"
	"io"
	"net"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/stretchr/testify/require"
)

// serve runs HandleConnection on one end of a pipe and returns the client end
// and a channel closed when the handler returns.
func serve(t *testing.T) (net.Conn, <-chan struct{}) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleConnection(server)
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

func TestHandleConnectionTimesOutSlowRequestHead(t *testing.T) {
	saved := requestHeadTimeout
	t.Cleanup(func() { requestHeadTimeout = saved })
	requestHeadTimeout = 50 * time.Millisecond

	client, done := serve(t)
	_, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: exam"))
	require.NoError(t, err)

	waitForHandler(t, done)
	_, err = client.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF, "the connection must be closed")
}

func TestHandleConnectionFinishesCompleteRequestHead(t *testing.T) {
	client, done := serve(t)
	_, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	require.NoError(t, err)
	waitForHandler(t, done)
}

func TestHandleConnectionReadsContentLengthBody(t *testing.T) {
	client, done := serve(t)
	_, err := client.Write([]byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello"))
	require.NoError(t, err)
	waitForHandler(t, done)
}

func TestHandleConnectionRejectsUnsupportedExpectation(t *testing.T) {
	client, done := serve(t)
	_, err := client.Write([]byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nExpect: x-custom\r\n\r\n"))
	require.NoError(t, err)
	waitForHandler(t, done)
	_, err = client.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF, "nothing may be written before closing")
}

// readBodyOverPipe parses a request head arriving on a pipe and returns the
// server end with the parsed request. send runs concurrently on the client end.
func readBodyOverPipe(t *testing.T, head string, send func(client net.Conn)) (net.Conn, *http1.Request) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	go func() {
		if _, err := client.Write([]byte(head)); err == nil {
			send(client)
		}
	}()
	request, err := http1.ReadRequest(bufio.NewReader(server))
	require.NoError(t, err)
	return server, request
}

func setBodyTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	saved := requestBodyTimeout
	t.Cleanup(func() { requestBodyTimeout = saved })
	requestBodyTimeout = timeout
}

func TestReadRequestBody(t *testing.T) {
	tests := []struct {
		name string
		head string
		body string
	}{
		{"no body", "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", ""},
		{"Content-Length body", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 7\r\n\r\n", "he\x00\r\nlo"},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			setBodyTimeout(t, 2*time.Second)
			server, request := readBodyOverPipe(t, test.head, func(client net.Conn) {
				_, _ = client.Write([]byte(test.body))
			})
			content, err := readRequestBody(server, request)
			require.NoError(t, err)
			require.Equal(t, test.body, string(content))
		})
	}
}

func TestReadRequestBodySendsContinue(t *testing.T) {
	setBodyTimeout(t, 2*time.Second)
	interim := make(chan string, 1)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nExpect: 100-continue\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		response := make([]byte, len(continueResponse))
		_, _ = io.ReadFull(client, response)
		interim <- string(response)
		_, _ = client.Write([]byte("hello"))
	})
	content, err := readRequestBody(server, request)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
	require.Equal(t, continueResponse, <-interim)
}

func TestReadRequestBodySkipsChunked(t *testing.T) {
	setBodyTimeout(t, 2*time.Second)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\nExpect: 100-continue\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(net.Conn) {})
	// A write would block on the pipe, so returning promptly shows no 100 was sent.
	content, err := readRequestBody(server, request)
	require.NoError(t, err)
	require.Nil(t, content)
}

func TestReadRequestBodyTimesOut(t *testing.T) {
	setBodyTimeout(t, 50*time.Millisecond)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		_, _ = client.Write([]byte("he"))
	})
	_, err := readRequestBody(server, request)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestReadRequestBodyTruncated(t *testing.T) {
	setBodyTimeout(t, 2*time.Second)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		_, _ = client.Write([]byte("he"))
		_ = client.Close()
	})
	content, err := readRequestBody(server, request)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, "he", string(content))
}
