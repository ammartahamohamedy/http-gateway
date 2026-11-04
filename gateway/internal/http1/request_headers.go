package http1

import "github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"

// RequestFields holds metadata parsed from the supported request fields for
// body framing and proxy policy. It only carries values that have a consumer;
// raw field values remain in Request.Headers. Cross-checks receive it by value
// as a read-only view; slice members share their backing arrays, so checks must
// not modify them.
type RequestFields struct {
	// Authority is the effective request authority: the target's authority for
	// CONNECT and absolute-form targets that have one, otherwise the Host value.
	// It is preserved as received and never selects the backend destination.
	Authority string

	// ContentLength is meaningful only when ContentLengthPresent is true, which
	// distinguishes an absent field from "Content-Length: 0".
	ContentLength        int64
	ContentLengthPresent bool

	// TransferCodings lists the lowercase Transfer-Encoding codings in applied
	// order, ending in chunked; it is nil when the field is absent. Listing a
	// coding does not mean the gateway can decode it.
	TransferCodings []string
}

// FieldError reports which supported field rule or cross-check rejected a
// request. Error returns the underlying message unchanged, since field errors
// already name their field; Name is for callers using errors.As. Unwrap keeps
// errors.Is working for sentinels such as fields.ErrInvalidHost.
type FieldError struct {
	Name string
	Err  error
}

func (e *FieldError) Error() string { return e.Err.Error() }

func (e *FieldError) Unwrap() error { return e.Err }

// requestFieldRule parses and validates one named field. apply receives only
// that field's separate field-line values, nil when absent, and must handle
// absence itself. It assigns to out only after validation succeeds.
type requestFieldRule struct {
	name  string
	apply func(values []string, line RequestLine, out *RequestFields) error
}

// requestCrossCheck validates a relationship between fields after every field
// rule has succeeded. It reads parsed metadata rather than reparsing fields and
// consults headers only when occurrence information matters.
type requestCrossCheck struct {
	name  string
	apply func(line RequestLine, headers header.Headers, parsed RequestFields) error
}

// requestFieldRules lists the implemented field rules in evaluation order:
// routing and framing fields first, so their errors take precedence. The
// planned order for the remaining fields is Connection, Expect, Content-Type,
// Accept, Content-Encoding, Accept-Encoding, Origin,
// Access-Control-Request-Method, and Access-Control-Request-Headers. Add a
// rule only with its working validator; fields without a rule are accepted
// after generic header validation.
var requestFieldRules = []requestFieldRule{
	{name: "Host", apply: applyHostRule},
	{name: "Content-Length", apply: applyContentLengthRule},
	{name: "Transfer-Encoding", apply: applyTransferEncodingRule},
}

// requestCrossChecks lists the implemented cross-field checks in evaluation
// order. Planned checks include Connection nominating routing or framing fields
// and Expect without a supported body framing context.
var requestCrossChecks = []requestCrossCheck{
	{name: "Transfer-Encoding", apply: checkFramingConflict},
}

// processRequestFields runs every field rule in order and then every
// cross-check, wrapping the first failure in a *FieldError. On failure it
// returns zero RequestFields so partially parsed metadata cannot escape.
func processRequestFields(line RequestLine, headers header.Headers) (RequestFields, error) {
	var parsed RequestFields
	for _, rule := range requestFieldRules {
		if err := rule.apply(headers.Values(rule.name), line, &parsed); err != nil {
			return RequestFields{}, &FieldError{Name: rule.name, Err: err}
		}
	}
	for _, check := range requestCrossChecks {
		if err := check.apply(line, headers, parsed); err != nil {
			return RequestFields{}, &FieldError{Name: check.name, Err: err}
		}
	}
	return parsed, nil
}
