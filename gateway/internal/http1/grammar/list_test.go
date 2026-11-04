package grammar

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty value", "", []string{}},
		{"only separators", " , ,\t, ", []string{}},
		{"one member", "a", []string{"a"}},
		{"trims members", " a\t,\tb ", []string{"a", "b"}},
		{"drops empty members", ",a,,b,", []string{"a", "b"}},
		{"comma in quoted-string", `a;x="1,2",b`, []string{`a;x="1,2"`, "b"}},
		{"escaped quote keeps quoting", `a;x="1\",2",b`, []string{`a;x="1\",2"`, "b"}},
		{"escaped backslash ends quoting", `a;x="1\\",b`, []string{`a;x="1\\"`, "b"}},
		{"backslash outside quotes is ordinary", `a\,b`, []string{`a\`, "b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			members, ok := SplitList(test.value)
			require.True(t, ok)
			require.Equal(t, test.want, members)
		})
	}
}

func TestSplitListRejectsUnterminatedQuotedString(t *testing.T) {
	for _, value := range []string{`"`, `a;x="1,b`, `a;x="1\"`} {
		members, ok := SplitList(value)
		require.False(t, ok, value)
		require.Nil(t, members, value)
	}
}

func TestSplitUnquoted(t *testing.T) {
	parts, ok := SplitUnquoted(`a;b="1;2";`, ';')
	require.True(t, ok)
	require.Equal(t, []string{"a", `b="1;2"`, ""}, parts)

	parts, ok = SplitUnquoted(`a;"b`, ';')
	require.False(t, ok)
	require.Nil(t, parts)
}

func TestTrimOWS(t *testing.T) {
	require.Equal(t, "a b", TrimOWS(" \ta b\t "))
	require.Equal(t, "\na\r", TrimOWS(" \na\r "), "only SP and HTAB are whitespace")
}

func TestIsQuotedString(t *testing.T) {
	for _, s := range []string{`""`, `"a b"`, "\"a\tb\"", `"a\"b"`, `"a\\"`, `"(,;=)"`, "\"\x80\xff\"", "\"\\\xff\""} {
		require.True(t, IsQuotedString(s), "%q", s)
	}
	for _, s := range []string{"", `"`, `a`, `"a`, `a"`, `"a"b"`, `"a\"`, "\"a\x00\"", "\"a\x7f\"", "\"a\nb\"", "\"\\\x01\""} {
		require.False(t, IsQuotedString(s), "%q", s)
	}
}
