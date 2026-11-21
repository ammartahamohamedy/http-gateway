# Go HTTP/1.1 Gateway Component

The handwritten Go HTTP/1.1 component of HTTP Gateway: a reverse proxy in
front of the Rust backend. Run the commands below from this `gateway` directory.

## Requirements

- Go 1.27.1 or newer
- Python for the verification commands below

## Current usage

The gateway handles one HTTP/1.1 exchange per TCP connection. It reads the request head and validates its syntax, its supported fields, and the gateway's policy: hosts it serves, methods (TRACE is refused), transfer codings, request-target form, and `Expect`. It buffers a Content-Length or chunked body of up to 1 MiB, sending `100 Continue` when the client expects it, and decodes gzip and deflate transfer codings. It then forwards the request to the backend, without hop-by-hop fields and with `Host` set to the backend's address, `Via` appended, and `Max-Forwards` decremented. Interim responses are relayed, apart from `100 Continue`. The final response is re-framed, with a known length kept and anything else sent chunked, and trailers are kept.

Refused requests get a status from the gateway: 400, 408, 413, 414, 417, 421, 431, 501, or 505. An `OPTIONS` request with `Max-Forwards: 0` gets 200 from the gateway itself. If the backend fails before any byte of its final response reaches the client, the client gets 502, or 504 on a timeout. After that the connection is closed. Every connection closes after its exchange, in stages, so a refused upload still receives its response. Connections are handled one at a time.

The target policy accepts origin-form, `http`/`https` absolute-form, and `OPTIONS *`. It rejects CONNECT, other schemes, ports outside 1–65535, paths starting with `//`, and `.`/`..` segments, including percent-encoded ones.

Start the gateway:

```sh
go run .
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:8080` | Address to listen on |
| `-upstream` | `127.0.0.1:8081` | Backend address, dialed for every exchange |
| `-hosts` | `localhost,127.0.0.1,[::1]` | Comma-separated hosts the gateway serves; other hosts get 421 |

Press `Ctrl+C` to stop the server.

## Tests

```sh
go test -v ./...
```

Tests cover request and response parsing, field validation, body framing, chunked and gzip/deflate decoding, trailers, the message writers, gateway policy and status mapping, forwarding against a fake upstream, interim responses, timeouts, and the staged close.

## Manual verification

With the gateway running on its defaults and no backend listening, run this in a second terminal:

```sh
python -c "import socket; s=socket.create_connection(('127.0.0.1',8080)); s.sendall(b'GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n'); print(s.recv(4096).decode()); s.close()"
```

The gateway cannot reach the backend, so it answers:

```text
HTTP/1.1 502 Bad Gateway
connection: close
content-length: 0
date: <current date>
```

Replace `Host: localhost` with `Host: evil.example` to see `421 Misdirected Request`. The gateway logs each failure, and the listener keeps running, so you can send the next request without restarting it.
