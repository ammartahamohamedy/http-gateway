package response

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
	"github.com/stretchr/testify/require"
)

func TestParseStatusCodeAcceptsEveryCodeInRange(t *testing.T) {
	for statusCode := 100; statusCode <= 599; statusCode++ {
		parsed, err := parseStatusCode(fmt.Sprint(statusCode))
		require.NoError(t, err)
		require.Equal(t, statusCode, parsed)
	}
}

func TestParseStatusCodeRejectsMalformedCodes(t *testing.T) {
	tests := []struct {
		name    string
		rawCode string
	}{
		{"empty", ""},
		{"one digit", "2"},
		{"two digits", "20"},
		{"four digits", "2000"},
		{"plus sign", "+20"},
		{"minus sign", "-20"},
		{"leading space", " 20"},
		{"trailing space", "20 "},
		{"letter", "2x0"},
		{"hexadecimal", "0x1"},
		{"fullwidth digits", "２００"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			statusCode, err := parseStatusCode(test.rawCode)
			require.ErrorContains(t, err, "is not three digits")
			require.Zero(t, statusCode)
		})
	}
}

func TestParseStatusCodeRejectsCodesOutOfRange(t *testing.T) {
	for rawCode := range slices.Values([]string{"000", "001", "099", "600", "999"}) {
		t.Run(rawCode, func(t *testing.T) {
			statusCode, err := parseStatusCode(rawCode)
			require.ErrorContains(t, err, "is outside 100-599")
			require.Zero(t, statusCode)
		})
	}
}

func TestReasonPhrasesCoverRegisteredCodes(t *testing.T) {
	registered := []int{
		100, 101, 102, 103,
		200, 201, 202, 203, 204, 205, 206, 207, 208, 226,
		300, 301, 302, 303, 304, 305, 307, 308,
		400, 401, 402, 403, 404, 405, 406, 407, 408, 409, 410, 411, 412, 413, 414,
		415, 416, 417, 421, 422, 423, 424, 425, 426, 428, 429, 431, 451,
		500, 501, 502, 503, 504, 505, 506, 507, 508, 510, 511,
	}

	require.ElementsMatch(t, registered, slices.Collect(maps.Keys(reasonPhrases)))
}

func TestReasonPhrasesUseRegistryDescriptions(t *testing.T) {
	tests := map[int]string{
		100: "Continue",
		103: "Early Hints",
		200: "OK",
		204: "No Content",
		304: "Not Modified",
		413: "Content Too Large",
		422: "Unprocessable Content",
		502: "Bad Gateway",
		510: "Not Extended",
	}

	for statusCode, phrase := range tests {
		require.Equal(t, phrase, reasonPhrases[statusCode], "status %d", statusCode)
	}
}

func TestReasonPhrasesAreValidReasonPhrases(t *testing.T) {
	for statusCode, phrase := range reasonPhrases {
		require.NotEmpty(t, phrase, "status %d", statusCode)
		require.True(t, grammar.IsFieldValue(phrase), "status %d", statusCode)
		require.Equal(t, strings.TrimSpace(phrase), phrase, "status %d", statusCode)
	}
}
