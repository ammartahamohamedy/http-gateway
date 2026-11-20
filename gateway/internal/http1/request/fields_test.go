package request

import (
	"bufio"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

var originLine = Line{Method: "GET", RequestTarget: "/", Version: "1.1"}

func TestFieldRulesOrder(t *testing.T) {
	var names []string
	for rule := range slices.Values(fieldRules) {
		names = append(names, rule.name)
	}

	require.Equal(t, []string{
		"Host", "Content-Length", "Transfer-Encoding", "Trailer", "Connection", "Expect",
		"Max-Forwards", "Content-Type", "Accept", "Content-Encoding", "Accept-Encoding",
		"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers",
	}, names)
}

func TestProcessFieldsFirstErrorIsDeterministic(t *testing.T) {
	headers := header.Section{"host": {"a", "b"}, "content-length": {"x"}}
	for range 20 {
		_, err := processFields(originLine, headers)
		var fieldErr *http1.FieldError
		require.ErrorAs(t, err, &fieldErr)
		require.Equal(t, "Host", fieldErr.Name)
	}
}

func TestFieldErrorWrapping(t *testing.T) {
	_, err := processFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"-1"}})
	require.ErrorIs(t, err, fields.ErrInvalidContentLength)
	var fieldErr *http1.FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Content-Length", fieldErr.Name)
	require.Equal(t, fields.ErrInvalidContentLength.Error(), err.Error())

	_, err = Read(bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\n\r\n")))
	require.ErrorIs(t, err, fields.ErrInvalidHost)
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Host", fieldErr.Name)
}

func TestReadFields(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    Fields
	}{
		{"absent length", "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
			Fields{Authority: "example.com"}},
		{"zero length", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\n\r\n",
			Fields{Authority: "example.com", ContentLengthPresent: true}},
		{"positive length", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0042\r\n\r\n",
			Fields{Authority: "example.com", ContentLength: 42, ContentLengthPresent: true}},
		{"absolute authority overrides Host", "GET http://target.example:8080/ HTTP/1.1\r\nHost: other.example\r\n\r\n",
			Fields{Authority: "target.example:8080"}},
		{"CONNECT authority", "CONNECT example.com:443 HTTP/1.1\r\nHost: other.example:443\r\n\r\n",
			Fields{Authority: "example.com:443"}},
		{"authority-less absolute", "GET urn:example:item HTTP/1.1\r\nHost:\r\n\r\n",
			Fields{}},
		{"transfer codings", "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: GZIP\r\nTransfer-Encoding: Chunked\r\n\r\n",
			Fields{Authority: "example.com", TransferCodings: []string{"gzip", "chunked"}}},
		{"connection options", "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: Close\r\nConnection: TE\r\n\r\n",
			Fields{Authority: "example.com", ConnectionOptions: []string{"close", "te"}}},
		{"expect with length", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nExpect: 100-Continue\r\n\r\n",
			Fields{Authority: "example.com", ContentLength: 5, ContentLengthPresent: true, Expectations: []string{"100-continue"}}},
		{"expect with chunked framing", "POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\nExpect: 100-continue\r\n\r\n",
			Fields{Authority: "example.com", TransferCodings: []string{"chunked"}, Expectations: []string{"100-continue"}}},
		{"unfamiliar expectation without content", "GET / HTTP/1.1\r\nHost: example.com\r\nExpect: x-custom\r\n\r\n",
			Fields{Authority: "example.com", Expectations: []string{"x-custom"}}},
		{"preflight fields", "OPTIONS / HTTP/1.1\r\nHost: example.com\r\nOrigin: https://app.example\r\nAccess-Control-Request-Method: PUT\r\nAccess-Control-Request-Headers: X-One, Content-Type\r\n\r\n",
			Fields{Authority: "example.com", Origin: "https://app.example", AccessControlRequestMethod: "PUT",
				AccessControlRequestHeaders: []string{"x-one", "content-type"}}},
		{"request fields without preflight context", "GET / HTTP/1.1\r\nHost: example.com\r\nAccess-Control-Request-Method: PUT\r\n\r\n",
			Fields{Authority: "example.com", AccessControlRequestMethod: "PUT"}},
		{"unknown valid fields accepted", "GET / HTTP/1.1\r\nHost: example.com\r\nX-Unfamiliar: \"a,b\"\r\nX-Unfamiliar: c\r\n\r\n",
			Fields{Authority: "example.com"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			request, err := Read(bufio.NewReader(strings.NewReader(test.message)))
			require.NoError(t, err)
			require.Equal(t, test.want, request.Fields)
		})
	}
}

