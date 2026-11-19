package body

import (
	"bufio"
	"bytes"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

const readerTestLimit = 1 << 20

func TestNewReader(t *testing.T) {
	response := func(statusCode int) Message {
		return Message{Version: "1.1", IsResponse: true, RequestMethod: "GET", StatusCode: statusCode}
	}

	tests := []struct {
		name string
		msg  Message
		wire string
		want string
		rest string
	}{
		{"request without framing fields", Message{Version: "1.1"}, "NEXT", "", "NEXT"},
		{"request Content-Length", Message{Version: "1.1", ContentLength: 5, ContentLengthPresent: true},
			"helloNEXT", "hello", "NEXT"},
		{"request Content-Length zero", Message{Version: "1.1", ContentLengthPresent: true}, "NEXT", "", "NEXT"},
		{"request chunked", Message{Version: "1.1", TransferCodings: []string{"chunked"}},
			"5\r\nhello\r\n0\r\n\r\nNEXT", "hello", "NEXT"},
		{"response Content-Length", func() Message {
			msg := response(200)
			msg.ContentLength, msg.ContentLengthPresent = 5, true
			return msg
		}(), "helloNEXT", "hello", "NEXT"},
		{"response to HEAD ignores Content-Length", func() Message {
			msg := response(200)
			msg.RequestMethod, msg.ContentLength, msg.ContentLengthPresent = "HEAD", 5, true
			return msg
		}(), "NEXT", "", "NEXT"},
		{"304 ignores Content-Length", func() Message {
			msg := response(304)
			msg.ContentLength, msg.ContentLengthPresent = 5, true
			return msg
		}(), "NEXT", "", "NEXT"},
		{"response chunked", func() Message {
			msg := response(200)
			msg.TransferCodings = []string{"chunked"}
			return msg
		}(), "5\r\nhello\r\n0\r\n\r\nNEXT", "hello", "NEXT"},
		{"response until close", response(200), "hello world", "hello world", ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			source := bufio.NewReader(strings.NewReader(test.wire))

			reader, err := NewReader(source, test.msg, readerTestLimit, nil)
			require.NoError(t, err)

			content, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, test.want, string(content))

			rest, err := io.ReadAll(source)
			require.NoError(t, err)
			require.Equal(t, test.rest, string(rest))
		})
	}
}

func TestNewReaderDecodesTransferCodings(t *testing.T) {
	encoded := gzipBytes(t, []byte("hello world"))
	chunks := strconv.FormatInt(int64(len(encoded)), 16) + "\r\n" + string(encoded) + "\r\n0\r\n\r\n"

	tests := []struct {
		name string
		msg  Message
		wire string
	}{
		{"chunked request", Message{Version: "1.1", TransferCodings: []string{"gzip", "chunked"}}, chunks},
		{"chunked response",
			Message{Version: "1.1", TransferCodings: []string{"gzip", "chunked"}, IsResponse: true, StatusCode: 200}, chunks},
		{"close-delimited response",
			Message{Version: "1.1", TransferCodings: []string{"gzip"}, IsResponse: true, StatusCode: 200}, string(encoded)},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader, err := NewReader(bufio.NewReader(strings.NewReader(test.wire)), test.msg, readerTestLimit, nil)
			require.NoError(t, err)

			content, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "hello world", string(content))
		})
	}
}

func TestNewReaderPassesTrailers(t *testing.T) {
	var trailers header.Section
	msg := Message{Version: "1.1", TransferCodings: []string{"chunked"}, IsResponse: true, StatusCode: 200}
	source := bufio.NewReader(strings.NewReader("5\r\nhello\r\n0\r\nServer-Timing: db;dur=5\r\n\r\n"))

	reader, err := NewReader(source, msg, readerTestLimit, func(section header.Section) { trailers = section })
	require.NoError(t, err)

	_, err = io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, header.Section{"server-timing": {"db;dur=5"}}, trailers)
}

func TestNewReaderRejectsDeclaredLengthOverLimit(t *testing.T) {
	declared := Message{Version: "1.1", ContentLength: 6, ContentLengthPresent: true}

	reader, err := NewReader(bufio.NewReader(strings.NewReader("hello!")), declared, 5, nil)
	require.ErrorIs(t, err, ErrBodyTooLarge)
	require.Nil(t, reader)

	declared.ContentLength = 5
	reader, err = NewReader(bufio.NewReader(strings.NewReader("hello")), declared, 5, nil)
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
}

func TestNewReaderLimitsReadData(t *testing.T) {
	tests := []struct {
		name  string
		msg   Message
		wire  string
		limit int64
	}{
		{"chunked data", Message{Version: "1.1", TransferCodings: []string{"chunked"}}, "6\r\nhello!\r\n0\r\n\r\n", 5},
		{"close-delimited data", Message{Version: "1.1", IsResponse: true, StatusCode: 200}, "hello!", 5},
		{"decoded data", Message{Version: "1.1", TransferCodings: []string{"gzip"}, IsResponse: true, StatusCode: 200},
			string(gzipBytes(t, bytes.Repeat([]byte("a"), 64))), 32},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader, err := NewReader(bufio.NewReader(strings.NewReader(test.wire)), test.msg, test.limit, nil)
			require.NoError(t, err)

			_, err = io.ReadAll(reader)
			require.ErrorIs(t, err, ErrBodyTooLarge)
		})
	}
}

func TestNewReaderReturnsFramingErrors(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want error
	}{
		{"request codings without chunked", Message{Version: "1.1", TransferCodings: []string{"gzip"}}, ErrInvalidFraming},
		{"unsupported version", Message{Version: "2.0"}, ErrInvalidFraming},
		{"2xx response to CONNECT", Message{Version: "1.1", IsResponse: true, RequestMethod: "CONNECT", StatusCode: 200},
			ErrUnsupportedFraming},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader, err := NewReader(bufio.NewReader(strings.NewReader("")), test.msg, readerTestLimit, nil)
			require.ErrorIs(t, err, test.want)
			require.Nil(t, reader)
		})
	}
}
