package proxy

import (
	"io"
	"net"
	"testing"
	"time"

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
