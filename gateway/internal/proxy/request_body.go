package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"slices"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
)

var (
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

// readRequestBody buffers the whole request body, first writing
// continueResponse if the client expects 100-continue. Reading fails after a
// pause longer than requestBodyIdleTimeout or once requestBodyTimeout passes.
func readRequestBody(connection net.Conn, message *request.Message) ([]byte, error) {
	defer func() {
		if err := message.Body.Close(); err != nil {
			log.Printf("closing request body: %v", err)
		}
	}()

	cutoff := time.Now().Add(requestBodyTimeout)

	if slices.Contains(message.Fields.Expectations, "100-continue") {
		if err := connection.SetWriteDeadline(time.Now().Add(requestBodyIdleTimeout)); err != nil {
			return nil, fmt.Errorf("setting write deadline: %w", err)
		}
		if _, err := io.WriteString(connection, continueResponse); err != nil {
			return nil, fmt.Errorf("writing 100 Continue: %w", err)
		}
	}

	return io.ReadAll(newIdleDeadlineReader(connection, message.Body, requestBodyIdleTimeout, cutoff))
}
