package http1

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

var originLine = RequestLine{Method: "GET", RequestTarget: "/", Version: "1.1"}

func TestRequestFieldRulesOrder(t *testing.T) {
	var names []string
	for _, rule := range requestFieldRules {
		names = append(names, rule.name)
	}
	require.Equal(t, []string{"Host", "Content-Length", "Transfer-Encoding", "Connection", "Expect", "Content-Type", "Accept", "Content-Encoding", "Accept-Encoding"}, names)
}

func TestProcessRequestFieldsFirstErrorIsDeterministic(t *testing.T) {
	headers := header.Headers{"host": {"a", "b"}, "content-length": {"x"}}
	for range 20 {
		_, err := processRequestFields(originLine, headers)
		var fieldErr *FieldError
		require.ErrorAs(t, err, &fieldErr)
		require.Equal(t, "Host", fieldErr.Name)
	}
}

func TestFieldErrorWrapping(t *testing.T) {
	_, err := processRequestFields(originLine, header.Headers{"host": {"example.com"}, "content-length": {"-1"}})
	require.ErrorIs(t, err, fields.ErrInvalidContentLength)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Content-Length", fieldErr.Name)
	require.Equal(t, fields.ErrInvalidContentLength.Error(), err.Error())

	_, err = ReadRequest(bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\n\r\n")))
	require.ErrorIs(t, err, fields.ErrInvalidHost)
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Host", fieldErr.Name)
}

func TestReadRequestFields(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    RequestFields
	}{
		{"absent length", "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
			RequestFields{Authority: "example.com"}},
		{"zero length", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\n\r\n",
			RequestFields{Authority: "example.com", ContentLengthPresent: true}},
		{"positive length", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0042\r\n\r\n",
			RequestFields{Authority: "example.com", ContentLength: 42, ContentLengthPresent: true}},
		{"absolute authority overrides Host", "GET http://target.example:8080/ HTTP/1.1\r\nHost: other.example\r\n\r\n",
			RequestFields{Authority: "target.example:8080"}},
		{"CONNECT authority", "CONNECT example.com:443 HTTP/1.1\r\nHost: other.example:443\r\n\r\n",
			RequestFields{Authority: "example.com:443"}},
		{"authority-less absolute", "GET urn:example:item HTTP/1.1\r\nHost:\r\n\r\n",
			RequestFields{}},
		{"transfer codings", "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: GZIP\r\nTransfer-Encoding: Chunked\r\n\r\n",
			RequestFields{Authority: "example.com", TransferCodings: []string{"gzip", "chunked"}}},
		{"connection options", "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: Close\r\nConnection: TE\r\n\r\n",
			RequestFields{Authority: "example.com", ConnectionOptions: []string{"close", "te"}}},
		{"expect with length", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nExpect: 100-Continue\r\n\r\n",
			RequestFields{Authority: "example.com", ContentLength: 5, ContentLengthPresent: true, Expectations: []string{"100-continue"}}},
		{"expect with chunked framing", "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\nExpect: 100-continue\r\n\r\n",
			RequestFields{Authority: "example.com", TransferCodings: []string{"chunked"}, Expectations: []string{"100-continue"}}},
		{"unfamiliar expectation without content", "GET / HTTP/1.1\r\nHost: example.com\r\nExpect: x-custom\r\n\r\n",
			RequestFields{Authority: "example.com", Expectations: []string{"x-custom"}}},
		{"unknown valid fields accepted", "GET / HTTP/1.1\r\nHost: example.com\r\nX-Unfamiliar: \"a,b\"\r\nX-Unfamiliar: c\r\n\r\n",
			RequestFields{Authority: "example.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := ReadRequest(bufio.NewReader(strings.NewReader(test.message)))
			require.NoError(t, err)
			require.Equal(t, test.want, request.Fields)
		})
	}
}

func TestProcessRequestFieldsReturnsZeroOnFailure(t *testing.T) {
	// Host succeeds and assigns Authority before Content-Length fails.
	parsed, err := processRequestFields(originLine, header.Headers{"host": {"example.com"}, "content-length": {"1, 1"}})
	require.Error(t, err)
	require.Zero(t, parsed)
}

