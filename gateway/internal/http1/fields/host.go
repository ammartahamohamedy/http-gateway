// Package fields validates and interprets individual HTTP fields using their
// values and the message context supplied by the caller.
package fields

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// ErrInvalidHost reports a missing, repeated, or invalid request Host field.
var ErrInvalidHost = errors.New("http1: request requires exactly one valid Host field")

// Host follows URI host/port syntax, not DNS label rules. Bracketed literals
// are captured for IPv6/IPvFuture validation; ports may be empty (RFC 3986).
var (
	hostValuePattern = regexp.MustCompile(`\A(?:\[([^\[\]]+)\]|(?:[A-Za-z0-9._~!$&'()*+,;=-]|%[0-9A-Fa-f]{2})*)(?::[0-9]*)?\z`)
	ipvFuturePattern = regexp.MustCompile(`\A[vV][0-9A-Fa-f]+\.[A-Za-z0-9._~!$&'()*+,;=:-]+\z`)
)

// ValidateRequestHost requires exactly one Host field-line value and checks its
// URI syntax. Values must already have surrounding SP/HTAB removed; method and
// requestTarget must come from a validated request-line. Empty Host is permitted
// only for an absolute target with no authority. It does not resolve names,
// select the routing authority, or rewrite values. Errors match ErrInvalidHost.
func ValidateRequestHost(values []string, method, requestTarget string) error {
	if len(values) != 1 {
		return ErrInvalidHost
	}
	value := values[0]
	if err := validateHostValue(value); err != nil {
		return err
	}
	if value == "" || value[0] == ':' {
		if value != "" || method == "CONNECT" {
			return ErrInvalidHost
		}
		target, err := url.ParseRequestURI(requestTarget)
		if err != nil || !target.IsAbs() || target.Host != "" {
			return ErrInvalidHost
		}
	}
	return nil
}

// RequestAuthority returns the request's effective authority after
// ValidateRequestHost succeeds for the same arguments. CONNECT uses its
// authority-form target, and an absolute-form target with an authority overrides
// Host (RFC 9112, Sections 3.2.2-3.2.3). Otherwise it returns hostValue, which
// is empty for an authority-less absolute target. The result is preserved as
// received; it identifies the requested authority, not the backend destination.
func RequestAuthority(hostValue, method, requestTarget string) string {
	if method == "CONNECT" {
		return requestTarget
	}
	if requestTarget == "*" || strings.HasPrefix(requestTarget, "/") {
		return hostValue
	}
	// Slice the raw authority rather than using url.URL.Host, which decodes
	// percent escapes in reg-name hosts.
	_, rest, _ := strings.Cut(requestTarget, ":")
	authority, found := strings.CutPrefix(rest, "//")
	if !found {
		return hostValue
	}
	if end := strings.IndexAny(authority, "/?"); end >= 0 {
		authority = authority[:end]
	}
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		authority = authority[at+1:]
	}
	if authority == "" {
		return hostValue
	}
	return authority
}

// validateHostValue checks Host's URI syntax without resolving names, decoding
// escapes, or checking TCP port ranges. Empty hosts are syntactically allowed;
// ValidateRequestHost checks whether the request context permits them.
func validateHostValue(value string) error {
	parts := hostValuePattern.FindStringSubmatch(value)
	if parts == nil {
		return ErrInvalidHost
	}
	literal := parts[1]
	if literal == "" || ipvFuturePattern.MatchString(literal) {
		return nil
	}
	address, err := netip.ParseAddr(literal)
	if err != nil || !address.Is6() || address.Zone() != "" {
		return ErrInvalidHost
	}
	return nil
}
