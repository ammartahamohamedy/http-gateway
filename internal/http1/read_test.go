package http1

import (
	"bufio"
	"errors"
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
	parsed, err := ReadRequest(bufio.NewReader(strings.NewReader(rawLine + "\r\n")))
	require.NoError(t, err)
	require.Equal(t, &Request{RequestLine: line}, parsed)
	parsed, err = ReadRequest(bufio.NewReaderSize(strings.NewReader(rawLine+"\r\n"), maxRequestLineBytes+1))
	require.NoError(t, err)
	require.Equal(t, &Request{RequestLine: line}, parsed)
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
		{"CRLF", strings.NewReader("GET / HTTP/1.1\r\n")},
		{"leading empty lines", strings.NewReader("\r\n\r\nGET / HTTP/1.1\r\n")},
		{"one byte per read", iotest.OneByteReader(strings.NewReader("GET / HTTP/1.1\r\n"))},
		{"data with EOF", iotest.DataErrReader(strings.NewReader("GET / HTTP/1.1\r\n"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := ReadRequest(bufio.NewReaderSize(test.reader, 16))
			require.NoError(t, err)
			require.Equal(t, &Request{RequestLine: RequestLine{Method: "GET", RequestTarget: "/", Version: "1.1"}}, request)
		})
	}
}

func TestReadRequestPreservesRemainingBytes(t *testing.T) {
	remainder := "Host: example.com\r\n\r\nbody\x00bytes"
	reader := bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\n" + remainder))
	request, err := ReadRequest(reader)
	require.NoError(t, err)
	require.Nil(t, request.Headers)
	require.Nil(t, request.Body)
	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, remainder, string(remaining))
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
		{"read failure", iotest.ErrReader(readErr), readErr},
		{"read failure after bytes", io.MultiReader(strings.NewReader("GET /"), iotest.ErrReader(readErr)), readErr},
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
		_, err := io.WriteString(client, "GET /live HTTP/1.1\r\n")
		written <- err
	}()

	select {
	case got := <-parsed:
		require.NoError(t, got.err)
		require.Equal(t, "/live", got.request.RequestLine.RequestTarget)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "parser waited for connection EOF after a complete request-line")
	}
	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "request-line write did not finish")
	}
}
