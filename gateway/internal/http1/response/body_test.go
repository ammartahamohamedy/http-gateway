package response

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestReadAttachesBody(t *testing.T) {
	tests := []struct {
		name          string
		requestMethod string
		raw           string
		want          string
		rest          string
	}{
		{"Content-Length", "GET", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhelloNEXT", "hello", "NEXT"},
		{"Content-Length zero", "POST", "HTTP/1.1 201 Created\r\nContent-Length: 0\r\n\r\nNEXT", "", "NEXT"},
		{"HEAD ignores Content-Length", "HEAD", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nNEXT", "", "NEXT"},
		{"204 has no body", "GET", "HTTP/1.1 204 No Content\r\n\r\nNEXT", "", "NEXT"},
		{"304 ignores Content-Length", "GET", "HTTP/1.1 304 Not Modified\r\nContent-Length: 5\r\n\r\nNEXT", "", "NEXT"},
		{"chunked", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n6\r\n world\r\n0\r\n\r\nNEXT",
			"hello world", "NEXT"},
		{"until close", "GET", "HTTP/1.1 200 OK\r\n\r\nhello world", "hello world", ""},
		{"until close after interim response", "GET", "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\n\r\nhello",
			"hello", ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(test.raw))

			message, err := Read(reader, test.requestMethod, nil)
			require.NoError(t, err)

			content, err := io.ReadAll(message.Body)
			require.NoError(t, err)
			require.Equal(t, test.want, string(content))
			require.NoError(t, message.Body.Close())

			rest, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, test.rest, string(rest))
		})
	}
}

func TestReadKeepsDeclaredLengthForHEAD(t *testing.T) {
	message, err := Read(bufio.NewReader(strings.NewReader("HTTP/1.1 200 OK\r\nContent-Length: 42\r\n\r\n")), "HEAD", nil)
	require.NoError(t, err)
	require.Equal(t, Fields{ContentLength: 42, ContentLengthPresent: true}, message.Fields)
}

func TestReadDecodesTransferCodings(t *testing.T) {
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	_, err := writer.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	chunks := strconv.FormatInt(int64(encoded.Len()), 16) + "\r\n" + encoded.String() + "\r\n0\r\n\r\n"

	tests := []struct {
		name string
		raw  string
	}{
		{"gzip then chunked", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip, chunked\r\n\r\n" + chunks},
		{"gzip until close", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip\r\n\r\n" + encoded.String()},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, err := Read(bufio.NewReader(strings.NewReader(test.raw)), "GET", nil)
			require.NoError(t, err)

			content, err := io.ReadAll(message.Body)
			require.NoError(t, err)
			require.Equal(t, "hello world", string(content))
		})
	}
}

func TestReadTrailers(t *testing.T) {
	const raw = "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nTrailer: Server-Timing\r\n\r\n" +
		"5\r\nhello\r\n0\r\nServer-Timing: db;dur=5\r\nContent-Length: 99\r\n\r\n"

	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", nil)
	require.NoError(t, err)
	require.Nil(t, message.Trailers)

	_, err = io.ReadAll(message.Body)
	require.NoError(t, err)
	require.Equal(t, header.Section{"server-timing": {"db;dur=5"}}, message.Trailers)
}

func TestReadContentLengthBodyHasNoTrailers(t *testing.T) {
	message, err := Read(bufio.NewReader(strings.NewReader("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello")), "GET", nil)
	require.NoError(t, err)

	_, err = io.ReadAll(message.Body)
	require.NoError(t, err)
	require.Nil(t, message.Trailers)
}

func TestReadBodyLimit(t *testing.T) {
	atLimit := "HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(maxBodyBytes) + "\r\n\r\n"
	message, err := Read(bufio.NewReader(strings.NewReader(atLimit)), "GET", nil)
	require.NoError(t, err)
	require.NotNil(t, message.Body)

	overLimit := "HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(maxBodyBytes+1) + "\r\n\r\n"
	message, err = Read(bufio.NewReader(strings.NewReader(overLimit)), "GET", nil)
	require.ErrorIs(t, err, body.ErrBodyTooLarge)
	requireStage(t, err, http1.StageBody)
	require.Nil(t, message)

	untilClose := "HTTP/1.1 200 OK\r\n\r\n" + strings.Repeat("a", maxBodyBytes+1)
	message, err = Read(bufio.NewReader(strings.NewReader(untilClose)), "GET", nil)
	require.NoError(t, err)

	content, err := io.ReadAll(message.Body)
	require.ErrorIs(t, err, body.ErrBodyTooLarge)
	require.Len(t, content, maxBodyBytes)
}

func TestReadRejectsTunnelResponse(t *testing.T) {
	message, err := Read(bufio.NewReader(strings.NewReader("HTTP/1.1 200 OK\r\n\r\n")), "CONNECT", nil)
	require.ErrorIs(t, err, body.ErrUnsupportedFraming)
	requireStage(t, err, http1.StageBody)
	require.Nil(t, message)
}

func TestReadTruncatedBody(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"Content-Length", "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nhello"},
		{"chunked", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhel"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, err := Read(bufio.NewReader(strings.NewReader(test.raw)), "GET", nil)
			require.NoError(t, err)

			_, err = io.ReadAll(message.Body)
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		})
	}
}
