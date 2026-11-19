package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/response"
)

var (
	// upstreamDialTimeout bounds connecting to the backend.
	upstreamDialTimeout = 5 * time.Second

	// upstreamHeadTimeout bounds writing the request and reading the response
	// head, interim responses included, so a backend that never answers cannot
	// hold the exchange. It also bounds each interim write to the client.
	upstreamHeadTimeout = 30 * time.Second

	// responseBodyIdleTimeout bounds each pause while copying the response
	// body, and responseBodyTimeout caps the whole copy and its writes to the
	// client, as the request body timeouts do for uploads.
	responseBodyIdleTimeout = 30 * time.Second
	responseBodyTimeout     = 5 * time.Minute
)

// forward runs one exchange with the backend at config.Upstream for message,
// whose body was buffered as content. It owns the upstream connection from
// dial to close: prepareRequest, request.Write, response.Read, prepareResponse,
// then response.Write to client, copying the body as it arrives.
//
// Interim responses are relayed to client as they arrive, except 100
// (Continue): the gateway answered the client's expectation itself and dropped
// Expect upstream. An interim response prepareResponse refuses is dropped. A
// final response whose transfer codings the gateway cannot decode is refused
// before anything of it is sent.
//
// committed reports whether the final response has started reaching client.
// Only before that can the caller answer an error with its own response;
// afterwards it can only close the connection. Relayed interim responses do not
// commit the exchange.
func forward(client net.Conn, message *request.Message, content []byte, config Config) (committed bool, err error) {
	upstream, err := net.DialTimeout("tcp", config.Upstream, upstreamDialTimeout)
	if err != nil {
		return false, fmt.Errorf("dialing upstream: %w", err)
	}

	defer func() {
		if err := upstream.Close(); err != nil {
			log.Printf("closing upstream connection: %v", err)
		}
	}()

	if err := upstream.SetDeadline(time.Now().Add(upstreamHeadTimeout)); err != nil {
		return false, fmt.Errorf("setting upstream deadline: %w", err)
	}

	requestMethod := message.Line.Method
	prepareRequest(message, content, config.Upstream)

	if err := request.Write(upstream, message); err != nil {
		return false, fmt.Errorf("writing request upstream: %w", err)
	}

	final, err := response.Read(bufio.NewReader(upstream), requestMethod, func(interim *response.Message) error {
		if interim.Line.StatusCode == 100 || prepareResponse(interim) != nil {
			return nil
		}

		if err := client.SetWriteDeadline(time.Now().Add(upstreamHeadTimeout)); err != nil {
			return err
		}

		return response.Write(client, interim, requestMethod)
	})
	if err != nil {
		return false, fmt.Errorf("reading response upstream: %w", err)
	}

	defer func() {
		if err := final.Body.Close(); err != nil {
			log.Printf("closing response body: %v", err)
		}
	}()

	if err := fields.CheckTransferCodingSupport(final.Fields.TransferCodings); err != nil {
		return false, err
	}

	if err := prepareResponse(final); err != nil {
		return false, err
	}

	cutoff := time.Now().Add(responseBodyTimeout)
	final.Body = struct {
		io.Reader
		io.Closer
	}{newIdleDeadlineReader(upstream, final.Body, responseBodyIdleTimeout, cutoff), final.Body}

	if err := client.SetWriteDeadline(cutoff); err != nil {
		return false, fmt.Errorf("setting client write deadline: %w", err)
	}

	if err := response.Write(client, final, requestMethod); err != nil {
		return true, fmt.Errorf("writing response to client: %w", err)
	}

	return true, nil
}
