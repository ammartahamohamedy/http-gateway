# HTTP Gateway

A Go–Rust project combining a handwritten Go HTTP/1.1 gateway, a Rust application
server with bounded in-memory text search, and a Rust/Ratatui client.

## Current scaffold

- The existing Go request-line parser, streaming line reader, executable and
  tests live under `gateway/`.
- The root Cargo workspace currently contains only `backend`, a minimal Rust
  binary package. It prints its scaffold status and exits; it is not an HTTP
  server yet.
- `client/src/`, `docs/`, `scripts/` and `tests/` are reserved directories for
  later work. Empty directories are not tracked by Git.
- Forwarding, Rust HTTP serving, search indexing/querying, and the Ratatui client
  remain to be implemented. No HTTP or TUI dependency has been selected yet.

## Layout

```text
http-gateway/
├── Cargo.toml                 # Virtual Rust workspace; backend is the only member
├── Cargo.lock                 # Shared Rust dependency lockfile
├── gateway/                   # Go module
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   └── internal/http1/        # Existing handwritten implementation and tests
├── backend/                   # Rust package: search-backend
│   ├── Cargo.toml
│   └── src/main.rs
├── client/src/                # Future Rust/Ratatui package; no manifest yet
├── docs/                      # Future public architecture/API documentation
├── scripts/                   # Future local orchestration
└── tests/                     # Future whole-system tests and fixtures
```

The backend's search, HTTP, configuration, lifecycle and execution modules will
be added alongside their implementations. The client becomes a separate workspace
package after indexing/search works through the Go gateway. No shared API-types
package is needed initially.

## Check and run

Requirements: Go 1.27.1 or newer and Rust 1.85 or newer with Cargo. The Rust
scaffold has no third-party dependencies.

From the repository root:

```sh
go -C gateway test ./...
cargo check --workspace --locked
cargo fmt --all -- --check
```

Start the existing Go request-line reader:

```sh
go -C gateway run .
```

It listens on `127.0.0.1:8080` by default, reads one request-line per connection,
and closes the connection. It does not send HTTP responses yet. See the
[Go component instructions](gateway/README.md) for manual verification.

Run the Rust entry point:

```sh
cargo run -q -p search-backend
```

It currently prints its implementation status and exits.

## Intended system

```text
Rust/Ratatui client <-- HTTP --> Go gateway <-- HTTP over OS TCP --> Rust search server
```

Go will implement request and response parsing, validation, framing,
serialization and forwarding by hand. Rust will use an HTTP library for HTTP
mechanics and implement a bounded in-memory text-search service. The client will
use a normal HTTP client library and communicate through the gateway.

Operating-system TCP is used on both sides. Custom TCP, FFI, persistence and
distributed search are outside the first release. Exact search/API semantics
and protocol limits still need definition; this is an educational project, not
a claim of full HTTP/1.1 compliance or production readiness.
