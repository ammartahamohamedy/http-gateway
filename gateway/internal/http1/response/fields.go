package response

import (
	"slices"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Fields holds metadata parsed from the supported response fields for body
// framing and proxy policy. It only carries values that have a consumer; raw
// field values remain in Message.Headers. Cross-checks receive it by value as a
// read-only view; slice members share their backing arrays, so checks must not
// modify them.
type Fields struct {
	// ContentLength is meaningful only when ContentLengthPresent is true, which
	// distinguishes an absent field from "Content-Length: 0".
	ContentLength        int64
	ContentLengthPresent bool

	// TransferCodings holds the codings from fields.ParseTransferEncoding; nil
	// when absent.
	TransferCodings []string

	// TrailerNames holds the names from fields.ParseTrailer; nil when absent.
	TrailerNames []string

	// ConnectionOptions holds the options from fields.ParseConnection; nil when
	// absent.
	ConnectionOptions []string

	// Vary holds the members from fields.ParseVary; nil when absent.
	Vary []string
}

// fieldRule parses and validates one named field. apply receives only that
// field's separate field-line values, nil when absent, and must handle absence
// itself. It assigns to out only after validation succeeds.
type fieldRule struct {
	name  string
	apply func(values []string, line Line, out *Fields) error
}

// crossCheck validates a relationship between fields, or between a field and
// the status code, after every field rule has succeeded. It reads parsed
// metadata rather than reparsing fields.
type crossCheck struct {
	name  string
	apply func(line Line, headers header.Section, parsed Fields) error
}

// fieldRules lists the field rules in evaluation order: framing and connection
// fields first, so their errors take precedence. Fields without a rule, such as
// Set-Cookie, are accepted after generic header validation.
var fieldRules = []fieldRule{
	{name: "Content-Length", apply: applyContentLengthRule},
	{name: "Transfer-Encoding", apply: applyTransferEncodingRule},
	{name: "Trailer", apply: applyTrailerRule},
	{name: "Connection", apply: applyConnectionRule},
	{name: "Content-Type", apply: applyContentTypeRule},
	{name: "Content-Encoding", apply: applyContentEncodingRule},
	{name: "Date", apply: applyDateRule},
	{name: "Location", apply: applyLocationRule},
	{name: "Allow", apply: applyAllowRule},
	{name: "Retry-After", apply: applyRetryAfterRule},
	{name: "Vary", apply: applyVaryRule},
}

// crossChecks lists the cross-field checks in evaluation order.
var crossChecks = []crossCheck{
	{name: "Transfer-Encoding", apply: checkFramingConflict},
	{name: "Content-Length", apply: checkBodilessContentLength},
	{name: "Transfer-Encoding", apply: checkBodilessTransferEncoding},
	{name: "Trailer", apply: checkTrailerNames},
	{name: "Connection", apply: checkConnectionOptions},
}

// processFields runs every field rule in order and then every cross-check,
// wrapping the first failure in a *http1.FieldError. On failure it returns zero
// Fields so partially parsed metadata cannot escape.
func processFields(line Line, headers header.Section) (Fields, error) {
	var parsed Fields

	for rule := range slices.Values(fieldRules) {
		if err := rule.apply(headers.Values(rule.name), line, &parsed); err != nil {
			return Fields{}, &http1.FieldError{Name: rule.name, Err: err}
		}
	}

	for check := range slices.Values(crossChecks) {
		if err := check.apply(line, headers, parsed); err != nil {
			return Fields{}, &http1.FieldError{Name: check.name, Err: err}
		}
	}

	return parsed, nil
}
