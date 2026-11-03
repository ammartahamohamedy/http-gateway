package http1

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
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
	// ErrInvalidHost reports a missing, repeated, or invalid request Host field.
	ErrInvalidHost = fields.ErrInvalidHost

	// ErrHeaderLineTooLong reports a physical header line over 8192 content bytes.
	ErrHeaderLineTooLong = errors.New("http1: header line exceeds 8192 bytes")
	// ErrHeadersTooLarge reports a header section over 32768 bytes, including CRLFs.
	ErrHeadersTooLarge = errors.New("http1: header section exceeds 32768 bytes")
	// ErrTooManyHeaders reports more than 100 fields, including repeated names.
	ErrTooManyHeaders = errors.New("http1: header section exceeds 100 fields")
)

// headerNamePattern matches a nonempty ASCII token (RFC 9110, Section 5.6.2).
var headerNamePattern = regexp.MustCompile("\\A[A-Za-z0-9!#$%&'*+.^_`|~-]+\\z")

// Headers maps lowercase field names to separate field-line values in received
// order for each name. Keys must be lowercase; order across names and original
// name casing are not retained.
type Headers map[string][]string

// Values returns matching field-line values in order, comparing names without
// regard to case. It does not combine or split values. It returns nil if no
// fields match, including for nil or empty Headers. The returned slice is
// independent of h.
func (h Headers) Values(name string) []string {
	return append([]string(nil), h[strings.ToLower(name)]...)
}

// processHeaders reads the bounded header section, unfolds continuations, and
// validates each field's generic syntax with ParseHeaderField. It lowercases names
// and appends values in order for each name without combining them. It does not
// validate Host or message framing semantics.
func processHeaders(reader *bufio.Reader) (Headers, error) {
	var headers Headers
	var pending strings.Builder
	sectionBytes := 0
	fieldCount := 0
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
			name, value, err := ParseHeaderField(pending.String())
			if err != nil {
				return nil, err
			}
			if headers == nil {
				headers = make(Headers)
			}
			name = strings.ToLower(name)
			headers[name] = append(headers[name], value)
			fieldCount++
			pending.Reset()
		}
		if rawLine == "" {
			return headers, nil
		}
		if fieldCount == maxHeaderFields {
			return nil, ErrTooManyHeaders
		}
		pending.WriteString(strings.TrimRight(rawLine, " \t"))
	}
}

// ParseHeaderField parses one field line without CRLF. It preserves name casing
// and value bytes, removing only surrounding SP/HTAB from the value. It checks
// generic syntax, not field-specific semantics, and does not enforce size limits.
// On error, both returned strings are empty.
func ParseHeaderField(rawLine string) (name, value string, err error) {
	parts := strings.SplitN(rawLine, ":", 2)
	if len(parts) != 2 {
		return "", "", errors.New("http1: header field is missing a colon")
	}
	name, value = parts[0], parts[1]
	if !headerNamePattern.MatchString(name) {
		return "", "", errors.New("http1: invalid header field name")
	}
	for _, b := range []byte(value) {
		if (b < 0x20 && b != '\t') || b == 0x7f {
			return "", "", errors.New("http1: invalid header field value")
		}
	}
	return name, strings.Trim(value, " \t"), nil
}
