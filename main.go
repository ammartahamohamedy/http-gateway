package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
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

		linesChan := streamLines(connection)

		for line := range linesChan {
			fmt.Println(strings.TrimSuffix(line, "\r"))
		}

		fmt.Println("Connection closed")
	}
}
