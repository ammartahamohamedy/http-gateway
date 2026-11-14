package http1

import (
	"bufio"
	"fmt"
	"io"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
)

// requestBody returns the body reader for the request's framing, reading from
// reader.
func requestBody(reader *bufio.Reader, line RequestLine, parsed RequestFields) (io.ReadCloser, error) {
	mode, err := body.Framing(body.Message{
		Version:              line.Version,
		ContentLength:        parsed.ContentLength,
		ContentLengthPresent: parsed.ContentLengthPresent,
		TransferCodings:      parsed.TransferCodings,
	})
	if err != nil {
		return nil, err
	}
	switch mode {
	case body.None:
		return body.NewFixedReader(reader, 0), nil
	case body.ContentLength:
		if parsed.ContentLength > maxRequestBodyBytes {
			return nil, ErrRequestBodyTooLarge
		}
		return body.NewFixedReader(reader, parsed.ContentLength), nil
	case body.Chunked:
		return nil, nil
	}
	return nil, fmt.Errorf("http1: unexpected request body framing %d", mode)
}
