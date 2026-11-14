package http1

import (
	"bufio"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

// requireEmptyBody checks that request has an empty body, then clears Body so
// the rest of the request can be compared by value.
func requireEmptyBody(t *testing.T, request *Request) *Request {
	t.Helper()
	require.NotNil(t, request.Body)
	content, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.Empty(t, content)
	request.Body = nil
	return request
}

func TestReadRequestAttachesBody(t *testing.T) {
	tests := []struct {
		name          string
		framing       string
		wire          string
		wantBody      string
		wantRemainder string
	}{
		{"no framing fields", "", "NEXT", "", "NEXT"},
		{"Content-Length zero", "Content-Length: 0\r\n", "NEXT", "", "NEXT"},
		{"Content-Length", "Content-Length: 7\r\n", "he\x00\r\nlo" + "NEXT", "he\x00\r\nlo", "NEXT"},
	}
	for test := range slices.Values(tests) {
		for fragmented := range slices.Values([]bool{false, true}) {
			t.Run(test.name+"/fragmented="+strconv.FormatBool(fragmented), func(t *testing.T) {
				var source io.Reader = strings.NewReader("POST / HTTP/1.1\r\nHost: example.com\r\n" + test.framing + "\r\n" + test.wire)
				if fragmented {
					source = iotest.OneByteReader(source)
				}
				reader := bufio.NewReader(source)
				request, err := ReadRequest(reader)
				require.NoError(t, err)
				content, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				require.Equal(t, test.wantBody, string(content))
				require.NoError(t, request.Body.Close())
				remaining, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, test.wantRemainder, string(remaining))
			})
		}
	}
}

func TestReadRequestLeavesChunkedBodyUnread(t *testing.T) {
	const chunked = "5\r\nhello\r\n0\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader("POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n" + chunked))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	require.Nil(t, request.Body)
	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, chunked, string(remaining))
}

func TestReadRequestBodyLimit(t *testing.T) {
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: "

	reader := bufio.NewReader(strings.NewReader(head + strconv.Itoa(maxRequestBodyBytes) + "\r\n\r\n" + strings.Repeat("a", maxRequestBodyBytes)))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	content, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.Len(t, content, maxRequestBodyBytes)

	request, err = ReadRequest(bufio.NewReader(strings.NewReader(head + strconv.Itoa(maxRequestBodyBytes+1) + "\r\n\r\n")))
	require.ErrorIs(t, err, ErrRequestBodyTooLarge)
	require.Nil(t, request)
}

func TestReadRequestTruncatedBody(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 10\r\n\r\nabc"))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	content, err := io.ReadAll(request.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, "abc", string(content))
}
