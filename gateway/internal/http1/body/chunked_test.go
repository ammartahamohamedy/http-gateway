package body

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/lineio"
	"github.com/stretchr/testify/require"
)

const chunkedTestLimit = 1 << 20

func decodeChunked(t *testing.T, source io.Reader, limit int64) (string, string, error) {
	t.Helper()
	reader := bufio.NewReaderSize(source, 16)
	content, err := io.ReadAll(NewChunkedReader(reader, limit, nil))
	remaining, remainingErr := io.ReadAll(reader)
	require.NoError(t, remainingErr)
	return string(content), string(remaining), err
}

func TestChunkedReaderDecodes(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want string
	}{
		{"single chunk", "5\r\nhello\r\n0\r\n\r\n", "hello"},
		{"several chunks", "5\r\nhello\r\n1\r\n \r\n5\r\nworld\r\n0\r\n\r\n", "hello world"},
		{"empty body", "0\r\n\r\n", ""},
		{"binary data", "4\r\n\r\n\x00\xff\r\n0\r\n\r\n", "\r\n\x00\xff"},
		{"uppercase hex", "A\r\n0123456789\r\n0\r\n\r\n", "0123456789"},
		{"lowercase hex", "a\r\n0123456789\r\n0\r\n\r\n", "0123456789"},
		{"leading zeros", "0005\r\nhello\r\n000\r\n\r\n", "hello"},
		{"many leading zeros", strings.Repeat("0", 4000) + "5\r\nhello\r\n0\r\n\r\n", "hello"},
		{"extension name", "5;name\r\nhello\r\n0\r\n\r\n", "hello"},
		{"extension value", "5;name=value\r\nhello\r\n0\r\n\r\n", "hello"},
		{"extension BWS", "5 ;\tname = value\r\nhello\r\n0\r\n\r\n", "hello"},
		{"quoted extension", "5;name=\"a;b=\\\"c\"\r\nhello\r\n0\r\n\r\n", "hello"},
		{"several extensions", "5;a;b=c ; d\r\nhello\r\n0;last\r\n\r\n", "hello"},
		{"trailers without callback", "5\r\nhello\r\n0\r\nX-Trailer: value\r\nY: z\r\n\r\n", "hello"},
	}

	for test := range slices.Values(tests) {
		for fragmented := range slices.Values([]bool{false, true}) {
			name := test.name
			if fragmented {
				name += " one byte per read"
			}
			t.Run(name, func(t *testing.T) {
				var source io.Reader = strings.NewReader(test.wire + "NEXT")
				if fragmented {
					source = iotest.OneByteReader(source)
				}
				content, remaining, err := decodeChunked(t, source, chunkedTestLimit)
				require.NoError(t, err)
				require.Equal(t, test.want, content)
				require.Equal(t, "NEXT", remaining)
			})
		}
	}
}

func TestChunkedReaderSmallBuffer(t *testing.T) {
	reader := NewChunkedReader(bufio.NewReader(strings.NewReader("5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n")), chunkedTestLimit, nil)
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

	require.Equal(t, "hello world", string(content))
}

func TestChunkedReaderRejectsMalformedFraming(t *testing.T) {
	tests := []struct {
		name string
		wire string
	}{
		{"empty size line", "\r\nhello\r\n0\r\n\r\n"},
		{"non-hex size", "g\r\n"},
		{"hex prefix", "0x5\r\nhello\r\n0\r\n\r\n"},
		{"plus sign", "+5\r\nhello\r\n0\r\n\r\n"},
		{"minus sign", "-5\r\nhello\r\n0\r\n\r\n"},
		{"leading whitespace", " 5\r\nhello\r\n0\r\n\r\n"},
		{"trailing whitespace", "5 \r\nhello\r\n0\r\n\r\n"},
		{"whitespace after extension", "5;a \r\nhello\r\n0\r\n\r\n"},
		{"empty extension", "5;\r\nhello\r\n0\r\n\r\n"},
		{"empty extension between", "5;a;;b\r\nhello\r\n0\r\n\r\n"},
		{"extension without name", "5;=v\r\nhello\r\n0\r\n\r\n"},
		{"extension without value", "5;a=\r\nhello\r\n0\r\n\r\n"},
		{"extension with space", "5;a b\r\nhello\r\n0\r\n\r\n"},
		{"unterminated quoted extension", "5;a=\"b\r\nhello\r\n0\r\n\r\n"},
		{"size overflow", "8000000000000000\r\n"},
		{"many hex digits", strings.Repeat("f", 17) + "\r\n"},
		{"data without CRLF", "5\r\nhelloXX0\r\n\r\n"},
		{"data with LF only", "5\r\nhello\n0\r\n\r\n"},
		{"bare LF size line", "5\nhello\r\n0\r\n\r\n"},
		{"size line too long", "5;" + strings.Repeat("a", maxChunkLineBytes) + "\r\nhello\r\n0\r\n\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := decodeChunked(t, strings.NewReader(test.wire), chunkedTestLimit)
			require.ErrorIs(t, err, ErrInvalidFraming)
		})
	}
}

