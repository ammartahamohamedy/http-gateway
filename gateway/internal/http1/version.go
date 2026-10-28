package http1

import (
	"fmt"
	"regexp"
	"strings"
)

// httpVersionPattern matches the complete HTTP/<digit>.<digit> token.
// Whether that version is supported is checked separately.
var httpVersionPattern = regexp.MustCompile(`\AHTTP/[0-9]\.[0-9]\z`)

// parseHTTPVersion validates a full HTTP version token and returns its numeric
// portion as a string, such as "1.1". Only HTTP/1.1 is supported. Malformed tokens
// and unsupported versions produce different error messages, not typed errors
// or sentinel values. On failure, the returned version string is empty.
func parseHTTPVersion(rawVersion string) (string, error) {
	if !httpVersionPattern.MatchString(rawVersion) {
		return "", fmt.Errorf("http1: invalid HTTP version %q", rawVersion)
	}
	if rawVersion != "HTTP/1.1" {
		return "", fmt.Errorf("http1: unsupported HTTP version %q", rawVersion)
	}
	return strings.TrimPrefix(rawVersion, "HTTP/"), nil
}
