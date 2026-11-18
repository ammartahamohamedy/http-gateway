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
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

// collectInterim returns an onInterim callback that appends each interim
// response to interims.
func collectInterim(interims *[]*Message) func(*Message) error {
	return func(interim *Message) error {
		*interims = append(*interims, interim)
		return nil
	}
}

func TestRead(t *testing.T) {
	const raw = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"

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

			message, err := Read(reader, "GET", nil)
			require.NoError(t, err)
			require.Equal(t, Line{Version: "1.1", StatusCode: 200, ReasonPhrase: "OK"}, message.Line)
			require.Equal(t, header.Section{
				"content-type":   {"application/json"},
				"content-length": {"2"},
			}, message.Headers)

			rest, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, "{}", string(rest))
		})
	}
}

func TestReadWithoutHeaderFields(t *testing.T) {
	message, err := Read(bufio.NewReader(strings.NewReader("HTTP/1.1 204 \r\n\r\n")), "GET", nil)
	require.NoError(t, err)
	require.Equal(t, Line{Version: "1.1", StatusCode: 204}, message.Line)
	require.Nil(t, message.Headers)
}

func TestReadKeepsRepeatedSetCookieValues(t *testing.T) {
	const raw = "HTTP/1.1 200 OK\r\n" +
		"Set-Cookie: session=a; Expires=Wed, 21 Oct 2026 07:28:00 GMT\r\n" +
		"Set-Cookie: theme=dark, light; Path=/\r\n\r\n"

	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		"session=a; Expires=Wed, 21 Oct 2026 07:28:00 GMT",
		"theme=dark, light; Path=/",
	}, message.Headers.Values("Set-Cookie"))
}

func TestReadPassesInterimResponses(t *testing.T) {
	const raw = "HTTP/1.1 100 Continue\r\n\r\n" +
		"HTTP/1.1 102 Processing\r\n\r\n" +
		"HTTP/1.1 103 Early Hints\r\nLink: </style.css>; rel=preload\r\n\r\n" +
		"HTTP/1.1 199 \r\n\r\n" +
		"HTTP/1.1 201 Created\r\nLocation: /documents/1\r\n\r\n"

	var interims []*Message
	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "POST", collectInterim(&interims))
	require.NoError(t, err)

	require.Equal(t, []*Message{
		{Line: Line{Version: "1.1", StatusCode: 100, ReasonPhrase: "Continue"}},
		{Line: Line{Version: "1.1", StatusCode: 102, ReasonPhrase: "Processing"}},
		{
			Line:    Line{Version: "1.1", StatusCode: 103, ReasonPhrase: "Early Hints"},
			Headers: header.Section{"link": {"</style.css>; rel=preload"}},
		},
		{Line: Line{Version: "1.1", StatusCode: 199}},
	}, interims)

	require.Equal(t, Line{Version: "1.1", StatusCode: 201, ReasonPhrase: "Created"}, message.Line)
	require.Equal(t, header.Section{"location": {"/documents/1"}}, message.Headers)
}

func TestReadPassesEachInterimResponseSeparately(t *testing.T) {
	const raw = "HTTP/1.1 103 Early Hints\r\nLink: </a.css>\r\n\r\n" +
		"HTTP/1.1 103 Early Hints\r\nLink: </b.css>\r\n\r\n" +
		"HTTP/1.1 200 OK\r\n\r\n"

	var interims []*Message
	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", collectInterim(&interims))
	require.NoError(t, err)

	require.Len(t, interims, 2)
	require.Equal(t, []string{"</a.css>"}, interims[0].Headers.Values("Link"))
	require.Equal(t, []string{"</b.css>"}, interims[1].Headers.Values("Link"))
	require.Nil(t, message.Headers)
}

func TestReadDiscardsInterimResponsesWithoutCallback(t *testing.T) {
	const raw = "HTTP/1.1 100 Continue\r\nX-Interim: 1\r\n\r\nHTTP/1.1 200 OK\r\n\r\n"

	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", nil)
	require.NoError(t, err)
	require.Equal(t, 200, message.Line.StatusCode)
	require.Nil(t, message.Headers)
}

func TestReadInterimResponseLimit(t *testing.T) {
	const interim = "HTTP/1.1 100 Continue\r\n\r\n"
	const final = "HTTP/1.1 200 OK\r\n\r\n"

	var interims []*Message
	atLimit := strings.Repeat(interim, maxInterimResponses) + final

	message, err := Read(bufio.NewReader(strings.NewReader(atLimit)), "GET", collectInterim(&interims))
	require.NoError(t, err)
	require.Equal(t, 200, message.Line.StatusCode)
	require.Len(t, interims, maxInterimResponses)

	interims = nil
	overLimit := strings.Repeat(interim, maxInterimResponses+1) + final

	message, err = Read(bufio.NewReader(strings.NewReader(overLimit)), "GET", collectInterim(&interims))
	require.ErrorIs(t, err, errTooManyInterimResponses)
	requireStage(t, err, http1.StageStatusLine)
	require.Nil(t, message)
	require.Len(t, interims, maxInterimResponses)
}

