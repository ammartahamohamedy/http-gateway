package body

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

const decodeTestLimit = 1 << 20

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	_, err := writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encoded.Bytes()
}

// framedBytes returns data as a framed body that never reads past its end.
func framedBytes(data []byte) io.ReadCloser {
	return NewFixedReader(bytes.NewReader(data), int64(len(data)))
}

func decodeAll(encoded []byte, codings []string, limit int64) (string, error) {
	content, err := io.ReadAll(Decode(framedBytes(encoded), codings, limit))
	return string(content), err
}

func TestDecodeWithoutCodingsReturnsFramed(t *testing.T) {
	framed := framedBytes([]byte("hello"))
	require.Same(t, framed, Decode(framed, nil, decodeTestLimit))
}

func TestDecodeGzip(t *testing.T) {
	encoded := gzipBytes(t, []byte("hello world"))
	tests := []struct {
		name   string
		source func() io.Reader
	}{
		{"buffered", func() io.Reader { return bytes.NewReader(encoded) }},
		{"one byte per read", func() io.Reader { return iotest.OneByteReader(bytes.NewReader(encoded)) }},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			framed := NewFixedReader(test.source(), int64(len(encoded)))
			content, err := io.ReadAll(Decode(framed, []string{"gzip"}, decodeTestLimit))
			require.NoError(t, err)
			require.Equal(t, "hello world", string(content))
		})
	}
}

func TestDecodeStackedGzipInReverseOrder(t *testing.T) {
	twice := gzipBytes(t, gzipBytes(t, []byte("hello")))
	content, err := decodeAll(twice, []string{"gzip", "gzip"}, decodeTestLimit)
	require.NoError(t, err)
	require.Equal(t, "hello", content)
}

func TestDecodeGzipMembers(t *testing.T) {
	members := append(gzipBytes(t, []byte("hello ")), gzipBytes(t, []byte("world"))...)
	content, err := decodeAll(members, []string{"gzip"}, decodeTestLimit)
	require.NoError(t, err)
	require.Equal(t, "hello world", content)
}

func TestDecodeRejectsMalformedGzip(t *testing.T) {
	encoded := gzipBytes(t, []byte("hello world"))
	badChecksum := slices.Clone(encoded)
	badChecksum[len(badChecksum)-5] ^= 0xff
	badData := slices.Clone(encoded)
	badData[10] = 0xff
	tests := []struct {
		name    string
		encoded []byte
	}{
		{"empty stream", nil},
		{"not gzip", []byte("hello world")},
		{"checksum mismatch", badChecksum},
		{"corrupt deflate data", badData},
		{"data after the stream", append(slices.Clone(encoded), "junk"...)},
		{"zero byte after the stream", append(slices.Clone(encoded), 0)},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeAll(test.encoded, []string{"gzip"}, decodeTestLimit)
			require.ErrorIs(t, err, ErrInvalidCoding)
		})
	}
}

func TestDecodeTruncatedGzip(t *testing.T) {
	encoded := gzipBytes(t, []byte(strings.Repeat("hello world ", 100)))
	for cut := range slices.Values([]int{5, len(encoded) / 2, len(encoded) - 1}) {
		// A complete body holding a cut stream is malformed.
		_, err := decodeAll(encoded[:cut], []string{"gzip"}, decodeTestLimit)
		require.ErrorIs(t, err, ErrInvalidCoding, "cut at %d", cut)
		require.NotErrorIs(t, err, io.ErrUnexpectedEOF, "cut at %d", cut)

		// A body cut by the connection is incomplete.
		framed := NewFixedReader(bytes.NewReader(encoded[:cut]), int64(len(encoded)))
		_, err = io.ReadAll(Decode(framed, []string{"gzip"}, decodeTestLimit))
		require.ErrorIs(t, err, io.ErrUnexpectedEOF, "cut at %d", cut)
		require.NotErrorIs(t, err, ErrInvalidCoding, "cut at %d", cut)
	}
}

func TestDecodeLimitsDecodedData(t *testing.T) {
	bomb := gzipBytes(t, make([]byte, 4*decodeTestLimit))
	require.Less(t, len(bomb), decodeTestLimit/100, "the encoded body is small")
	content, err := decodeAll(bomb, []string{"gzip"}, decodeTestLimit)
	require.ErrorIs(t, err, ErrBodyTooLarge)
	require.Len(t, content, decodeTestLimit, "only data within the limit is returned")

	exact := gzipBytes(t, make([]byte, decodeTestLimit))
	content, err = decodeAll(exact, []string{"gzip"}, decodeTestLimit)
	require.NoError(t, err)
	require.Len(t, content, decodeTestLimit)

	_, err = decodeAll(gzipBytes(t, []byte("a")), []string{"gzip"}, -1)
	require.ErrorIs(t, err, ErrBodyTooLarge)
}

func TestDecodeUnsupportedCoding(t *testing.T) {
	_, err := decodeAll([]byte("data"), []string{"compress"}, decodeTestLimit)
	require.ErrorIs(t, err, ErrUnsupportedCoding)
}

// countingReader counts reads from its source.
type countingReader struct {
	source io.Reader
	reads  int
}

func (counting *countingReader) Read(buffer []byte) (int, error) {
	counting.reads++
	return counting.source.Read(buffer)
}

