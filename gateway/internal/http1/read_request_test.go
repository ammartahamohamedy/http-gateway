package http1

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestReadRequestSizeLimit(t *testing.T) {
	prefix, suffix := "GET /", " HTTP/1.1"
	rawLine := prefix + strings.Repeat("a", maxRequestLineBytes-len(prefix)-len(suffix)) + suffix
	oversizedRawLine := prefix + strings.Repeat("a", maxRequestLineBytes+1-len(prefix)-len(suffix)) + suffix

	line, err := ParseRequestLine(rawLine)
	require.NoError(t, err)
	parsed, err := ReadRequest(bufio.NewReader(strings.NewReader(rawLine + "\r\nHost: example.com\r\n\r\n")))
	require.NoError(t, err)
	require.Equal(t, &Request{RequestLine: line, Headers: header.Section{"host": {"example.com"}}, Fields: RequestFields{Authority: "example.com"}}, parsed)
	parsed, err = ReadRequest(bufio.NewReaderSize(strings.NewReader(rawLine+"\r\nHost: example.com\r\n\r\n"), maxRequestLineBytes+1))
	require.NoError(t, err)
	require.Equal(t, &Request{RequestLine: line, Headers: header.Section{"host": {"example.com"}}, Fields: RequestFields{Authority: "example.com"}}, parsed)
	_, err = ReadRequest(bufio.NewReader(strings.NewReader(rawLine + "\r")))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)

	_, err = ReadRequest(bufio.NewReader(strings.NewReader(oversizedRawLine + "\r\n")))
	require.ErrorIs(t, err, ErrRequestLineTooLong)
	_, err = ReadRequest(bufio.NewReader(strings.NewReader(oversizedRawLine)))
	require.ErrorIs(t, err, ErrRequestLineTooLong)
	_, err = ReadRequest(bufio.NewReader(strings.NewReader(oversizedRawLine + "\r")))
	require.ErrorIs(t, err, ErrRequestLineTooLong)
}

func TestReadRequest(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader io.Reader
	}{
		{"CRLF", strings.NewReader("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")},
		{"leading empty lines", strings.NewReader("\r\n\r\nGET / HTTP/1.1\r\nHost: example.com\r\n\r\n")},
		{"one byte per read", iotest.OneByteReader(strings.NewReader("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))},
		{"data with EOF", iotest.DataErrReader(strings.NewReader("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := ReadRequest(bufio.NewReaderSize(test.reader, 16))
			require.NoError(t, err)
			require.Equal(t, &Request{
				RequestLine: RequestLine{Method: "GET", RequestTarget: "/", Version: "1.1"},
				Headers:     header.Section{"host": {"example.com"}},
				Fields:      RequestFields{Authority: "example.com"},
			}, request)
		})
	}
}

func TestReadRequestPreservesRemainingBytes(t *testing.T) {
	remainder := "body\x00bytes\r\nGET /next HTTP/1.1\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n" + remainder))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	require.Equal(t, header.Section{"host": {"example.com"}}, request.Headers)
	require.Equal(t, []string{"example.com"}, request.Headers.Values("HOST"))
	require.Nil(t, request.Body)
	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, remainder, string(remaining))
}

