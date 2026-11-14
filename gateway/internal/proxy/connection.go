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

	// requestBodyTimeout bounds reading the whole request body, including any
	// 100 (Continue) response, so a client dribbling bytes cannot hold the
	// connection.
	requestBodyTimeout = 30 * time.Second
)

// continueResponse is the 100 (Continue) interim response (RFC 9110, Section
// 10.1.1).
const continueResponse = "HTTP/1.1 100 Continue\r\n\r\n"

// HandleConnection reads one request head within requestHeadTimeout, checks
// transfer coding, target, and expectation policy, buffers the body within
// requestBodyTimeout, and prints the request if accepted. It attempts to close
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
		// 413 for http1.ErrRequestBodyTooLarge; 501 for unsupported or unknown
		// transfer codings.
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
		log.Printf("reading request body: %v", err)
		return
	}

	fmt.Printf("Method: %s\nRequest target: %s\nVersion: %s\nAuthority: %s\n",
		request.RequestLine.Method, request.RequestLine.RequestTarget,
		request.RequestLine.Version, request.Fields.Authority)
	if request.Body == nil {
		fmt.Println("Body: chunked, not decoded")
	} else {
		fmt.Printf("Body: %d bytes\n", len(content))
	}
}

// readRequestBody buffers the whole request body within requestBodyTimeout,
// first writing continueResponse if the client expects 100-continue. A nil body
// reads and writes nothing.
func readRequestBody(connection net.Conn, request *http1.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	defer func() {
		if err := request.Body.Close(); err != nil {
			log.Printf("closing request body: %v", err)
		}
	}()
	if err := connection.SetDeadline(time.Now().Add(requestBodyTimeout)); err != nil {
		return nil, fmt.Errorf("setting body deadline: %w", err)
	}
	if slices.Contains(request.Fields.Expectations, "100-continue") {
		if _, err := io.WriteString(connection, continueResponse); err != nil {
			return nil, fmt.Errorf("writing 100 Continue: %w", err)
		}
	}
	return io.ReadAll(request.Body)
}
