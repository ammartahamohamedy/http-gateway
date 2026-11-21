package proxy

import (
	"bufio"
	"slices"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/stretchr/testify/require"
)

func TestCheckRequestTarget(t *testing.T) {
	tests := []struct {
		name      string
		startLine string
		host      string
		wantErr   error
	}{
		{"origin", "GET /search?q=go HTTP/1.1", "example.com", nil},
		{"origin root", "GET / HTTP/1.1", "example.com", nil},
		{"origin with port", "GET / HTTP/1.1", "example.com:8080", nil},
		{"empty Host port", "GET / HTTP/1.1", "example.com:", nil},
		{"IPv6 Host", "GET / HTTP/1.1", "[::1]:65535", nil},
		{"dots inside segments", "GET /a.b/..c/.d./%2e%2ex HTTP/1.1", "example.com", nil},
		{"dots in query", "GET /a?p=../.. HTTP/1.1", "example.com", nil},
		{"empty inner segment", "GET /a//b HTTP/1.1", "example.com", nil},
		{"absolute HTTP", "GET http://example.com/a HTTP/1.1", "other.example", nil},
		{"absolute HTTPS scheme case", "GET HTTPS://example.com:443 HTTP/1.1", "other.example", nil},
		{"absolute query only", "GET http://example.com?q HTTP/1.1", "other.example", nil},
		{"asterisk", "OPTIONS * HTTP/1.1", "example.com", nil},

		{"CONNECT", "CONNECT example.com:443 HTTP/1.1", "example.com:443", errUnsupportedTargetForm},
		{"host:port as scheme", "GET example.com:80 HTTP/1.1", "example.com", errUnsupportedScheme},
		{"localhost:port as scheme", "GET localhost:8080 HTTP/1.1", "localhost", errUnsupportedScheme},
		{"mailto", "GET mailto:a@b HTTP/1.1", "example.com", errUnsupportedScheme},
		{"ftp", "GET ftp://x/y HTTP/1.1", "example.com", errUnsupportedScheme},
		{"URN", "GET urn:example:item HTTP/1.1", "", errUnsupportedScheme},
		{"Host port too large", "GET / HTTP/1.1", "example.com:65536", errInvalidPort},
		{"Host port zero", "GET / HTTP/1.1", "example.com:0", errInvalidPort},
		{"Host port overflow", "GET / HTTP/1.1", "example.com:99999999999999999999", errInvalidPort},
		{"asterisk Host port", "OPTIONS * HTTP/1.1", "example.com:99999", errInvalidPort},
		{"absolute port", "GET http://h:99999/ HTTP/1.1", "other.example", errInvalidPort},
		{"absolute port ignores Host", "GET http://h:80/ HTTP/1.1", "other.example:99999", nil},
		{"network-path origin", "GET //evil.example/x HTTP/1.1", "example.com", errAmbiguousPath},
		{"network-path in absolute", "GET http://example.com//evil.example/x HTTP/1.1", "example.com", errAmbiguousPath},
		{"dot-dot", "GET /a/../../b HTTP/1.1", "example.com", errAmbiguousPath},
		{"dot", "GET /a/./b HTTP/1.1", "example.com", errAmbiguousPath},
		{"trailing dot-dot", "GET /a/.. HTTP/1.1", "example.com", errAmbiguousPath},
		{"encoded dot-dot", "GET /a/%2e%2E/b HTTP/1.1", "example.com", errAmbiguousPath},
		{"mixed encoded dot-dot", "GET /a/.%2e HTTP/1.1", "example.com", errAmbiguousPath},
		{"encoded dot", "GET /%2E/b HTTP/1.1", "example.com", errAmbiguousPath},
		{"absolute dot-dot", "GET http://example.com/a/../b HTTP/1.1", "example.com", errAmbiguousPath},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			raw := test.startLine + "\r\nHost: " + test.host + "\r\n\r\n"
			message, err := request.Read(bufio.NewReader(strings.NewReader(raw)))
			require.NoError(t, err)
			err = checkRequestTarget(message)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOriginTarget(t *testing.T) {
	tests := []struct {
		method string
		target string
		want   string
	}{
		{"GET", "http://example.com/a/b?q=1", "/a/b?q=1"},
		{"GET", "http://example.com", "/"},
		{"GET", "http://example.com:8080", "/"},
		{"GET", "HTTPS://example.com/", "/"},
		{"GET", "http://example.com?q", "/?q"},
		{"GET", "http://[::1]:8080/a", "/a"},
		{"GET", "http://example.com/a%2Fb?x=%20", "/a%2Fb?x=%20"},
		{"OPTIONS", "http://example.com", "*"},
		{"OPTIONS", "http://example.com:8080", "*"},
		{"OPTIONS", "http://example.com/", "/"},
		{"OPTIONS", "http://example.com?q", "/?q"},
		{"OPTIONS", "http://example.com?", "/?"},
		{"OPTIONS", "http://example.com/a", "/a"},
	}

	for test := range slices.Values(tests) {
		t.Run(test.method+" "+test.target, func(t *testing.T) {
			require.Equal(t, test.want, originTarget(test.method, test.target))
		})
	}
}

func TestSplitAuthority(t *testing.T) {
	tests := []struct {
		authority string
		host      string
		port      string
	}{
		{"example.com", "example.com", ""},
		{"example.com:8080", "example.com", "8080"},
		{"example.com:", "example.com", ""},
		{"[::1]", "[::1]", ""},
		{"[::1]:8080", "[::1]", "8080"},
		{"", "", ""},
	}

	for test := range slices.Values(tests) {
		t.Run(test.authority, func(t *testing.T) {
			host, port := splitAuthority(test.authority)
			require.Equal(t, test.host, host)
			require.Equal(t, test.port, port)
		})
	}
}

func TestCheckHost(t *testing.T) {
	hosts := []string{"localhost", "127.0.0.1", "[::1]", "Example.com"}

	tests := []struct {
		name      string
		authority string
		hosts     []string
		served    bool
	}{
		{"name", "localhost", hosts, true},
		{"name with port", "localhost:8080", hosts, true},
		{"case-insensitive", "EXAMPLE.COM", hosts, true},
		{"IPv4 literal", "127.0.0.1:8080", hosts, true},
		{"IPv6 literal", "[::1]:8080", hosts, true},
		{"another host", "evil.example", hosts, false},
		{"served name as a suffix", "evil.localhost", hosts, false},
		{"empty authority", "", append(slices.Clone(hosts), ""), false},
		{"no hosts configured", "localhost", nil, false},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			err := checkHost(test.authority, test.hosts)
			if test.served {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUnservedHost)
		})
	}
}
