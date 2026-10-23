package main

import (
	"io"
	"log"
	"strings"
)

func streamLines(file io.ReadCloser) <-chan string {
	linesChan := make(chan string)

	go func() {
		defer close(linesChan)
		defer func() {
			if err := file.Close(); err != nil {
				log.Fatal(err)
			}
		}()

		var pendingLine strings.Builder
		readBuffer := make([]byte, 8)

		for {
			bytesRead, err := file.Read(readBuffer)

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
				log.Fatal(err)
			}
		}

		if pendingLine.Len() > 0 {
			linesChan <- pendingLine.String()
		}
	}()

	return linesChan
}
