package proxy

import (
	"bufio"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/stretchr/testify/require"
)

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

func setBodyTimeouts(t *testing.T, idle, total time.Duration) {
	t.Helper()
	savedIdle, savedTotal := requestBodyIdleTimeout, requestBodyTimeout
	t.Cleanup(func() { requestBodyIdleTimeout, requestBodyTimeout = savedIdle, savedTotal })
	requestBodyIdleTimeout, requestBodyTimeout = idle, total
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
			setBodyTimeouts(t, 2*time.Second, 2*time.Second)
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
	setBodyTimeouts(t, 2*time.Second, 2*time.Second)
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

func TestReadRequestBodyDecodesChunked(t *testing.T) {
	setBodyTimeouts(t, 2*time.Second, 2*time.Second)
	interim := make(chan string, 1)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\nExpect: 100-continue\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		response := make([]byte, len(continueResponse))
		_, _ = io.ReadFull(client, response)
		interim <- string(response)
		_, _ = client.Write([]byte("5\r\nhello\r\n0\r\n\r\n"))
	})
	content, err := readRequestBody(server, request)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
	require.Equal(t, continueResponse, <-interim)
}

func TestReadRequestBodyIdleTimeout(t *testing.T) {
	setBodyTimeouts(t, 50*time.Millisecond, 2*time.Second)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		_, _ = client.Write([]byte("he"))
	})
	_, err := readRequestBody(server, request)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestReadRequestBodyTruncated(t *testing.T) {
	setBodyTimeouts(t, 2*time.Second, 2*time.Second)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		_, _ = client.Write([]byte("he"))
		_ = client.Close()
	})
	content, err := readRequestBody(server, request)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, "he", string(content))
}

// trickle writes each byte of data to client, pausing between bytes.
func trickle(client net.Conn, data string, pause time.Duration) {
	for index := range len(data) {
		if _, err := client.Write([]byte{data[index]}); err != nil {
			return
		}
		time.Sleep(pause)
	}
}

func TestReadRequestBodyToleratesSlowSender(t *testing.T) {
	setBodyTimeouts(t, 200*time.Millisecond, 2*time.Second)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 6\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		trickle(client, "hello!", 50*time.Millisecond)
	})
	content, err := readRequestBody(server, request)
	require.NoError(t, err, "a steady sender may take longer than one idle timeout overall")
	require.Equal(t, "hello!", string(content))
}

func TestReadRequestBodyOverallTimeout(t *testing.T) {
	setBodyTimeouts(t, 200*time.Millisecond, 150*time.Millisecond)
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 100\r\n\r\n"
	server, request := readBodyOverPipe(t, head, func(client net.Conn) {
		trickle(client, strings.Repeat("a", 100), 20*time.Millisecond)
	})
	_, err := readRequestBody(server, request)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}
