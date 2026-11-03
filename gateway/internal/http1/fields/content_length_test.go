package fields

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseContentLength(t *testing.T) {
	tests := []struct {
		name        string
		values      []string
		wantLength  int64
		wantPresent bool
	}{
		{"absent nil", nil, 0, false},
		{"absent empty slice", []string{}, 0, false},
		{"zero", []string{"0"}, 0, true},
		{"positive", []string{"42"}, 42, true},
		{"leading zeros", []string{"00042"}, 42, true},
		{"many zero digits", []string{strings.Repeat("0", 8192)}, 0, true},
		{"many leading zeros", []string{strings.Repeat("0", 100) + "42"}, 42, true},
		{"maximum int64", []string{"9223372036854775807"}, 9223372036854775807, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			length, present, err := ParseContentLength(test.values)
			require.NoError(t, err)
			require.Equal(t, test.wantLength, length)
			require.Equal(t, test.wantPresent, present)
		})
	}
}

func TestParseContentLengthRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty value", []string{""}},
		{"spaces only", []string{" "}},
		{"tab only", []string{"\t"}},
		{"leading whitespace", []string{" 42"}},
		{"trailing whitespace", []string{"42\t"}},
		{"internal space", []string{"4 2"}},
		{"internal tab", []string{"4\t2"}},
		{"plus sign", []string{"+42"}},
		{"minus sign", []string{"-42"}},
		{"negative zero", []string{"-0"}},
		{"fraction", []string{"4.2"}},
		{"exponent", []string{"4e2"}},
		{"hexadecimal", []string{"0x2a"}},
		{"digit separator", []string{"4_2"}},
		{"Unicode digits", []string{"\u0664\u0662"}},
		{"NUL", []string{"42\x00"}},
		{"invalid UTF-8", []string{"42\xff"}},
		{"trailing LF", []string{"42\n"}},
		{"CRLF", []string{"42\r\n"}},
		{"identical repeated lines", []string{"42", "42"}},
		{"conflicting repeated lines", []string{"42", "43"}},
		{"empty repeated line", []string{"42", ""}},
		{"identical comma list", []string{"42,42"}},
		{"identical comma list with whitespace", []string{"42, 42"}},
		{"conflicting comma list", []string{"42,43"}},
		{"trailing comma", []string{"42,"}},
		{"int64 overflow", []string{"9223372036854775808"}},
		{"uint64 maximum", []string{"18446744073709551615"}},
		{"very large numeral", []string{strings.Repeat("9", 8192)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			length, present, err := ParseContentLength(test.values)
			require.ErrorIs(t, err, ErrInvalidContentLength)
			require.True(t, present)
			require.Zero(t, length)
		})
	}
}