func TestReadRequestValidatesHost(t *testing.T) {
	tests := []struct {
		name      string
		startLine string
		headers   string
		wantHost  string
		wantErr   error
	}{
		{"mixed case name", "GET / HTTP/1.1", "hOsT: Example.COM:8080", "Example.COM:8080", nil},
		{"outer whitespace", "GET / HTTP/1.1", "Host: \t example.com \t", "example.com", nil},
		{"IPv6", "GET / HTTP/1.1", "Host: [2001:db8::1]:443", "[2001:db8::1]:443", nil},
		{"empty port", "GET / HTTP/1.1", "Host: example.com:", "example.com:", nil},
		{"asterisk target", "OPTIONS * HTTP/1.1", "Host: example.com", "example.com", nil},
		{"CONNECT target", "CONNECT example.com:443 HTTP/1.1", "Host: example.com:443", "example.com:443", nil},
		{"authority-less absolute target", "GET urn:example:item HTTP/1.1", "Host:", "", nil},
		{"absolute authority takes precedence later", "GET http://target.example/ HTTP/1.1", "Host: other.example", "other.example", nil},
		{"missing", "GET / HTTP/1.1", "", "", fields.ErrInvalidHost},
		{"empty origin host", "GET / HTTP/1.1", "Host:", "", fields.ErrInvalidHost},
		{"whitespace-only origin host", "GET / HTTP/1.1", "Host: \t", "", fields.ErrInvalidHost},
		{"empty absolute HTTP host", "GET http://example.com/ HTTP/1.1", "Host:", "", fields.ErrInvalidHost},
		{"empty asterisk host", "OPTIONS * HTTP/1.1", "Host:", "", fields.ErrInvalidHost},
		{"empty CONNECT host", "CONNECT example.com:443 HTTP/1.1", "Host:", "", fields.ErrInvalidHost},
		{"port without host", "GET / HTTP/1.1", "Host: :80", "", fields.ErrInvalidHost},
		{"identical duplicates", "GET / HTTP/1.1", "Host: example.com\r\nHost: example.com", "", fields.ErrInvalidHost},
		{"mixed case duplicates", "GET / HTTP/1.1", "Host: example.com\r\nX: value\r\nhOsT: other.example", "", fields.ErrInvalidHost},
		{"comma and space", "GET / HTTP/1.1", "Host: example.com, other.example", "", fields.ErrInvalidHost},
		{"userinfo", "GET / HTTP/1.1", "Host: user@example.com", "", fields.ErrInvalidHost},
		{"URI instead of authority", "GET / HTTP/1.1", "Host: http://example.com", "", fields.ErrInvalidHost},
		{"invalid port", "GET / HTTP/1.1", "Host: example.com:http", "", fields.ErrInvalidHost},
		{"unbracketed IPv6", "GET / HTTP/1.1", "Host: 2001:db8::1", "", fields.ErrInvalidHost},
		{"malformed IPv6", "GET / HTTP/1.1", "Host: [2001:db8::g]", "", fields.ErrInvalidHost},
		{"fold inside authority", "GET / HTTP/1.1", "Host: example.\r\n com", "", fields.ErrInvalidHost},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			section := "\r\n"
			if test.headers != "" {
				section = test.headers + "\r\n\r\n"
			}
			reader := bufio.NewReader(iotest.OneByteReader(strings.NewReader(test.startLine + "\r\n" + section + "body")))
			request, err := ReadRequest(reader)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Nil(t, request)
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{test.wantHost}, request.Headers.Values("HOST"))
			}
			remaining, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "body", string(remaining))
		})
	}
}

func TestReadRequestValidatesContentLength(t *testing.T) {
	tests := []struct {
		name        string
		headerLines string
		wantValues  []string
		wantErr     error
	}{
		{"absent", "", nil, nil},
		{"zero", "Content-Length: 0\r\n", []string{"0"}, nil},
		{"positive", "Content-Length: 4\r\n", []string{"4"}, nil},
		{"mixed case and whitespace", "cOnTeNt-LeNgTh:\t 4 \t\r\n", []string{"4"}, nil},
		{"leading zeros preserved", "Content-Length: 0004\r\n", []string{"0004"}, nil},
		{"maximum int64", "Content-Length: 9223372036854775807\r\n", []string{"9223372036854775807"}, nil},
		{"empty", "Content-Length:\r\n", nil, fields.ErrInvalidContentLength},
		{"signed", "Content-Length: +4\r\n", nil, fields.ErrInvalidContentLength},
		{"internal whitespace", "Content-Length: 4 2\r\n", nil, fields.ErrInvalidContentLength},
		{"folded digits", "Content-Length: 4\r\n 2\r\n", nil, fields.ErrInvalidContentLength},
		{"identical duplicates", "Content-Length: 4\r\nContent-Length: 4\r\n", nil, fields.ErrInvalidContentLength},
		{"conflicting mixed case duplicates", "Content-Length: 4\r\nX: value\r\ncOnTeNt-LeNgTh: 5\r\n", nil, fields.ErrInvalidContentLength},
		{"identical comma list", "Content-Length: 4, 4\r\n", nil, fields.ErrInvalidContentLength},
		{"conflicting comma list", "Content-Length: 4, 5\r\n", nil, fields.ErrInvalidContentLength},
		{"overflow", "Content-Length: 9223372036854775808\r\n", nil, fields.ErrInvalidContentLength},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const remainder = "body\x00bytes\r\n"
			raw := "POST / HTTP/1.1\r\nHost: example.com\r\n" + test.headerLines + "\r\n" + remainder
			reader := bufio.NewReader(iotest.OneByteReader(strings.NewReader(raw)))
			request, err := ReadRequest(reader)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Nil(t, request)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.wantValues, request.Headers.Values("Content-Length"))
				require.Nil(t, request.Body)
			}
			remaining, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, remainder, string(remaining))
		})
	}
}

