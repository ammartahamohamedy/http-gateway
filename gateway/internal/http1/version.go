package http1

import (
	"fmt"
	"regexp"
)

// httpVersionPattern matches the complete HTTP/<digit>.<digit> token.
// Whether that version is supported is checked separately.
var httpVersionPattern = regexp.MustCompile(`\AHTTP/[0-9]\.[0-9]\z`)

// parseHTTPVersion validates a full HTTP version token for a request or response
// and returns the version to process the message as: "1.0" or "1.1". Major
// versions other than 1 are unsupported. A higher HTTP/1 minor version is
// processed as 1.1, the highest this implementation conforms to (RFC 9110,
// Section 6.2). Whether a message may use 1.0 is decided by its caller.
// Malformed tokens and unsupported versions produce different error messages,
// not typed errors or sentinel values. On failure, the returned version string
// is empty.
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