func TestRequestCrossChecksRunAfterFieldRules(t *testing.T) {
	saved := requestCrossChecks
	t.Cleanup(func() { requestCrossChecks = saved })

	checkErr := errors.New("cross-check failed")
	var seen []RequestFields
	requestCrossChecks = []requestCrossCheck{{
		name: "Content-Length",
		apply: func(_ RequestLine, _ header.Headers, parsed RequestFields) error {
			seen = append(seen, parsed)
			return checkErr
		},
	}}

	_, err := processRequestFields(originLine, header.Headers{"host": {"example.com"}, "content-length": {"x"}})
	require.ErrorIs(t, err, fields.ErrInvalidContentLength)
	require.Empty(t, seen, "cross-checks must not run after a field rule fails")

	parsed, err := processRequestFields(originLine, header.Headers{"host": {"example.com"}, "content-length": {"7"}})
	require.ErrorIs(t, err, checkErr)
	require.Zero(t, parsed)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Content-Length", fieldErr.Name)
	require.Equal(t, []RequestFields{{Authority: "example.com", ContentLength: 7, ContentLengthPresent: true}}, seen)
}

func TestTransferEncodingErrors(t *testing.T) {
	headers := header.Headers{"host": {"example.com"}, "transfer-encoding": {"gzip"}}
	parsed, err := processRequestFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidTransferEncoding)
	require.Zero(t, parsed)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Transfer-Encoding", fieldErr.Name)
}

func TestFramingConflict(t *testing.T) {
	tests := []struct {
		name    string
		headers header.Headers
	}{
		{"positive length", header.Headers{"host": {"example.com"}, "content-length": {"5"}, "transfer-encoding": {"chunked"}}},
		{"zero length", header.Headers{"host": {"example.com"}, "content-length": {"0"}, "transfer-encoding": {"chunked"}}},
		{"coding list", header.Headers{"host": {"example.com"}, "content-length": {"5"}, "transfer-encoding": {"gzip, chunked"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processRequestFields(originLine, test.headers)
			require.ErrorIs(t, err, fields.ErrFramingConflict)
			require.NotErrorIs(t, err, fields.ErrInvalidTransferEncoding)
			require.Zero(t, parsed)
			var fieldErr *FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Transfer-Encoding", fieldErr.Name)
		})
	}
}

func TestMalformedTransferEncodingPrecedesFramingConflict(t *testing.T) {
	headers := header.Headers{"host": {"example.com"}, "content-length": {"5"}, "transfer-encoding": {"xchunked"}}
	_, err := processRequestFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidTransferEncoding)
}

func TestReadRequestRejectsFramingConflict(t *testing.T) {
	message := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n"
	request, err := ReadRequest(bufio.NewReader(strings.NewReader(message)))
	require.ErrorIs(t, err, fields.ErrFramingConflict)
	require.Nil(t, request)
}

func TestConnectionErrors(t *testing.T) {
	headers := header.Headers{"host": {"example.com"}, "connection": {"keep alive"}}
	parsed, err := processRequestFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidConnection)
	require.Zero(t, parsed)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Connection", fieldErr.Name)
}

func TestConnectionNominatingProtectedField(t *testing.T) {
	for _, value := range []string{"host", "Content-Length", "close, Transfer-Encoding"} {
		t.Run(value, func(t *testing.T) {
			headers := header.Headers{"host": {"example.com"}, "connection": {value}}
			parsed, err := processRequestFields(originLine, headers)
			require.ErrorIs(t, err, fields.ErrProtectedConnectionOption)
			require.NotErrorIs(t, err, fields.ErrInvalidConnection)
			require.Zero(t, parsed)
			var fieldErr *FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Connection", fieldErr.Name)
		})
	}
}

func TestFramingConflictPrecedesConnectionCheck(t *testing.T) {
	headers := header.Headers{
		"host":              {"example.com"},
		"content-length":    {"5"},
		"transfer-encoding": {"chunked"},
		"connection":        {"content-length"},
	}
	_, err := processRequestFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrFramingConflict)
}

