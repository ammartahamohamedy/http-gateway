package http1

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func TestReadCRLFLine(t *testing.T) {
	tests := []struct {
		name   string
		reader io.Reader
		want   string
	}{
		{"ordinary line", strings.NewReader("first\r\n"), "first"},
		{"empty line", strings.NewReader("\r\n"), ""},
		{"whitespace preserved", strings.NewReader(" value\t \r\n"), " value\t "},
		{"bytes preserved", strings.NewReader("\xff\x00\xc3\r\n"), "\xff\x00\xc3"},
		{"fragmented input", iotest.OneByteReader(strings.NewReader("value\r\n")), "value"},
		{"data with EOF", iotest.DataErrReader(strings.NewReader("value\r\n")), "value"},
		{"multiple buffer fragments", strings.NewReader(strings.Repeat("a", 64) + "\r\n"), strings.Repeat("a", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line, err := readCRLFLine(bufio.NewReaderSize(test.reader, 16), 64)
			require.NoError(t, err)
			require.Equal(t, test.want, line)
		})
	}
}

func TestReadCRLFLinePreservesMessageBoundaries(t *testing.T) {
	body := "body\r\nbytes\n\x00\xff"
	reader := bufio.NewReader(strings.NewReader("first\r\n\r\nlast\r\n" + body))

	line, err := readCRLFLine(reader, 5)
	require.NoError(t, err)
	require.Equal(t, "first", line)

	line, err = readCRLFLine(reader, 0)
	require.NoError(t, err)
	require.Empty(t, line, "an empty line must be returned, not skipped")

	line, err = readCRLFLine(reader, 4)
	require.NoError(t, err)
	require.Equal(t, "last", line)

	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, []byte(body), remaining)
}

func TestReadCRLFLineLimits(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		limit   int
		want    string
		wantErr error
	}{
		{"zero limit empty line", "\r\n", 0, "", nil},
		{"zero limit nonempty line", "a\r\n", 0, "", errLineTooLong},
		{"content exactly at limit", "abc\r\n", 3, "abc", nil},
		{"content exceeds limit", "abcd\r\n", 3, "", errLineTooLong},
		{"oversized without terminator", "abcd", 3, "", errLineTooLong},
		{"partial terminator at limit", "abc\r", 3, "", io.ErrUnexpectedEOF},
		{"oversized with partial terminator", "abcd\r", 3, "", errLineTooLong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line, err := readCRLFLine(bufio.NewReader(strings.NewReader(test.input)), test.limit)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, line)
		})
	}

	content := strings.Repeat("a", 32)
	reader := bufio.NewReaderSize(strings.NewReader(content+"\r\n"), len(content)+1)
	line, err := readCRLFLine(reader, len(content))
	require.NoError(t, err)
	require.Equal(t, content, line, "a split CRLF must not use the content budget")
}

func TestReadCRLFLineErrors(t *testing.T) {
	readErr := errors.New("input failed")
	tests := []struct {
		name   string
		reader io.Reader
		want   error
	}{
		{"no input", strings.NewReader(""), io.EOF},
		{"unterminated content", strings.NewReader("value"), io.ErrUnexpectedEOF},
		{"CR only", strings.NewReader("\r"), io.ErrUnexpectedEOF},
		{"partial CRLF", strings.NewReader("value\r"), io.ErrUnexpectedEOF},
		{"bare LF", strings.NewReader("value\n"), errInvalidLineEnding},
		{"empty bare LF", strings.NewReader("\n"), errInvalidLineEnding},
		{"embedded CR", strings.NewReader("bad\rvalue\r\n"), errInvalidLineEnding},
		{"extra CR", strings.NewReader("value\r\r\n"), errInvalidLineEnding},
		{"read failure", iotest.ErrReader(readErr), readErr},
		{"partial data and read failure", iotest.DataErrReader(io.MultiReader(strings.NewReader("value"), iotest.ErrReader(readErr))), readErr},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readCRLFLine(bufio.NewReader(test.reader), 64)
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestReadCRLFLinePreservesErrorAfterCompleteLine(t *testing.T) {
	readErr := errors.New("input failed")
	source := iotest.DataErrReader(io.MultiReader(strings.NewReader("ok\r\n"), iotest.ErrReader(readErr)))
	reader := bufio.NewReader(source)

	line, err := readCRLFLine(reader, 2)
	require.NoError(t, err)
	require.Equal(t, "ok", line)
	_, err = readCRLFLine(reader, 2)
	require.ErrorIs(t, err, readErr)
}
