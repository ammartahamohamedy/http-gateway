package response

import (
	"bufio"
	"slices"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

var okLine = Line{Version: "1.1", StatusCode: 200, ReasonPhrase: "OK"}

func TestFieldRulesOrder(t *testing.T) {
	var names []string
	for rule := range slices.Values(fieldRules) {
		names = append(names, rule.name)
	}

	require.Equal(t, []string{
		"Content-Length", "Transfer-Encoding", "Trailer", "Connection",
		"Content-Type", "Content-Encoding",
		"Date", "Location", "Allow", "Retry-After", "Vary",
	}, names)
}

func TestCrossChecksOrder(t *testing.T) {
	var names []string
	for check := range slices.Values(crossChecks) {
		names = append(names, check.name)
	}

	require.Equal(t, []string{
		"Transfer-Encoding", "Content-Length", "Transfer-Encoding", "Trailer", "Connection",
	}, names)
}

func TestProcessFields(t *testing.T) {
	tests := []struct {
		name    string
		line    Line
		headers header.Section
		want    Fields
	}{
		{"no fields", okLine, nil, Fields{}},
		{"Content-Length", okLine, header.Section{"content-length": {"12"}},
			Fields{ContentLength: 12, ContentLengthPresent: true}},
		{"Content-Length zero", okLine, header.Section{"content-length": {"0"}},
			Fields{ContentLengthPresent: true}},
		{"chunked", okLine, header.Section{"transfer-encoding": {"gzip, chunked"}},
			Fields{TransferCodings: []string{"gzip", "chunked"}}},
		{"codings without chunked", okLine, header.Section{"transfer-encoding": {"gzip"}},
			Fields{TransferCodings: []string{"gzip"}}},
		{"trailer names", okLine, header.Section{"transfer-encoding": {"chunked"}, "trailer": {"Server-Timing"}},
			Fields{TransferCodings: []string{"chunked"}, TrailerNames: []string{"server-timing"}}},
		{"connection options", okLine, header.Section{"connection": {"close, Keep-Alive"}},
			Fields{ConnectionOptions: []string{"close", "keep-alive"}}},
		{"vary", okLine, header.Section{"vary": {"Accept-Encoding, Origin"}},
			Fields{Vary: []string{"accept-encoding", "origin"}}},
		{"validate-only fields leave metadata unchanged", okLine, header.Section{
			"content-type":     {"application/json"},
			"content-encoding": {"gzip"},
			"date":             {"Sun, 06 Nov 1994 08:49:37 GMT"},
			"location":         {"/documents/1"},
			"allow":            {"GET, HEAD"},
			"retry-after":      {"120"},
		}, Fields{}},
		{"fields without a rule pass", okLine, header.Section{
			"set-cookie": {"a=1; Expires=Wed, 21 Oct 2026 07:28:00 GMT", "b=2"},
			"server":     {"backend/1.0"},
		}, Fields{}},
		{"304 may declare the representation length", Line{Version: "1.1", StatusCode: 304},
			header.Section{"content-length": {"42"}}, Fields{ContentLength: 42, ContentLengthPresent: true}},
		{"204 without framing fields", Line{Version: "1.1", StatusCode: 204},
			header.Section{"date": {"Sun, 06 Nov 1994 08:49:37 GMT"}}, Fields{}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(test.line, test.headers)
			require.NoError(t, err)
			require.Equal(t, test.want, parsed)
		})
	}
}

func TestProcessFieldsRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		field string
		value string
		want  error
	}{
		{"Content-Length", "-1", fields.ErrInvalidContentLength},
		{"Transfer-Encoding", "chunked, chunked", fields.ErrInvalidTransferEncoding},
		{"Trailer", "a b", fields.ErrInvalidTrailer},
		{"Connection", "a/b", fields.ErrInvalidConnection},
		{"Content-Type", "json", fields.ErrInvalidContentType},
		{"Content-Encoding", "g zip", fields.ErrInvalidContentEncoding},
		{"Date", "yesterday", fields.ErrInvalidDate},
		{"Location", "/a b", fields.ErrInvalidLocation},
		{"Allow", "GET POST", fields.ErrInvalidAllow},
		{"Retry-After", "soon", fields.ErrInvalidRetryAfter},
		{"Vary", `"Origin"`, fields.ErrInvalidVary},
	}

	for test := range slices.Values(tests) {
		t.Run(test.field, func(t *testing.T) {
			headers := header.Section{strings.ToLower(test.field): {test.value}}

			parsed, err := processFields(okLine, headers)
			require.ErrorIs(t, err, test.want)
			requireFieldError(t, err, test.field)
			require.Equal(t, Fields{}, parsed)
		})
	}
}

