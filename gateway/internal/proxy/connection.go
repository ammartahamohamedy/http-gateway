package proxy

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
)

// requestHeadTimeout bounds reading the whole request head, so a client sending
// it slowly cannot hold the connection. A per-line cap alone would not stop that.
var requestHeadTimeout = 10 * time.Second

// HandleConnection reads one request head within requestHeadTimeout, checks
// transfer coding, target, and expectation policy, buffers the body
// (readRequestBody), and prints the request if accepted. It attempts to close
// the connection before returning. Deadline, read, validation, policy, and
// close errors are logged without terminating the listener. Apart from 100
// (Continue), no response is sent, and nothing is forwarded.
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

	message, err := request.Read(reader)
	if err == nil {
		err = fields.CheckTransferCodingSupport(message.Fields.TransferCodings)
	}
	if err != nil {
		log.Printf("reading request: %v", err)
		return
	}

	if err := checkRequestTarget(message); err != nil {
		log.Printf("rejecting request: %v", err)
		return
	}

	if err := fields.CheckExpectationSupport(message.Fields.Expectations); err != nil {
		log.Printf("rejecting request: %v", err)
		return
	}

	content, err := readRequestBody(connection, message)
	if err != nil {
		log.Printf("reading request body: %v", err)
		return
	}

	fmt.Printf("Method: %s\nRequest target: %s\nVersion: %s\nAuthority: %s\n",
		message.Line.Method, message.Line.RequestTarget,
		message.Line.Version, message.Fields.Authority)
	fmt.Printf("Body: %d bytes\n", len(content))
}
