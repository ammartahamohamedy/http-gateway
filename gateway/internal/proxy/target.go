// Package proxy coordinates the gateway exchange: connection ownership,
// forwarding policy, and invoking the handwritten http1 readers and writers.
package proxy

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
)

var (
	// errUnsupportedTargetForm reports authority-form (CONNECT); tunnels are
	// outside the first release.
	errUnsupportedTargetForm = errors.New("proxy: CONNECT tunnels are not supported")
	// errUnsupportedScheme reports an absolute-form target whose scheme is not
	// http or https, including "host:port" parsed as a scheme.
	errUnsupportedScheme = errors.New("proxy: absolute-form target requires an http or https scheme")
	// errInvalidPort reports an authority port outside 1 through 65535.
	errInvalidPort = errors.New("proxy: authority port must be between 1 and 65535")
	// errAmbiguousPath reports a path a backend could resolve differently from
	// the gateway: a leading "//" or a dot-segment, literal or percent-encoded.
	errAmbiguousPath = errors.New("proxy: request path contains an empty leading segment or dot-segment")
	// errUnservedHost reports a request whose authority names a host the
	// gateway does not serve.
	errUnservedHost = errors.New("proxy: request names a host the gateway does not serve")
)

// checkHost requires authority, the request's effective authority, to name one
// of hosts, compared case-insensitively without the port (RFC 9110, Section
// 7.4). An empty host is never served.
func checkHost(authority string, hosts []string) error {
	host, _ := splitAuthority(authority)

	served := slices.ContainsFunc(hosts, func(candidate string) bool {
		return strings.EqualFold(candidate, host)
	})
	if host == "" || !served {
		return errUnservedHost
	}

	return nil
}

// checkRequestTarget applies gateway policy to a request accepted by
// request.Read. Origin-form, http(s) absolute-form, and asterisk-form are
// forwarded; CONNECT is rejected. The effective authority's port, if nonempty,
// must be in range. Paths are rejected rather than normalized, so forwarded
// target bytes are never rewritten.
func checkRequestTarget(message *request.Message) error {
	target := message.Line.RequestTarget
	var path string

	switch message.Line.TargetForm() {
	case request.AuthorityForm:
		return errUnsupportedTargetForm
	case request.AsteriskForm:
		return checkPort(message.Fields.Authority)
	case request.OriginForm:
		path = target
	case request.AbsoluteForm:
		scheme, _, _ := strings.Cut(target, ":")
		if !strings.EqualFold(scheme, "http") && !strings.EqualFold(scheme, "https") {
			return errUnsupportedScheme
		}
		path = originTarget(message.Line.Method, target)
	}

	if err := checkPort(message.Fields.Authority); err != nil {
		return err
	}

	return checkPath(path)
}

// originTarget returns the request-target sent to the origin server for an
// http(s) absolute-form target (RFC 9112, Section 3.2.2): its origin-form, the
// path and query with "/" for an empty path (Section 3.2.1), except that an
// OPTIONS request with an empty path and no query becomes "*", since it asks
// about the server as a whole (Section 3.2.4). request.Read guarantees "//" and
// a nonempty host for such targets.
func originTarget(method, target string) string {
	rest := strings.TrimPrefix(target[strings.IndexByte(target, ':')+1:], "//")

	start := strings.IndexAny(rest, "/?")
	switch {
	case start < 0 && method == "OPTIONS":
		return "*"
	case start < 0:
		return "/"
	case rest[start] == '?':
		return "/" + rest[start:]
	}

	return rest[start:]
}

// checkPort validates the port of a URI authority already checked by request.Read.
// An empty port is permitted (RFC 3986, Section 3.2.3).
func checkPort(authority string) error {
	_, port := splitAuthority(authority)
	if port == "" {
		return nil
	}

	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return errInvalidPort
	}

	return nil
}

// checkPath rejects a path beginning with "//" and any "." or ".." segment,
// comparing %2E escapes without regard to case. The query is not inspected.
func checkPath(path string) error {
	path = strings.SplitN(path, "?", 2)[0]
	if strings.HasPrefix(path, "//") {
		return errAmbiguousPath
	}

	for segment := range strings.SplitSeq(path, "/") {
		switch strings.ReplaceAll(strings.ToLower(segment), "%2e", ".") {
		case ".", "..":
			return errAmbiguousPath
		}
	}

	return nil
}

// splitAuthority splits a URI authority already checked by request.Read into
// its host, with brackets kept around an IP literal, and its port, empty when
// absent.
func splitAuthority(authority string) (host, port string) {
	colon := strings.LastIndexByte(authority, ':')
	if colon < 0 || colon < strings.LastIndexByte(authority, ']') {
		return authority, ""
	}

	return authority[:colon], authority[colon+1:]
}
