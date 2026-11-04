package http1

import (
	"bufio"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// ReadRequest skips up to 4 leading empty CRLF lines, parses the request-line, and
// collects headers through the terminating empty line. Obsolete folding is
// replaced with one SP per continuation before field parsing. Header names are
// stored lowercase, with separate values in order for each name. The supported
// request fields are then validated in a fixed order and their metadata stored
// in Fields; failures are returned as *FieldError. Exactly one Host field is
// required, with valid URI host/port syntax and an empty value only for
// authority-less absolute targets. Body remains nil; body bytes stay in reader.
// Gateway policy and message framing remain separate.
//
// The request-line and physical header lines allow 8192 content bytes each.
// Headers allow 32768 bytes including CRLFs, and at most 100 fields. EOF before
// a request-line matches io.EOF; EOF before headers finish matches
// io.ErrUnexpectedEOF. All errors return a nil Request.
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

	return &request, nil
}
