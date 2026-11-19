package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/proxy"
)

func main() {
	var address = flag.String("addr", "127.0.0.1:8080", "TCP address to listen on (host:port)")
	var upstream = flag.String("upstream", "127.0.0.1:8081", "TCP address of the Rust backend (host:port)")

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

	config := proxy.Config{Upstream: *upstream}

	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		fmt.Println("Connection accepted")

		proxy.HandleConnection(connection, config)

		fmt.Println("Connection closed")
	}
}
