package body

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func TestUntilCloseReader(t *testing.T) {
	tests := []struct {
		name   string
		source io.Reader
		want   string
	}{
		{"empty", strings.NewReader(""), ""},
		{"whole input", strings.NewReader("hello world"), "hello world"},
		{"one byte per read", iotest.OneByteReader(strings.NewReader("hello world")), "hello world"},
		{"data with EOF", iotest.DataErrReader(strings.NewReader("hello world")), "hello world"},
		{"exactly the limit", strings.NewReader("hello"), "hello"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			content, err := io.ReadAll(newUntilCloseReader(test.source, 5+6))
			require.NoError(t, err)
			require.Equal(t, test.want, string(content))
		})
	}
}

func TestUntilCloseReaderLimit(t *testing.T) {
	content, err := io.ReadAll(newUntilCloseReader(strings.NewReader("hello world"), 5))
	require.ErrorIs(t, err, ErrBodyTooLarge)
	require.Equal(t, "hello", string(content))

	content, err = io.ReadAll(newUntilCloseReader(strings.NewReader("a"), -1))
	require.ErrorIs(t, err, ErrBodyTooLarge)
	require.Empty(t, content)
}

func TestUntilCloseReaderPassesSourceErrors(t *testing.T) {
	readErr := errors.New("connection reset")
	source := io.MultiReader(strings.NewReader("hel"), iotest.ErrReader(readErr))

	content, err := io.ReadAll(newUntilCloseReader(source, 64))
	require.ErrorIs(t, err, readErr)
	require.Equal(t, "hel", string(content))
}

func TestUntilCloseReaderClose(t *testing.T) {
	source := strings.NewReader("hello world")
	reader := newUntilCloseReader(source, 64)

	buffer := make([]byte, 5)
	_, err := reader.Read(buffer)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	_, err = reader.Read(buffer)
	require.ErrorIs(t, err, ErrBodyClosed)

	rest, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, " world", string(rest))
}
