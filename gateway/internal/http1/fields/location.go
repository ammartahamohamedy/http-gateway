package fields

import (
	"errors"
	"net/url"
	"regexp"
)

// ErrInvalidLocation reports a repeated or malformed Location value.
var ErrInvalidLocation = errors.New("http1: Location requires one URI reference")

// locationPattern matches the characters of a URI-reference: ASCII URI
// characters and complete %HH escapes, with at most one "#" starting a fragment
// that excludes "#", "[", and "]" (RFC 3986, Sections 3.5 and 4.1).
var locationPattern = regexp.MustCompile(
	`\A(?:[A-Za-z0-9._~:/?\[\]@!$&'()*+,;=-]|%[0-9A-Fa-f]{2})*` +
		`(?:#(?:[A-Za-z0-9._~:/?@!$&'()*+,;=-]|%[0-9A-Fa-f]{2})*)?\z`,
)

// ValidateLocation checks that a supplied Location field holds exactly one
// URI-reference, absolute or relative (RFC 9110, Section 10.2.2). Values must
// have surrounding SP/HTAB already removed; absence is valid. The reference is
// neither resolved nor rewritten. Errors match ErrInvalidLocation.
func ValidateLocation(values []string) error {
	if len(values) == 0 {
		return nil
	}

	if len(values) != 1 || !locationPattern.MatchString(values[0]) {
		return ErrInvalidLocation
	}

	if _, err := url.Parse(values[0]); err != nil {
		return ErrInvalidLocation
	}

	return nil
}
