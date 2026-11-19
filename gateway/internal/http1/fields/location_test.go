package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateLocation(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent", nil},
		{"absolute path", []string{"/documents/1"}},
		{"relative path", []string{"documents/1"}},
		{"query only", []string{"?q=search"}},
		{"fragment", []string{"/documents/1#section-2"}},
		{"empty fragment", []string{"/documents/1#"}},
		{"absolute URI", []string{"http://example.com/documents/1?a=b"}},
		{"network-path reference", []string{"//example.com/documents/1"}},
		{"IPv6 literal", []string{"http://[::1]:8080/"}},
		{"percent escapes", []string{"/a%20b?q=%2F"}},
		{"sub-delims", []string{"/a;b=c,d!$&'()*+"}},
		{"empty reference", []string{""}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateLocation(test.values))
		})
	}
}

func TestValidateLocationRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"repeated field lines", []string{"/a", "/a"}},
		{"space", []string{"/a b"}},
		{"tab", []string{"/a\tb"}},
		{"non-ASCII", []string{"/مرحبا"}},
		{"obs-text", []string{"/\xff"}},
		{"bad percent escape", []string{"/%zz"}},
		{"short percent escape", []string{"/%2"}},
		{"second fragment marker", []string{"/a#b#c"}},
		{"bracket in fragment", []string{"/a#[b]"}},
		{"backslash", []string{`\a`}},
		{"double quote", []string{`/"a"`}},
		{"unclosed IPv6 literal", []string{"http://[::1/"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateLocation(test.values), ErrInvalidLocation)
		})
	}
}
