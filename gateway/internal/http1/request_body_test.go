package http1

import (
	"bufio"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
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

func TestReadRequestDecodesChunkedBody(t *testing.T) {
	for fragmented := range slices.Values([]bool{false, true}) {
		t.Run("fragmented="+strconv.FormatBool(fragmented), func(t *testing.T) {
			var source io.Reader = strings.NewReader("POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n" +
				"5\r\nhello\r\n6;ext=1\r\n world\r\n0\r\nX-Trailer: v\r\n\r\nNEXT")
			if fragmented {
				source = iotest.OneByteReader(source)
			}
			reader := bufio.NewReader(source)
			request, err := ReadRequest(reader)
			require.NoError(t, err)
			content, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Equal(t, "hello world", string(content))
			remaining, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "NEXT", string(remaining))
		})
	}
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
	require.ErrorIs(t, err, body.ErrBodyTooLarge)
	require.Nil(t, request)
}

func TestReadRequestChunkedBodyLimit(t *testing.T) {
	head := "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n"
	atLimit := strconv.FormatInt(maxRequestBodyBytes, 16) + "\r\n" + strings.Repeat("a", maxRequestBodyBytes) + "\r\n0\r\n\r\n"
	request, err := ReadRequest(bufio.NewReader(strings.NewReader(head + atLimit)))
	require.NoError(t, err)
	content, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.Len(t, content, maxRequestBodyBytes)

	overLimit := "1\r\na\r\n" + strconv.FormatInt(maxRequestBodyBytes, 16) + "\r\n"
	request, err = ReadRequest(bufio.NewReader(strings.NewReader(head + overLimit)))
	require.NoError(t, err)
	_, err = io.ReadAll(request.Body)
	require.ErrorIs(t, err, body.ErrBodyTooLarge)
}

func TestReadRequestTruncatedBody(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 10\r\n\r\nabc"))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	content, err := io.ReadAll(request.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, "abc", string(content))
}
