package http1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHTTPVersion(t *testing.T) {
	version, err := parseHTTPVersion("HTTP/1.1")
	require.NoError(t, err)
	require.Equal(t, "1.1", version)
}

func TestParseHTTPVersionRejectsMalformedTokens(t *testing.T) {
	for _, rawVersion := range []string{
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
	} {
		t.Run(rawVersion, func(t *testing.T) {
			version, err := parseHTTPVersion(rawVersion)
			require.ErrorContains(t, err, "invalid HTTP version")
			require.Empty(t, version)
		})
	}
}

func TestParseHTTPVersionRejectsUnsupportedVersions(t *testing.T) {
	for _, rawVersion := range []string{"HTTP/1.0", "HTTP/2.0", "HTTP/9.9"} {
		t.Run(rawVersion, func(t *testing.T) {
			version, err := parseHTTPVersion(rawVersion)
			require.ErrorContains(t, err, "unsupported HTTP version")
			require.Empty(t, version)
		})
	}
}