func TestReadRequestRejectsInvalidRequestLine(t *testing.T) {
	for _, rawLine := range []string{
		"GET  HTTP/1.1",
		"GE/T / HTTP/1.1",
		"GET / HTTP/1.0",
	} {
		t.Run(rawLine, func(t *testing.T) {
			_, parseErr := ParseRequestLine(rawLine)
			require.Error(t, parseErr)
			request, err := ReadRequest(bufio.NewReader(strings.NewReader(rawLine + "\r\n")))
			require.EqualError(t, err, parseErr.Error())
			require.Nil(t, request)
		})
	}
}

func TestReadRequestErrors(t *testing.T) {
	readErr := errors.New("connection failed")
	tests := []struct {
		name   string
		reader io.Reader
		want   error
	}{
		{"empty input", strings.NewReader(""), io.EOF},
		{"empty lines only", strings.NewReader("\r\n"), io.EOF},
		{"missing terminator", strings.NewReader("GET / HTTP/1.1"), io.ErrUnexpectedEOF},
		{"missing LF", strings.NewReader("GET / HTTP/1.1\r"), io.ErrUnexpectedEOF},
		{"missing header section", strings.NewReader("GET / HTTP/1.1\r\n"), io.ErrUnexpectedEOF},
		{"unterminated header", strings.NewReader("GET / HTTP/1.1\r\nX: value"), io.ErrUnexpectedEOF},
		{"missing header LF", strings.NewReader("GET / HTTP/1.1\r\nX: value\r"), io.ErrUnexpectedEOF},
		{"missing empty line", strings.NewReader("GET / HTTP/1.1\r\nX: value\r\n"), io.ErrUnexpectedEOF},
		{"partial final empty line", strings.NewReader("GET / HTTP/1.1\r\nX: value\r\n\r"), io.ErrUnexpectedEOF},
		{"unterminated continuation", strings.NewReader("GET / HTTP/1.1\r\nX: value\r\n more"), io.ErrUnexpectedEOF},
		{"read failure", iotest.ErrReader(readErr), readErr},
		{"read failure after bytes", io.MultiReader(strings.NewReader("GET /"), iotest.ErrReader(readErr)), readErr},
		{"read failure in headers", io.MultiReader(strings.NewReader("GET / HTTP/1.1\r\nX: value\r\n"), iotest.ErrReader(readErr)), readErr},
		{"header line limit", strings.NewReader("GET / HTTP/1.1\r\nX:" + strings.Repeat("a", requestHeaderLimits.LineBytes) + "\r\n"), header.ErrLineTooLong},
		{"header count limit", strings.NewReader("GET / HTTP/1.1\r\n" + strings.Repeat("X: a\r\n", requestHeaderLimits.Fields+1) + "\r\n"), header.ErrTooManyFields},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := ReadRequest(bufio.NewReader(test.reader))
			require.ErrorIs(t, err, test.want)
			require.Nil(t, request)
		})
	}

	_, err := ReadRequest(bufio.NewReader(strings.NewReader("GET / HTTP/1.1\n")))
	require.ErrorContains(t, err, "CRLF")
}

