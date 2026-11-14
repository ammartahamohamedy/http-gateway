package body

import (
	"errors"
	"fmt"
)

// Mode is how a message body's end is found. Framing never returns the zero
// value, Unknown, so an unassigned Mode cannot pass for None.
type Mode int

const (
	Unknown       Mode = iota // Zero value; never returned by Framing.
	None                      // No body.
	ContentLength             // Exactly Message.ContentLength bytes, possibly zero.
	Chunked                   // Chunked coding; earlier codings are decoded after it.
	UntilClose                // Until the sender closes the connection; responses only.
)

var (
	// ErrInvalidFraming reports inputs for which RFC 9112 defines no body length.
	ErrInvalidFraming = errors.New("http1: message body framing is invalid")

	// ErrUnsupportedFraming reports a tunnel after a 2xx response to CONNECT.
	ErrUnsupportedFraming = errors.New("http1: message body framing is unsupported")
)

// Message holds plain framing inputs, keeping this package independent of the
// http1 message types.
type Message struct {
	Version string // "1.0" or "1.1".

	// ContentLength is meaningful only when ContentLengthPresent is true.
	ContentLength        int64
	ContentLengthPresent bool

	// TransferCodings lists lowercase codings in applied order, as returned by
	// fields.ParseTransferEncoding; nil when absent.
	TransferCodings []string

	// Response-only inputs. RequestMethod is the method of the answered request.
	IsResponse    bool
	RequestMethod string
	StatusCode    int
}

// Framing reports how msg's body is delimited, following RFC 9112, Section 6.3.
// It reads no body data and checks neither coding support nor size limits.
//
// HEAD responses and 1xx, 204, and 304 responses have no body. A 2xx response
// to CONNECT is unsupported. Transfer-Encoding ending in chunked is Chunked and
// overrides Content-Length; other codings are UntilClose for responses and
// invalid for requests. Otherwise Content-Length applies, and its absence means
// None for requests and UntilClose for responses.
//
// Content-Length: 0 is ContentLength, not None, because the mode records how
// the end is found: forwarding keeps the declared zero, and a response ends at
// once instead of at connection close.
//
// Invalid inputs: a version other than "1.0" or "1.1", a negative
// Content-Length, a response status outside 100-599, and Transfer-Encoding in
// an HTTP/1.0 message with a body (RFC 9112, Section 6.1). Errors match
// ErrInvalidFraming or ErrUnsupportedFraming and return Unknown.
func Framing(msg Message) (Mode, error) {
	if msg.Version != "1.0" && msg.Version != "1.1" {
		return Unknown, fmt.Errorf("%w: version %q is not 1.0 or 1.1", ErrInvalidFraming, msg.Version)
	}
	if msg.ContentLengthPresent && msg.ContentLength < 0 {
		return Unknown, fmt.Errorf("%w: negative Content-Length %d", ErrInvalidFraming, msg.ContentLength)
	}
	if msg.IsResponse {
		if msg.StatusCode < 100 || msg.StatusCode > 599 {
			return Unknown, fmt.Errorf("%w: status %d is outside 100-599", ErrInvalidFraming, msg.StatusCode)
		}
		if msg.RequestMethod == "HEAD" || msg.StatusCode < 200 || msg.StatusCode == 204 || msg.StatusCode == 304 {
			return None, nil
		}
		if msg.RequestMethod == "CONNECT" && msg.StatusCode < 300 {
			return Unknown, fmt.Errorf("%w: tunnel after a 2xx response to CONNECT", ErrUnsupportedFraming)
		}
	}
	if len(msg.TransferCodings) > 0 {
		if msg.Version == "1.0" {
			return Unknown, fmt.Errorf("%w: Transfer-Encoding in an HTTP/1.0 message", ErrInvalidFraming)
		}
		if msg.TransferCodings[len(msg.TransferCodings)-1] == "chunked" {
			return Chunked, nil
		}
		if msg.IsResponse {
			return UntilClose, nil
		}
		return Unknown, fmt.Errorf("%w: request Transfer-Encoding does not end in chunked", ErrInvalidFraming)
	}
	if msg.ContentLengthPresent {
		return ContentLength, nil
	}
	if msg.IsResponse {
		return UntilClose, nil
	}
	return None, nil
}
