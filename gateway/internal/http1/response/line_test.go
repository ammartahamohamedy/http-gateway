package response

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/stretchr/testify/require"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name         string
		rawLine      string
		statusCode   int
		reasonPhrase string
	}{
		{"registered phrase", "HTTP/1.1 200 OK", 200, "OK"},
		{"phrase with spaces", "HTTP/1.1 404 Not Found", 404, "Not Found"},
		{"empty phrase after required space", "HTTP/1.1 204 ", 204, ""},
		{"unregistered phrase preserved", "HTTP/1.1 200 Fine", 200, "Fine"},
		{"leading phrase space preserved", "HTTP/1.1 200  OK", 200, " OK"},
		{"trailing phrase space preserved", "HTTP/1.1 200 OK ", 200, "OK "},
		{"phrase with HTAB", "HTTP/1.1 200 O\tK", 200, "O\tK"},
		{"phrase with obs-text", "HTTP/1.1 200 \xd8\xaa\xd9\x85\xff", 200, "\xd8\xaa\xd9\x85\xff"},
		{"phrase with colon and digits", "HTTP/1.1 503 Retry: 120 s", 503, "Retry: 120 s"},
		{"interim response", "HTTP/1.1 100 Continue", 100, "Continue"},
		{"unregistered interim code", "HTTP/1.1 199 ", 199, ""},
		{"unregistered success code", "HTTP/1.1 299 Custom", 299, "Custom"},
		{"unused registry code", "HTTP/1.1 306 ", 306, ""},
		{"lowest code", "HTTP/1.1 100 ", 100, ""},
		{"highest code", "HTTP/1.1 599 ", 599, ""},
		{"higher minor version processed as 1.1", "HTTP/1.2 200 OK", 200, "OK"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			line, err := parseLine(test.rawLine)
			require.NoError(t, err)
			require.Equal(t, Line{
				Version:      "1.1",
				StatusCode:   test.statusCode,
				ReasonPhrase: test.reasonPhrase,
			}, line)
		})
	}
}

func TestParseLineRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		rawLine string
		message string
	}{
		{"empty line", "", "space after the HTTP version and after the status code"},
		{"version only", "HTTP/1.1", "space after the HTTP version and after the status code"},
		{"missing space after code", "HTTP/1.1 200", "space after the HTTP version and after the status code"},
		{"tab after version", "HTTP/1.1\t200 OK", "space after the HTTP version and after the status code"},
		{"tab after code", "HTTP/1.1 200\tOK", "space after the HTTP version and after the status code"},
		{"leading space", " HTTP/1.1 200 OK", "invalid HTTP version"},
		{"repeated space before code", "HTTP/1.1  200 OK", "is not three digits"},
		{"lowercase version", "http/1.1 200 OK", "invalid HTTP version"},
		{"short version", "HTTP/1 200 OK", "invalid HTTP version"},
		{"HTTP 1.0 unsupported", "HTTP/1.0 200 OK", "unsupported HTTP version"},
		{"HTTP 2.0 unsupported", "HTTP/2.0 200 OK", "unsupported HTTP version"},
		{"two-digit code", "HTTP/1.1 20 OK", "is not three digits"},
		{"four-digit code", "HTTP/1.1 2000 OK", "is not three digits"},
		{"signed code", "HTTP/1.1 +20 OK", "is not three digits"},
		{"letter in code", "HTTP/1.1 2x0 OK", "is not three digits"},
		{"code below range", "HTTP/1.1 099 OK", "is outside 100-599"},
		{"code above range", "HTTP/1.1 600 OK", "is outside 100-599"},
		{"phrase NUL", "HTTP/1.1 200 O\x00K", "invalid reason phrase"},
		{"phrase control byte", "HTTP/1.1 200 O\x01K", "invalid reason phrase"},
		{"phrase DEL", "HTTP/1.1 200 O\x7fK", "invalid reason phrase"},
		{"phrase bare CR", "HTTP/1.1 200 O\rK", "invalid reason phrase"},
		{"phrase bare LF", "HTTP/1.1 200 O\nK", "invalid reason phrase"},
		{"unexpected terminator", "HTTP/1.1 200 OK\r\n", "invalid reason phrase"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			line, err := parseLine(test.rawLine)
			require.ErrorContains(t, err, test.message)
			require.Equal(t, Line{}, line)
		})
	}
}