func TestReadRequestBeforeConnectionCloses(t *testing.T) {
	source, client := net.Pipe()
	t.Cleanup(func() {
		_ = source.Close()
		_ = client.Close()
	})

	type result struct {
		request *Request
		err     error
	}
	parsed := make(chan result, 1)
	go func() {
		request, err := ReadRequest(bufio.NewReader(source))
		parsed <- result{request, err}
	}()
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(client, "GET /live HTTP/1.1\r\nHost: example.com\r\n\r\n")
		written <- err
	}()

	select {
	case got := <-parsed:
		require.NoError(t, got.err)
		require.Equal(t, "/live", got.request.RequestLine.RequestTarget)
		require.Equal(t, header.Section{"host": {"example.com"}}, got.request.Headers)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "parser waited for connection EOF after complete headers")
	}
	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "request headers write did not finish")
	}
}

func TestReadRequestHeadersAndUnfolding(t *testing.T) {
	message := "GET / HTTP/1.1\r\n" +
		"X-Test: first \t\r\n\t second  \tpart \t\r\n  third:fourth\r\n" +
		"Host: example.com\r\nx-test: \"a,b\"\r\nX-Empty:\r\n\t\r\n" +
		"X-Bytes: \x80\xff\xc3\r\n" +
		"Set-Cookie: a=1; Expires=Wed, 09 Jun 2027 10:18:14 GMT\r\n" +
		"X-Test: last\r\nset-cookie: b=2\r\n\r\nbody"
	want := header.Section{
		"x-test":     {"first second  \tpart third:fourth", "\"a,b\"", "last"},
		"host":       {"example.com"},
		"x-empty":    {""},
		"x-bytes":    {"\x80\xff\xc3"},
		"set-cookie": {"a=1; Expires=Wed, 09 Jun 2027 10:18:14 GMT", "b=2"},
	}
	for _, test := range []struct {
		name   string
		reader io.Reader
	}{
		{"buffered", strings.NewReader(message)},
		{"one byte per read", iotest.OneByteReader(strings.NewReader(message))},
		{"data with EOF", iotest.DataErrReader(strings.NewReader(message))},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := bufio.NewReaderSize(test.reader, 16)
			request, err := ReadRequest(reader)
			require.NoError(t, err)
			require.Equal(t, want, request.Headers)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "body", string(body))
		})
	}
}

func TestReadRequestHeaderFragmentation(t *testing.T) {
	message := "GET / HTTP/1.1\r\nX: first\r\n\tsecond\r\nY: last\r\nHost: example.com\r\n\r\nbody"
	for split := 1; split < len(message); split++ {
		t.Run(fmt.Sprintf("split_%d", split), func(t *testing.T) {
			source := io.MultiReader(strings.NewReader(message[:split]), strings.NewReader(message[split:]))
			reader := bufio.NewReaderSize(source, 16)
			request, err := ReadRequest(reader)
			require.NoError(t, err)
			require.Equal(t, header.Section{"x": {"first second"}, "y": {"last"}, "host": {"example.com"}}, request.Headers)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "body", string(body))
		})
	}
}

func TestReadRequestRejectsMalformedHeaders(t *testing.T) {
	for _, section := range []string{
		"Host : example.com\r\n\r\n",
		"Missing-Colon\r\n\r\n",
		": value\r\n\r\n",
		" first\r\n\r\n",
		"\t\r\n\r\n",
		"X: ok\r\nBad Name: value\r\n\r\n",
		"X: a\x00b\r\n\r\n",
		"X: ok\r\n bad\x7fvalue\r\n\r\n",
		"X: ok\n\r\n",
		"X: bad\rvalue\r\n\r\n",
		"X: ok\r\n\n",
	} {
		t.Run(fmt.Sprintf("%q", section), func(t *testing.T) {
			request, err := ReadRequest(bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\n" + section)))
			require.Error(t, err)
			require.Nil(t, request)
		})
	}
}
