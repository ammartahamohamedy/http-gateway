package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	file, err := os.Open("gopher-notes.txt")
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	readBuffer := make([]byte, 8)
	var pendingLine strings.Builder

	for {
		bytesRead, err := file.Read(readBuffer)

		if bytesRead > 0 {
			chunk := string(readBuffer[:bytesRead])
			pendingLine.WriteString(chunk)
			bufferedText := pendingLine.String()
			lineParts := strings.Split(bufferedText, "\n")
			pendingLine.Reset()
			for i := range len(lineParts) - 1 {
				fmt.Printf(">> %s\n", lineParts[i])
			}
			pendingLine.WriteString(lineParts[len(lineParts)-1])
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
	}

	if pendingLine.Len() > 0 {
		fmt.Printf(">> %s\n", pendingLine.String())
	}
}
