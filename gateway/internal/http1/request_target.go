package http1

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// requestTargetPattern accepts ASCII URI characters and complete %HH escapes,
// rejecting whitespace, controls, fragments, and malformed escapes anywhere
// in the target. URI structure is checked separately; escapes are not decoded.
var requestTargetPattern = regexp.MustCompile(`\A(?:[A-Za-z0-9._~:/?\[\]@!$&'()*+,;=-]|%[0-9A-Fa-f]{2})+\z`)

// TargetForm identifies one of the four request-target forms (RFC 9112,
// Section 3.2).
type TargetForm int

const (
	OriginForm TargetForm = iota + 1
	AbsoluteForm
	AuthorityForm
	AsteriskForm
)

// TargetForm reports the form of a request-target accepted by ParseRequestLine.
// The result is meaningful only for a validated RequestLine.
func (line RequestLine) TargetForm() TargetForm {
	switch {
	case line.Method == "CONNECT":
		return AuthorityForm
	case line.RequestTarget == "*":
		return AsteriskForm
	case strings.HasPrefix(line.RequestTarget, "/"):
		return OriginForm
	default:
		return AbsoluteForm
	}
}

// validateRequestTarget checks the character repertoire, percent escapes, and
// basic URI structure, including method-specific target forms. CONNECT requires
// a nonempty host and decimal port; asterisk-form is permitted only for OPTIONS.
// Absolute-form accepts any absolute-URI, including opaque ones such as URNs,
// so "host:port" parses as a URI whose scheme is "host". Absolute HTTP(S) URIs
// must include a host and exclude userinfo. The original target is preserved,
// including percent-escape encoding. This function does not resolve hosts,
// check port ranges, or restrict schemes; gateway policy decides which forms
// and schemes are forwarded.
func validateRequestTarget(method, requestTarget string) error {
	if !requestTargetPattern.MatchString(requestTarget) {
		return fmt.Errorf("http1: invalid request-target %q", requestTarget)
	}

	if method == "CONNECT" {
		host, port, err := net.SplitHostPort(requestTarget)
		if err != nil || host == "" || port == "" {
			return errors.New("http1: CONNECT requires a host and numeric port")
		}
		if !grammar.IsDigits(port) {
			return errors.New("http1: CONNECT requires a numeric port")
		}
		authority, err := url.Parse("http://" + requestTarget)
		if err != nil || authority.Host != requestTarget {
			return errors.New("http1: invalid CONNECT authority")
		}
		return nil
	}

	if requestTarget == "*" {
		if method != "OPTIONS" {
			return errors.New("http1: asterisk-form requires OPTIONS")
		}
		return nil
	}

	target, err := url.ParseRequestURI(requestTarget)
	if err != nil {
		return fmt.Errorf("http1: invalid request-target: %w", err)
	}
	if strings.EqualFold(target.Scheme, "http") || strings.EqualFold(target.Scheme, "https") {
		if target.Host == "" || target.User != nil {
			return errors.New("http1: HTTP URI requires a host and must not include userinfo")
		}
	}
	return nil
}
