package http1

import (
	"fmt"
	"regexp"
)

// httpVersionPattern matches the complete HTTP/<digit>.<digit> token.
// Whether that version is supported is checked separately.
var httpVersionPattern = regexp.MustCompile(`\AHTTP/[0-9]\.[0-9]\z`)

// parseHTTPVersion validates an HTTP version token and returns the version to
// process the message as: "1.0", or "1.1" for HTTP/1.1 and higher HTTP/1 minors
// (RFC 9110, Section 6.2). Other majors are unsupported; callers decide whether
// 1.0 is allowed. Malformed and unsupported versions return different messages,
// without sentinels, and an empty version.
func parseHTTPVersion(rawVersion string) (string, error) {
	if !httpVersionPattern.MatchString(rawVersion) {
		return "", fmt.Errorf("http1: invalid HTTP version %q", rawVersion)
	}
	major, minor := rawVersion[len("HTTP/")], rawVersion[len("HTTP/1.")]
	if major != '1' {
		return "", fmt.Errorf("http1: unsupported HTTP version %q", rawVersion)
	}
	if minor == '0' {
		return "1.0", nil
	}
	return "1.1", nil
}
