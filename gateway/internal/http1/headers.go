package http1

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// These are local implementation limits, not protocol-wide maxima. The section
// budget includes every header CRLF and the final empty line, but not the
// request-line. Continuations use the byte budgets, not the field count.
const (
	maxHeaderLineBytes    = 8 * 1024
	maxHeaderSectionBytes = 32 * 1024
	maxHeaderFields       = 100
)

var (
	// ErrHeaderLineTooLong reports a physical header line over 8192 content bytes.
	ErrHeaderLineTooLong = errors.New("http1: header line exceeds 8192 bytes")
	// ErrHeadersTooLarge reports a header section over 32768 bytes, including CRLFs.
	ErrHeadersTooLarge = errors.New("http1: header section exceeds 32768 bytes")
	// ErrTooManyHeaders reports more than 100 fields, including repeated names.
	ErrTooManyHeaders = errors.New("http1: header section exceeds 100 fields")
)

// headerNamePattern matches a nonempty ASCII token (RFC 9110, Section 5.6.2).
var headerNamePattern = regexp.MustCompile("\\A[A-Za-z0-9!#$%&'*+.^_`|~-]+\\z")

// HeaderField stores one header field's name and value without validation or
// normalization. Headers slices can retain repeated fields as separate entries
// in their original order; this type does not combine values.
type HeaderField struct {
	Name  string
	Value string
}

// processHeaders reads the bounded header section, unfolds continuations, and
// validates each field's generic syntax with ParseHeaderField. It preserves order
// and repeated names; it does not validate Host or message framing semantics.
func processHeaders(reader *bufio.Reader) ([]HeaderField, error) {
	var headers []HeaderField
	var pending strings.Builder
	sectionBytes := 0
	for {
		lineLimit := min(maxHeaderLineBytes, maxHeaderSectionBytes-sectionBytes-2)
		if lineLimit < 0 {
			return nil, ErrHeadersTooLarge
		}
		rawLine, err := readCRLFLine(reader, lineLimit)
		if errors.Is(err, errLineTooLong) {
			if lineLimit < maxHeaderLineBytes {
				return nil, ErrHeadersTooLarge
			}
			return nil, ErrHeaderLineTooLong
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, fmt.Errorf("http1: reading headers: %w", err)
		}
		sectionBytes += len(rawLine) + 2
		if rawLine != "" && (rawLine[0] == ' ' || rawLine[0] == '\t') {
			if pending.Len() == 0 {
				return nil, errors.New("http1: header continuation without a preceding field")
			}
			pending.WriteByte(' ')
			pending.WriteString(strings.Trim(rawLine, " \t"))
			continue
		}
		if pending.Len() > 0 {
			field, err := ParseHeaderField(pending.String())
			if err != nil {
				return nil, err
			}
			headers = append(headers, field)
			pending.Reset()
		}
		if rawLine == "" {
			return headers, nil
		}
		if len(headers) == maxHeaderFields {
			return nil, ErrTooManyHeaders
		}
		pending.WriteString(strings.TrimRight(rawLine, " \t"))
	}
}

// ParseHeaderField parses one field line without CRLF. It preserves name casing
// and value bytes, removing only surrounding SP/HTAB from the value. It checks
// generic syntax, not field-specific semantics, and does not enforce size limits.
func ParseHeaderField(rawLine string) (HeaderField, error) {
	parts := strings.SplitN(rawLine, ":", 2)
	if len(parts) != 2 {
		return HeaderField{}, errors.New("http1: header field is missing a colon")
	}
	name, value := parts[0], parts[1]
	if !headerNamePattern.MatchString(name) {
		return HeaderField{}, errors.New("http1: invalid header field name")
	}
	for _, b := range []byte(value) {
		if (b < 0x20 && b != '\t') || b == 0x7f {
			return HeaderField{}, errors.New("http1: invalid header field value")
		}
	}
	return HeaderField{Name: name, Value: strings.Trim(value, " \t")}, nil
}
