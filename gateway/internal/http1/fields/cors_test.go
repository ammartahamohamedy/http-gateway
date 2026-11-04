package fields

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOrigin(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"absent nil", nil, ""},
		{"absent empty slice", []string{}, ""},
		{"https", []string{"https://example.com"}, "https://example.com"},
		{"port", []string{"http://localhost:8080"}, "http://localhost:8080"},
		{"null", []string{"null"}, "null"},
		{"custom scheme", []string{"chrome-extension://abc"}, "chrome-extension://abc"},
		{"case preserved", []string{"HTTP://Example.COM"}, "HTTP://Example.COM"},
		{"IPv4", []string{"http://127.0.0.1:3000"}, "http://127.0.0.1:3000"},
		{"IPv6", []string{"http://[::1]:3000"}, "http://[::1]:3000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			origin, err := ParseOrigin(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, origin)
		})
	}
}

func TestParseOriginRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty value", []string{""}},
		{"repeated field lines", []string{"https://a.example", "https://b.example"}},
		{"uppercase null", []string{"NULL"}},
		{"no scheme", []string{"example.com"}},
		{"scheme-relative", []string{"//example.com"}},
		{"scheme only", []string{"https://"}},
		{"file scheme without host", []string{"file://"}},
		{"port without host", []string{"https://:443"}},
		{"scheme starting with digit", []string{"1a://example.com"}},
		{"empty scheme", []string{"://example.com"}},
		{"trailing slash", []string{"https://example.com/"}},
		{"path", []string{"https://example.com/a"}},
		{"query", []string{"https://example.com?a=b"}},
		{"fragment", []string{"https://example.com#a"}},
		{"user information", []string{"https://user@example.com"}},
		{"space-separated list", []string{"https://a.example https://b.example"}},
		{"comma-separated list", []string{"https://a.example,https://b.example"}},
		{"nonnumeric port", []string{"https://example.com:x"}},
		{"invalid IPv6", []string{"http://[::1"}},
		{"obs-text", []string{"https://ex\xffample.com"}},
		{"NUL", []string{"https://example.com\x00"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			origin, err := ParseOrigin(test.values)
			require.ErrorIs(t, err, ErrInvalidOrigin)
			require.Empty(t, origin)
		})
	}
}

func TestParseAccessControlRequestMethod(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"absent nil", nil, ""},
		{"absent empty slice", []string{}, ""},
		{"standard method", []string{"PUT"}, "PUT"},
		{"case preserved", []string{"patch"}, "patch"},
		{"extension method", []string{"X-CUSTOM"}, "X-CUSTOM"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method, err := ParseAccessControlRequestMethod(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, method)
		})
	}
}

func TestParseAccessControlRequestMethodRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty value", []string{""}},
		{"repeated field lines", []string{"PUT", "PUT"}},
		{"list", []string{"GET, PUT"}},
		{"space", []string{"PU T"}},
		{"non-token", []string{"PUT/1"}},
		{"NUL", []string{"PUT\x00"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method, err := ParseAccessControlRequestMethod(test.values)
			require.ErrorIs(t, err, ErrInvalidAccessControlRequestMethod)
			require.Empty(t, method)
		})
	}
}

func TestParseAccessControlRequestHeaders(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent nil", nil, nil},
		{"absent empty slice", []string{}, nil},
		{"one name", []string{"content-type"}, []string{"content-type"}},
		{"lowercased", []string{"X-Custom, Authorization"}, []string{"x-custom", "authorization"}},
		{"repeated field lines form one list", []string{"a", "b"}, []string{"a", "b"}},
		{"empty list members", []string{",a,, b ,"}, []string{"a", "b"}},
		{"one empty line among others", []string{"", "a"}, []string{"a"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			names, err := ParseAccessControlRequestHeaders(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, names)
		})
	}
}

func TestParseAccessControlRequestHeadersRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty value", []string{""}},
		{"only commas", []string{" , , "}},
		{"empty lines only", []string{"", ""}},
		{"space inside name", []string{"x custom"}},
		{"missing comma", []string{"a b"}},
		{"non-token", []string{"a:b"}},
		{"quoted name", []string{`"a"`}},
		{"unterminated quote", []string{`"a`}},
		{"obs-text", []string{"a\xff"}},
		{"invalid second line", []string{"a", "b c"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			names, err := ParseAccessControlRequestHeaders(test.values)
			require.ErrorIs(t, err, ErrInvalidAccessControlRequestHeaders)
			require.Nil(t, names)
		})
	}
}

func TestParseAccessControlRequestHeadersKeepsInput(t *testing.T) {
	values := []string{"X-One", "X-Two"}
	_, err := ParseAccessControlRequestHeaders(values)
	require.NoError(t, err)
	require.Equal(t, []string{"X-One", "X-Two"}, values)
}
