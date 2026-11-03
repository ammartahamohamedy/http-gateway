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
	require.Equal(t, []string{"Host", "Content-Length"}, names)
}

func TestProcessRequestFieldsFirstErrorIsDeterministic(t *testing.T) {
	headers := header.Section{"host": {"a", "b"}, "content-length": {"x"}}
	for range 20 {
		_, err := processRequestFields(originLine, headers)
		var fieldErr *FieldError
		require.ErrorAs(t, err, &fieldErr)
		require.Equal(t, "Host", fieldErr.Name)
	}
}

func TestFieldErrorWrapping(t *testing.T) {
	_, err := processRequestFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"-1"}})
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
	parsed, err := processRequestFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"1, 1"}})
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
		apply: func(_ RequestLine, _ header.Section, parsed RequestFields) error {
			seen = append(seen, parsed)
			return checkErr
		},
	}}

	_, err := processRequestFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"x"}})
	require.ErrorIs(t, err, fields.ErrInvalidContentLength)
	require.Empty(t, seen, "cross-checks must not run after a field rule fails")

	parsed, err := processRequestFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"7"}})
	require.ErrorIs(t, err, checkErr)
	require.Zero(t, parsed)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Content-Length", fieldErr.Name)
	require.Equal(t, []RequestFields{{Authority: "example.com", ContentLength: 7, ContentLengthPresent: true}}, seen)
}
