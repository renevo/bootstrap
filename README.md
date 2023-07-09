# Bootstrap Package

Package to bootstrap my projects, this really just takes care of a lot of the duplicate work, so I don't need to do it every single time.

## Usage

Use at your own risk, not intended for external usage, but hey, its on the internet.

## Grafana Cloud

These things are wired up with Open Telemetry, and my primary use case is Grafana Cloud.

* [Open Telemetry Collection](https://grafana.com/blog/2021/04/13/how-to-send-traces-to-grafana-clouds-tempo-service-with-opentelemetry-collector/)


## SSL Certificates

The http module supports hosting with SSL, this is a very basic implementation and in the future, ACME will be supported.

There are a few ways to turn on SSL.

### Command line flags

`-sslkey=<path to key> -sslcert=<path to cert>` can be provided on the command line.

### Environment Variables

`HTTPS_KEY=<path to key> HTTPS_CERTIFICATE=<path to cert>` can be provided in the environment.

### Configuration

HCL Configuration can also be made 

```hcl
http ":443" {
  key_file="<path to key>"
  cert_file="<path to key>"
}
```

### Generating

As a reminder, you can use openssl to quickly generate a temporarly self-signed certificate for testing.

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -sha256 -days 3650 -nodes -subj "/C=US/ST=California/L=Orange/O=Local/OU=Applications/CN=localhost"
```

