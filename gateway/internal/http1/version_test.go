package http1

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHTTPVersion(t *testing.T) {
	tests := []struct {
		rawVersion string
		want       string
	}{
		{"HTTP/1.1", "1.1"},
		{"HTTP/1.0", "1.0"},
		{"HTTP/1.2", "1.1"},
		{"HTTP/1.9", "1.1"},
	}
	for test := range slices.Values(tests) {
		t.Run(test.rawVersion, func(t *testing.T) {
			version, err := parseHTTPVersion(test.rawVersion)
			require.NoError(t, err)
			require.Equal(t, test.want, version)
		})
	}
}

func TestParseHTTPVersionRejectsMalformedTokens(t *testing.T) {
	for rawVersion := range slices.Values([]string{
		"",
		"1.1",
		"http/1.1",
		"HTTP/1",
		"HTTP/11.1",
		"HTTP/1.11",
		"HTTP/a.b",
		"HTTP/1x1",
		" HTTP/1.1",
		"HTTP/1.1 ",
		"HTTP/1.1\r\n",
		"HTTP/1.1\nextra",
		"HTTP/1.\xff",
	}) {
		t.Run(rawVersion, func(t *testing.T) {
			version, err := parseHTTPVersion(rawVersion)
			require.ErrorContains(t, err, "invalid HTTP version")
			require.Empty(t, version)
		})
	}
}

func TestParseHTTPVersionRejectsUnsupportedVersions(t *testing.T) {
	for rawVersion := range slices.Values([]string{"HTTP/0.9", "HTTP/2.0", "HTTP/9.9"}) {
		t.Run(rawVersion, func(t *testing.T) {
			version, err := parseHTTPVersion(rawVersion)
			require.ErrorContains(t, err, "unsupported HTTP version")
			require.Empty(t, version)
		})
	}
}
