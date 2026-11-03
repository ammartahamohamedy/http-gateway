package fields

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRequestHost(t *testing.T) {
	tests := []struct {
		name          string
		values        []string
		method        string
		requestTarget string
		wantErr       error
	}{
		{"origin", []string{"example.com"}, "GET", "/", nil},
		{"absolute", []string{"other.example"}, "GET", "http://target.example/", nil},
		{"CONNECT", []string{"example.com:443"}, "CONNECT", "example.com:443", nil},
		{"asterisk", []string{"example.com"}, "OPTIONS", "*", nil},
		{"authority-less absolute", []string{""}, "GET", "urn:example:item", nil},
		{"missing", nil, "GET", "/", ErrInvalidHost},
		{"duplicates", []string{"example.com", "example.com"}, "GET", "/", ErrInvalidHost},
		{"malformed", []string{"user@example.com"}, "GET", "/", ErrInvalidHost},
		{"port only", []string{":80"}, "GET", "/", ErrInvalidHost},
		{"empty origin", []string{""}, "GET", "/", ErrInvalidHost},
		{"empty absolute HTTP", []string{""}, "GET", "http://example.com/", ErrInvalidHost},
		{"empty CONNECT", []string{""}, "CONNECT", "example.com:443", ErrInvalidHost},
		{"empty asterisk", []string{""}, "OPTIONS", "*", ErrInvalidHost},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequestHost(test.values, test.method, test.requestTarget)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateHostValue(t *testing.T) {
	for _, value := range []string{
		"", "example.com", "Example.COM", "localhost", "example.com.",
		"127.0.0.1", "example.com:8080", "example.com:", "example.com:999999",
		"xn--bcher-kva.example", "%C3%A9xample.example", "service_name.example",
		"a,b", "a!$&'()*+;=.example", ":80",
		"[::1]", "[2001:db8::1]:443", "[::ffff:192.0.2.1]", "[::1]:",
		"[v1.future:host]:80", "[VF.future]",
	} {
		t.Run(fmt.Sprintf("valid_%q", value), func(t *testing.T) {
			require.NoError(t, validateHostValue(value))
		})
	}
	for _, value := range []string{
		"example.com, other.example", "a b", "a\tb", "example.com\r\n",
		"user@example.com", "http://example.com", "example.com/path",
		"example.com?query", "example.com#fragment", "example.com\\path",
		"example.com:+80", "example.com:-1", "example.com:http", "a:80:90",
		"bad%escape", "bad%2", "b\u00fccher.example", "bad\x00host", "bad\xffhost",
		"::1", "[::1", "::1]", "[]", "[127.0.0.1]", "[2001:db8::g]",
		"[::1::2]", "[fe80::1%eth0]", "[fe80::1%25eth0]", "[v1.]", "[vG.host]",
		"[v1.bad%20host]", "[::1]extra", "[::1]:service",
	} {
		t.Run(fmt.Sprintf("invalid_%q", value), func(t *testing.T) {
			require.ErrorIs(t, validateHostValue(value), ErrInvalidHost)
		})
	}
}

func TestRequestAuthority(t *testing.T) {
	tests := []struct {
		name          string
		host          string
		method        string
		requestTarget string
		want          string
	}{
		{"origin uses Host", "example.com:8080", "GET", "/a?b", "example.com:8080"},
		{"asterisk uses Host", "example.com", "OPTIONS", "*", "example.com"},
		{"CONNECT uses target", "other.example:443", "CONNECT", "example.com:443", "example.com:443"},
		{"absolute overrides Host", "other.example", "GET", "http://target.example:8080/a?b", "target.example:8080"},
		{"absolute without path", "other.example", "GET", "https://target.example?q", "target.example"},
		{"absolute escapes preserved", "other.example", "GET", "http://%C3%A9xample.example/", "%C3%A9xample.example"},
		{"absolute IPv6", "other.example", "GET", "http://[::1]:80/", "[::1]:80"},
		{"scheme case", "other.example", "GET", "HTTP://Target.Example/", "Target.Example"},
		{"authority-less absolute", "", "GET", "urn:example:item", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateRequestHost([]string{test.host}, test.method, test.requestTarget))
			require.Equal(t, test.want, RequestAuthority(test.host, test.method, test.requestTarget))
		})
	}
}
