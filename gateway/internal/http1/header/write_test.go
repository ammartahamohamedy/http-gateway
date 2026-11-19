package header

import (
	"bufio"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name    string
		section Section
		want    string
	}{
		{"nil section", nil, "\r\n"},
		{"empty section", Section{}, "\r\n"},
		{"one field", Section{"host": {"example.com"}}, "host: example.com\r\n\r\n"},
		{"names sorted", Section{"vary": {"Origin"}, "content-type": {"text/plain"}, "date": {"Sun, 06 Nov 1994 08:49:37 GMT"}},
			"content-type: text/plain\r\ndate: Sun, 06 Nov 1994 08:49:37 GMT\r\nvary: Origin\r\n\r\n"},
		{"repeated values keep order on separate lines", Section{"set-cookie": {
			"session=a; Expires=Wed, 21 Oct 2026 07:28:00 GMT", "theme=dark, light"}},
			"set-cookie: session=a; Expires=Wed, 21 Oct 2026 07:28:00 GMT\r\nset-cookie: theme=dark, light\r\n\r\n"},
		{"empty value", Section{"x-empty": {""}}, "x-empty:\r\n\r\n"},
		{"inner HTAB and SP", Section{"x-inner": {"a\tb c"}}, "x-inner: a\tb c\r\n\r\n"},
		{"obs-text", Section{"x-obs": {"caf\xe9"}}, "x-obs: caf\xe9\r\n\r\n"},
		{"name without values", Section{"x-none": nil}, "\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			require.NoError(t, Write(&output, test.section))
			require.Equal(t, test.want, output.String())
		})
	}
}

func TestWriteRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		section Section
	}{
		{"empty name", Section{"": {"a"}}},
		{"name with space", Section{"x name": {"a"}}},
		{"name with colon", Section{"x:name": {"a"}}},
		{"value with CRLF", Section{"x-split": {"a\r\nset-cookie: evil=1"}}},
		{"value with bare LF", Section{"x-split": {"a\nb"}}},
		{"value with bare CR", Section{"x-split": {"a\rb"}}},
		{"value with NUL", Section{"x-nul": {"a\x00b"}}},
		{"value with DEL", Section{"x-del": {"a\x7fb"}}},
		{"leading SP", Section{"x-space": {" a"}}},
		{"trailing HTAB", Section{"x-tab": {"a\t"}}},
		{"invalid value after valid fields", Section{"a": {"ok"}, "b": {"ok", "bad\r\n"}}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			require.ErrorIs(t, Write(&output, test.section), ErrInvalidField)
			require.Empty(t, output.String())
		})
	}
}

// failingWriter returns err from every Write.
type failingWriter struct {
	err error
}

func (failing failingWriter) Write([]byte) (int, error) { return 0, failing.err }

func TestWritePassesWriterErrors(t *testing.T) {
	writeErr := errors.New("connection reset")
	require.ErrorIs(t, Write(failingWriter{err: writeErr}, Section{"host": {"example.com"}}), writeErr)
}

// countingWriter counts Write calls.
type countingWriter struct {
	strings.Builder
	writes int
}

func (counting *countingWriter) Write(data []byte) (int, error) {
	counting.writes++
	return counting.Builder.Write(data)
}

func TestWriteUsesOneWriteCall(t *testing.T) {
	var output countingWriter
	require.NoError(t, Write(&output, Section{"a": {"1", "2"}, "b": {"3"}}))
	require.Equal(t, 1, output.writes)
}

func TestWriteRoundTripsThroughRead(t *testing.T) {
	section := Section{
		"content-type": {"application/json"},
		"set-cookie":   {"a=1; Expires=Wed, 21 Oct 2026 07:28:00 GMT", "b=2"},
		"x-empty":      {""},
	}

	var output strings.Builder
	require.NoError(t, Write(&output, section))

	parsed, err := Read(bufio.NewReader(strings.NewReader(output.String())), Limits{LineBytes: 1024, SectionBytes: 4096, Fields: 10})
	require.NoError(t, err)
	require.Equal(t, section, parsed)
}

func TestWriteKeepsInput(t *testing.T) {
	section := Section{"b": {"2", "1"}, "a": {"x"}}

	var output strings.Builder
	require.NoError(t, Write(&output, section))
	require.Equal(t, Section{"b": {"2", "1"}, "a": {"x"}}, section)
}
