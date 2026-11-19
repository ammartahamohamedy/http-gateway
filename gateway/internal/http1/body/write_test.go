package body

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestWriteNoneDoesNotReadSource(t *testing.T) {
	source := &countingReader{source: strings.NewReader("ignored")}

	var output bytes.Buffer
	require.NoError(t, Write(&output, source, None, 0, nil))
	require.Empty(t, output.String())
	require.Zero(t, source.reads)
}

func TestWriteContentLength(t *testing.T) {
	tests := []struct {
		name   string
		source io.Reader
		length int64
		want   string
	}{
		{"exact length", strings.NewReader("hello"), 5, "hello"},
		{"one byte per read", iotest.OneByteReader(strings.NewReader("hello")), 5, "hello"},
		{"data with EOF", iotest.DataErrReader(strings.NewReader("hello")), 5, "hello"},
		{"zero length", strings.NewReader(""), 0, ""},
		{"negative length treated as zero", strings.NewReader(""), -1, ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, Write(&output, test.source, ContentLength, test.length, nil))
			require.Equal(t, test.want, output.String())
		})
	}
}

func TestWriteContentLengthRejectsMismatchedSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		length  int64
		written string
	}{
		{"shorter source", "hel", 5, "hel"},
		{"empty source", "", 5, ""},
		{"longer source", "hello!", 5, "hello"},
		{"data for zero length", "a", 0, ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.ErrorIs(t, Write(&output, strings.NewReader(test.source), ContentLength, test.length, nil), ErrBodyLength)
			require.Equal(t, test.written, output.String())
		})
	}
}

func TestWriteContentLengthPassesSourceErrors(t *testing.T) {
	readErr := errors.New("upstream reset")

	tests := []struct {
		name   string
		source io.Reader
	}{
		{"within the length", io.MultiReader(strings.NewReader("hel"), iotest.ErrReader(readErr))},
		{"after the length", io.MultiReader(strings.NewReader("hello"), iotest.ErrReader(readErr))},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := Write(&output, test.source, ContentLength, 5, nil)
			require.ErrorIs(t, err, readErr)
			require.NotErrorIs(t, err, ErrBodyLength)
		})
	}
}

func TestWriteChunked(t *testing.T) {
	tests := []struct {
		name   string
		source io.Reader
		want   string
	}{
		{"empty source", strings.NewReader(""), "0\r\n\r\n"},
		{"one read", strings.NewReader("hello world"), "b\r\nhello world\r\n0\r\n\r\n"},
		{"one chunk per read", iotest.OneByteReader(strings.NewReader("abc")), "1\r\na\r\n1\r\nb\r\n1\r\nc\r\n0\r\n\r\n"},
		{"data with EOF", iotest.DataErrReader(strings.NewReader("abc")), "3\r\nabc\r\n0\r\n\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, Write(&output, test.source, Chunked, 0, nil))
			require.Equal(t, test.want, output.String())
		})
	}
}

func TestWriteChunkedSplitsLargeReads(t *testing.T) {
	data := bytes.Repeat([]byte("a"), chunkBufferBytes+10)

	var output bytes.Buffer
	require.NoError(t, Write(&output, bytes.NewReader(data), Chunked, 0, nil))
	require.True(t, strings.HasPrefix(output.String(), "8000\r\n"))
	require.Contains(t, output.String(), "\r\na\r\n"+string(data[:10])+"\r\n0\r\n\r\n")
}

func TestWriteChunkedTrailers(t *testing.T) {
	sourceEnded := false
	source := iotest.OneByteReader(&eofRecorder{reader: strings.NewReader("hi"), ended: &sourceEnded})

	var output bytes.Buffer
	err := Write(&output, source, Chunked, 0, func() header.Section {
		require.True(t, sourceEnded, "trailers must be requested after the source ends")
		return header.Section{"server-timing": {"db;dur=5"}, "x-checksum": {"abc"}}
	})
	require.NoError(t, err)
	require.Equal(t, "1\r\nh\r\n1\r\ni\r\n0\r\nserver-timing: db;dur=5\r\nx-checksum: abc\r\n\r\n", output.String())
}

func TestWriteChunkedWithoutTrailers(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, Write(&output, strings.NewReader("hi"), Chunked, 0, func() header.Section { return nil }))
	require.Equal(t, "2\r\nhi\r\n0\r\n\r\n", output.String())
}

func TestWriteChunkedNeverCompletesAFailedBody(t *testing.T) {
	readErr := errors.New("upstream reset")

	var output bytes.Buffer
	err := Write(&output, io.MultiReader(strings.NewReader("hel"), iotest.ErrReader(readErr)), Chunked, 0, nil)
	require.ErrorIs(t, err, readErr)
	require.Equal(t, "3\r\nhel\r\n", output.String())

	output.Reset()
	err = Write(&output, strings.NewReader("hi"), Chunked, 0, func() header.Section {
		return header.Section{"x-split": {"a\r\nb"}}
	})
	require.ErrorIs(t, err, header.ErrInvalidField)
	require.Equal(t, "2\r\nhi\r\n", output.String())
}

func TestWritePassesWriterErrors(t *testing.T) {
	writeErr := errors.New("client reset")

	tests := []struct {
		name   string
		mode   Mode
		source string
	}{
		{"Content-Length", ContentLength, "hello"},
		{"chunk", Chunked, "hello"},
		{"last chunk", Chunked, ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			err := Write(failingWriter{err: writeErr}, strings.NewReader(test.source), test.mode, 5, nil)
			require.ErrorIs(t, err, writeErr)
		})
	}
}

func TestWriteRejectsUnsupportedModes(t *testing.T) {
	for mode := range slices.Values([]Mode{Unknown, UntilClose}) {
		var output bytes.Buffer
		require.ErrorIs(t, Write(&output, strings.NewReader("hello"), mode, 5, nil), ErrUnsupportedFraming)
		require.Empty(t, output.String())
	}
}

func TestWriteRoundTripsThroughNewReader(t *testing.T) {
	data := strings.Repeat("hello world ", chunkBufferBytes/8)

	var fixed bytes.Buffer
	require.NoError(t, Write(&fixed, strings.NewReader(data), ContentLength, int64(len(data)), nil))

	reader, err := NewReader(bufio.NewReader(&fixed), Message{Version: "1.1", ContentLength: int64(len(data)), ContentLengthPresent: true}, readerTestLimit, nil)
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, data, string(content))

	var chunked bytes.Buffer
	require.NoError(t, Write(&chunked, strings.NewReader(data), Chunked, 0, func() header.Section {
		return header.Section{"x-checksum": {"abc"}}
	}))

	var trailers header.Section
	reader, err = NewReader(bufio.NewReader(&chunked), Message{Version: "1.1", TransferCodings: []string{"chunked"}},
		readerTestLimit, func(section header.Section) { trailers = section })
	require.NoError(t, err)
	content, err = io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, data, string(content))
	require.Equal(t, header.Section{"x-checksum": {"abc"}}, trailers)
}

// eofRecorder sets ended once reader returns io.EOF.
type eofRecorder struct {
	reader io.Reader
	ended  *bool
}

func (recorder *eofRecorder) Read(buffer []byte) (int, error) {
	count, err := recorder.reader.Read(buffer)
	if errors.Is(err, io.EOF) {
		*recorder.ended = true
	}

	return count, err
}

// failingWriter returns err from every Write.
type failingWriter struct {
	err error
}

func (failing failingWriter) Write([]byte) (int, error) { return 0, failing.err }
