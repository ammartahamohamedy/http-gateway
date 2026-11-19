package proxy

import (
	"bufio"
	"log"
	"net"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
)

// requestHeadTimeout bounds reading the whole request head, so a client sending
// it slowly cannot hold the connection. A per-line cap alone would not stop that.
var requestHeadTimeout = 10 * time.Second

// Config holds the gateway settings HandleConnection needs.
type Config struct {
	// Upstream is the backend's TCP address, host:port: dialed for every
	// exchange and sent as the forwarded request's Host.
	Upstream string
}

// HandleConnection runs one exchange on connection, the client socket it owns.
// It reads the request head within requestHeadTimeout, applies transfer
// coding, target, and expectation policy, buffers the body (readRequestBody),
// and forwards the request to config.Upstream (forward).
//
// A request that fails to read or is refused gets the status
// requestErrorStatus chooses, and an upstream failure before the final
// response is committed gets the one upstreamErrorStatus chooses, both written
// by writeErrorResponse. The connection is closed before returning, and every
// error is logged without terminating the listener.
func HandleConnection(connection net.Conn, config Config) {
	defer func() {
		if err := connection.Close(); err != nil {
			log.Printf("closing connection: %v", err)
		}
	}()

	reject := func(err error, requestMethod string) {
		log.Printf("rejecting request: %v", err)
		if status := requestErrorStatus(err); status != 0 {
			writeErrorResponse(connection, status, requestMethod)
		}
	}

	if err := connection.SetReadDeadline(time.Now().Add(requestHeadTimeout)); err != nil {
		log.Printf("setting read deadline: %v", err)
		return
	}

	message, err := request.Read(bufio.NewReader(connection))
	if err != nil {
		reject(err, "")
		return
	}

	requestMethod := message.Line.Method

	if err := fields.CheckTransferCodingSupport(message.Fields.TransferCodings); err != nil {
		reject(err, requestMethod)
		return
	}

	if err := checkRequestTarget(message); err != nil {
		reject(err, requestMethod)
		return
	}

	if err := fields.CheckExpectationSupport(message.Fields.Expectations); err != nil {
		reject(err, requestMethod)
		return
	}

	content, err := readRequestBody(connection, message)
	if err != nil {
		reject(err, requestMethod)
		return
	}

	committed, err := forward(connection, message, content, config)
	if err != nil {
		log.Printf("forwarding request: %v", err)
		if !committed {
			writeErrorResponse(connection, upstreamErrorStatus(err), requestMethod)
		}
	}
}