func TestParseLineDoesNotEnforceReadLimit(t *testing.T) {
	reasonPhrase := strings.Repeat("a", maxLineBytes)

	line, err := parseLine("HTTP/1.1 200 " + reasonPhrase)
	require.NoError(t, err)
	require.Equal(t, Line{Version: "1.1", StatusCode: 200, ReasonPhrase: reasonPhrase}, line)
}

func TestReadLine(t *testing.T) {
	const raw = "HTTP/1.1 200 OK\r\nServer: backend\r\n"

	tests := []struct {
		name   string
		source io.Reader
	}{
		{"whole input", strings.NewReader(raw)},
		{"one byte per read", iotest.OneByteReader(strings.NewReader(raw))},
		{"data with EOF", iotest.DataErrReader(strings.NewReader(raw))},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			reader := bufio.NewReader(test.source)

			line, err := readLine(reader)
			require.NoError(t, err)
			require.Equal(t, Line{Version: "1.1", StatusCode: 200, ReasonPhrase: "OK"}, line)

			rest, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "Server: backend\r\n", string(rest))
		})
	}
}

func TestReadLineLimit(t *testing.T) {
	const prefix = "HTTP/1.1 200 "

	atLimit := prefix + strings.Repeat("a", maxLineBytes-len(prefix))
	overLimit := atLimit + "a"

	line, err := readLine(bufio.NewReader(strings.NewReader(atLimit + "\r\n")))
	require.NoError(t, err)
	require.Equal(t, atLimit[len(prefix):], line.ReasonPhrase)

	for raw := range slices.Values([]string{overLimit + "\r\n", overLimit, overLimit + "\r"}) {
		line, err = readLine(bufio.NewReader(strings.NewReader(raw)))
		require.ErrorIs(t, err, errLineTooLong)
		require.Equal(t, Line{}, line)
	}
}

func TestReadLineErrors(t *testing.T) {
	readErr := errors.New("read failed")

	tests := []struct {
		name   string
		source io.Reader
		want   error
	}{
		{"EOF before status line", strings.NewReader(""), io.EOF},
		{"EOF within status line", strings.NewReader("HTTP/1.1 200 OK"), io.ErrUnexpectedEOF},
		{"EOF after CR", strings.NewReader("HTTP/1.1 200 OK\r"), io.ErrUnexpectedEOF},
		{"bare LF terminator", strings.NewReader("HTTP/1.1 200 OK\n"), http1.ErrInvalidLineEnding},
		{"bare CR within line", strings.NewReader("HTTP/1.1 200 O\rK\r\n"), http1.ErrInvalidLineEnding},
		{"read failure", iotest.ErrReader(readErr), readErr},
		{"read failure after bytes", io.MultiReader(strings.NewReader("HTTP/1.1"), iotest.ErrReader(readErr)), readErr},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			line, err := readLine(bufio.NewReader(test.source))
			require.ErrorIs(t, err, test.want)
			require.ErrorContains(t, err, "reading status line")
			require.Equal(t, Line{}, line)
		})
	}
}

func TestReadLineDoesNotSkipEmptyLines(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\r\nHTTP/1.1 200 OK\r\n"))

	line, err := readLine(reader)
	require.ErrorContains(t, err, "space after the HTTP version and after the status code")
	require.Equal(t, Line{}, line)
}

func TestReadLineReturnsParseErrorsUnchanged(t *testing.T) {
	_, parseErr := parseLine("HTTP/1.0 200 OK")

	_, err := readLine(bufio.NewReader(strings.NewReader("HTTP/1.0 200 OK\r\n")))
	require.EqualError(t, err, parseErr.Error())
}

func TestParseLineUnsupportedVersionsMatchTheSentinel(t *testing.T) {
	for rawLine := range slices.Values([]string{"HTTP/1.0 200 OK", "HTTP/2.0 200 OK"}) {
		t.Run(rawLine, func(t *testing.T) {
			_, err := parseLine(rawLine)
			require.ErrorIs(t, err, http1.ErrUnsupportedVersion)
		})
	}
}
