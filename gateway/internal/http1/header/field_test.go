package header

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseField(t *testing.T) {
	tests := []struct {
		name      string
		rawLine   string
		wantName  string
		wantValue string
	}{
		{"simple", "Host: example.com", "Host", "example.com"},
		{"name casing", "cOnTeNt-TyPe:text/plain", "cOnTeNt-TyPe", "text/plain"},
		{"token punctuation", "!#$%&'*+-.^_`|~09AZaz:v", "!#$%&'*+-.^_`|~09AZaz", "v"},
		{"empty value", "X-Test:", "X-Test", ""},
		{"only whitespace", "X-Test: \t \t", "X-Test", ""},
		{"surrounding whitespace", "X-Test:\t  a  \tb \t", "X-Test", "a  \tb"},
		{"first colon", "X-Test: a:b:c", "X-Test", "a:b:c"},
		{"quoted comma", "X-Test: \"a,b\", c", "X-Test", "\"a,b\", c"},
		{"opaque bytes", "X-Test: \x80\xff\xc3", "X-Test", "\x80\xff\xc3"},
		{"Unicode whitespace preserved", "X-Test: \u00a0value\u00a0 ", "X-Test", "\u00a0value\u00a0"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			name, value, err := parseField(test.rawLine)
			require.NoError(t, err)
			require.Equal(t, test.wantName, name)
			require.Equal(t, test.wantValue, value)
		})
	}
}

func TestParseFieldRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		rawLine string
	}{
		{"empty line", ""},
		{"missing colon", "Host example.com"},
		{"empty name", ": value"},
		{"space before colon", "Host : example.com"},
		{"tab before colon", "Host\t: example.com"},
		{"leading space", " Host: example.com"},
		{"leading tab", "\tHost: example.com"},
		{"space in name", "Bad Name: value"},
		{"delimiter in name", "Bad/Name: value"},
		{"control in name", "Bad\x00Name: value"},
		{"non-ASCII name", "H\u00f6st: value"},
		{"invalid UTF-8 name", "X-\xff: value"},
		{"unexpected CRLF", "Host: example.com\r\n"},
		{"folded value", "X-Test: a\r\n b"},
		{"quoted control", "X-Test: \"a\x0bb\""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			name, value, err := parseField(test.rawLine)
			require.Error(t, err)
			require.Empty(t, name)
			require.Empty(t, value)
		})
	}
}

func TestParseFieldValueBytes(t *testing.T) {
	for octet := range 256 {
		t.Run(fmt.Sprintf("0x%02X", octet), func(t *testing.T) {
			value := "a" + string([]byte{byte(octet)}) + "z"
			name, gotValue, err := parseField("X-Test: " + value)
			if (octet < 0x20 && octet != 0x09) || octet == 0x7f {
				require.Error(t, err)
				require.Empty(t, name)
				require.Empty(t, gotValue)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "X-Test", name)
			require.Equal(t, value, gotValue)
		})
	}
}