func TestChunkedReaderLineErrorsKeepCause(t *testing.T) {
	_, _, err := decodeChunked(t, strings.NewReader("5;"+strings.Repeat("a", maxChunkLineBytes)+"\r\n"), chunkedTestLimit)
	require.ErrorIs(t, err, lineio.ErrLineTooLong)
	_, _, err = decodeChunked(t, strings.NewReader("5\nhello"), chunkedTestLimit)
	require.ErrorIs(t, err, lineio.ErrInvalidLineEnding)
}

func TestChunkedReaderRejectsMalformedTrailers(t *testing.T) {
	_, _, err := decodeChunked(t, strings.NewReader("0\r\nno colon\r\n\r\n"), chunkedTestLimit)
	require.ErrorContains(t, err, "reading trailers")
}

func TestChunkedReaderTrailerLimit(t *testing.T) {
	trailers := "0\r\nX: " + strings.Repeat("a", trailerLimits.SectionBytes) + "\r\n\r\n"
	_, _, err := decodeChunked(t, strings.NewReader(trailers), chunkedTestLimit)
	require.Error(t, err)
}

func TestChunkedReaderPrematureEOF(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want string
	}{
		{"empty source", "", ""},
		{"inside size line", "5", ""},
		{"after size line", "5\r\n", ""},
		{"inside data", "5\r\nhel", "hel"},
		{"before data CRLF", "5\r\nhello", "hello"},
		{"inside data CRLF", "5\r\nhello\r", "hello"},
		{"before last chunk", "5\r\nhello\r\n", "hello"},
		{"before trailer end", "5\r\nhello\r\n0\r\n", "hello"},
		{"inside trailers", "0\r\nX: v\r\n", ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			content, _, err := decodeChunked(t, strings.NewReader(test.wire), chunkedTestLimit)
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.Equal(t, test.want, content)
		})
	}
}

func TestChunkedReaderLimit(t *testing.T) {
	content, _, err := decodeChunked(t, strings.NewReader("5\r\nhello\r\n5\r\nworld\r\n0\r\n\r\n"), 10)
	require.NoError(t, err)
	require.Equal(t, "helloworld", content)

	tests := []struct {
		name  string
		wire  string
		limit int64
		want  string
	}{
		{"single chunk over limit", "b\r\nhello world\r\n0\r\n\r\n", 10, ""},
		{"chunks over limit", "5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n", 10, "hello"},
		{"negative limit", "1\r\na\r\n0\r\n\r\n", -1, ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			content, _, err := decodeChunked(t, strings.NewReader(test.wire), test.limit)
			require.ErrorIs(t, err, ErrBodyTooLarge)
			require.Equal(t, test.want, content)
		})
	}
}

func TestChunkedReaderErrorsAreSticky(t *testing.T) {
	reader := NewChunkedReader(bufio.NewReader(strings.NewReader("g\r\n5\r\nhello\r\n0\r\n\r\n")), chunkedTestLimit, nil)
	for range 2 {
		count, err := reader.Read(make([]byte, 8))
		require.Zero(t, count)
		require.ErrorIs(t, err, ErrInvalidFraming)
	}
}

func TestChunkedReaderPropagatesSourceErrors(t *testing.T) {
	errSource := errors.New("connection reset")
	source := io.MultiReader(strings.NewReader("5\r\nhe"), iotest.ErrReader(errSource))
	content, err := io.ReadAll(NewChunkedReader(bufio.NewReader(source), chunkedTestLimit, nil))
	require.ErrorIs(t, err, errSource)
	require.Equal(t, "he", string(content))
}

func TestChunkedReaderClose(t *testing.T) {
	const wire = "5\r\nhello\r\n0\r\n\r\n"
	source := bufio.NewReader(strings.NewReader(wire))
	reader := NewChunkedReader(source, chunkedTestLimit, nil)
	require.NoError(t, reader.Close())
	count, err := reader.Read(make([]byte, 5))
	require.Zero(t, count)
	require.ErrorIs(t, err, ErrBodyClosed)
	remaining, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, wire, string(remaining), "Close must not drain the source")
}

func TestChunkedReaderPassesTrailers(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want header.Headers
	}{
		{"no trailers", "5\r\nhello\r\n0\r\n\r\n", nil},
		{"trailers", "5\r\nhello\r\n0\r\nX-Checksum: abc\r\nx-checksum: def\r\nServer-Timing: db\r\n\r\n",
			header.Headers{"x-checksum": {"abc", "def"}, "server-timing": {"db"}}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			var got header.Headers
			reader := NewChunkedReader(bufio.NewReader(strings.NewReader(test.wire)), chunkedTestLimit, func(trailers header.Headers) {
				calls++
				got = trailers
			})
			content, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "hello", string(content))
			require.Equal(t, 1, calls)
			require.Equal(t, test.want, got)
			_, err = reader.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.EOF)
			require.Equal(t, 1, calls, "trailers are passed once")
		})
	}
}

func TestChunkedReaderSkipsCallbackOnError(t *testing.T) {
	called := false
	reader := NewChunkedReader(bufio.NewReader(strings.NewReader("0\r\nno colon\r\n\r\n")), chunkedTestLimit, func(header.Headers) { called = true })
	_, err := io.ReadAll(reader)
	require.Error(t, err)
	require.False(t, called)
}
