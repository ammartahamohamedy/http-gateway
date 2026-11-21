package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"time"

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
// whose body was buffered as content, and owns the upstream connection from
// dial to close. Its stages are dialUpstream, sendRequest, receiveResponse, and
// relayResponse.
//
// committed reports whether the final response has started reaching client.
// Only before that can the caller answer an error with its own response;
// afterwards it can only close the connection. Relayed interim responses do not
// commit the exchange.
func forward(client net.Conn, message *request.Message, content []byte, config Config) (committed bool, err error) {
	upstream, err := dialUpstream(config.Upstream)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := upstream.Close(); err != nil {
			log.Printf("closing upstream connection: %v", err)
		}
	}()

	requestMethod := message.Line.Method

	if err := sendRequest(upstream, message, content, config.Upstream); err != nil {
		return false, err
	}

	final, err := receiveResponse(upstream, client, requestMethod)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := final.Body.Close(); err != nil {
			log.Printf("closing response body: %v", err)
		}
	}()

	return relayResponse(client, upstream, final, requestMethod)
}

// dialUpstream connects to the backend at address within upstreamDialTimeout
// and starts the upstreamHeadTimeout deadline, which covers sending the request
// and reading every response head.
func dialUpstream(address string) (net.Conn, error) {
	upstream, err := net.DialTimeout("tcp", address, upstreamDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dialing upstream: %w", err)
	}

	if err := upstream.SetDeadline(time.Now().Add(upstreamHeadTimeout)); err != nil {
		if closeErr := upstream.Close(); closeErr != nil {
			log.Printf("closing upstream connection: %v", closeErr)
		}
		return nil, fmt.Errorf("setting upstream deadline: %w", err)
	}

	return upstream, nil
}

// sendRequest prepares message for the backend at address (prepareRequest),
// with content as its body, and writes it to upstream.
func sendRequest(upstream net.Conn, message *request.Message, content []byte, address string) error {
	prepareRequest(message, content, address)

	if err := request.Write(upstream, message); err != nil {
		return fmt.Errorf("writing request upstream: %w", err)
	}

	return nil
}

// receiveResponse reads the final response to a requestMethod request from
// upstream, relaying interim responses to client as they arrive (relayInterim),
// and prepares it for client (prepareResponse). A final response that
// prepareResponse refuses is closed and returned as an error before anything
// of it is sent, so the caller still owns the answer. Transfer codings are not
// checked here: one the gateway cannot decode fails on the first body read,
// before any byte reaches the client, and a bodiless response is never read.
func receiveResponse(upstream, client net.Conn, requestMethod string) (*response.Message, error) {
	final, err := response.Read(bufio.NewReader(upstream), requestMethod, relayInterim(client, requestMethod))
	if err != nil {
		return nil, fmt.Errorf("reading response upstream: %w", err)
	}

	if err := prepareResponse(final); err != nil {
		if closeErr := final.Body.Close(); closeErr != nil {
			log.Printf("closing response body: %v", closeErr)
		}
		return nil, err
	}

	return final, nil
}

// relayInterim returns the response.Read callback that relays interim
// responses to a requestMethod request to client, each within
// upstreamHeadTimeout. It drops 100 (Continue), since the gateway answered the
// client's expectation itself and dropped Expect upstream, and drops an interim
// response prepareResponse refuses rather than failing the exchange.
func relayInterim(client net.Conn, requestMethod string) func(*response.Message) error {
	return func(interim *response.Message) error {
		if interim.Line.StatusCode == 100 || prepareResponse(interim) != nil {
			return nil
		}

		if err := client.SetWriteDeadline(time.Now().Add(upstreamHeadTimeout)); err != nil {
			return err
		}

		return response.Write(client, interim, requestMethod)
	}
}

// relayResponse writes final, the response to a requestMethod request, to
// client, copying its body from upstream as it arrives. Each body read may
// wait responseBodyIdleTimeout, and the copy and its writes end within
// responseBodyTimeout. The exchange commits with the first byte that reaches
// client (commitWriter), so a failure while response.Write still holds
// everything in its buffer leaves it uncommitted.
func relayResponse(client, upstream net.Conn, final *response.Message, requestMethod string) (committed bool, err error) {
	cutoff := time.Now().Add(responseBodyTimeout)
	final.Body = newIdleDeadlineReader(upstream, final.Body, responseBodyIdleTimeout, cutoff)

	if err := client.SetWriteDeadline(cutoff); err != nil {
		return false, fmt.Errorf("setting client write deadline: %w", err)
	}

	output := &commitWriter{writer: client}
	if err := response.Write(output, final, requestMethod); err != nil {
		return output.committed, fmt.Errorf("writing response to client: %w", err)
	}

	return true, nil
}

// commitWriter passes writes to writer and records whether any byte reached it.
type commitWriter struct {
	writer    io.Writer
	committed bool
}

func (output *commitWriter) Write(data []byte) (int, error) {
	count, err := output.writer.Write(data)
	if count > 0 {
		output.committed = true
	}

	return count, err
}
