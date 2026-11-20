package request

import (
	"slices"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// Fields holds metadata parsed from the supported request fields for
// body framing and proxy policy. It only carries values that have a consumer;
// raw field values remain in Message.Headers. Cross-checks receive it by value
// as a read-only view; slice members share their backing arrays, so checks must
// not modify them.
type Fields struct {
	// Authority is the effective request authority from fields.RequestAuthority.
	Authority string

	// ContentLength is meaningful only when ContentLengthPresent is true, which
	// distinguishes an absent field from "Content-Length: 0".
	ContentLength        int64
	ContentLengthPresent bool

	// TransferCodings holds the codings from fields.ParseTransferEncoding, ending in
	// chunked; nil when absent.
	TransferCodings []string

	// TrailerNames holds the names from fields.ParseTrailer; nil when absent.
	TrailerNames []string

	// ConnectionOptions holds the options from fields.ParseConnection; nil when
	// absent.
	ConnectionOptions []string

	// Expectations holds the members from fields.ParseExpect; nil when absent.
	Expectations []string

	// MaxForwards is meaningful only when MaxForwardsPresent is true. Both are
	// set only for OPTIONS and TRACE, the methods Max-Forwards applies to.
	MaxForwards        int64
	MaxForwardsPresent bool

	// Origin is the value from fields.ParseOrigin, or empty when absent.
	Origin string

	// AccessControlRequestMethod is the value from
	// fields.ParseAccessControlRequestMethod, or empty when absent.
	AccessControlRequestMethod string

	// AccessControlRequestHeaders holds the names from
	// fields.ParseAccessControlRequestHeaders; nil when absent.
	AccessControlRequestHeaders []string
}

// fieldRule parses and validates one named field. apply receives only
// that field's separate field-line values, nil when absent, and must handle
// absence itself. It assigns to out only after validation succeeds.
type fieldRule struct {
	name  string
	apply func(values []string, line Line, out *Fields) error
}

// crossCheck validates a relationship between fields after every field
// rule has succeeded. It reads parsed metadata rather than reparsing fields and
// consults headers only when occurrence information matters.
type crossCheck struct {
	name  string
	apply func(line Line, headers header.Section, parsed Fields) error
}

// fieldRules lists the field rules in evaluation order: routing and
// framing fields first, so their errors take precedence. Fields without a rule
// are accepted after generic header validation.
var fieldRules = []fieldRule{
	{name: "Host", apply: applyHostRule},
	{name: "Content-Length", apply: applyContentLengthRule},
	{name: "Transfer-Encoding", apply: applyTransferEncodingRule},
	{name: "Trailer", apply: applyTrailerRule},
	{name: "Connection", apply: applyConnectionRule},
	{name: "Expect", apply: applyExpectRule},
	{name: "Max-Forwards", apply: applyMaxForwardsRule},
	{name: "Content-Type", apply: applyContentTypeRule},
	{name: "Accept", apply: applyAcceptRule},
	{name: "Content-Encoding", apply: applyContentEncodingRule},
	{name: "Accept-Encoding", apply: applyAcceptEncodingRule},
	{name: "Origin", apply: applyOriginRule},
	{name: "Access-Control-Request-Method", apply: applyAccessControlRequestMethodRule},
	{name: "Access-Control-Request-Headers", apply: applyAccessControlRequestHeadersRule},
}

// crossChecks lists the cross-field checks in evaluation order.
var crossChecks = []crossCheck{
	{name: "Transfer-Encoding", apply: checkFramingConflict},
	{name: "Trailer", apply: checkTrailerNames},
	{name: "Connection", apply: checkConnectionOptions},
	{name: "Expect", apply: checkExpectContent},
}

// processFields runs every field rule in order and then every
// cross-check, wrapping the first failure in a *http1.FieldError. On failure it
// returns zero Fields so partially parsed metadata cannot escape.
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
