# Go HTTP/1.1 Gateway Component

The handwritten Go HTTP/1.1 component of HTTP Gateway. Run the commands below
from this `gateway` directory. Full request/response forwarding is planned;
the current executable parses only the request head as described below.

## Requirements

- Go 1.27.1 or newer
- Python for the verification command below

## Current usage

The program parses one HTTP/1.1 request per TCP connection: it validates the request head and its supported fields, applies the gateway's transfer-coding, request-target, and `Expect` policy, reads a Content-Length body of up to 1 MiB within 30 seconds (sending `100 Continue` when the client expects it), prints the request-line fields, effective authority, and body size, and closes the connection. Connections are handled one at a time. Chunked bodies are not decoded yet, nothing is forwarded, and the server sends no final HTTP response.

The target policy accepts origin-form, `http`/`https` absolute-form, and `OPTIONS *`. It rejects CONNECT, other schemes, ports outside 1–65535, paths starting with `//`, and `.`/`..` segments, including percent-encoded ones.

Request-lines require CRLF termination and single-space separators. The size limit is 8192 bytes, excluding the two-byte CRLF terminator. Method tokens are case-sensitive and preserved as received.

From the project directory, start the listener:

```sh
go run .
```

The default address is `127.0.0.1:8080`. To choose another address:

```sh
go run . -addr 127.0.0.1:9000
```

Press `Ctrl+C` to stop the server.

## Tests

```sh
go test -v ./...
```

Tests cover request-line and header parsing, field validation, target policy, and shared CRLF reading, including size limits, empty lines, fragmented reads, and preservation of subsequent bytes.

## Manual verification

With the server listening on `127.0.0.1:9000`, run this command in a second terminal:

```sh
python -c "import socket; s=socket.create_connection(('127.0.0.1',9000)); s.settimeout(5); s.sendall(b'GET /hello?name=Gopher HTTP/1.1\r\nHost: example.com\r\n\r\n'); assert s.recv(1) == b''; s.close()"
```

Expected server output:

```text
Connection accepted
Method: GET
Request target: /hello?name=Gopher
Version: 1.1
Authority: example.com
Body: 0 bytes
Connection closed
```

The client keeps its sending side open while waiting for the server to close the connection. This checks that parsing completes at the empty line ending the header section without waiting for client EOF. Run the client command again to check that the server accepts another connection without restarting.
