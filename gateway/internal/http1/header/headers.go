// Package header reads and parses an HTTP/1.1 field section. It checks generic
// field syntax and caller-supplied limits, not field-specific semantics.
package header

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/lineio"
)

var (
	// ErrLineTooLong reports a physical header line over Limits.LineBytes.
	ErrLineTooLong = errors.New("http1: header line exceeds its byte limit")
	// ErrSectionTooLarge reports a header section over Limits.SectionBytes.
	ErrSectionTooLarge = errors.New("http1: header section exceeds its byte limit")
	// ErrTooManyFields reports more than Limits.Fields field lines.
	ErrTooManyFields = errors.New("http1: header section exceeds its field limit")
)

// Limits bounds one header section. They are local implementation limits, not
// protocol-wide maxima. SectionBytes includes every header CRLF and the final
// empty line, but not the start-line. Continuations use the byte budgets, not
// the field count. Requests and responses may use different limits.
type Limits struct {
	LineBytes    int
	SectionBytes int
	Fields       int
}

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

// ReadHeadersSection reads the bounded header section, unfolds continuations, and
// validates each field's generic syntax with parseHeaderField. It lowercases names
// and appends values in order for each name without combining them. It does not
// validate field-specific semantics or message framing.
func ReadHeadersSection(reader *bufio.Reader, limits Limits) (Headers, error) {
	var headers Headers
	var pending strings.Builder
	sectionBytes := 0
	fieldCount := 0
	for {
		lineLimit := min(limits.LineBytes, limits.SectionBytes-sectionBytes-2)
		if lineLimit < 0 {
			return nil, ErrSectionTooLarge
		}
		rawLine, err := lineio.ReadCRLFLine(reader, lineLimit)
		if errors.Is(err, lineio.ErrLineTooLong) {
			if lineLimit < limits.LineBytes {
				return nil, ErrSectionTooLarge
			}
			return nil, ErrLineTooLong
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
			name, value, err := parseHeaderField(pending.String())
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
		if fieldCount == limits.Fields {
			return nil, ErrTooManyFields
		}
		pending.WriteString(strings.TrimRight(rawLine, " \t"))
	}
}

// parseHeaderField parses one field line without CRLF. It preserves name casing and
// value bytes, removing only surrounding SP/HTAB from the value. It checks
// generic syntax, not field-specific semantics, and does not enforce size limits.
// On error, both returned strings are empty.
func parseHeaderField(rawLine string) (name, value string, err error) {
	name, value, found := strings.Cut(rawLine, ":")
	if !found {
		return "", "", errors.New("http1: header field is missing a colon")
	}
	if !grammar.IsToken(name) {
		return "", "", errors.New("http1: invalid header field name")
	}
	if !grammar.IsFieldValue(value) {
		return "", "", errors.New("http1: invalid header field value")
	}
	return name, strings.Trim(value, " \t"), nil
}
