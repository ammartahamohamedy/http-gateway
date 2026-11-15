package body

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLimitedBody(t *testing.T) {
	tests := []struct {
		name    string
		limit   int64
		want    string
		wantErr error
	}{
		{"under the limit", 20, "hello world", nil},
		{"at the limit", 11, "hello world", nil},
		{"over the limit", 5, "hello", ErrBodyTooLarge},
		{"zero limit", 0, "", ErrBodyTooLarge},
		{"negative limit", -1, "", ErrBodyTooLarge},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader := newLimitedBody(strings.NewReader("hello world"), io.NopCloser(nil), test.limit)
			content, err := io.ReadAll(reader)
			if test.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.wantErr)
			}
			require.Equal(t, test.want, string(content))
		})
	}
}

func TestLimitedBodyErrorsAreSticky(t *testing.T) {
	errSource := errors.New("connection reset")
	reader := newLimitedBody(io.MultiReader(strings.NewReader("he"), &failingReader{err: errSource}), io.NopCloser(nil), 10)
	_, err := io.ReadAll(reader)
	require.ErrorIs(t, err, errSource)
	_, err = reader.Read(make([]byte, 1))
	require.ErrorIs(t, err, errSource)
}

// failingReader always fails with err.
type failingReader struct {
	err error
}

func (failing *failingReader) Read([]byte) (int, error) {
	return 0, failing.err
}

func TestLimitedBodyClose(t *testing.T) {
	closer := &closeRecorder{Reader: strings.NewReader("")}
	reader := newLimitedBody(strings.NewReader("hello"), closer, 10)
	require.NoError(t, reader.Close())
	require.True(t, closer.closed)
	_, err := reader.Read(make([]byte, 5))
	require.ErrorIs(t, err, ErrBodyClosed)
}
