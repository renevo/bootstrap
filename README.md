# Bootstrap

Bootstrap assembles and runs an HTTP application with structured logging,
configuration, OpenTelemetry, optional NATS connectivity, and static file
serving. It requires Go 1.26 or later.

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
panic recovery, proxy-header handling, and OpenTelemetry tracing. It also
exposes these built-in endpoints:

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
Token, NKEY, and credentials-file authentication can be configured; use
`-generate-config` for the complete setting list.

## OpenTelemetry

The OpenTelemetry module always installs a Prometheus metrics exporter used by
the `/metrics` endpoint. Set `otel.grpc.address` or `OTEL_GRPC_ADDRESS` to
export traces to an OTLP gRPC collector. The collector connection currently
uses insecure transport credentials, so deploy it only across a trusted or
separately secured connection.

