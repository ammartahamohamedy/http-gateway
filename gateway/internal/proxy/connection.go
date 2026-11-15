package proxy

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
)

// requestHeadTimeout bounds reading the whole request head, so a client sending
// it slowly cannot hold the connection. A per-line cap alone would not stop that.
var requestHeadTimeout = 10 * time.Second

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
		// codings; otherwise choose by the http1.StageError stage.
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
