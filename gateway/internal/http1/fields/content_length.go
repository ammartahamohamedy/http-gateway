package fields

import (
	"errors"
	"regexp"
	"strconv"
)

// ErrInvalidContentLength reports a repeated, malformed, or unsupported numeric
// Content-Length value. The supported range is 0 through the maximum int64.
var ErrInvalidContentLength = errors.New("http1: Content-Length requires one decimal value within int64 range")

// contentLengthPattern matches Content-Length's 1*DIGIT grammar (RFC 9110, 8.6).
var contentLengthPattern = regexp.MustCompile(`\A[0-9]+\z`)

// ParseContentLength parses field-line values with surrounding SP/HTAB already
// removed. Absence returns (0, false, nil); a supplied field sets present to true,
// including on error. Repeated field lines and comma-separated values are rejected
// even when identical. Leading zeros are allowed. Values must fit a nonnegative
// int64; this is a local numeric limit, not a body-size policy or an RFC maximum.
// Errors match ErrInvalidContentLength and return a zero length. The input is not
// changed. This function does not read a body or check Transfer-Encoding conflicts.
func ParseContentLength(values []string) (length int64, present bool, err error) {
	if len(values) == 0 {
		return 0, false, nil
	}
	if len(values) != 1 || !contentLengthPattern.MatchString(values[0]) {
		return 0, true, ErrInvalidContentLength
	}
	length, err = strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		return 0, true, ErrInvalidContentLength
	}
	return length, true, nil
}
