package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent", nil},
		{"IMF-fixdate", []string{"Sun, 06 Nov 1994 08:49:37 GMT"}},
		{"rfc850-date", []string{"Sunday, 06-Nov-94 08:49:37 GMT"}},
		{"asctime-date with one-digit day", []string{"Sun Nov  6 08:49:37 1994"}},
		{"asctime-date with two-digit day", []string{"Wed Nov 16 08:49:37 1994"}},
		{"asctime-date with zero-padded day", []string{"Sun Nov 06 08:49:37 1994"}},
		{"leap second", []string{"Sat, 31 Dec 2016 23:59:60 GMT"}},
		{"leap day", []string{"Thu, 29 Feb 2024 00:00:00 GMT"}},
		{"day name not checked against date", []string{"Mon, 06 Nov 1994 08:49:37 GMT"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateDate(test.values))
		})
	}
}

func TestValidateDateRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty", []string{""}},
		{"repeated field lines", []string{"Sun, 06 Nov 1994 08:49:37 GMT", "Sun, 06 Nov 1994 08:49:37 GMT"}},
		{"lowercase day name", []string{"sun, 06 Nov 1994 08:49:37 GMT"}},
		{"lowercase month", []string{"Sun, 06 nov 1994 08:49:37 GMT"}},
		{"lowercase zone", []string{"Sun, 06 Nov 1994 08:49:37 gmt"}},
		{"other zone", []string{"Sun, 06 Nov 1994 08:49:37 UTC"}},
		{"numeric offset", []string{"Sun, 06 Nov 1994 08:49:37 +0000"}},
		{"one-digit day", []string{"Sun, 6 Nov 1994 08:49:37 GMT"}},
		{"one-digit hour", []string{"Sun, 06 Nov 1994 8:49:37 GMT"}},
		{"two-digit year in IMF-fixdate", []string{"Sun, 06 Nov 94 08:49:37 GMT"}},
		{"long day name in IMF-fixdate", []string{"Sunday, 06 Nov 1994 08:49:37 GMT"}},
		{"short day name in rfc850-date", []string{"Sun, 06-Nov-94 08:49:37 GMT"}},
		{"extra space", []string{"Sun,  06 Nov 1994 08:49:37 GMT"}},
		{"trailing text", []string{"Sun, 06 Nov 1994 08:49:37 GMT x"}},
		{"asctime-date with two spaces before a two-digit day", []string{"Wed Nov  16 08:49:37 1994"}},
		{"day out of range", []string{"Tue, 31 Feb 1994 08:49:37 GMT"}},
		{"day zero", []string{"Sun, 00 Nov 1994 08:49:37 GMT"}},
		{"hour out of range", []string{"Sun, 06 Nov 1994 24:00:00 GMT"}},
		{"minute out of range", []string{"Sun, 06 Nov 1994 08:60:37 GMT"}},
		{"second out of range", []string{"Sun, 06 Nov 1994 08:49:61 GMT"}},
		{"February 29 outside a leap year", []string{"Tue, 29 Feb 2023 00:00:00 GMT"}},
		{"ISO 8601", []string{"1994-11-06T08:49:37Z"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateDate(test.values), ErrInvalidDate)
		})
	}
}
