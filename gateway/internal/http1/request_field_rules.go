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

// checkFramingConflict rejects a request framed by both Transfer-Encoding and
// Content-Length rather than letting one override the other (RFC 9112, Section
// 6.3). This is strict by choice: Content-Length: 0 conflicts too.
func checkFramingConflict(_ RequestLine, _ header.Headers, parsed RequestFields) error {
	if parsed.ContentLengthPresent && len(parsed.TransferCodings) > 0 {
		return fields.ErrFramingConflict
	}
	return nil
}
