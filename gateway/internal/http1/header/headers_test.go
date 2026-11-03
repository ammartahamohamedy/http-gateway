package header

import (
	"bufio"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSectionValues(t *testing.T) {
	headers := Section{
		"x-test":       {"first", "\"second,third\"\tpart\xff", ""},
		"content-type": {"text/plain; charset=utf-8"},
	}
	tests := []struct {
		name    string
		headers Section
		query   string
		want    []string
	}{
		{"lowercase", headers, "x-test", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"uppercase", headers, "X-TEST", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"mixed case", headers, "x-TeSt", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"single value", headers, "content-type", []string{"text/plain; charset=utf-8"}},
		{"missing name", headers, "Host", nil},
		{"nil headers", nil, "X-Test", nil},
		{"empty headers", Section{}, "X-Test", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.headers.Values(test.query))
		})
	}
}

func TestSectionValuesReturnsIndependentSlice(t *testing.T) {
	headers := Section{"x-test": {"original"}}
	values := headers.Values("x-test")
	require.Equal(t, []string{"original"}, values)
	values[0] = "changed"
	require.Equal(t, Section{"x-test": {"original"}}, headers)
}

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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, value, err := ParseField(test.rawLine)
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, value, err := ParseField(test.rawLine)
			require.Error(t, err)
			require.Empty(t, name)
			require.Empty(t, value)
		})
	}
}

func TestParseFieldValueBytes(t *testing.T) {
	for b := 0; b <= 255; b++ {
		t.Run(fmt.Sprintf("0x%02X", b), func(t *testing.T) {
			value := "a" + string([]byte{byte(b)}) + "z"
			name, gotValue, err := ParseField("X-Test: " + value)
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

// testLimits matches the request limits used by http1.ReadRequest.
var testLimits = Limits{LineBytes: 8 * 1024, SectionBytes: 32 * 1024, Fields: 100}

func TestReadSectionLimits(t *testing.T) {
	fieldLine := func(size int) string { return "X: " + strings.Repeat("a", size-3) }
	prefix := strings.Repeat(fieldLine(testLimits.LineBytes)+"\r\n", 3)
	lastLineSize := testLimits.SectionBytes - len(prefix) - 4
	exactSection := prefix + fieldLine(lastLineSize) + "\r\n\r\n"
	require.Len(t, exactSection, testLimits.SectionBytes)
	tests := []struct {
		name    string
		section string
		count   int
		wantErr error
	}{
		{"line at limit", fieldLine(testLimits.LineBytes) + "\r\n\r\n", 1, nil},
		{"line over limit", fieldLine(testLimits.LineBytes+1) + "\r\n\r\n", 0, ErrLineTooLong},
		{"oversized unterminated line", fieldLine(testLimits.LineBytes + 1), 0, ErrLineTooLong},
		{"oversized partial terminator", fieldLine(testLimits.LineBytes+1) + "\r", 0, ErrLineTooLong},
		{"section at limit", exactSection, 4, nil},
		{"section over limit", prefix + fieldLine(lastLineSize+1) + "\r\n\r\n", 0, ErrSectionTooLarge},
		{"section budget while reading", prefix + fieldLine(lastLineSize+3), 0, ErrSectionTooLarge},
		{"count at limit", strings.Repeat("X: a\r\n", testLimits.Fields) + "\r\n", testLimits.Fields, nil},
		{"count over limit", strings.Repeat("X: a\r\n", testLimits.Fields+1) + "\r\n", 0, ErrTooManyFields},
		{"mixed case count over limit", strings.Repeat("X: a\r\nx: b\r\n", testLimits.Fields/2) + "X: c\r\n\r\n", 0, ErrTooManyFields},
		{"continuations are not fields", "X: a\r\n" + strings.Repeat(" b\r\n", testLimits.Fields+1) + "\r\n", 1, nil},
		{"large unfolded field", fieldLine(testLimits.LineBytes) + "\r\n " + strings.Repeat("b", testLimits.LineBytes-1) + "\r\n\r\n", 1, nil},
		{"oversized continuation", "X: a\r\n " + strings.Repeat("b", testLimits.LineBytes) + "\r\n\r\n", 0, ErrLineTooLong},
		{"continuations use section budget", "X: a\r\n" + strings.Repeat(" \t\r\n", testLimits.SectionBytes/4), 0, ErrSectionTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			headers, err := ReadSection(bufio.NewReaderSize(strings.NewReader(test.section), 16), testLimits)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Nil(t, headers)
				return
			}
			require.NoError(t, err)
			count := 0
			for _, values := range headers {
				count += len(values)
			}
			require.Equal(t, test.count, count)
		})
	}
}
