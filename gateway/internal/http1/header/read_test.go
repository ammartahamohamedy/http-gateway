package header

import (
	"bufio"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testLimits matches the request limits used by request.Read.
var testLimits = Limits{LineBytes: 8 * 1024, SectionBytes: 32 * 1024, Fields: 100}

func TestReadHeadersSectionLimits(t *testing.T) {
	fieldLine := func(size int) string { return "X: " + strings.Repeat("a", size-3) }
	prefix := strings.Repeat(fieldLine(testLimits.LineBytes)+"\r\n", 3)
	lastLineSize := testLimits.SectionBytes - len(prefix) - 4
	exactSection := prefix + fieldLine(lastLineSize) + "\r\n\r\n"
	require.Len(t, exactSection, testLimits.SectionBytes)
	tests := []struct {
		name    string
		section string
		count   int
		wantErr error
	}{
		{"line at limit", fieldLine(testLimits.LineBytes) + "\r\n\r\n", 1, nil},
		{"line over limit", fieldLine(testLimits.LineBytes+1) + "\r\n\r\n", 0, ErrLineTooLong},
		{"oversized unterminated line", fieldLine(testLimits.LineBytes + 1), 0, ErrLineTooLong},
		{"oversized partial terminator", fieldLine(testLimits.LineBytes+1) + "\r", 0, ErrLineTooLong},
		{"section at limit", exactSection, 4, nil},
		{"section over limit", prefix + fieldLine(lastLineSize+1) + "\r\n\r\n", 0, ErrSectionTooLarge},
		{"section budget while reading", prefix + fieldLine(lastLineSize+3), 0, ErrSectionTooLarge},
		{"count at limit", strings.Repeat("X: a\r\n", testLimits.Fields) + "\r\n", testLimits.Fields, nil},
		{"count over limit", strings.Repeat("X: a\r\n", testLimits.Fields+1) + "\r\n", 0, ErrTooManyFields},
		{"mixed case count over limit", strings.Repeat("X: a\r\nx: b\r\n", testLimits.Fields/2) + "X: c\r\n\r\n", 0, ErrTooManyFields},
		{"continuations are not fields", "X: a\r\n" + strings.Repeat(" b\r\n", testLimits.Fields+1) + "\r\n", 1, nil},
		{"large unfolded field", fieldLine(testLimits.LineBytes) + "\r\n " + strings.Repeat("b", testLimits.LineBytes-1) + "\r\n\r\n", 1, nil},
		{"oversized continuation", "X: a\r\n " + strings.Repeat("b", testLimits.LineBytes) + "\r\n\r\n", 0, ErrLineTooLong},
		{"continuations use section budget", "X: a\r\n" + strings.Repeat(" \t\r\n", testLimits.SectionBytes/4), 0, ErrSectionTooLarge},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			headers, err := ReadHeadersSection(bufio.NewReaderSize(strings.NewReader(test.section), 16), testLimits)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Nil(t, headers)
				return
			}
			require.NoError(t, err)
			count := 0
			for values := range maps.Values(headers) {
				count += len(values)
			}
			require.Equal(t, test.count, count)
		})
	}
}

func TestReadHeadersSectionRejectsMalformedSections(t *testing.T) {
	tests := []struct {
		name    string
		section string
		want    error
	}{
		{"continuation without a field", " folded\r\n\r\n", nil},
		{"malformed field line", "no colon\r\n\r\n", nil},
		{"EOF inside the section", "X: v\r\n", io.ErrUnexpectedEOF},
		{"EOF before any line", "", io.ErrUnexpectedEOF},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			headers, err := ReadHeadersSection(bufio.NewReader(strings.NewReader(test.section)), testLimits)
			require.Error(t, err)
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
			}
			require.Nil(t, headers)
		})
	}
}

func TestReadHeadersSectionPropagatesReadErrors(t *testing.T) {
	errSource := errors.New("connection reset")
	source := io.MultiReader(strings.NewReader("X: v\r\n"), &failingReader{err: errSource})
	headers, err := ReadHeadersSection(bufio.NewReader(source), testLimits)
	require.ErrorIs(t, err, errSource)
	require.Nil(t, headers)
}

// failingReader always fails with err.
type failingReader struct {
	err error
}

func (failing *failingReader) Read([]byte) (int, error) {
	return 0, failing.err
}
