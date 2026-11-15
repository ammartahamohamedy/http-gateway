package proxy

import (
	"io"
	"net"
	"time"
)

// newIdleDeadlineReader returns source read with connection's read deadline
// moved to idle from now before each Read, never past cutoff. Only a pause
// longer than idle, or reaching cutoff, fails a read. A connection that rejects
// the deadline is closed, so Read still runs and reports the cause, such as an
// early end of the message.
func newIdleDeadlineReader(connection net.Conn, source io.Reader, idle time.Duration, cutoff time.Time) io.Reader {
	return &idleDeadlineReader{connection: connection, source: source, idle: idle, cutoff: cutoff}
}

type idleDeadlineReader struct {
	connection net.Conn
	source     io.Reader
	idle       time.Duration
	cutoff     time.Time
}

func (reader *idleDeadlineReader) Read(buffer []byte) (int, error) {
	deadline := time.Now().Add(reader.idle)
	if deadline.After(reader.cutoff) {
		deadline = reader.cutoff
	}
	_ = reader.connection.SetReadDeadline(deadline)
	return reader.source.Read(buffer)
}
