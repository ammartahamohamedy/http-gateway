package body

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModeZeroValueIsUnknown(t *testing.T) {
	var mode Mode
	require.Equal(t, Unknown, mode)
}

func TestFramingRequests(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want Mode
	}{
		{"no framing fields", Message{Version: "1.1"}, None},
		{"Content-Length zero", Message{Version: "1.1", ContentLengthPresent: true}, ContentLength},
		{"positive Content-Length", Message{Version: "1.1", ContentLength: 5, ContentLengthPresent: true}, ContentLength},
		{"chunked", Message{Version: "1.1", TransferCodings: []string{"chunked"}}, Chunked},
		{"coding list ending in chunked", Message{Version: "1.1", TransferCodings: []string{"gzip", "deflate", "chunked"}}, Chunked},
		{"Transfer-Encoding overrides Content-Length", Message{Version: "1.1", ContentLength: 5, ContentLengthPresent: true, TransferCodings: []string{"chunked"}}, Chunked},
		{"HTTP/1.0 with Content-Length", Message{Version: "1.0", ContentLength: 5, ContentLengthPresent: true}, ContentLength},
		{"HTTP/1.0 without framing fields", Message{Version: "1.0"}, None},
		{"response-only inputs ignored", Message{Version: "1.1", ContentLength: 5, ContentLengthPresent: true, RequestMethod: "HEAD", StatusCode: 204}, ContentLength},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			mode, err := Framing(test.msg)
			require.NoError(t, err)
			require.Equal(t, test.want, mode)
		})
	}
}

func TestFramingResponses(t *testing.T) {
	response := func(method string, status int) Message {
		return Message{Version: "1.1", IsResponse: true, RequestMethod: method, StatusCode: status}
	}
	withLength := func(msg Message, length int64) Message {
		msg.ContentLength, msg.ContentLengthPresent = length, true
		return msg
	}
	withCodings := func(msg Message, codings ...string) Message {
		msg.TransferCodings = codings
		return msg
	}
	http10 := func(msg Message) Message {
		msg.Version = "1.0"
		return msg
	}

	tests := []struct {
		name string
		msg  Message
		want Mode
	}{
		{"HEAD with Content-Length", withLength(response("HEAD", 200), 5), None},
		{"HEAD with chunked", withCodings(response("HEAD", 200), "chunked"), None},
		{"HEAD error status", withLength(response("HEAD", 404), 5), None},
		{"100 Continue", response("GET", 100), None},
		{"101 Switching Protocols", response("GET", 101), None},
		{"199", response("GET", 199), None},
		{"204 with Content-Length", withLength(response("GET", 204), 5), None},
		{"304 with chunked", withCodings(response("GET", 304), "chunked"), None},
		{"non-2xx response to CONNECT", withLength(response("CONNECT", 407), 5), ContentLength},
		{"chunked", withCodings(response("GET", 200), "chunked"), Chunked},
		{"coding list ending in chunked", withCodings(response("POST", 201), "gzip", "chunked"), Chunked},
		{"Transfer-Encoding overrides Content-Length", withLength(withCodings(response("GET", 200), "chunked"), 5), Chunked},
		{"Transfer-Encoding not ending in chunked", withCodings(response("GET", 200), "gzip"), UntilClose},
		{"chunked before another coding", withCodings(response("GET", 200), "chunked", "gzip"), UntilClose},
		{"Content-Length zero", withLength(response("GET", 200), 0), ContentLength},
		{"positive Content-Length", withLength(response("GET", 500), 5), ContentLength},
		{"no framing fields", response("GET", 200), UntilClose},
		{"HTTP/1.0 with Content-Length", http10(withLength(response("GET", 200), 5)), ContentLength},
		{"HTTP/1.0 without framing fields", http10(response("GET", 200)), UntilClose},
		{"HTTP/1.0 HEAD with Transfer-Encoding", http10(withCodings(response("HEAD", 200), "chunked")), None},
		{"lowercase head is not HEAD", withLength(response("head", 200), 5), ContentLength},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			mode, err := Framing(test.msg)
			require.NoError(t, err)
			require.Equal(t, test.want, mode)
		})
	}
}

func TestFramingErrors(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want error
	}{
		{"zero Message", Message{}, ErrInvalidFraming},
		{"HTTP/2 version", Message{Version: "2.0"}, ErrInvalidFraming},
		{"version with prefix", Message{Version: "HTTP/1.1"}, ErrInvalidFraming},
		{"request negative Content-Length", Message{Version: "1.1", ContentLength: -1, ContentLengthPresent: true}, ErrInvalidFraming},
		{"request Transfer-Encoding not ending in chunked", Message{Version: "1.1", TransferCodings: []string{"gzip"}}, ErrInvalidFraming},
		{"request chunked before another coding", Message{Version: "1.1", TransferCodings: []string{"chunked", "gzip"}}, ErrInvalidFraming},
		{"HTTP/1.0 request with Transfer-Encoding", Message{Version: "1.0", TransferCodings: []string{"chunked"}}, ErrInvalidFraming},
		{"HTTP/1.0 response with Transfer-Encoding and Content-Length", Message{Version: "1.0", IsResponse: true, RequestMethod: "GET", StatusCode: 200, ContentLength: 5, ContentLengthPresent: true, TransferCodings: []string{"chunked"}}, ErrInvalidFraming},
		{"response negative Content-Length", Message{Version: "1.1", IsResponse: true, RequestMethod: "GET", StatusCode: 200, ContentLength: -1, ContentLengthPresent: true}, ErrInvalidFraming},
		{"response without status", Message{Version: "1.1", IsResponse: true, RequestMethod: "GET"}, ErrInvalidFraming},
		{"status below 100", Message{Version: "1.1", IsResponse: true, RequestMethod: "GET", StatusCode: 99}, ErrInvalidFraming},
		{"status above 599", Message{Version: "1.1", IsResponse: true, RequestMethod: "GET", StatusCode: 600}, ErrInvalidFraming},
		{"200 response to CONNECT", Message{Version: "1.1", IsResponse: true, RequestMethod: "CONNECT", StatusCode: 200}, ErrUnsupportedFraming},
		{"299 response to CONNECT with Content-Length", Message{Version: "1.1", IsResponse: true, RequestMethod: "CONNECT", StatusCode: 299, ContentLength: 5, ContentLengthPresent: true}, ErrUnsupportedFraming},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			mode, err := Framing(test.msg)
			require.ErrorIs(t, err, test.want)
			require.Equal(t, Unknown, mode)
		})
	}
}

func TestFramingKeepsInput(t *testing.T) {
	codings := []string{"gzip", "chunked"}
	_, err := Framing(Message{Version: "1.1", TransferCodings: codings})
	require.NoError(t, err)
	require.Equal(t, []string{"gzip", "chunked"}, codings)
}
