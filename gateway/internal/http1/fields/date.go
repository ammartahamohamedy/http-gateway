package fields

import (
	"errors"
	"regexp"
	"slices"
	"time"
)

// ErrInvalidDate reports a missing, repeated, or malformed Date value.
var ErrInvalidDate = errors.New("http1: Date requires one HTTP-date")

// HTTP-date building blocks. Names are case-sensitive (RFC 9110, Section 5.6.7).
const (
	shortDayNames = `(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun)`
	longDayNames  = `(?:Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)`
	monthNames    = `(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)`

	// timeOfDay captures the second, which may be 60 for a leap second.
	timeOfDay = `[0-9]{2}:[0-9]{2}:([0-9]{2})`
)

// httpDateFormats pairs the exact syntax of each HTTP-date format with the time
// layout that checks its calendar ranges: IMF-fixdate, then the obsolete
// rfc850-date and asctime-date that recipients must also accept (RFC 9110,
// Section 5.6.7).
var httpDateFormats = []struct {
	pattern *regexp.Regexp
	layout  string
}{
	{
		regexp.MustCompile(`\A` + shortDayNames + `, [0-9]{2} ` + monthNames + ` [0-9]{4} ` + timeOfDay + ` GMT\z`),
		"Mon, 02 Jan 2006 15:04:05 GMT",
	},
	{
		regexp.MustCompile(`\A` + longDayNames + `, [0-9]{2}-` + monthNames + `-[0-9]{2} ` + timeOfDay + ` GMT\z`),
		"Monday, 02-Jan-06 15:04:05 GMT",
	},
	{
		regexp.MustCompile(`\A` + shortDayNames + ` ` + monthNames + ` (?:[0-9]{2}| [0-9]) ` + timeOfDay + ` [0-9]{4}\z`),
		"Mon Jan _2 15:04:05 2006",
	},
}

// ValidateDate checks that a supplied Date field holds exactly one HTTP-date
// (RFC 9110, Section 6.6.1). Values must have surrounding SP/HTAB already
// removed; absence is valid. Errors match ErrInvalidDate.
func ValidateDate(values []string) error {
	if len(values) == 0 {
		return nil
	}

	if len(values) != 1 || !isHTTPDate(values[0]) {
		return ErrInvalidDate
	}

	return nil
}

// isHTTPDate reports whether text matches one HTTP-date format exactly and
// names a real calendar date and time. The day name is not checked against the
// date. time.Parse rejects second 60, so a leap second is checked as 59.
func isHTTPDate(text string) bool {
	for format := range slices.Values(httpDateFormats) {
		indexes := format.pattern.FindStringSubmatchIndex(text)
		if indexes == nil {
			continue
		}

		secondStart, secondEnd := indexes[2], indexes[3]
		if text[secondStart:secondEnd] == "60" {
			text = text[:secondStart] + "59" + text[secondEnd:]
		}

		_, err := time.Parse(format.layout, text)
		return err == nil
	}

	return false
}
