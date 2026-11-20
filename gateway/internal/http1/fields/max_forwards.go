package fields

import (
	"errors"
	"math"
	"strconv"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// ErrInvalidMaxForwards reports a repeated or malformed Max-Forwards value.
var ErrInvalidMaxForwards = errors.New("http1: Max-Forwards requires one decimal value")

// ParseMaxForwards parses field-line values with surrounding SP/HTAB already
// removed. Absence returns (0, false, nil); a supplied field sets present to
// true, including on error. The value is 1*DIGIT (RFC 9110, Section 7.6.2) on a
// single field line; leading zeros are allowed. A value beyond int64 range
// parses as math.MaxInt64, the largest value supported, since a forwarded
// Max-Forwards is bounded by the recipient's maximum anyway. Errors match
// ErrInvalidMaxForwards and return a zero value.
func ParseMaxForwards(values []string) (remaining int64, present bool, err error) {
	if len(values) == 0 {
		return 0, false, nil
	}

	if len(values) != 1 || !grammar.IsDigits(values[0]) {
		return 0, true, ErrInvalidMaxForwards
	}

	remaining, err = strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		return math.MaxInt64, true, nil
	}

	return remaining, true, nil
}
