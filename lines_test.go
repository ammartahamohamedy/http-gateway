package main

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStreamLines(t *testing.T) {
	tests := []struct {
		name   string
		reader io.Reader
		want   []string
	}{
		{
			name:   "empty input",
			reader: strings.NewReader(""),
			want:   []string{},
		},
		{
			name:   "newline terminated lines",
			reader: strings.NewReader("first\nsecond\n"),
			want:   []string{"first", "second"},
		},
		{
			name:   "final line without newline",
			reader: strings.NewReader("first\nlast"),
			want:   []string{"first", "last"},
		},
		{
			name:   "blank lines",
			reader: strings.NewReader("\nfirst\n\nlast\n\n"),
			want:   []string{"", "first", "", "last", ""},
		},
		{
			name:   "newlines at read boundaries",
			reader: strings.NewReader("1234567\n12345678\nend"),
			want:   []string{"1234567", "12345678", "end"},
		},
		{
			name:   "line spanning many reads",
			reader: strings.NewReader(strings.Repeat("gopher", 100) + "\n"),
			want:   []string{strings.Repeat("gopher", 100)},
		},
		{
			name:   "UTF-8 spanning reads",
			reader: strings.NewReader("1234567🐹\nمرحبا\n"),
			want:   []string{"1234567🐹", "مرحبا"},
		},
		{
			name:   "one byte per read",
			reader: iotest.OneByteReader(strings.NewReader("first\n\nlast")),
			want:   []string{"first", "", "last"},
		},
		{
			name:   "data returned together with EOF",
			reader: iotest.DataErrReader(strings.NewReader("first\nlast")),
			want:   []string{"first", "last"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &trackedReadCloser{
				Reader: test.reader,
				closed: make(chan struct{}),
			}

			require.Equal(t, test.want, collectLines(t, streamLines(source)))
			requireSourceClosed(t, source)
		})
	}
}

func TestStreamLinesErrors(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name     string
		reader   io.Reader
		closeErr error
		want     []string
	}{
		{
			name:   "read failure",
			reader: iotest.ErrReader(readErr),
			want:   []string{},
		},
		{
			name: "data returned together with read failure",
			reader: iotest.DataErrReader(io.MultiReader(
				strings.NewReader("ready\n"),
				iotest.ErrReader(readErr),
			)),
			want: []string{"ready"},
		},
		{
			name:     "close failure",
			reader:   strings.NewReader("ready\n"),
			closeErr: errors.New("close failed"),
			want:     []string{"ready"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &trackedReadCloser{
				Reader:   test.reader,
				closed:   make(chan struct{}),
				closeErr: test.closeErr,
			}

			require.Equal(t, test.want, collectLines(t, streamLines(source)))
			requireSourceClosed(t, source)
		})
	}
}

func TestStreamLinesBeforeConnectionCloses(t *testing.T) {
	source, client := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = source.Close()
	})

	lines := streamLines(source)
	writeResult := make(chan error, 1)
	go func() {
		_, err := io.WriteString(client, "first\nsecond")
		writeResult <- err
	}()

	select {
	case line, open := <-lines:
		require.True(t, open, "channel closed before receiving a line")
		require.Equal(t, "first", line)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "complete line was not delivered while the connection was open")
	}

	select {
	case err := <-writeResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "connection write did not complete")
	}

	require.NoError(t, client.Close())
	require.Equal(t, []string{"second"}, collectLines(t, lines))
}

type trackedReadCloser struct {
	io.Reader
	closed   chan struct{}
	closeErr error
}

func (source *trackedReadCloser) Close() error {
	close(source.closed)
	return source.closeErr
}

func requireSourceClosed(t *testing.T, source *trackedReadCloser) {
	t.Helper()
	select {
	case <-source.closed:
	default:
		require.FailNow(t, "input must be closed before the lines channel closes")
	}
}

func collectLines(t *testing.T, channel <-chan string) []string {
	t.Helper()
	lines := make([]string, 0)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case line, open := <-channel:
			if !open {
				return lines
			}
			lines = append(lines, line)
		case <-timeout.C:
			require.FailNow(t, "timed out waiting for the lines channel to close")
			return nil
		}
	}
}
