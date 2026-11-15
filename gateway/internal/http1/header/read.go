package header

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
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

var (
	// ErrLineTooLong reports a physical header line over Limits.LineBytes.
	ErrLineTooLong = errors.New("http1: header line exceeds its byte limit")
	// ErrSectionTooLarge reports a header section over Limits.SectionBytes.
	ErrSectionTooLarge = errors.New("http1: header section exceeds its byte limit")
	// ErrTooManyFields reports more than Limits.Fields field lines.
	ErrTooManyFields = errors.New("http1: header section exceeds its field limit")
)

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
		rawLine, err := readHeaderLine(reader, limits, sectionBytes)
		if err != nil {
			return nil, err
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
			if headers, err = addField(headers, pending.String()); err != nil {
				return nil, err
			}
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

// readHeaderLine reads one physical header line within limits.LineBytes and the
// section bytes left. An oversized line returns ErrLineTooLong, or
// ErrSectionTooLarge when the section budget is the tighter limit; EOF returns
// io.ErrUnexpectedEOF, since the section is unfinished.
func readHeaderLine(reader *bufio.Reader, limits Limits, sectionBytes int) (string, error) {
	lineLimit := min(limits.LineBytes, limits.SectionBytes-sectionBytes-2)
	if lineLimit < 0 {
		return "", ErrSectionTooLarge
	}

	rawLine, err := http1.ReadCRLFLine(reader, lineLimit)
	if errors.Is(err, http1.ErrLineTooLong) {
		if lineLimit < limits.LineBytes {
			return "", ErrSectionTooLarge
		}
		return "", ErrLineTooLong
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", fmt.Errorf("http1: reading headers: %w", err)
	}

	return rawLine, nil
}

// addField parses one unfolded field line with parseHeaderField and appends its
// value under the lowercase name, creating headers when nil.
func addField(headers Headers, line string) (Headers, error) {
	name, value, err := parseHeaderField(line)
	if err != nil {
		return nil, err
	}

	if headers == nil {
		headers = make(Headers)
	}

	name = strings.ToLower(name)
	headers[name] = append(headers[name], value)
	return headers, nil
}
