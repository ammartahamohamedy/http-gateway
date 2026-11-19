package response

import (
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

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

func applyContentTypeRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateContentType(values)
}

func applyContentEncodingRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateContentEncoding(values)
}

func applyDateRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateDate(values)
}

func applyLocationRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateLocation(values)
}

func applyAllowRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateAllow(values)
}

func applyRetryAfterRule(values []string, _ Line, _ *Fields) error {
	return fields.ValidateRetryAfter(values)
}

func applyVaryRule(values []string, _ Line, out *Fields) error {
	members, err := fields.ParseVary(values)
	if err != nil {
		return err
	}

	out.Vary = members
	return nil
}

// checkFramingConflict rejects a response framed by both Transfer-Encoding and
// Content-Length rather than letting Transfer-Encoding override it (RFC 9112,
// Section 6.3). This is strict by choice: Content-Length: 0 conflicts too.
func checkFramingConflict(_ Line, _ header.Section, parsed Fields) error {
	if parsed.ContentLengthPresent && len(parsed.TransferCodings) > 0 {
		return fields.ErrFramingConflict
	}

	return nil
}

// checkBodilessContentLength rejects the Content-Length that
// fields.ErrFramingOnBodilessStatus forbids.
func checkBodilessContentLength(line Line, _ header.Section, parsed Fields) error {
	if (line.StatusCode < 200 || line.StatusCode == 204) && parsed.ContentLengthPresent {
		return fields.ErrFramingOnBodilessStatus
	}

	return nil
}

// checkBodilessTransferEncoding rejects the Transfer-Encoding that
// fields.ErrFramingOnBodilessStatus forbids.
func checkBodilessTransferEncoding(line Line, _ header.Section, parsed Fields) error {
	if (line.StatusCode < 200 || line.StatusCode == 204) && len(parsed.TransferCodings) > 0 {
		return fields.ErrFramingOnBodilessStatus
	}

	return nil
}

// checkTrailerNames applies fields.ValidateTrailerNames.
func checkTrailerNames(_ Line, _ header.Section, parsed Fields) error {
	return fields.ValidateTrailerNames(parsed.TrailerNames)
}

// checkConnectionOptions applies fields.ValidateConnectionOptions.
func checkConnectionOptions(_ Line, _ header.Section, parsed Fields) error {
	return fields.ValidateConnectionOptions(parsed.ConnectionOptions)
}
