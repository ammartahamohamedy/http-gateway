package proxy

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/request"
)

// errUnsupportedMethod reports TRACE, which the gateway does not implement
// rather than reflect requests back to clients.
var errUnsupportedMethod = errors.New("proxy: TRACE is not supported")

// lingerTimeout bounds how long closeClient keeps reading after the response.
var lingerTimeout = 2 * time.Second

// requestHeadTimeout bounds reading the whole request head, so a client sending
// it slowly cannot hold the connection. A per-line cap alone would not stop that.
var requestHeadTimeout = 10 * time.Second

// Config holds the gateway settings HandleConnection needs.
type Config struct {
	// Upstream is the backend's TCP address, host:port: dialed for every
	// exchange and sent as the forwarded request's Host.
	Upstream string

	// Hosts lists the host names and IP literals the gateway serves, IPv6
	// literals in brackets. A request for any other host, or for any host when
	// Hosts is empty, is refused with 421 (Misdirected Request), which also
	// stops DNS rebinding through browsers.
	Hosts []string
}

// HandleConnection runs one exchange on connection, the client socket it owns.
// It reads the request head within requestHeadTimeout, applies the gateway's
// request policy (admitRequest), buffers the body (readRequestBody), and
// forwards the request to config.Upstream (forward). A request whose
// Max-Forwards has reached 0 is answered by the gateway itself as its final
// recipient, 200 with no content, instead of being forwarded (RFC 9110, Section
// 7.6.2).
//
// A request that fails to read or is refused gets the status
// requestErrorStatus chooses, and an upstream failure before the final
// response is committed gets the one upstreamErrorStatus chooses, both written
// by writeGatewayResponse. The connection is closed in stages (closeClient)
// before returning, and every error is logged without terminating the listener.
func HandleConnection(connection net.Conn, config Config) {
	defer closeClient(connection)

	reject := func(err error, requestMethod string) {
		log.Printf("rejecting request: %v", err)
		if status := requestErrorStatus(err); status != 0 {
			writeGatewayResponse(connection, status, requestMethod)
		}
	}

	if err := connection.SetReadDeadline(time.Now().Add(requestHeadTimeout)); err != nil {
		log.Printf("setting read deadline: %v", err)
		return
	}

	message, err := request.Read(bufio.NewReader(connection))
	if err != nil {
		reject(err, "")
		return
	}

	requestMethod := message.Line.Method

	if err := admitRequest(message, config); err != nil {
		reject(err, requestMethod)
		return
	}

	if message.Fields.MaxForwardsPresent && message.Fields.MaxForwards == 0 {
		writeGatewayResponse(connection, 200, requestMethod)
		return
	}

	content, err := readRequestBody(connection, message)
	if err != nil {
		reject(err, requestMethod)
		return
	}

	committed, err := forward(connection, message, content, config)
	if err != nil {
		log.Printf("forwarding request: %v", err)
		if !committed {
			writeGatewayResponse(connection, upstreamErrorStatus(err), requestMethod)
		}
	}
}

// admitRequest applies the gateway's request policy before the body is read: a
// host it serves (checkHost), a method it implements, transfer codings it can decode, a request-target it
// serves (checkRequestTarget), and expectations it supports. The first refusal is
// returned for requestErrorStatus to answer.
func admitRequest(message *request.Message, config Config) error {
	if err := checkHost(message.Fields.Authority, config.Hosts); err != nil {
		return err
	}

	if message.Line.Method == "TRACE" {
		return errUnsupportedMethod
	}

	if err := fields.CheckTransferCodingSupport(message.Fields.TransferCodings); err != nil {
		return err
	}

	if err := checkRequestTarget(message); err != nil {
		return err
	}

	return fields.CheckExpectationSupport(message.Fields.Expectations)
}

// closeClient closes connection in stages (RFC 9112, Section 9.6): it ends the
// sending side, then discards what the client still sends until the client
// closes or lingerTimeout passes, and only then closes. Closing at once with
// request bytes unread makes the kernel reset the connection, which can
// destroy the response before the client reads it, as when a refused upload is
// still arriving. A connection that cannot be half-closed is closed at once.
func closeClient(connection net.Conn) {
	defer func() {
		if err := connection.Close(); err != nil {
			log.Printf("closing connection: %v", err)
		}
	}()

	halfCloser, ok := connection.(interface{ CloseWrite() error })
	if !ok || halfCloser.CloseWrite() != nil {
		return
	}

	if connection.SetReadDeadline(time.Now().Add(lingerTimeout)) != nil {
		return
	}

	_, _ = io.Copy(io.Discard, connection)
}
