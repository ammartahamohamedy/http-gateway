package fields

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// continueExpectation is the only expectation RFC 9110 defines.
const continueExpectation = "100-continue"

var (
	// ErrInvalidExpect reports a malformed Expect list.
	ErrInvalidExpect = errors.New("http1: Expect requires a valid expectation list")

	// ErrExpectWithoutContent reports 100-continue on a request with no content,
	// where a client could wait for an interim response that has no purpose.
	ErrExpectWithoutContent = errors.New("http1: Expect 100-continue requires request content")

	// ErrUnsupportedExpectation reports an expectation the gateway cannot meet.
	ErrUnsupportedExpectation = errors.New("http1: unsupported expectation")
)

// ParseExpect returns the request's expectations in received order, or nil when
// the field is absent or empty. Values must have surrounding SP/HTAB already
// removed. Repeated field lines form one list, and empty members are ignored
// (RFC 9110, Section 5.6.1).
//
// Each member is token [ "=" ( token / quoted-string ) parameters ] (RFC 9110,
// Section 10.1.1), so parameters require a value. Names are lowercased because
// the field is case-insensitive; any value and parameters are kept as received,
// which keeps 100-continue=x distinct from 100-continue. Unfamiliar expectations
// are accepted here; CheckExpectationSupport checks support. Errors match
// ErrInvalidExpect and return nil. The input is not changed.
func ParseExpect(values []string) ([]string, error) {
	var expectations []string
	for value := range slices.Values(values) {
		members, ok := grammar.SplitList(value)
		if !ok {
			return nil, ErrInvalidExpect
		}
		for member := range slices.Values(members) {
			expectation, ok := parseExpectation(member)
			if !ok {
				return nil, ErrInvalidExpect
			}
			expectations = append(expectations, expectation)
		}
	}
	return expectations, nil
}

// ValidateExpectContent rejects 100-continue when hasContent is false; clients
// must not send it on a request without content (RFC 9110, Section 10.1.1). It
// expects expectations from ParseExpect. Errors match ErrExpectWithoutContent.
func ValidateExpectContent(expectations []string, hasContent bool) error {
	if !hasContent && slices.Contains(expectations, continueExpectation) {
		return ErrExpectWithoutContent
	}
	return nil
}

// CheckExpectationSupport checks expectations from ParseExpect; the gateway
// supports only continueExpectation. The first other member returns
// ErrUnsupportedExpectation.
func CheckExpectationSupport(expectations []string) error {
	for expectation := range slices.Values(expectations) {
		if expectation != continueExpectation {
			return fmt.Errorf("%w: %q", ErrUnsupportedExpectation, expectation)
		}
	}
	return nil
}

// parseExpectation validates one list member and returns it with its name
// lowercased. The name is checked first so lowercasing only sees ASCII.
func parseExpectation(member string) (string, bool) {
	parts, ok := grammar.SplitUnquoted(member, ';')
	if !ok {
		return "", false
	}
	head := grammar.TrimOWS(parts[0])
	name, value, hasValue := strings.Cut(head, "=")
	if !grammar.IsToken(name) {
		return "", false
	}
	parameters := parts[1:]
	if !hasValue {
		return strings.ToLower(name), len(parameters) == 0
	}
	if !grammar.IsToken(value) && !grammar.IsQuotedString(value) {
		return "", false
	}
	for parameter := range slices.Values(parameters) {
		parameter = grammar.TrimOWS(parameter)
		if parameter != "" && !grammar.IsParameter(parameter) {
			return "", false
		}
	}
	return strings.ToLower(name) + member[len(name):], true
}
