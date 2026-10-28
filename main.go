package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/ammartahamohamedy/http-gateway/internal/http1"
)

func main() {
	var address = flag.String("addr", "127.0.0.1:8080", "TCP address to listen on (host:port)")

	flag.Parse()

	listener, err := net.Listen("tcp", *address)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			log.Printf("closing listener: %v", err)
		}
	}()

	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		fmt.Println("Connection accepted")

		handleConnection(connection)

		fmt.Println("Connection closed")
	}
}

// handleConnection reads one request-line and prints its fields if valid.
// It attempts to close the connection before returning. Read, validation, and
// close errors are logged without terminating the listener.
func handleConnection(connection net.Conn) {
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
	fmt.Printf("Method: %s\nRequest target: %s\nVersion: %s\n",
		request.RequestLine.Method, request.RequestLine.RequestTarget, request.RequestLine.Version)
}