func TestReadStopsOnInterimCallbackError(t *testing.T) {
	const raw = "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\n\r\n"

	callbackErr := errors.New("forwarding failed")
	reader := bufio.NewReader(strings.NewReader(raw))

	message, err := Read(reader, "GET", func(*Message) error { return callbackErr })
	require.Same(t, callbackErr, err)
	require.Nil(t, message)

	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "HTTP/1.1 200 OK\r\n\r\n", string(rest))
}

func TestReadRejectsSwitchingProtocols(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"as first response", "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\n\r\n"},
		{"after interim response", "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 101 Switching Protocols\r\n\r\n"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, err := Read(bufio.NewReader(strings.NewReader(test.raw)), "GET", nil)
			require.ErrorIs(t, err, errSwitchingProtocols)
			requireStage(t, err, http1.StageStatusLine)
			require.Nil(t, message)
		})
	}
}

func TestReadTagsFailedStage(t *testing.T) {
	readErr := errors.New("read failed")
	oversizedField := "X: " + strings.Repeat("a", headerLimits.LineBytes)

	tests := []struct {
		name   string
		source io.Reader
		stage  http1.Stage
		want   error
	}{
		{"EOF before status line", strings.NewReader(""), http1.StageStatusLine, io.EOF},
		{"EOF within status line", strings.NewReader("HTTP/1.1 200 OK"), http1.StageStatusLine, io.ErrUnexpectedEOF},
		{"bare LF status line", strings.NewReader("HTTP/1.1 200 OK\n\r\n"), http1.StageStatusLine, http1.ErrInvalidLineEnding},
		{"status line too long", strings.NewReader("HTTP/1.1 200 " + strings.Repeat("a", maxLineBytes) + "\r\n\r\n"), http1.StageStatusLine, errLineTooLong},
		{"read failure", iotest.ErrReader(readErr), http1.StageStatusLine, readErr},
		{"EOF before final status line", strings.NewReader("HTTP/1.1 100 Continue\r\n\r\n"), http1.StageStatusLine, io.ErrUnexpectedEOF},
		{"EOF within headers", strings.NewReader("HTTP/1.1 200 OK\r\nServer: backend\r\n"), http1.StageHeaders, io.ErrUnexpectedEOF},
		{"EOF within interim headers", strings.NewReader("HTTP/1.1 100 Continue\r\n"), http1.StageHeaders, io.ErrUnexpectedEOF},
		{"read failure in headers", io.MultiReader(strings.NewReader("HTTP/1.1 200 OK\r\n"), iotest.ErrReader(readErr)), http1.StageHeaders, readErr},
		{"header line limit", strings.NewReader("HTTP/1.1 200 OK\r\n" + oversizedField + "\r\n\r\n"), http1.StageHeaders, header.ErrLineTooLong},
		{"header field limit", strings.NewReader("HTTP/1.1 200 OK\r\n" + strings.Repeat("X: a\r\n", headerLimits.Fields+1) + "\r\n"), http1.StageHeaders, header.ErrTooManyFields},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, err := Read(bufio.NewReader(test.source), "GET", nil)
			require.ErrorIs(t, err, test.want)
			requireStage(t, err, test.stage)
			require.Nil(t, message)
		})
	}
}

func TestReadTagsMalformedInputByStage(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		stage http1.Stage
	}{
		{"HTTP 1.0 status line", "HTTP/1.0 200 OK\r\n\r\n", http1.StageStatusLine},
		{"missing space after code", "HTTP/1.1 204\r\n\r\n", http1.StageStatusLine},
		{"code out of range", "HTTP/1.1 600 \r\n\r\n", http1.StageStatusLine},
		{"leading empty line", "\r\nHTTP/1.1 200 OK\r\n\r\n", http1.StageStatusLine},
		{"malformed final status line after interim", "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 2x0 OK\r\n\r\n", http1.StageStatusLine},
		{"whitespace before first field", "HTTP/1.1 200 OK\r\n Server: backend\r\n\r\n", http1.StageHeaders},
		{"field without colon", "HTTP/1.1 200 OK\r\nServer\r\n\r\n", http1.StageHeaders},
		{"malformed interim field", "HTTP/1.1 103 Early Hints\r\nLink : </a.css>\r\n\r\nHTTP/1.1 200 OK\r\n\r\n", http1.StageHeaders},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			message, err := Read(bufio.NewReader(strings.NewReader(test.raw)), "GET", nil)
			requireStage(t, err, test.stage)
			require.Nil(t, message)
		})
	}
}

func TestReadUnfoldsObsoleteLineFolding(t *testing.T) {
	const raw = "HTTP/1.1 200 OK\r\nX-Folded: first\r\n  second\r\n\r\n"

	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"first second"}, message.Headers.Values("X-Folded"))
}

func TestReadStageErrorMessageIsUnchanged(t *testing.T) {
	_, parseErr := parseLine("HTTP/1.0 200 OK")

	_, err := Read(bufio.NewReader(strings.NewReader("HTTP/1.0 200 OK\r\n\r\n")), "GET", nil)
	require.EqualError(t, err, parseErr.Error())
}

// requireStage asserts that err is a *http1.StageError for stage.
func requireStage(t *testing.T, err error, stage http1.Stage) {
	t.Helper()

	var stageErr *http1.StageError
	require.ErrorAs(t, err, &stageErr)
	require.Equal(t, stage, stageErr.Stage)
}
