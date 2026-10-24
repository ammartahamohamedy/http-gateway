# HTTP from TCP

HTTP from TCP in Go.

## Requirements

- Go 1.27.1 or newer
- Python for the verification command below

## Current usage

The program currently reads incoming TCP data line by line and prints it to the console. Connections are handled one at a time.

From the project directory, start the listener:

```sh
go run .
```

The default address is `127.0.0.1:8080`. To choose another address:

```sh
go run . -addr 127.0.0.1:9000
```

Press `Ctrl+C` to stop the server.

## Manual verification

With the server listening on `127.0.0.1:9000`, run this command in a second terminal:

```sh
python -c "import socket; s=socket.create_connection(('127.0.0.1',9000)); s.sendall(b'Hello over TCP\n\nA line longer than eight bytes\nFinal line without newline'); s.shutdown(socket.SHUT_WR); s.close()"
```

Expected server output:

```text
Connection accepted
Hello over TCP

A line longer than eight bytes
Final line without newline
Connection closed
```

This checks a blank line, a line longer than the eight-byte read buffer, and a final line without a trailing newline. Run the client command again to check that the server accepts another connection without restarting.
