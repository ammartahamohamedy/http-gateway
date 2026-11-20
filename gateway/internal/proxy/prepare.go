package proxy

import (
	"bytes"
	"io"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
)

// gatewayVia is the Via member the gateway adds: the protocol version it
// received and a pseudonym in place of its host name (RFC 9110, Section 7.6.3).
const gatewayVia = "1.1 http-gateway"

var (
	// hopByHopFields lists the fields that describe one connection and are
	// never forwarded, besides those Connection nominates (RFC 9110, Section
	// 7.6.1). Content-Length, Transfer-Encoding, and Trailer are framing, which
	// the http1 writers set themselves.
	hopByHopFields = []string{"connection", "keep-alive", "proxy-connection", "te", "upgrade"}

	// forwardingFields lists client-supplied forwarding metadata, dropped since
	// the gateway cannot vouch for it.
	forwardingFields = []string{"forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto"}
)

// prepareRequest turns message, whose body was buffered as content, into the
// request sent to upstream, in place. It drops the hop-by-hop fields,
// client-supplied forwarding metadata, and Proxy-Authorization. It removes
// Expect after handling the expectation, sets Host to upstream, appends
// gatewayVia, sets Connection: close for the new connection, and decrements a
// parsed Max-Forwards (RFC 9110, Section 7.6.2). An absolute-form target
// becomes origin-form. The body becomes content, declared with its length
// unless the client sent no body. It removes hop-by-hop trailers before
// request.Write frames the remaining fields.
func prepareRequest(message *request.Message, content []byte, upstream string) {
	headers := withoutHopByHop(message.Headers, message.Fields.ConnectionOptions)

	for name := range slices.Values(forwardingFields) {
		delete(headers, name)
	}

	delete(headers, "expect")
	delete(headers, "proxy-authorization")
	headers["host"] = []string{upstream}
	headers["via"] = append(slices.Clone(headers["via"]), gatewayVia)
	headers["connection"] = []string{"close"}
	if message.Fields.MaxForwardsPresent && message.Fields.MaxForwards > 0 {
		headers["max-forwards"] = []string{strconv.FormatInt(message.Fields.MaxForwards-1, 10)}
	}
	message.Headers = headers
	if len(message.Trailers) > 0 {
		message.Trailers = withoutHopByHop(message.Trailers, message.Fields.ConnectionOptions)
	}

	if message.Line.TargetForm() == request.AbsoluteForm {
		message.Line.RequestTarget = originForm(message.Line.RequestTarget)
	}

	hadBody := message.Fields.ContentLengthPresent || len(message.Fields.TransferCodings) > 0
	message.Fields.ContentLength = int64(len(content))
	message.Fields.ContentLengthPresent = hadBody
	message.Fields.TransferCodings = nil
	message.Body = io.NopCloser(bytes.NewReader(content))
}

// prepareResponse turns message, an interim or final response read from
// upstream, into the one sent to the client, in place. It drops the hop-by-hop
// fields and appends gatewayVia; a final response also gets a Date when it has
// none (RFC 9110, Section 6.6.1) and Connection: close. Interim responses skip
// response.Read's field stage, so Connection is parsed here; an invalid list
// returns the fields.ParseConnection error and leaves message unchanged, and
// the response is not relayed.
func prepareResponse(message *response.Message) error {
	options, err := fields.ParseConnection(message.Headers.Values("Connection"))
	if err != nil {
		return err
	}

	headers := withoutHopByHop(message.Headers, options)
	headers["via"] = append(slices.Clone(headers["via"]), gatewayVia)

	if message.Line.StatusCode >= 200 {
		if len(headers["date"]) == 0 {
			headers["date"] = []string{fields.FormatDate(time.Now())}
		}
		headers["connection"] = []string{"close"}
	}

	message.Headers = headers
	return nil
}

// withoutHopByHop returns a copy of a field section without hopByHopFields and the
// fields that options, lowercase Connection options, nominate.
func withoutHopByHop(received header.Section, options []string) header.Section {
	section := maps.Clone(received)
	if section == nil {
		section = make(header.Section)
	}

	for name := range slices.Values(slices.Concat(hopByHopFields, options)) {
		delete(section, name)
	}

	return section
}
