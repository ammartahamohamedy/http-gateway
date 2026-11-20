package proxy

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// deadlineRecorder records read deadlines and optionally rejects them.
type deadlineRecorder struct {
	net.Conn
	deadlines []time.Time
	err       error
}

func (recorder *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	recorder.deadlines = append(recorder.deadlines, deadline)
	return recorder.err
}

func TestIdleDeadlineReaderMovesDeadlineBeforeEachRead(t *testing.T) {
	recorder := &deadlineRecorder{}
	start := time.Now()
	reader := newIdleDeadlineReader(recorder, io.NopCloser(strings.NewReader("hello")), time.Minute, start.Add(time.Hour))
	for range 2 {
		_, err := reader.Read(make([]byte, 2))
		require.NoError(t, err)
	}

	require.Len(t, recorder.deadlines, 2)
	for _, deadline := range recorder.deadlines {
		require.WithinDuration(t, start.Add(time.Minute), deadline, time.Second)
	}
}

func TestIdleDeadlineReaderStopsAtCutoff(t *testing.T) {
	recorder := &deadlineRecorder{}
	cutoff := time.Now().Add(time.Second)
	reader := newIdleDeadlineReader(recorder, io.NopCloser(strings.NewReader("hello")), time.Hour, cutoff)
	_, err := reader.Read(make([]byte, 5))
	require.NoError(t, err)
	require.Equal(t, []time.Time{cutoff}, recorder.deadlines)
}

func TestIdleDeadlineReaderReadsWhenDeadlineIsRejected(t *testing.T) {
	recorder := &deadlineRecorder{err: errors.New("connection closed")}
	reader := newIdleDeadlineReader(recorder, io.NopCloser(strings.NewReader("hello")), time.Minute, time.Now().Add(time.Hour))
	buffer := make([]byte, 5)
	count, err := reader.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, "hello", string(buffer[:count]))
}

type closeRecorder struct {
	io.Reader
	closed bool
}

func (recorder *closeRecorder) Close() error {
	recorder.closed = true
	return nil
}

func TestIdleDeadlineReaderClosesItsSource(t *testing.T) {
	source := &closeRecorder{Reader: strings.NewReader("hello")}
	reader := newIdleDeadlineReader(&deadlineRecorder{}, source, time.Minute, time.Now().Add(time.Hour))
	require.NoError(t, reader.Close())
	require.True(t, source.closed)
}
