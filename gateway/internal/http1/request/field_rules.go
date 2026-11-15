package request

import (
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

func applyHostRule(values []string, line Line, out *Fields) error {
	err := fields.ValidateRequestHost(values, line.Method, line.RequestTarget)
	if err != nil {
		return err
	}

	out.Authority = fields.RequestAuthority(values[0], line.Method, line.RequestTarget)
	return nil
}

func applyContentLengthRule(values []string, _ Line, out *Fields) error {
	length, present, err := fields.ParseContentLength(values)
	if err != nil {
		return err
	}

	out.ContentLength = length
	out.ContentLengthPresent = present
	return nil
}

func applyTransferEncodingRule(values []string, _ Line, out *Fields) error {
	codings, err := fields.ParseTransferEncoding(values)
	if err != nil {
		return err
	}

	if err := fields.ValidateRequestTransferCodings(codings); err != nil {
		return err
	}

	out.TransferCodings = codings
	return nil
}

func applyTrailerRule(values []string, _ Line, out *Fields) error {
	names, err := fields.ParseTrailer(values)
	if err != nil {
		return err
	}

	out.TrailerNames = names
	return nil
}

func applyConnectionRule(values []string, _ Line, out *Fields) error {
	options, err := fields.ParseConnection(values)
	if err != nil {
		return err
	}

	out.ConnectionOptions = options
	return nil
}

func applyExpectRule(values []string, _ Line, out *Fields) error {
	expectations, err := fields.ParseExpect(values)
	if err != nil {
		return err
	}

	out.Expectations = expectations
	return nil
}

func applyContentTypeRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateContentType(values)
}

func applyAcceptRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateAccept(values)
}

func applyContentEncodingRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateContentEncoding(values)
}

func applyAcceptEncodingRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateAcceptEncoding(values)
}

func applyOriginRule(values []string, _ Line, out *Fields) error {
	origin, err := fields.ParseOrigin(values)
	if err != nil {
		return err
	}

	out.Origin = origin
	return nil
}

func applyAccessControlRequestMethodRule(values []string, _ Line, out *Fields) error {
	method, err := fields.ParseAccessControlRequestMethod(values)
	if err != nil {
		return err
	}

	out.AccessControlRequestMethod = method
	return nil
}

func applyAccessControlRequestHeadersRule(values []string, _ Line, out *Fields) error {
	names, err := fields.ParseAccessControlRequestHeaders(values)
	if err != nil {
		return err
	}

	out.AccessControlRequestHeaders = names
	return nil
}

// checkFramingConflict rejects a request framed by both Transfer-Encoding and
// Content-Length rather than letting one override the other (RFC 9112, Section
// 6.3). This is strict by choice: Content-Length: 0 conflicts too.
func checkFramingConflict(_ Line, _ header.Headers, parsed Fields) error {
	if parsed.ContentLengthPresent && len(parsed.TransferCodings) > 0 {
		return fields.ErrFramingConflict
	}

	return nil
}

// checkTrailerNames applies fields.ValidateTrailerNames.
func checkTrailerNames(_ Line, _ header.Headers, parsed Fields) error {
	return fields.ValidateTrailerNames(parsed.TrailerNames)
}

// checkConnectionOptions applies fields.ValidateConnectionOptions.
func checkConnectionOptions(_ Line, _ header.Headers, parsed Fields) error {
	return fields.ValidateConnectionOptions(parsed.ConnectionOptions)
}

// checkExpectContent applies fields.ValidateExpectContent. Chunked framing counts
// as content.
func checkExpectContent(_ Line, _ header.Headers, parsed Fields) error {
	hasContent := parsed.ContentLength > 0 || len(parsed.TransferCodings) > 0
	return fields.ValidateExpectContent(parsed.Expectations, hasContent)
}
