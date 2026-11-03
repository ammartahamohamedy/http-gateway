package http1

import "bufio"

// ReadRequest skips leading empty CRLF lines, parses the request-line, and
// collects headers through the terminating empty line. Obsolete folding is
// replaced with one SP per continuation before field parsing. Field order,
// repeated names, and name casing are preserved. Body remains nil; body bytes
// stay in reader. Host and message framing semantics are not yet validated.
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
	request.Headers, err = processHeaders(reader)
	if err != nil {
		return nil, err
	}

	return &request, nil
}