func TestProcessFieldsReturnsZeroOnFailure(t *testing.T) {
	// Host succeeds and assigns Authority before Content-Length fails.
	parsed, err := processFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"1, 1"}})
	require.Error(t, err)
	require.Zero(t, parsed)
}

func TestCrossChecksRunAfterFieldRules(t *testing.T) {
	saved := crossChecks
	t.Cleanup(func() { crossChecks = saved })

	checkErr := errors.New("cross-check failed")
	var seen []Fields
	crossChecks = []crossCheck{{
		name: "Content-Length",
		apply: func(_ Line, _ header.Section, parsed Fields) error {
			seen = append(seen, parsed)
			return checkErr
		},
	}}

	_, err := processFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"x"}})
	require.ErrorIs(t, err, fields.ErrInvalidContentLength)
	require.Empty(t, seen, "cross-checks must not run after a field rule fails")

	parsed, err := processFields(originLine, header.Section{"host": {"example.com"}, "content-length": {"7"}})
	require.ErrorIs(t, err, checkErr)
	require.Zero(t, parsed)
	var fieldErr *http1.FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Content-Length", fieldErr.Name)
	require.Equal(t, []Fields{{Authority: "example.com", ContentLength: 7, ContentLengthPresent: true}}, seen)
}

func TestTransferEncodingErrors(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"malformed list", "chun ked", fields.ErrInvalidTransferEncoding},
		{"chunked not final", "gzip", fields.ErrChunkedNotFinal},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			headers := header.Section{"host": {"example.com"}, "transfer-encoding": {test.value}}
			parsed, err := processFields(originLine, headers)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Transfer-Encoding", fieldErr.Name)
		})
	}
}

func TestFramingConflict(t *testing.T) {
	tests := []struct {
		name    string
		headers header.Section
	}{
		{"positive length", header.Section{"host": {"example.com"}, "content-length": {"5"}, "transfer-encoding": {"chunked"}}},
		{"zero length", header.Section{"host": {"example.com"}, "content-length": {"0"}, "transfer-encoding": {"chunked"}}},
		{"coding list", header.Section{"host": {"example.com"}, "content-length": {"5"}, "transfer-encoding": {"gzip, chunked"}}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(originLine, test.headers)
			require.ErrorIs(t, err, fields.ErrFramingConflict)
			require.NotErrorIs(t, err, fields.ErrInvalidTransferEncoding)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Transfer-Encoding", fieldErr.Name)
		})
	}
}

func TestInvalidTransferEncodingPrecedesFramingConflict(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"malformed list", "chun ked", fields.ErrInvalidTransferEncoding},
		{"chunked not final", "xchunked", fields.ErrChunkedNotFinal},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			headers := header.Section{"host": {"example.com"}, "content-length": {"5"}, "transfer-encoding": {test.value}}
			_, err := processFields(originLine, headers)
			require.ErrorIs(t, err, test.want)
			require.NotErrorIs(t, err, fields.ErrFramingConflict)
		})
	}
}

func TestReadRejectsFramingConflict(t *testing.T) {
	message := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n"
	request, err := Read(bufio.NewReader(strings.NewReader(message)))
	require.ErrorIs(t, err, fields.ErrFramingConflict)
	require.Nil(t, request)
}

