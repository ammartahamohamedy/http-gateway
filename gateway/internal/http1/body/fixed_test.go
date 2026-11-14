package body

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func TestFixedReaderReadsExactLength(t *testing.T) {
	tests := []struct {
		name   string
		source func(string) io.Reader
	}{
		{"buffered", func(s string) io.Reader { return strings.NewReader(s) }},
		{"one byte per read", func(s string) io.Reader { return iotest.OneByteReader(strings.NewReader(s)) }},
		{"data with EOF", func(s string) io.Reader { return iotest.DataErrReader(strings.NewReader(s)) }},
		{"half reads", func(s string) io.Reader { return iotest.HalfReader(strings.NewReader(s)) }},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader := bufio.NewReaderSize(test.source("hello\x00\r\nNEXT"), 16)
			content, err := io.ReadAll(NewFixedReader(reader, 8))
			require.NoError(t, err)
			require.Equal(t, "hello\x00\r\n", string(content))
			remaining, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "NEXT", string(remaining))
		})
	}
}

func TestFixedReaderReturnsEOFWithFinalBytes(t *testing.T) {
	reader := NewFixedReader(strings.NewReader("helloNEXT"), 5)
	buffer := make([]byte, 16)
	count, err := reader.Read(buffer)
	require.Equal(t, 5, count)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, "hello", string(buffer[:count]))
	count, err = reader.Read(buffer)
	require.Zero(t, count)
	require.ErrorIs(t, err, io.EOF)
}

func TestFixedReaderSmallBuffer(t *testing.T) {
	reader := NewFixedReader(strings.NewReader("hello"), 5)
	buffer := make([]byte, 2)
	var content []byte
	for {
		count, err := reader.Read(buffer)
		content = append(content, buffer[:count]...)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
	}
	require.Equal(t, "hello", string(content))
}

func TestFixedReaderEmptyBody(t *testing.T) {
	for length := range slices.Values([]int64{0, -1}) {
		source := strings.NewReader("NEXT")
		count, err := NewFixedReader(source, length).Read(make([]byte, 4))
		require.Zero(t, count)
		require.ErrorIs(t, err, io.EOF)
		require.Equal(t, 4, source.Len(), "the source must not be read")
	}
}

func TestFixedReaderPrematureEOF(t *testing.T) {
	tests := []struct {
		name   string
		source io.Reader
		want   string
	}{
		{"empty source", strings.NewReader(""), ""},
		{"short source", strings.NewReader("hel"), "hel"},
		{"short data with EOF", iotest.DataErrReader(strings.NewReader("hel")), "hel"},
		{"short one byte reads", iotest.OneByteReader(strings.NewReader("hel")), "hel"},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			content, err := io.ReadAll(NewFixedReader(test.source, 5))
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.Equal(t, test.want, string(content))
		})
	}
}

func TestFixedReaderPropagatesSourceErrors(t *testing.T) {
	errSource := errors.New("connection reset")
	source := io.MultiReader(strings.NewReader("he"), iotest.ErrReader(errSource))
	content, err := io.ReadAll(NewFixedReader(source, 5))
	require.ErrorIs(t, err, errSource)
	require.Equal(t, "he", string(content))
}

func TestFixedReaderStopsReadingAtLength(t *testing.T) {
	// TimeoutReader fails its second read, which a complete body must not make.
	content, err := io.ReadAll(NewFixedReader(iotest.TimeoutReader(strings.NewReader("hello")), 5))
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
}

func TestFixedReaderClose(t *testing.T) {
	source := strings.NewReader("hello")
	reader := NewFixedReader(source, 5)
	require.NoError(t, reader.Close())
	require.NoError(t, reader.Close())
	count, err := reader.Read(make([]byte, 5))
	require.Zero(t, count)
	require.ErrorIs(t, err, ErrBodyClosed)
	require.Equal(t, 5, source.Len(), "Close must not drain the source")
}
