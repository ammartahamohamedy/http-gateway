package http1

import "github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"

// validateRequestHeaders coordinates field-specific validation and rules involving
// multiple request fields after request-line and generic header parsing. It
// currently validates Host followed by Content-Length, returning the first error
// unchanged. Parsed length metadata is not retained until body framing is added.
func validateRequestHeaders(request *Request) error {
	if err := fields.ValidateRequestHost(
		request.Headers.Values("Host"),
		request.RequestLine.Method,
		request.RequestLine.RequestTarget,
	); err != nil {
		return err
	}
	_, _, err := fields.ParseContentLength(request.Headers.Values("Content-Length"))
	return err
}