func TestConnectionErrors(t *testing.T) {
	headers := header.Section{"host": {"example.com"}, "connection": {"keep alive"}}
	parsed, err := processFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidConnection)
	require.Zero(t, parsed)
	var fieldErr *http1.FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Connection", fieldErr.Name)
}

func TestConnectionNominatingProtectedField(t *testing.T) {
	for value := range slices.Values([]string{"host", "Content-Length", "close, Transfer-Encoding"}) {
		t.Run(value, func(t *testing.T) {
			headers := header.Section{"host": {"example.com"}, "connection": {value}}
			parsed, err := processFields(originLine, headers)
			require.ErrorIs(t, err, fields.ErrProtectedConnectionOption)
			require.NotErrorIs(t, err, fields.ErrInvalidConnection)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Connection", fieldErr.Name)
		})
	}
}

func TestFramingConflictPrecedesConnectionCheck(t *testing.T) {
	headers := header.Section{
		"host":              {"example.com"},
		"content-length":    {"5"},
		"transfer-encoding": {"chunked"},
		"connection":        {"content-length"},
	}

	_, err := processFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrFramingConflict)
}

func TestReadRejectsProtectedConnectionOption(t *testing.T) {
	message := "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: host\r\n\r\n"
	request, err := Read(bufio.NewReader(strings.NewReader(message)))
	require.ErrorIs(t, err, fields.ErrProtectedConnectionOption)
	require.Nil(t, request)
}

func TestExpectErrors(t *testing.T) {
	headers := header.Section{"host": {"example.com"}, "expect": {"100 continue"}}
	parsed, err := processFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidExpect)
	require.Zero(t, parsed)
	var fieldErr *http1.FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Expect", fieldErr.Name)
}

func TestExpectWithoutContent(t *testing.T) {
	tests := []struct {
		name    string
		headers header.Section
	}{
		{"no framing", header.Section{"host": {"example.com"}, "expect": {"100-continue"}}},
		{"zero length", header.Section{"host": {"example.com"}, "content-length": {"0"}, "expect": {"100-continue"}}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(originLine, test.headers)
			require.ErrorIs(t, err, fields.ErrExpectWithoutContent)
			require.NotErrorIs(t, err, fields.ErrInvalidExpect)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Expect", fieldErr.Name)
		})
	}
}

func TestConnectionCheckPrecedesExpectCheck(t *testing.T) {
	headers := header.Section{"host": {"example.com"}, "connection": {"host"}, "expect": {"100-continue"}}
	_, err := processFields(originLine, headers)
	require.ErrorIs(t, err, fields.ErrProtectedConnectionOption)
}

func TestReadRejectsExpectWithoutContent(t *testing.T) {
	message := "GET / HTTP/1.1\r\nHost: example.com\r\nExpect: 100-continue\r\n\r\n"
	request, err := Read(bufio.NewReader(strings.NewReader(message)))
	require.ErrorIs(t, err, fields.ErrExpectWithoutContent)
	require.Nil(t, request)
}

func TestMediaTypeFieldErrors(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		headers header.Section
		want    error
	}{
		{"content type", "Content-Type", header.Section{"host": {"example.com"}, "content-type": {"text"}}, fields.ErrInvalidContentType},
		{"accept", "Accept", header.Section{"host": {"example.com"}, "accept": {"*/html"}}, fields.ErrInvalidAccept},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(originLine, test.headers)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, test.field, fieldErr.Name)
		})
	}
}

func TestContentCodingFieldErrors(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		headers header.Section
		want    error
	}{
		{"content encoding", "Content-Encoding", header.Section{"host": {"example.com"}, "content-encoding": {"gzip;q=1"}}, fields.ErrInvalidContentEncoding},
		{"accept encoding", "Accept-Encoding", header.Section{"host": {"example.com"}, "accept-encoding": {"gzip;q=2"}}, fields.ErrInvalidAcceptEncoding},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(originLine, test.headers)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, test.field, fieldErr.Name)
		})
	}
}

