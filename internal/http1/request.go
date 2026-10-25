package http1

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Patterns are compiled once. The \A and \z anchors require a whole-input match.
var (
	// methodPattern matches a nonempty HTTP token: ASCII letters, digits, and
	// the punctuation permitted by the RFC's tchar grammar.
	methodPattern = regexp.MustCompile("\\A[A-Za-z0-9!#$%&'*+.^_`|~-]+\\z")

	// requestTargetPattern accepts ASCII URI characters and complete %HH escapes,
	// rejecting whitespace, controls, fragments, and malformed escapes anywhere
	// in the target. URI structure is checked separately; escapes are not decoded.
	requestTargetPattern = regexp.MustCompile(`\A(?:[A-Za-z0-9._~:/?\[\]@!$&'()*+,;=-]|%[0-9A-Fa-f]{2})+\z`)

	// portPattern matches one or more ASCII decimal digits, excluding signs
	// and service names. It checks numeric syntax, not the port's value range.
	portPattern = regexp.MustCompile(`\A[0-9]+\z`)
)

// RequestLine contains the method, request-target, and HTTP version number
// from a request's start-line, excluding its CRLF terminator.
type RequestLine struct {
	Method        string
	RequestTarget string
	// Version is the numeric protocol version, such as "1.1".
	Version string
}

// Request holds an HTTP/1.x request-line, header fields, and an optional body
// stream. ReadRequest currently populates only RequestLine, leaving Headers
// and Body nil and their bytes unread in the buffered reader.
type Request struct {
	RequestLine RequestLine
	Headers     []HeaderField
	Body        io.ReadCloser
}

// ParseRequestLine parses line contents without a CRLF terminator. It requires
// exactly three nonempty components separated by single spaces and supports
// HTTP/1.1. Method case and request-target encoding are preserved, and Version
// contains the version number "1.1". It does not check whether a method is
// implemented by the server. This function does not enforce a size limit;
// ReadRequest applies the input limit before calling it. Direct callers
// are responsible for bounding input when a size limit is required.
func ParseRequestLine(rawLine string) (RequestLine, error) {
	parts := strings.Split(rawLine, " ")
	if len(parts) != 3 {
		return RequestLine{}, errors.New(
			"http1: expected exactly three space-separated components",
		)
	}

	method, requestTarget, version := parts[0], parts[1], parts[2]

	if method == "" || requestTarget == "" || version == "" {
		return RequestLine{}, errors.New(
			"http1: request-line components must not be empty",
		)
	}

	if !methodPattern.MatchString(method) {
		return RequestLine{}, fmt.Errorf("http1: invalid method %q", method)
	}
	if err := validateRequestTarget(method, requestTarget); err != nil {
		return RequestLine{}, err
	}
	versionNumber, err := parseHTTPVersion(version)
	if err != nil {
		return RequestLine{}, err
	}

	return RequestLine{
		Method:        method,
		RequestTarget: requestTarget,
		Version:       versionNumber,
	}, nil
}

// validateRequestTarget checks the character repertoire, percent escapes, and
// basic URI structure, including method-specific target forms. CONNECT requires
// a nonempty host and decimal port; asterisk-form is permitted only for OPTIONS.
// Absolute HTTP(S) URIs must include a host and exclude userinfo. The original
// target is preserved, including percent-escape encoding. This function does not
// resolve hosts or check port ranges.
func validateRequestTarget(method, requestTarget string) error {
	if !requestTargetPattern.MatchString(requestTarget) {
		return fmt.Errorf("http1: invalid request-target %q", requestTarget)
	}

	if method == "CONNECT" {
		host, port, err := net.SplitHostPort(requestTarget)
		if err != nil || host == "" || port == "" {
			return errors.New("http1: CONNECT requires a host and numeric port")
		}
		if !portPattern.MatchString(port) {
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
