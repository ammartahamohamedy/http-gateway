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
	require.Equal(t, &Request{RequestLine: line, Headers: Headers{"host": {"example.com"}}}, parsed)
	parsed, err = ReadRequest(bufio.NewReaderSize(strings.NewReader(rawLine+"\r\nHost: example.com\r\n\r\n"), maxRequestLineBytes+1))
	require.NoError(t, err)
	require.Equal(t, &Request{RequestLine: line, Headers: Headers{"host": {"example.com"}}}, parsed)
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
				Headers:     Headers{"host": {"example.com"}},
			}, request)
		})
	}
}

func TestReadRequestPreservesRemainingBytes(t *testing.T) {
	remainder := "body\x00bytes\r\nGET /next HTTP/1.1\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n" + remainder))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	require.Equal(t, Headers{"host": {"example.com"}}, request.Headers)
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
		{"missing", "GET / HTTP/1.1", "", "", ErrInvalidHost},
		{"empty origin host", "GET / HTTP/1.1", "Host:", "", ErrInvalidHost},
		{"whitespace-only origin host", "GET / HTTP/1.1", "Host: \t", "", ErrInvalidHost},
		{"empty absolute HTTP host", "GET http://example.com/ HTTP/1.1", "Host:", "", ErrInvalidHost},
		{"empty asterisk host", "OPTIONS * HTTP/1.1", "Host:", "", ErrInvalidHost},
		{"empty CONNECT host", "CONNECT example.com:443 HTTP/1.1", "Host:", "", ErrInvalidHost},
		{"port without host", "GET / HTTP/1.1", "Host: :80", "", ErrInvalidHost},
		{"identical duplicates", "GET / HTTP/1.1", "Host: example.com\r\nHost: example.com", "", ErrInvalidHost},
		{"mixed case duplicates", "GET / HTTP/1.1", "Host: example.com\r\nX: value\r\nhOsT: other.example", "", ErrInvalidHost},
		{"comma and space", "GET / HTTP/1.1", "Host: example.com, other.example", "", ErrInvalidHost},
		{"userinfo", "GET / HTTP/1.1", "Host: user@example.com", "", ErrInvalidHost},
		{"URI instead of authority", "GET / HTTP/1.1", "Host: http://example.com", "", ErrInvalidHost},
		{"invalid port", "GET / HTTP/1.1", "Host: example.com:http", "", ErrInvalidHost},
		{"unbracketed IPv6", "GET / HTTP/1.1", "Host: 2001:db8::1", "", ErrInvalidHost},
		{"malformed IPv6", "GET / HTTP/1.1", "Host: [2001:db8::g]", "", ErrInvalidHost},
		{"fold inside authority", "GET / HTTP/1.1", "Host: example.\r\n com", "", ErrInvalidHost},
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
		{"header line limit", strings.NewReader("GET / HTTP/1.1\r\nX:" + strings.Repeat("a", maxHeaderLineBytes) + "\r\n"), ErrHeaderLineTooLong},
		{"header count limit", strings.NewReader("GET / HTTP/1.1\r\n" + strings.Repeat("X: a\r\n", maxHeaderFields+1) + "\r\n"), ErrTooManyHeaders},
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
		require.Equal(t, Headers{"host": {"example.com"}}, got.request.Headers)
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
	want := Headers{
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
			require.Equal(t, Headers{"x": {"first second"}, "y": {"last"}, "host": {"example.com"}}, request.Headers)
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

func TestProcessHeadersLimits(t *testing.T) {
	fieldLine := func(size int) string { return "X: " + strings.Repeat("a", size-3) }
	prefix := strings.Repeat(fieldLine(maxHeaderLineBytes)+"\r\n", 3)
	lastLineSize := maxHeaderSectionBytes - len(prefix) - 4
	exactSection := prefix + fieldLine(lastLineSize) + "\r\n\r\n"
	require.Len(t, exactSection, maxHeaderSectionBytes)
	tests := []struct {
		name    string
		section string
		count   int
		wantErr error
	}{
		{"line at limit", fieldLine(maxHeaderLineBytes) + "\r\n\r\n", 1, nil},
		{"line over limit", fieldLine(maxHeaderLineBytes+1) + "\r\n\r\n", 0, ErrHeaderLineTooLong},
		{"oversized unterminated line", fieldLine(maxHeaderLineBytes + 1), 0, ErrHeaderLineTooLong},
		{"oversized partial terminator", fieldLine(maxHeaderLineBytes+1) + "\r", 0, ErrHeaderLineTooLong},
		{"section at limit", exactSection, 4, nil},
		{"section over limit", prefix + fieldLine(lastLineSize+1) + "\r\n\r\n", 0, ErrHeadersTooLarge},
		{"section budget while reading", prefix + fieldLine(lastLineSize+3), 0, ErrHeadersTooLarge},
		{"count at limit", strings.Repeat("X: a\r\n", maxHeaderFields) + "\r\n", maxHeaderFields, nil},
		{"count over limit", strings.Repeat("X: a\r\n", maxHeaderFields+1) + "\r\n", 0, ErrTooManyHeaders},
		{"mixed case count over limit", strings.Repeat("X: a\r\nx: b\r\n", maxHeaderFields/2) + "X: c\r\n\r\n", 0, ErrTooManyHeaders},
		{"continuations are not fields", "X: a\r\n" + strings.Repeat(" b\r\n", maxHeaderFields+1) + "\r\n", 1, nil},
		{"large unfolded field", fieldLine(maxHeaderLineBytes) + "\r\n " + strings.Repeat("b", maxHeaderLineBytes-1) + "\r\n\r\n", 1, nil},
		{"oversized continuation", "X: a\r\n " + strings.Repeat("b", maxHeaderLineBytes) + "\r\n\r\n", 0, ErrHeaderLineTooLong},
		{"continuations use section budget", "X: a\r\n" + strings.Repeat(" \t\r\n", maxHeaderSectionBytes/4), 0, ErrHeadersTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			headers, err := processHeaders(bufio.NewReaderSize(strings.NewReader(test.section), 16))
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Nil(t, headers)
				return
			}
			require.NoError(t, err)
			count := 0
			for _, values := range headers {
				count += len(values)
			}
			require.Equal(t, test.count, count)
		})
	}
}
