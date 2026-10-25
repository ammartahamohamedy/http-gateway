package http1

import (
	"bufio"
	"errors"
	"io"
	"regexp"
)

var (
	errLineTooLong       = errors.New("http1: line exceeds its content length limit")
	errInvalidLineEnding = errors.New("http1: line requires CRLF termination without bare CR or LF")

	// crlfLinePattern matches one complete CRLF-terminated line. It permits
	// empty content but rejects CR and LF within the content itself.
	crlfLinePattern = regexp.MustCompile(`\A[^\r\n]*\r\n\z`)
)

// readCRLFLine reads one line and removes its CRLF terminator. Empty lines are
// returned as empty strings; they are not skipped. Other content bytes are
// preserved without decoding, trimming, or interpreting them as HTTP fields.
//
// maxContentBytes excludes CRLF and must be nonnegative. Oversized lines return
// errLineTooLong before EOF or framing checks. Once LF is found, a missing
// preceding CR or any embedded CR returns errInvalidLineEnding. For input within
// the size limit, EOF before any bytes produces an error matching io.EOF; EOF
// within a line returns io.ErrUnexpectedEOF. Other read errors are propagated.
// A complete line can be returned before a pending read error is surfaced by a
// subsequent read.
//
// Errors return an empty string rather than partial content. The input is never
// closed. On success, subsequent data remains available through the same reader.
// On error, the reader may be positioned partway through a line.
func readCRLFLine(reader *bufio.Reader, maxContentBytes int) (string, error) {
	var rawLine []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		bufferedBytes := len(rawLine) + len(fragment)
		if bufferedBytes > 2 && bufferedBytes-2 > maxContentBytes {
			return "", errLineTooLong
		}
		rawLine = append(rawLine, fragment...)
		contentBytes := len(rawLine)
		if contentBytes > 0 && rawLine[contentBytes-1] == '\n' {
			contentBytes--
		}
		if contentBytes > 0 && rawLine[contentBytes-1] == '\r' {
			contentBytes--
		}
		if contentBytes > maxContentBytes {
			return "", errLineTooLong
		}

		if err == nil {
			if !crlfLinePattern.Match(rawLine) {
				return "", errInvalidLineEnding
			}
			return string(rawLine[:len(rawLine)-2]), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(rawLine) > 0 {
			return "", io.ErrUnexpectedEOF
		}
		return "", err
	}
}
