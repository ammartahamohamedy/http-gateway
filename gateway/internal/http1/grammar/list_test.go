package grammar

import (
	"slices"
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

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			members, ok := SplitList(test.value)
			require.True(t, ok)
			require.Equal(t, test.want, members)
		})
	}
}

func TestSplitListRejectsUnterminatedQuotedString(t *testing.T) {
	for value := range slices.Values([]string{`"`, `a;x="1,b`, `a;x="1\"`}) {
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
	for text := range slices.Values([]string{`""`, `"a b"`, "\"a\tb\"", `"a\"b"`, `"a\\"`, `"(,;=)"`, "\"\x80\xff\"", "\"\\\xff\""}) {
		require.True(t, IsQuotedString(text), "%q", text)
	}

	for text := range slices.Values([]string{"", `"`, `a`, `"a`, `a"`, `"a"b"`, `"a\"`, "\"a\x00\"", "\"a\x7f\"", "\"a\nb\"", "\"\\\x01\""}) {
		require.False(t, IsQuotedString(text), "%q", text)
	}
}

func TestIsParameter(t *testing.T) {
	for text := range slices.Values([]string{"a=b", `a="b c"`, `a="x=y"`, `a=""`}) {
		require.True(t, IsParameter(text), "%q", text)
	}

	for text := range slices.Values([]string{"", "a", "=b", "a=", "a =b", "a= b", "a=b=c", `a="b`, `"a"=b`}) {
		require.False(t, IsParameter(text), "%q", text)
	}
}

func TestIsChunkExtension(t *testing.T) {
	for text := range slices.Values([]string{"a", "a=b", "a = b", "a=\t\"b;c\"", `a="x=y"`, `a=""`}) {
		require.True(t, IsChunkExtension(text), "%q", text)
	}

	for text := range slices.Values([]string{"", " a", "a ", "=b", "a=", "a b", "a=b c", "a=b=c", `a="b`, `"a"=b`}) {
		require.False(t, IsChunkExtension(text), "%q", text)
	}
}

func TestSplitTokenList(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent", nil, nil},
		{"empty value", []string{""}, nil},
		{"only commas", []string{" , ,"}, nil},
		{"one token", []string{"close"}, []string{"close"}},
		{"case preserved", []string{"Keep-Alive, GET"}, []string{"Keep-Alive", "GET"}},
		{"surrounding whitespace", []string{" a ,\tb "}, []string{"a", "b"}},
		{"empty members dropped", []string{",a,,b,"}, []string{"a", "b"}},
		{"repeated lines form one list", []string{"a, b", "c"}, []string{"a", "b", "c"}},
		{"asterisk is a token", []string{"*"}, []string{"*"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			tokens, ok := SplitTokenList(test.values)
			require.True(t, ok)
			require.Equal(t, test.want, tokens)
		})
	}
}

func TestSplitTokenListRejectsNonTokens(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"space inside member", []string{"a b"}},
		{"separator", []string{"a/b"}},
		{"quoted member", []string{`"a"`}},
		{"unterminated quote", []string{`"a`}},
		{"parameter", []string{"a;b=c"}},
		{"obs-text", []string{"a\xff"}},
		{"invalid second line", []string{"a", "b c"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			tokens, ok := SplitTokenList(test.values)
			require.False(t, ok)
			require.Nil(t, tokens)
		})
	}
}
