package http1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRequestLine(t *testing.T) {
	tests := []struct {
		name          string
		rawLine       string
		method        string
		requestTarget string
	}{
		{"origin form", "GET / HTTP/1.1", "GET", "/"},
		{"query and escapes", "GET /a%2Fb?q=hello%20world HTTP/1.1", "GET", "/a%2Fb?q=hello%20world"},
		{"query escape casing and literal plus", "GET /?q=%2f%2F&x=a+b HTTP/1.1", "GET", "/?q=%2f%2F&x=a+b"},
		{"escaped percent is not decoded twice", "GET /a%25ZZ?q=%25 HTTP/1.1", "GET", "/a%25ZZ?q=%25"},
		{"absolute form", "GET http://example.com/a?q=1 HTTP/1.1", "GET", "http://example.com/a?q=1"},
		{"HTTPS absolute form", "GET https://example.com/ HTTP/1.1", "GET", "https://example.com/"},
		{"absolute URI query escapes", "GET http://example.com/?q=%20 HTTP/1.1", "GET", "http://example.com/?q=%20"},
		{"opaque absolute URI escapes", "GET urn:example:%2F HTTP/1.1", "GET", "urn:example:%2F"},
		{"asterisk form", "OPTIONS * HTTP/1.1", "OPTIONS", "*"},
		{"authority form", "CONNECT example.com:443 HTTP/1.1", "CONNECT", "example.com:443"},
		{"IPv6 authority", "CONNECT [::1]:443 HTTP/1.1", "CONNECT", "[::1]:443"},
		{"method case preserved", "get / HTTP/1.1", "get", "/"},
		{"extension method", "CUSTOM-42 / HTTP/1.1", "CUSTOM-42", "/"},
		{"method token characters", "M!#$%&'*+-.^_`|~09 / HTTP/1.1", "M!#$%&'*+-.^_`|~09", "/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line, err := ParseRequestLine(test.rawLine)
			require.NoError(t, err)
			require.Equal(t, RequestLine{
				Method:        test.method,
				RequestTarget: test.requestTarget,
				Version:       "1.1",
			}, line)
		})
	}
}

func TestParseRequestLineRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		rawLine string
	}{
		{"empty line", ""},
		{"missing method", " / HTTP/1.1"},
		{"missing target", "GET  HTTP/1.1"},
		{"missing version", "GET / "},
		{"missing component", "GET /"},
		{"extra component", "GET / HTTP/1.1 extra"},
		{"leading space", " GET / HTTP/1.1"},
		{"trailing space", "GET / HTTP/1.1 "},
		{"repeated separator", "GET /  HTTP/1.1"},
		{"tab separator", "GET\t/ HTTP/1.1"},
		{"invalid method delimiter", "GE/T / HTTP/1.1"},
		{"non-ASCII method", "GÉT / HTTP/1.1"},
		{"invalid UTF-8 method", "G\xffT / HTTP/1.1"},
		{"target whitespace", "GET /a\tb HTTP/1.1"},
		{"target control byte", "GET /a\x00b HTTP/1.1"},
		{"target bare CR", "GET /a\rb HTTP/1.1"},
		{"target bare LF", "GET /a\nb HTTP/1.1"},
		{"target fragment", "GET /a#fragment HTTP/1.1"},
		{"invalid URI character", "GET /a|b HTTP/1.1"},
		{"unescaped non-ASCII target", "GET /مرحبا HTTP/1.1"},
		{"invalid UTF-8 target", "GET /\xff HTTP/1.1"},
		{"bad percent escape", "GET /%zz HTTP/1.1"},
		{"bad query escape", "GET /?q=%ZZ HTTP/1.1"},
		{"missing query escape digits", "GET /?q=% HTTP/1.1"},
		{"short query escape", "GET /?q=%2 HTTP/1.1"},
		{"invalid second query escape digit", "GET /?q=%2G HTTP/1.1"},
		{"invalid absolute URI query escape", "GET http://example.com/?q=%ZZ HTTP/1.1"},
		{"invalid opaque URI escape", "GET urn:example:%ZZ HTTP/1.1"},
		{"relative target", "GET relative/path HTTP/1.1"},
		{"absolute HTTP URI without host", "GET http:///path HTTP/1.1"},
		{"HTTP URI userinfo", "GET http://user@example.com/ HTTP/1.1"},
		{"asterisk requires OPTIONS", "GET * HTTP/1.1"},
		{"CONNECT origin target", "CONNECT /path HTTP/1.1"},
		{"CONNECT missing port", "CONNECT example.com HTTP/1.1"},
		{"CONNECT empty host", "CONNECT :443 HTTP/1.1"},
		{"CONNECT empty port", "CONNECT example.com: HTTP/1.1"},
		{"CONNECT service name", "CONNECT example.com:https HTTP/1.1"},
		{"CONNECT path", "CONNECT example.com:443/path HTTP/1.1"},
		{"CONNECT userinfo", "CONNECT user@example.com:443 HTTP/1.1"},
		{"CONNECT query", "CONNECT example.com:443?x HTTP/1.1"},
		{"CONNECT query before port separator", "CONNECT example.com?:443 HTTP/1.1"},
		{"CONNECT path before port separator", "CONNECT example.com/path:443 HTTP/1.1"},
		{"lowercase version", "GET / http/1.1"},
		{"HTTP 1.0 unsupported", "GET / HTTP/1.0"},
		{"unexpected terminator", "GET / HTTP/1.1\r\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line, err := ParseRequestLine(test.rawLine)
			require.Error(t, err)
			require.Equal(t, RequestLine{}, line)
		})
	}
}

func TestParseRequestLineDoesNotEnforceReadLimit(t *testing.T) {
	requestTarget := "/" + strings.Repeat("a", maxRequestLineBytes)
	line, err := ParseRequestLine("GET " + requestTarget + " HTTP/1.1")
	require.NoError(t, err)
	require.Equal(t, RequestLine{Method: "GET", RequestTarget: requestTarget, Version: "1.1"}, line)
}
