package http1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeadersValues(t *testing.T) {
	headers := Headers{
		"x-test":       {"first", "\"second,third\"\tpart\xff", ""},
		"content-type": {"text/plain; charset=utf-8"},
	}
	tests := []struct {
		name    string
		headers Headers
		query   string
		want    []string
	}{
		{"lowercase", headers, "x-test", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"uppercase", headers, "X-TEST", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"mixed case", headers, "x-TeSt", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"single value", headers, "content-type", []string{"text/plain; charset=utf-8"}},
		{"missing name", headers, "Host", nil},
		{"nil headers", nil, "X-Test", nil},
		{"empty headers", Headers{}, "X-Test", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.headers.Values(test.query))
		})
	}
}

func TestHeadersValuesReturnsIndependentSlice(t *testing.T) {
	headers := Headers{"x-test": {"original"}}
	values := headers.Values("x-test")
	require.Equal(t, []string{"original"}, values)
	values[0] = "changed"
	require.Equal(t, Headers{"x-test": {"original"}}, headers)
}

func TestParseHeaderField(t *testing.T) {
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, value, err := ParseHeaderField(test.rawLine)
			require.NoError(t, err)
			require.Equal(t, test.wantName, name)
			require.Equal(t, test.wantValue, value)
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
			name, value, err := ParseHeaderField(test.rawLine)
			require.Error(t, err)
			require.Empty(t, name)
			require.Empty(t, value)
		})
	}
}

func TestParseHeaderFieldValueBytes(t *testing.T) {
	for b := 0; b <= 255; b++ {
		t.Run(fmt.Sprintf("0x%02X", b), func(t *testing.T) {
			value := "a" + string([]byte{byte(b)}) + "z"
			name, gotValue, err := ParseHeaderField("X-Test: " + value)
			if (b < 0x20 && b != 0x09) || b == 0x7f {
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
