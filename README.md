# Bootstrap

[![CI](https://github.com/renevo/bootstrap/actions/workflows/ci.yaml/badge.svg)](https://github.com/renevo/bootstrap/actions/workflows/ci.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/renevo/bootstrap.svg)](https://pkg.go.dev/github.com/renevo/bootstrap)
[![Go Version](https://img.shields.io/github/go-mod/go-version/renevo/bootstrap?logo=go)](go.mod)
[![License](https://img.shields.io/github/license/renevo/bootstrap)](LICENSE)

Bootstrap assembles and runs an HTTP application with structured logging,
configuration, OpenTelemetry, optional NATS connectivity, and static file
serving. It requires Go 1.26 or later.

> [!WARNING]
> Bootstrap is pre-release software until version 1.0. APIs and behavior may
> change without notice. Use it at your own risk, both before and after 1.0.

## Usage

```go
package main

import (
  "embed"
  "io/fs"
  "net/http"

  "github.com/renevo/bootstrap"
)

//go:embed static/*
var content embed.FS

func main() {
  static, err := fs.Sub(content, "static")
  if err != nil {
    panic(err)
  }

  if err := bootstrap.HTTP("example", "1.0.0", http.FS(static)); err != nil {
    panic(err)
  }
}
```

Pass `nil` as the file system when the application does not serve static
content. Additional `application.Option` values may be passed after it to add
or configure application modules.

The bootstrap registers these command-line flags:

| Flag | Description |
| --- | --- |
| `-config <path>` | Load an HCL or JSON configuration file. |
| `-debug` | Enable debug logging. |
| `-generate-config` | Print the default HCL configuration and exit. |
| `-json` | Write JSON logs. |
| `-no-color` | Disable color in text logs. |

Configuration is read from the file selected by `-config`, then from the
environment. Environment variables override file values. Their names are the
uppercase setting path with separators replaced by underscores, such as
`HTTP_ADDRESS`, `NATS_ADDRESS`, and `OTEL_GRPC_ADDRESS`.

## HTTP

The HTTP server listens on `:8080` by default. It includes request logging,
panic recovery, proxy-header handling, and OpenTelemetry tracing and metrics.
Route templates are used for span names and the `http.route` metric dimension,
so route parameters do not create unbounded telemetry. Static file requests use
the synthetic `StaticFile` route. Not-found spans are named `{METHOD} 404`,
while method-not-allowed requests retain the matched route template.

When present, Cloudflare tunnel headers are added to spans as
`cloudflare.tunnel.ray`, `cloudflare.tunnel.ipcountry`,
`cloudflare.tunnel.connecting_ip`, and `cloudflare.tunnel.warp_tag_id`.

The server also exposes these built-in endpoints:

| Path | Description |
| --- | --- |
| `/metrics` | Prometheus metrics. |
| `/api/health` | A JSON health response. |

Modules may implement `modules/http.Routable` to add handlers or middleware to
the shared Gorilla Mux router. Static content, when supplied, is registered
after module routes.

The server timeouts default to a 5-second read timeout, 10-second write
timeout, 2-minute idle timeout, and 30-second graceful shutdown timeout. Run
the application with `-generate-config` to see every available setting and its
current default.

### TLS Certificates

TLS is enabled when both `http.cert_file` and `http.key_file` are set. The
server loads the certificate and private key directly; automatic certificate
management is not provided.

Set the values in HCL:

```hcl
http {
  address = ":443"
  cert_file = "/path/to/cert.pem"
  key_file = "/path/to/key.pem"
}
```

Or use the equivalent environment variables:

```bash
HTTP_ADDRESS=:443 \
HTTP_CERT_FILE=/path/to/cert.pem \
HTTP_KEY_FILE=/path/to/key.pem \
go run .
```

For local testing, a self-signed certificate can be generated with OpenSSL:

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -sha256 -days 3650 -nodes -subj "/C=US/ST=California/L=Orange/O=Local/OU=Applications/CN=localhost"
```

## NATS

The NATS module is inactive unless `nats.address` (or `NATS_ADDRESS`) is set.
When connected, it registers the `*nats.Conn` with the application IoC context.

Initial connection retry is always enabled, so start-up succeeds even when the
server is not yet reachable. The connection is registered while it is still
reconnecting, and the module logs the pending status instead of failing. On
shutdown the connection is drained, bounded by `nats.drain_timeout`.

Provide secrets through protected environment variables or a protected
configuration source rather than committed HCL. The `password`, `token`, and
`secret` settings are masked in `-generate-config` output and are never logged.

### Authentication

Exactly one authentication mode may be configured.

| Mode | Settings |
| --- | --- |
| Anonymous | none |
| User/password | `nats.username` and `nats.password` (both required) |
| Token | `nats.token` only |
| JWT/seed | `nats.token` (JWT) and `nats.secret` (seed) |
| Credentials file | `nats.credentials_file` |

Combining modes, setting only one of username/password, or setting
`nats.secret` without `nats.token` is a configuration error.

```hcl
nats {
  address = "nats://nats.example.com:4222"
  username = "app"
  # password supplied by NATS_PASSWORD
}
```

```bash
NATS_ADDRESS=nats://nats.example.com:4222 \
NATS_USERNAME=app \
NATS_PASSWORD=... \
go run .
```

### TLS

Set `nats.root_ca_file` to verify the server against a private certificate
authority. Set `nats.client_cert_file` and `nats.client_key_file` together for
mutual TLS. Certificate verification is always performed; there is no
insecure skip-verify setting.

```hcl
nats {
  address = "tls://nats.example.com:4222"
  root_ca_file = "/path/to/ca.pem"
  client_cert_file = "/path/to/client.pem"
  client_key_file = "/path/to/client-key.pem"
}
```

### Connection settings

The tunings below are initialized from the NATS client defaults.

| Setting | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `nats.connect_timeout` | `NATS_CONNECT_TIMEOUT` | `2s` | Dial timeout per server. |
| `nats.reconnect_wait` | `NATS_RECONNECT_WAIT` | `2s` | Delay between reconnect attempts. |
| `nats.max_reconnects` | `NATS_MAX_RECONNECTS` | `60` | Attempts per server; `-1` is unlimited and `0` disables reconnecting. |
| `nats.ping_interval` | `NATS_PING_INTERVAL` | `2m` | Interval between client pings. |
| `nats.max_pings_outstanding` | `NATS_MAX_PINGS_OUTSTANDING` | `2` | Unanswered pings before the connection is stale. |
| `nats.drain_timeout` | `NATS_DRAIN_TIMEOUT` | `30s` | Bound on the graceful drain at shutdown. |

Other environment variables follow the same pattern, such as `NATS_NAME`,
`NATS_TOKEN`, `NATS_SECRET`, `NATS_CREDENTIALS_FILE`, `NATS_ROOT_CA_FILE`,
`NATS_CLIENT_CERT_FILE`, and `NATS_CLIENT_KEY_FILE`. Run the application with
`-generate-config` for the complete setting list.

### Connection logging

Connection lifecycle events are logged with structured fields. Initial
connection, successful reconnection, and the expected shutdown close are logged
at info level. Disconnects, failed reconnect attempts, and lame-duck mode are
logged at warn level. Asynchronous subscription errors and an unexpected
terminal close are logged at error level. Discovered-server changes are logged
at debug level. Credentials are stripped from any logged server URL, and
configuration values are never logged.

## OpenTelemetry

The OpenTelemetry module always installs a Prometheus metrics exporter used by
the `/metrics` endpoint. HTTP instrumentation emits standard server request
duration, request body size, and response body size metrics. Set
`otel.grpc.address` or `OTEL_GRPC_ADDRESS` to export traces to an OTLP gRPC
collector. The collector connection currently uses insecure transport
credentials, so deploy it only across a trusted or separately secured
connection.

