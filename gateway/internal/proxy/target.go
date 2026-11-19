// Package proxy coordinates the gateway exchange: connection ownership,
// forwarding policy, and invoking the handwritten http1 readers and writers.
package proxy

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
)

var (
	// ErrUnsupportedTargetForm reports authority-form (CONNECT); tunnels are
	// outside the first release.
	ErrUnsupportedTargetForm = errors.New("proxy: CONNECT tunnels are not supported")
	// ErrUnsupportedScheme reports an absolute-form target whose scheme is not
	// http or https, including "host:port" parsed as a scheme.
	ErrUnsupportedScheme = errors.New("proxy: absolute-form target requires an http or https scheme")
	// ErrInvalidPort reports an authority port outside 1 through 65535.
	ErrInvalidPort = errors.New("proxy: authority port must be between 1 and 65535")
	// ErrAmbiguousPath reports a path a backend could resolve differently from
	// the gateway: a leading "//" or a dot-segment, literal or percent-encoded.
	ErrAmbiguousPath = errors.New("proxy: request path contains an empty leading segment or dot-segment")
)

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
		return ErrUnsupportedTargetForm
	case request.AsteriskForm:
		return checkPort(message.Fields.Authority)
	case request.OriginForm:
		path = target
	case request.AbsoluteForm:
		scheme, _, _ := strings.Cut(target, ":")
		if !strings.EqualFold(scheme, "http") && !strings.EqualFold(scheme, "https") {
			return ErrUnsupportedScheme
		}
		path = originForm(target)
	}

	if err := checkPort(message.Fields.Authority); err != nil {
		return err
	}

	return checkPath(path)
}

// originForm returns the origin-form of an http(s) absolute-form target: its
// path and query, with "/" for an empty path (RFC 9112, Section 3.2.1). A
// request forwarded to an origin server uses origin-form (Section 3.2.2).
// request.Read guarantees "//" and a nonempty authority for such targets.
func originForm(target string) string {
	rest := strings.TrimPrefix(target[strings.IndexByte(target, ':')+1:], "//")

	start := strings.IndexAny(rest, "/?")
	if start < 0 {
		return "/"
	}

	if rest[start] == '?' {
		return "/" + rest[start:]
	}

	return rest[start:]
}

// checkPort validates the port of a URI authority already checked by request.Read.
// An empty port is permitted (RFC 3986, Section 3.2.3).
func checkPort(authority string) error {
	colon := strings.LastIndexByte(authority, ':')
	if colon < 0 || colon < strings.LastIndexByte(authority, ']') {
		return nil
	}

	port := authority[colon+1:]
	if port == "" {
		return nil
	}

	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return ErrInvalidPort
	}

	return nil
}

// checkPath rejects a path beginning with "//" and any "." or ".." segment,
// comparing %2E escapes without regard to case. The query is not inspected.
func checkPath(path string) error {
	path = strings.SplitN(path, "?", 2)[0]
	if strings.HasPrefix(path, "//") {
		return ErrAmbiguousPath
	}

	for segment := range strings.SplitSeq(path, "/") {
		switch strings.ReplaceAll(strings.ToLower(segment), "%2e", ".") {
		case ".", "..":
			return ErrAmbiguousPath
		}
	}

	return nil
}