func TestDecodeIsLazy(t *testing.T) {
	encoded := gzipBytes(t, []byte("hello"))
	source := &countingReader{source: bytes.NewReader(encoded)}
	decoded := Decode(NewFixedReader(source, int64(len(encoded))), []string{"gzip"}, decodeTestLimit)
	require.Zero(t, source.reads, "Decode must not read")
	content, err := io.ReadAll(decoded)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
	require.Positive(t, source.reads)
}

func TestDecodePassesSourceErrors(t *testing.T) {
	errSource := errors.New("connection reset")
	encoded := gzipBytes(t, []byte(strings.Repeat("hello world ", 100)))
	source := io.MultiReader(bytes.NewReader(encoded[:20]), iotest.ErrReader(errSource))
	_, err := io.ReadAll(Decode(NewFixedReader(source, int64(len(encoded))), []string{"gzip"}, decodeTestLimit))
	require.ErrorIs(t, err, errSource)
	require.NotErrorIs(t, err, ErrInvalidCoding)
}

func TestDecodeErrorsAreSticky(t *testing.T) {
	decoded := Decode(framedBytes([]byte("not gzip")), []string{"gzip"}, decodeTestLimit)
	for range 2 {
		_, err := decoded.Read(make([]byte, 8))
		require.ErrorIs(t, err, ErrInvalidCoding)
	}
}

// closeRecorder records whether Close was called.
type closeRecorder struct {
	io.Reader
	closed bool
}

func (recorder *closeRecorder) Close() error {
	recorder.closed = true
	return nil
}

func TestDecodeClose(t *testing.T) {
	framed := &closeRecorder{Reader: bytes.NewReader(gzipBytes(t, []byte("hello")))}
	decoded := Decode(framed, []string{"gzip"}, decodeTestLimit)
	require.NoError(t, decoded.Close())
	require.True(t, framed.closed)
	_, err := decoded.Read(make([]byte, 5))
	require.ErrorIs(t, err, ErrBodyClosed)
}

func zlibBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := zlib.NewWriter(&encoded)
	_, err := writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encoded.Bytes()
}

func TestDecodeDeflate(t *testing.T) {
	encoded := zlibBytes(t, []byte("hello world"))
	for fragmented := range slices.Values([]bool{false, true}) {
		var source io.Reader = bytes.NewReader(encoded)
		if fragmented {
			source = iotest.OneByteReader(source)
		}
		content, err := io.ReadAll(Decode(NewFixedReader(source, int64(len(encoded))), []string{"deflate"}, decodeTestLimit))
		require.NoError(t, err, "fragmented=%v", fragmented)
		require.Equal(t, "hello world", string(content))
	}
}

func TestDecodeAppliesCodingsInReverse(t *testing.T) {
	// Applied order gzip then deflate: decode deflate first, then gzip.
	encoded := zlibBytes(t, gzipBytes(t, []byte("hello")))
	content, err := decodeAll(encoded, []string{"gzip", "deflate"}, decodeTestLimit)
	require.NoError(t, err)
	require.Equal(t, "hello", content)

	_, err = decodeAll(encoded, []string{"deflate", "gzip"}, decodeTestLimit)
	require.ErrorIs(t, err, ErrInvalidCoding, "a wrong declared order must fail")
}

func TestDecodeRejectsMalformedDeflate(t *testing.T) {
	encoded := zlibBytes(t, []byte("hello world"))
	badChecksum := slices.Clone(encoded)
	badChecksum[len(badChecksum)-1] ^= 0xff
	var raw bytes.Buffer
	rawWriter, err := flate.NewWriter(&raw, flate.DefaultCompression)
	require.NoError(t, err)
	_, err = rawWriter.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, rawWriter.Close())
	tests := []struct {
		name    string
		encoded []byte
	}{
		{"empty stream", nil},
		{"not zlib", []byte("hello world")},
		{"raw deflate without zlib header", raw.Bytes()},
		{"checksum mismatch", badChecksum},
		{"data after the stream", append(slices.Clone(encoded), "junk"...)},
		{"zero byte after the stream", append(slices.Clone(encoded), 0)},
		{"cut short", encoded[:len(encoded)-3]},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeAll(test.encoded, []string{"deflate"}, decodeTestLimit)
			require.ErrorIs(t, err, ErrInvalidCoding)
			require.NotErrorIs(t, err, io.ErrUnexpectedEOF)
		})
	}
}

func TestDecodeDeflateIncompleteBody(t *testing.T) {
	encoded := zlibBytes(t, []byte(strings.Repeat("hello world ", 100)))
	framed := NewFixedReader(bytes.NewReader(encoded[:len(encoded)/2]), int64(len(encoded)))
	_, err := io.ReadAll(Decode(framed, []string{"deflate"}, decodeTestLimit))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestDecodeLimitsDecodedDeflate(t *testing.T) {
	bomb := zlibBytes(t, make([]byte, 4*decodeTestLimit))
	_, err := decodeAll(bomb, []string{"deflate"}, decodeTestLimit)
	require.ErrorIs(t, err, ErrBodyTooLarge)
}

func TestDecodePassesSourceErrorAfterStream(t *testing.T) {
	errSource := errors.New("connection reset")
	encoded := zlibBytes(t, []byte("hello"))
	source := io.MultiReader(bytes.NewReader(encoded), iotest.ErrReader(errSource))
	framed := NewFixedReader(source, int64(len(encoded))+10)
	_, err := io.ReadAll(Decode(framed, []string{"deflate"}, decodeTestLimit))
	require.ErrorIs(t, err, errSource)
}
