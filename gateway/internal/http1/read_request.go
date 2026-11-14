package http1

import (
	"bufio"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// ReadRequest reads one request from reader: the request-line, after skipping up
// to maxLeadingEmptyLines empty lines, then the header section and the supported
// fields, validated by processRequestFields. Field failures are returned as
// *FieldError. Gateway policy remains separate.
//
// Body reads lazily from reader, so bytes after the body stay there; Request
// describes its framing.
//
// Size limits are local: maxRequestLineBytes, requestHeaderLimits, and
// maxRequestBodyBytes. EOF before a request-line matches io.EOF; EOF before
// headers finish matches io.ErrUnexpectedEOF. All errors return a nil Request.
//
// The caller must reuse the same buffered reader for subsequent bytes. On error,
// the reader may be positioned partway through a line. The input is never closed.
func ReadRequest(reader *bufio.Reader) (*Request, error) {
	var request Request
	var err error

	// Request-line stage: read and parse one request-line.
	request.RequestLine, err = processRequestLine(reader)
	if err != nil {
		return nil, err
	}

	// Header stage: read, unfold, and parse the complete header section.
	request.Headers, err = header.ReadHeadersSection(reader, requestHeaderLimits)
	if err != nil {
		return nil, err
	}

	// Field stage: validate supported fields and collect their metadata.
	request.Fields, err = processRequestFields(request.RequestLine, request.Headers)
	if err != nil {
		return nil, err
	}

	// Body stage: decide framing and attach a body reader over the same reader.
	request.Body, err = requestBody(reader, request.RequestLine, request.Fields)
	if err != nil {
		return nil, err
	}

	return &request, nil
}