func TestContentCodingFieldsLeaveMetadataUnchanged(t *testing.T) {
	message := "GET / HTTP/1.1\r\nHost: example.com\r\nContent-Encoding: gzip\r\nAccept-Encoding: br;q=0.5\r\n\r\n"
	request, err := Read(bufio.NewReader(strings.NewReader(message)))
	require.NoError(t, err)
	require.Equal(t, Fields{Authority: "example.com"}, request.Fields)
	require.Equal(t, []string{"gzip"}, request.Headers.Values("Content-Encoding"))
}

func TestMediaTypeFieldsLeaveMetadataUnchanged(t *testing.T) {
	message := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\nContent-Type: text/plain\r\nAccept: */*;q=0.5\r\n\r\n"
	request, err := Read(bufio.NewReader(strings.NewReader(message)))
	require.NoError(t, err)
	require.Equal(t, Fields{Authority: "example.com", ContentLengthPresent: true}, request.Fields)
	require.Equal(t, []string{"text/plain"}, request.Headers.Values("Content-Type"))
}

func TestCORSFieldErrors(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		headers header.Section
		want    error
	}{
		{"origin", "Origin", header.Section{"host": {"example.com"}, "origin": {"https://a.example/"}}, fields.ErrInvalidOrigin},
		{"method", "Access-Control-Request-Method", header.Section{"host": {"example.com"}, "access-control-request-method": {"GET, PUT"}}, fields.ErrInvalidAccessControlRequestMethod},
		{"headers", "Access-Control-Request-Headers", header.Section{"host": {"example.com"}, "access-control-request-headers": {""}}, fields.ErrInvalidAccessControlRequestHeaders},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(originLine, test.headers)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, test.field, fieldErr.Name)
		})
	}
}

func TestTrailerField(t *testing.T) {
	parsed, err := processFields(originLine, header.Section{
		"host":              {"example.com"},
		"transfer-encoding": {"chunked"},
		"trailer":           {"X-Checksum, Server-Timing"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"x-checksum", "server-timing"}, parsed.TrailerNames)

	tests := []struct {
		name  string
		value string
		want  error
	}{
		{"malformed list", "a b", fields.ErrInvalidTrailer},
		{"forbidden field announced", "X-Checksum, Content-Length", fields.ErrForbiddenTrailerField},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(originLine, header.Section{"host": {"example.com"}, "trailer": {test.value}})
			require.ErrorIs(t, err, test.want)
			require.Zero(t, parsed)
			var fieldErr *http1.FieldError
			require.ErrorAs(t, err, &fieldErr)
			require.Equal(t, "Trailer", fieldErr.Name)
		})
	}
}

func TestMaxForwardsField(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		values      []string
		wantValue   int64
		wantPresent bool
	}{
		{"OPTIONS", "OPTIONS", []string{"3"}, 3, true},
		{"OPTIONS zero", "OPTIONS", []string{"0"}, 0, true},
		{"TRACE", "TRACE", []string{"7"}, 7, true},
		{"OPTIONS absent", "OPTIONS", nil, 0, false},
		{"other methods ignore it", "GET", []string{"3"}, 0, false},
		{"other methods ignore an invalid value", "GET", []string{"x"}, 0, false},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			headers := header.Section{"host": {"example.com"}}
			if test.values != nil {
				headers["max-forwards"] = test.values
			}

			parsed, err := processFields(Line{Method: test.method, RequestTarget: "/", Version: "1.1"}, headers)
			require.NoError(t, err)
			require.Equal(t, test.wantValue, parsed.MaxForwards)
			require.Equal(t, test.wantPresent, parsed.MaxForwardsPresent)
		})
	}
}

func TestMaxForwardsErrors(t *testing.T) {
	headers := header.Section{"host": {"example.com"}, "max-forwards": {"-1"}}
	parsed, err := processFields(Line{Method: "OPTIONS", RequestTarget: "/", Version: "1.1"}, headers)
	require.ErrorIs(t, err, fields.ErrInvalidMaxForwards)
	require.Zero(t, parsed)
	var fieldErr *http1.FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Max-Forwards", fieldErr.Name)
}
