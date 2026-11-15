package body

import (
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanDecode(t *testing.T) {
	for coding := range slices.Values([]string{"gzip", "deflate"}) {
		require.True(t, CanDecode(coding), coding)
	}

	for coding := range slices.Values([]string{"chunked", "compress", "x-gzip", "GZIP", ""}) {
		require.False(t, CanDecode(coding), coding)
	}
}

func TestCodingDecoderIsFormatError(t *testing.T) {
	gzipDecoder, deflateDecoder := decoders["gzip"], decoders["deflate"]
	require.True(t, gzipDecoder.isFormatError(gzip.ErrChecksum))
	require.True(t, gzipDecoder.isFormatError(fmt.Errorf("wrapped: %w", gzip.ErrHeader)))
	require.True(t, deflateDecoder.isFormatError(zlib.ErrDictionary))
	require.True(t, deflateDecoder.isFormatError(flate.CorruptInputError(3)), "both use flate")
	require.False(t, gzipDecoder.isFormatError(zlib.ErrChecksum), "another coding's errors")
	require.False(t, deflateDecoder.isFormatError(errors.New("connection reset")))
}
