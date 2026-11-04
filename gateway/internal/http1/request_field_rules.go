package http1

import (
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

func applyHostRule(values []string, line RequestLine, out *RequestFields) error {
	err := fields.ValidateRequestHost(values, line.Method, line.RequestTarget)
	if err != nil {
		return err
	}
	out.Authority = fields.RequestAuthority(values[0], line.Method, line.RequestTarget)
	return nil
}

func applyContentLengthRule(values []string, _ RequestLine, out *RequestFields) error {
	length, present, err := fields.ParseContentLength(values)
	if err != nil {
		return err
	}
	out.ContentLength = length
	out.ContentLengthPresent = present
	return nil
}

func applyTransferEncodingRule(values []string, _ RequestLine, out *RequestFields) error {
	codings, err := fields.ParseTransferEncoding(values)
	if err != nil {
		return err
	}
	out.TransferCodings = codings
	return nil
}

func applyConnectionRule(values []string, _ RequestLine, out *RequestFields) error {
	options, err := fields.ParseConnection(values)
	if err != nil {
		return err
	}
	out.ConnectionOptions = options
	return nil
}

func applyExpectRule(values []string, _ RequestLine, out *RequestFields) error {
	expectations, err := fields.ParseExpect(values)
	if err != nil {
		return err
	}
	out.Expectations = expectations
	return nil
}

func applyContentTypeRule(values []string, _ RequestLine, _ *RequestFields) error {
	return fields.ValidateContentType(values)
}

func applyAcceptRule(values []string, _ RequestLine, _ *RequestFields) error {
	return fields.ValidateAccept(values)
}

func applyContentEncodingRule(values []string, _ RequestLine, _ *RequestFields) error {
	return fields.ValidateContentEncoding(values)
}

func applyAcceptEncodingRule(values []string, _ RequestLine, _ *RequestFields) error {
	return fields.ValidateAcceptEncoding(values)
}

// checkFramingConflict rejects a request framed by both Transfer-Encoding and
// Content-Length rather than letting one override the other (RFC 9112, Section
// 6.3). This is strict by choice: Content-Length: 0 conflicts too.
func checkFramingConflict(_ RequestLine, _ header.Headers, parsed RequestFields) error {
	if parsed.ContentLengthPresent && len(parsed.TransferCodings) > 0 {
		return fields.ErrFramingConflict
	}
	return nil
}

// checkConnectionOptions rejects nominations of routing and framing fields,
// which forwarding would otherwise have to remove.
func checkConnectionOptions(_ RequestLine, _ header.Headers, parsed RequestFields) error {
	return fields.ValidateConnectionOptions(parsed.ConnectionOptions)
}

// checkExpectContent rejects 100-continue on a request with no content. Chunked
// framing counts as content here, even though the gateway does not decode it.
func checkExpectContent(_ RequestLine, _ header.Headers, parsed RequestFields) error {
	hasContent := parsed.ContentLength > 0 || len(parsed.TransferCodings) > 0
	return fields.ValidateExpectContent(parsed.Expectations, hasContent)
}
