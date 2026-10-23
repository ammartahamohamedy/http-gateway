package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	file, err := os.Open("gopher-notes.txt")
	if err != nil {
		log.Fatal(err)
	}

	linesChan := streamLines(file)

	for line := range linesChan {
		fmt.Printf(">> %s\n", line)
	}
}
