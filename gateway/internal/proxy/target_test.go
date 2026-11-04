package proxy

import (
	"bufio"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
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

		{"CONNECT", "CONNECT example.com:443 HTTP/1.1", "example.com:443", ErrUnsupportedTargetForm},
		{"host:port as scheme", "GET example.com:80 HTTP/1.1", "example.com", ErrUnsupportedScheme},
		{"localhost:port as scheme", "GET localhost:8080 HTTP/1.1", "localhost", ErrUnsupportedScheme},
		{"mailto", "GET mailto:a@b HTTP/1.1", "example.com", ErrUnsupportedScheme},
		{"ftp", "GET ftp://x/y HTTP/1.1", "example.com", ErrUnsupportedScheme},
		{"URN", "GET urn:example:item HTTP/1.1", "", ErrUnsupportedScheme},
		{"Host port too large", "GET / HTTP/1.1", "example.com:65536", ErrInvalidPort},
		{"Host port zero", "GET / HTTP/1.1", "example.com:0", ErrInvalidPort},
		{"Host port overflow", "GET / HTTP/1.1", "example.com:99999999999999999999", ErrInvalidPort},
		{"asterisk Host port", "OPTIONS * HTTP/1.1", "example.com:99999", ErrInvalidPort},
		{"absolute port", "GET http://h:99999/ HTTP/1.1", "other.example", ErrInvalidPort},
		{"absolute port ignores Host", "GET http://h:80/ HTTP/1.1", "other.example:99999", nil},
		{"network-path origin", "GET //evil.example/x HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"network-path in absolute", "GET http://example.com//evil.example/x HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"dot-dot", "GET /a/../../b HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"dot", "GET /a/./b HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"trailing dot-dot", "GET /a/.. HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"encoded dot-dot", "GET /a/%2e%2E/b HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"mixed encoded dot-dot", "GET /a/.%2e HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"encoded dot", "GET /%2E/b HTTP/1.1", "example.com", ErrAmbiguousPath},
		{"absolute dot-dot", "GET http://example.com/a/../b HTTP/1.1", "example.com", ErrAmbiguousPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := test.startLine + "\r\nHost: " + test.host + "\r\n\r\n"
			request, err := http1.ReadRequest(bufio.NewReader(strings.NewReader(message)))
			require.NoError(t, err)
			err = checkRequestTarget(request)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
