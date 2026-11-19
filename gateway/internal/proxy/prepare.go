package proxy

import (
	"bytes"
	"io"
	"maps"
	"slices"
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
// request sent to upstream, in place. It drops the hop-by-hop fields, the
// forwarding metadata, and Expect, since the gateway already answered any
// expectation itself; sets Host to upstream; appends gatewayVia; and asks for
// Connection: close, since every exchange uses its own connection. An
// absolute-form target becomes origin-form. The body becomes content, declared
// with its length unless the client sent no body; trailers stay, so request.Write
// frames them.
func prepareRequest(message *request.Message, content []byte, upstream string) {
	headers := withoutHopByHop(message.Headers, message.Fields.ConnectionOptions)

	for name := range slices.Values(forwardingFields) {
		delete(headers, name)
	}

	delete(headers, "expect")
	headers["host"] = []string{upstream}
	headers["via"] = append(slices.Clone(headers["via"]), gatewayVia)
	headers["connection"] = []string{"close"}
	message.Headers = headers

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

// withoutHopByHop returns a copy of headers without hopByHopFields and the
// fields that options, lowercase Connection options, nominate.
func withoutHopByHop(headers header.Section, options []string) header.Section {
	section := maps.Clone(headers)
	if section == nil {
		section = make(header.Section)
	}

	for name := range slices.Values(slices.Concat(hopByHopFields, options)) {
		delete(section, name)
	}

	return section
}
