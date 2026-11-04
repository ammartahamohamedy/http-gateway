package proxy

import (
	"bufio"
	"fmt"
	"log"
	"net"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
)

// HandleConnection reads one request, applies target policy, and prints its
// fields if accepted. It attempts to close the connection before returning.
// Read, validation, policy, and close errors are logged without terminating
// the listener. No response is sent and nothing is forwarded yet.
func HandleConnection(connection net.Conn) {
	defer func() {
		if err := connection.Close(); err != nil {
			log.Printf("closing connection: %v", err)
		}
	}()

	reader := bufio.NewReader(connection)
	request, err := http1.ReadRequest(reader)
	if err != nil {
		log.Printf("reading request: %v", err)
		return
	}
	if err := checkRequestTarget(request); err != nil {
		log.Printf("rejecting request: %v", err)
		return
	}
	fmt.Printf("Method: %s\nRequest target: %s\nVersion: %s\nAuthority: %s\n",
		request.RequestLine.Method, request.RequestLine.RequestTarget,
		request.RequestLine.Version, request.Fields.Authority)
}
