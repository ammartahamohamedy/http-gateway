package body

import (
	"errors"
	"fmt"
)

// Mode is how a message body's end is found. The zero value, Unknown, means not
// yet determined; framing returns it only with an error, so an unassigned Mode
// cannot pass for None.
type Mode int

const (
	Unknown       Mode = iota // Zero value.
	None                      // No body.
	ContentLength             // Exactly Message.ContentLength bytes, possibly zero.
	Chunked                   // Chunked coding; earlier codings are decoded after it.
	UntilClose                // Until the sender closes the connection; responses only.
)

var (
	// ErrInvalidFraming reports inputs for which RFC 9112 defines no body length,
	// or a malformed chunked body.
	ErrInvalidFraming = errors.New("http1: message body framing is invalid")

	// ErrUnsupportedFraming reports a tunnel after a 2xx response to CONNECT.
	ErrUnsupportedFraming = errors.New("http1: message body framing is unsupported")
)

// Message holds the plain inputs NewReader frames a body from.
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

// framing reports how msg's body is delimited, deciding in the order of RFC
// 9112, Section 6.3: the response status (statusFraming), then
// Transfer-Encoding (transferCodingFraming), then Content-Length, whose absence
// means None for requests and UntilClose for responses. It reads no body data
// and checks neither coding support nor size limits. Errors, including the
// inputs checkFramingInputs rejects, match ErrInvalidFraming or
// ErrUnsupportedFraming and return Unknown.
//
// Content-Length: 0 is ContentLength, not None, because the mode records how
// the end is found: forwarding keeps the declared zero, and a response ends at
// once instead of at connection close.
func framing(msg Message) (Mode, error) {
	if err := checkFramingInputs(msg); err != nil {
		return Unknown, err
	}

	if msg.IsResponse {
		mode, err := statusFraming(msg)
		if err != nil || mode != Unknown {
			return mode, err
		}
	}

	if len(msg.TransferCodings) > 0 {
		return transferCodingFraming(msg)
	}

	if msg.ContentLengthPresent {
		return ContentLength, nil
	}

	if msg.IsResponse {
		return UntilClose, nil
	}

	return None, nil
}

// checkFramingInputs rejects inputs no framing applies to: a version other than
// "1.0" or "1.1", a negative Content-Length, and a response status outside
// 100-599.
func checkFramingInputs(msg Message) error {
	if msg.Version != "1.0" && msg.Version != "1.1" {
		return fmt.Errorf("%w: version %q is not 1.0 or 1.1", ErrInvalidFraming, msg.Version)
	}

	if msg.ContentLengthPresent && msg.ContentLength < 0 {
		return fmt.Errorf("%w: negative Content-Length %d", ErrInvalidFraming, msg.ContentLength)
	}

	if msg.IsResponse && (msg.StatusCode < 100 || msg.StatusCode > 599) {
		return fmt.Errorf("%w: status %d is outside 100-599", ErrInvalidFraming, msg.StatusCode)
	}

	return nil
}

// statusFraming applies the response rules that decide framing before any
// field (RFC 9112, Section 6.3, items 1 and 2): a response to HEAD and a 1xx,
// 204, or 304 response have no body, and a 2xx response to CONNECT starts a
// tunnel, which is unsupported. Unknown means the status leaves framing to the
// fields.
func statusFraming(msg Message) (Mode, error) {
	if msg.RequestMethod == "HEAD" || msg.StatusCode < 200 || msg.StatusCode == 204 || msg.StatusCode == 304 {
		return None, nil
	}

	if msg.RequestMethod == "CONNECT" && msg.StatusCode < 300 {
		return Unknown, fmt.Errorf("%w: tunnel after a 2xx response to CONNECT", ErrUnsupportedFraming)
	}

	return Unknown, nil
}

// transferCodingFraming frames a message carrying Transfer-Encoding (RFC 9112,
// Section 6.3, items 3 and 4): codings ending in chunked are Chunked and
// override Content-Length; other codings are UntilClose for responses and
// invalid for requests. Transfer-Encoding in an HTTP/1.0 message is invalid
// (RFC 9112, Section 6.1).
func transferCodingFraming(msg Message) (Mode, error) {
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
