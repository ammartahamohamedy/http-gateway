package main

import (
	"io"
	"log"
	"strings"
)

func streamLines(source io.ReadCloser) <-chan string {
	linesChan := make(chan string)

	go func() {
		defer close(linesChan)
		defer func() {
			if err := source.Close(); err != nil {
				log.Printf("closing input: %v", err)
			}
		}()

		var pendingLine strings.Builder
		readBuffer := make([]byte, 8)

		for {
			bytesRead, err := source.Read(readBuffer)

			if bytesRead > 0 {
				chunk := string(readBuffer[:bytesRead])
				pendingLine.WriteString(chunk)
				bufferedText := pendingLine.String()
				lineParts := strings.Split(bufferedText, "\n")
				pendingLine.Reset()
				for i := range len(lineParts) - 1 {
					linesChan <- lineParts[i]
				}
				pendingLine.WriteString(lineParts[len(lineParts)-1])
			}

			if err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("reading input: %v", err)
				return
			}
		}

		if pendingLine.Len() > 0 {
			linesChan <- pendingLine.String()
		}
	}()

	return linesChan
}