func TestReadRequestRejectsProtectedConnectionOption(t *testing.T) {
	message := "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: host\r\n\r\n"
	request, err := ReadRequest(bufio.NewReader(strings.NewReader(message)))
	require.ErrorIs(t, err, fields.ErrProtectedConnectionOption)
	require.Nil(t, request)
}

func TestExpectErrors(t *testing.T) {
	headers := header.Headers{"host": {"example.com"}, "expect": {"100 continue"}}
	parsed, err := processRequestFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidExpect)
	require.Zero(t, parsed)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Expect", fieldErr.Name)
}

func TestExpectWithoutContent(t *testing.T) {
	tests := []struct {
		name    string
		headers header.Headers
	}{
		{"no framing", header.Headers{"host": {"example.com"}, "expect": {"100-continue"}}},
		{"zero length", header.Headers{"host": {"example.com"}, "content-length": {"0"}, "expect": {"100-continue"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processRequestFields(originLine, test.headers)
			require.ErrorIs(t, err, fields.ErrExpectWithoutContent)
			require.NotErrorIs(t, err, fields.ErrInvalidExpect)
			require.Zero(t, parsed)
			var fieldErr *FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Expect", fieldErr.Name)
		})
	}
}

func TestConnectionCheckPrecedesExpectCheck(t *testing.T) {
	headers := header.Headers{"host": {"example.com"}, "connection": {"host"}, "expect": {"100-continue"}}
	_, err := processRequestFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrProtectedConnectionOption)
}

func TestReadRequestRejectsExpectWithoutContent(t *testing.T) {
	message := "GET / HTTP/1.1\r\nHost: example.com\r\nExpect: 100-continue\r\n\r\n"
	request, err := ReadRequest(bufio.NewReader(strings.NewReader(message)))
	require.ErrorIs(t, err, fields.ErrExpectWithoutContent)
	require.Nil(t, request)
}

func TestMediaTypeFieldErrors(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		headers header.Headers
		want    error
	}{
		{"content type", "Content-Type", header.Headers{"host": {"example.com"}, "content-type": {"text"}}, fields.ErrInvalidContentType},
		{"accept", "Accept", header.Headers{"host": {"example.com"}, "accept": {"*/html"}}, fields.ErrInvalidAccept},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processRequestFields(originLine, test.headers)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, test.field, fieldErr.Name)
		})
	}
}

func TestContentCodingFieldErrors(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		headers header.Headers
		want    error
	}{
		{"content encoding", "Content-Encoding", header.Headers{"host": {"example.com"}, "content-encoding": {"gzip;q=1"}}, fields.ErrInvalidContentEncoding},
		{"accept encoding", "Accept-Encoding", header.Headers{"host": {"example.com"}, "accept-encoding": {"gzip;q=2"}}, fields.ErrInvalidAcceptEncoding},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processRequestFields(originLine, test.headers)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, test.field, fieldErr.Name)
		})
	}
}

func TestContentCodingFieldsLeaveMetadataUnchanged(t *testing.T) {
	message := "GET / HTTP/1.1\r\nHost: example.com\r\nContent-Encoding: gzip\r\nAccept-Encoding: br;q=0.5\r\n\r\n"
	request, err := ReadRequest(bufio.NewReader(strings.NewReader(message)))
	require.NoError(t, err)
	require.Equal(t, RequestFields{Authority: "example.com"}, request.Fields)
	require.Equal(t, []string{"gzip"}, request.Headers.Values("Content-Encoding"))
}

func TestMediaTypeFieldsLeaveMetadataUnchanged(t *testing.T) {
	message := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\nContent-Type: text/plain\r\nAccept: */*;q=0.5\r\n\r\n"
	request, err := ReadRequest(bufio.NewReader(strings.NewReader(message)))
	require.NoError(t, err)
	require.Equal(t, RequestFields{Authority: "example.com", ContentLengthPresent: true}, request.Fields)
	require.Equal(t, []string{"text/plain"}, request.Headers.Values("Content-Type"))
}
