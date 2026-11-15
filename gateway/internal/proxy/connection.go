package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"slices"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
)

var (
	// requestHeadTimeout bounds reading the whole request head, so a client
	// sending it slowly cannot hold the connection. A per-line cap alone would
	// not stop that.
	requestHeadTimeout = 10 * time.Second

	// requestBodyIdleTimeout bounds each pause while reading the request body,
	// and the 100 (Continue) write, so slow but steady clients succeed.
	requestBodyIdleTimeout = 30 * time.Second

	// requestBodyTimeout caps the whole body read, so a client sending a byte
	// just within each idle timeout cannot hold the connection indefinitely.
	requestBodyTimeout = 5 * time.Minute
)

// continueResponse is the 100 (Continue) interim response (RFC 9110, Section
// 10.1.1).
const continueResponse = "HTTP/1.1 100 Continue\r\n\r\n"

// HandleConnection reads one request head within requestHeadTimeout, checks
// transfer coding, target, and expectation policy, buffers the body
// (readRequestBody), and prints the request if accepted. It attempts to close
// the connection before returning. Deadline, read, validation, policy, and
// close errors are logged without terminating the listener. Apart from 100
// (Continue), no response is sent, and nothing is forwarded yet.
func HandleConnection(connection net.Conn) {
	defer func() {
		if err := connection.Close(); err != nil {
			log.Printf("closing connection: %v", err)
		}
	}()

	if err := connection.SetReadDeadline(time.Now().Add(requestHeadTimeout)); err != nil {
		log.Printf("setting read deadline: %v", err)
		return
	}

	reader := bufio.NewReader(connection)

	request, err := http1.ReadRequest(reader)
	if err == nil {
		err = fields.CheckTransferCodingSupport(request.Fields.TransferCodings)
	}
	if err != nil {
		// TODO: Match wrapped errors with errors.Is and respond before closing:
		// 400 for fields.ErrInvalidTransferEncoding and fields.ErrChunkedNotFinal;
		// 413 for body.ErrBodyTooLarge; 501 for unsupported or unknown transfer
		// codings.
		log.Printf("reading request: %v", err)
		return
	}

	if err := checkRequestTarget(request); err != nil {
		log.Printf("rejecting request: %v", err)
		return
	}
	if err := fields.CheckExpectationSupport(request.Fields.Expectations); err != nil {
		// TODO: Respond 417 before closing.
		log.Printf("rejecting request: %v", err)
		return
	}

	content, err := readRequestBody(connection, request)
	if err != nil {
		// TODO: Respond before closing: 400 for body.ErrInvalidFraming and 413
		// for body.ErrBodyTooLarge.
		log.Printf("reading request body: %v", err)
		return
	}

	fmt.Printf("Method: %s\nRequest target: %s\nVersion: %s\nAuthority: %s\n",
		request.RequestLine.Method, request.RequestLine.RequestTarget,
		request.RequestLine.Version, request.Fields.Authority)
	fmt.Printf("Body: %d bytes\n", len(content))
}

// readRequestBody buffers the whole request body, first writing
// continueResponse if the client expects 100-continue. Reading fails after a
// pause longer than requestBodyIdleTimeout or once requestBodyTimeout passes.
func readRequestBody(connection net.Conn, request *http1.Request) ([]byte, error) {
	defer func() {
		if err := request.Body.Close(); err != nil {
			log.Printf("closing request body: %v", err)
		}
	}()
	cutoff := time.Now().Add(requestBodyTimeout)
	if slices.Contains(request.Fields.Expectations, "100-continue") {
		if err := connection.SetWriteDeadline(time.Now().Add(requestBodyIdleTimeout)); err != nil {
			return nil, fmt.Errorf("setting write deadline: %w", err)
		}
		if _, err := io.WriteString(connection, continueResponse); err != nil {
			return nil, fmt.Errorf("writing 100 Continue: %w", err)
		}
	}
	return io.ReadAll(&idleDeadlineReader{connection: connection, source: request.Body, cutoff: cutoff})
}

// idleDeadlineReader moves the connection's read deadline to
// requestBodyIdleTimeout from now before each Read, never past cutoff. A
// connection that rejects the deadline is closed, so Read still runs and
// reports the cause, such as an early end of the body.
type idleDeadlineReader struct {
	connection net.Conn
	source     io.Reader
	cutoff     time.Time
}

func (reader *idleDeadlineReader) Read(buffer []byte) (int, error) {
	deadline := time.Now().Add(requestBodyIdleTimeout)
	if deadline.After(reader.cutoff) {
		deadline = reader.cutoff
	}
	_ = reader.connection.SetReadDeadline(deadline)
	return reader.source.Read(buffer)
}