func TestProcessFieldsCrossChecks(t *testing.T) {
	noContent := Line{Version: "1.1", StatusCode: 204}
	interim := Line{Version: "1.1", StatusCode: 103}

	tests := []struct {
		name    string
		line    Line
		headers header.Section
		field   string
		want    error
	}{
		{"framing conflict", okLine,
			header.Section{"content-length": {"2"}, "transfer-encoding": {"chunked"}},
			"Transfer-Encoding", fields.ErrFramingConflict},
		{"framing conflict with zero length", okLine,
			header.Section{"content-length": {"0"}, "transfer-encoding": {"gzip"}},
			"Transfer-Encoding", fields.ErrFramingConflict},
		{"Content-Length on 204", noContent,
			header.Section{"content-length": {"0"}},
			"Content-Length", fields.ErrFramingOnBodilessStatus},
		{"Transfer-Encoding on 204", noContent,
			header.Section{"transfer-encoding": {"chunked"}},
			"Transfer-Encoding", fields.ErrFramingOnBodilessStatus},
		{"Content-Length on 1xx", interim,
			header.Section{"content-length": {"0"}},
			"Content-Length", fields.ErrFramingOnBodilessStatus},
		{"Transfer-Encoding on 1xx", interim,
			header.Section{"transfer-encoding": {"chunked"}},
			"Transfer-Encoding", fields.ErrFramingOnBodilessStatus},
		{"framing conflict precedes bodiless check", noContent,
			header.Section{"content-length": {"0"}, "transfer-encoding": {"chunked"}},
			"Transfer-Encoding", fields.ErrFramingConflict},
		{"forbidden trailer name", okLine,
			header.Section{"transfer-encoding": {"chunked"}, "trailer": {"Content-Length"}},
			"Trailer", fields.ErrForbiddenTrailerField},
		{"protected connection option", okLine,
			header.Section{"connection": {"Transfer-Encoding"}},
			"Connection", fields.ErrProtectedConnectionOption},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := processFields(test.line, test.headers)
			require.ErrorIs(t, err, test.want)
			requireFieldError(t, err, test.field)
			require.Equal(t, Fields{}, parsed)
		})
	}
}

func TestProcessFieldsRunsCrossChecksAfterFieldRules(t *testing.T) {
	headers := header.Section{
		"content-length":    {"2"},
		"transfer-encoding": {"chunked"},
		"vary":              {`"Origin"`},
	}

	_, err := processFields(okLine, headers)
	require.ErrorIs(t, err, fields.ErrInvalidVary)
	requireFieldError(t, err, "Vary")
}

func TestProcessFieldsFirstErrorIsDeterministic(t *testing.T) {
	headers := header.Section{"content-length": {"x"}, "date": {"x"}, "vary": {"a b"}}

	for range 20 {
		_, err := processFields(okLine, headers)
		requireFieldError(t, err, "Content-Length")
	}
}

func TestReadTagsFieldErrors(t *testing.T) {
	const raw = "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n"

	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", nil)
	require.ErrorIs(t, err, fields.ErrFramingOnBodilessStatus)
	requireStage(t, err, http1.StageFields)
	requireFieldError(t, err, "Content-Length")
	require.Nil(t, message)
}

func TestReadSkipsFieldRulesForInterimResponses(t *testing.T) {
	const raw = "HTTP/1.1 103 Early Hints\r\nDate: yesterday\r\n\r\n" +
		"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"

	var interims []*Message
	message, err := Read(bufio.NewReader(strings.NewReader(raw)), "GET", collectInterim(&interims))
	require.NoError(t, err)
	require.Len(t, interims, 1)
	require.Equal(t, Fields{}, interims[0].Fields)
	require.Equal(t, Fields{ContentLengthPresent: true}, message.Fields)
}

// requireFieldError asserts that err wraps a *http1.FieldError for name.
func requireFieldError(t *testing.T, err error, name string) {
	t.Helper()

	var fieldErr *http1.FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, name, fieldErr.Name)
}
