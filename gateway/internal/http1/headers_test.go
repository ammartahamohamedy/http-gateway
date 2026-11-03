package http1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHeaderField(t *testing.T) {
	tests := []struct {
		name    string
		rawLine string
		want    HeaderField
	}{
		{"simple", "Host: example.com", HeaderField{"Host", "example.com"}},
		{"name casing", "cOnTeNt-TyPe:text/plain", HeaderField{"cOnTeNt-TyPe", "text/plain"}},
		{"token punctuation", "!#$%&'*+-.^_`|~09AZaz:v", HeaderField{"!#$%&'*+-.^_`|~09AZaz", "v"}},
		{"empty value", "X-Test:", HeaderField{"X-Test", ""}},
		{"only whitespace", "X-Test: \t \t", HeaderField{"X-Test", ""}},
		{"surrounding whitespace", "X-Test:\t  a  \tb \t", HeaderField{"X-Test", "a  \tb"}},
		{"first colon", "X-Test: a:b:c", HeaderField{"X-Test", "a:b:c"}},
		{"quoted comma", "X-Test: \"a,b\", c", HeaderField{"X-Test", "\"a,b\", c"}},
		{"opaque bytes", "X-Test: \x80\xff\xc3", HeaderField{"X-Test", "\x80\xff\xc3"}},
		{"Unicode whitespace preserved", "X-Test: \u00a0value\u00a0 ", HeaderField{"X-Test", "\u00a0value\u00a0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, err := ParseHeaderField(test.rawLine)
			require.NoError(t, err)
			require.Equal(t, test.want, field)
		})
	}
}

func TestParseHeaderFieldRejectsMalformedInput(t *testing.T) {
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, err := ParseHeaderField(test.rawLine)
			require.Error(t, err)
			require.Equal(t, HeaderField{}, field)
		})
	}
}

func TestParseHeaderFieldValueBytes(t *testing.T) {
	for b := 0; b <= 255; b++ {
		t.Run(fmt.Sprintf("0x%02X", b), func(t *testing.T) {
			value := "a" + string([]byte{byte(b)}) + "z"
			field, err := ParseHeaderField("X-Test: " + value)
			if (b < 0x20 && b != 0x09) || b == 0x7f {
				require.Error(t, err)
				require.Equal(t, HeaderField{}, field)
				return
			}
			require.NoError(t, err)
			require.Equal(t, HeaderField{"X-Test", value}, field)
		})
	}
}
