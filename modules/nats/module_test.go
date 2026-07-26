package nats

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	testJWT  = "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.e30.c2ln"
	testSeed = "SUAJ5PSJDTIQKCVJTS6M4QAMBFLHSKUTIFEHRTQXAMJJHRQZQ2LQKPRQ7Q"
)

func baseConfig() *cfg {
	m := New().(*module)

	return m.cfg
}

func applyOptions(t *testing.T, opts []nats.Option) nats.Options {
	t.Helper()

	natsOpts := nats.GetDefaultOptions()
	for _, opt := range opts {
		if err := opt(&natsOpts); err != nil {
			t.Fatalf("apply option: %v", err)
		}
	}

	return natsOpts
}

func TestNewUsesNativeDefaults(t *testing.T) {
	defaults := nats.GetDefaultOptions()
	c := baseConfig()

	if c.ConnectTimeout != defaults.Timeout {
		t.Errorf("connect_timeout = %v, want %v", c.ConnectTimeout, defaults.Timeout)
	}

	if c.ReconnectWait != defaults.ReconnectWait {
		t.Errorf("reconnect_wait = %v, want %v", c.ReconnectWait, defaults.ReconnectWait)
	}

	if c.MaxReconnects != defaults.MaxReconnect {
		t.Errorf("max_reconnects = %v, want %v", c.MaxReconnects, defaults.MaxReconnect)
	}

	if c.PingInterval != defaults.PingInterval {
		t.Errorf("ping_interval = %v, want %v", c.PingInterval, defaults.PingInterval)
	}

	if c.MaxPingsOutstanding != defaults.MaxPingsOut {
		t.Errorf("max_pings_outstanding = %v, want %v", c.MaxPingsOutstanding, defaults.MaxPingsOut)
	}

	if c.DrainTimeout != defaults.DrainTimeout {
		t.Errorf("drain_timeout = %v, want %v", c.DrainTimeout, defaults.DrainTimeout)
	}
}

func TestConfigValidateAuthenticationModes(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*cfg)
		wantErr string
	}{
		{
			name:   "anonymous",
			mutate: func(c *cfg) {},
		},
		{
			name:   "username and password",
			mutate: func(c *cfg) { c.Username, c.Password = "user", "pass" },
		},
		{
			name:   "token only",
			mutate: func(c *cfg) { c.Token = "token" },
		},
		{
			name:   "token and secret",
			mutate: func(c *cfg) { c.Token, c.Secret = testJWT, testSeed },
		},
		{
			name:   "credentials file",
			mutate: func(c *cfg) { c.CredentialsFile = "/tmp/user.creds" },
		},
		{
			name:    "username without password",
			mutate:  func(c *cfg) { c.Username = "user" },
			wantErr: "nats.password",
		},
		{
			name:    "password without username",
			mutate:  func(c *cfg) { c.Password = "pass" },
			wantErr: "nats.username",
		},
		{
			name:    "secret without token",
			mutate:  func(c *cfg) { c.Secret = testSeed },
			wantErr: "nats.secret",
		},
		{
			name:    "username and token",
			mutate:  func(c *cfg) { c.Username, c.Password, c.Token = "user", "pass", "token" },
			wantErr: "only one of",
		},
		{
			name:    "token and credentials file",
			mutate:  func(c *cfg) { c.Token, c.CredentialsFile = "token", "/tmp/user.creds" },
			wantErr: "only one of",
		},
		{
			name:    "username and credentials file",
			mutate:  func(c *cfg) { c.Username, c.Password, c.CredentialsFile = "user", "pass", "/tmp/user.creds" },
			wantErr: "only one of",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := baseConfig()
			test.mutate(c)

			err := c.validate()
			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("validate() = %v, want nil", err)

			case test.wantErr != "" && err == nil:
				t.Fatalf("validate() = nil, want error containing %q", test.wantErr)

			case test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr):
				t.Fatalf("validate() = %v, want error containing %q", err, test.wantErr)
			}

			if err != nil {
				assertNoSecrets(t, err.Error())
			}
		})
	}
}

func TestConfigValidateTLSAndTunings(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*cfg)
		wantErr string
	}{
		{
			name:   "root ca alone",
			mutate: func(c *cfg) { c.RootCAFile = "/tmp/ca.pem" },
		},
		{
			name:   "client cert pair",
			mutate: func(c *cfg) { c.ClientCertFile, c.ClientKeyFile = "/tmp/c.pem", "/tmp/k.pem" },
		},
		{
			name:    "client cert without key",
			mutate:  func(c *cfg) { c.ClientCertFile = "/tmp/c.pem" },
			wantErr: "nats.client_key_file",
		},
		{
			name:    "client key without cert",
			mutate:  func(c *cfg) { c.ClientKeyFile = "/tmp/k.pem" },
			wantErr: "nats.client_cert_file",
		},
		{
			name:    "zero connect timeout",
			mutate:  func(c *cfg) { c.ConnectTimeout = 0 },
			wantErr: "nats.connect_timeout",
		},
		{
			name:    "negative reconnect wait",
			mutate:  func(c *cfg) { c.ReconnectWait = -time.Second },
			wantErr: "nats.reconnect_wait",
		},
		{
			name:    "zero ping interval",
			mutate:  func(c *cfg) { c.PingInterval = 0 },
			wantErr: "nats.ping_interval",
		},
		{
			name:    "zero drain timeout",
			mutate:  func(c *cfg) { c.DrainTimeout = 0 },
			wantErr: "nats.drain_timeout",
		},
		{
			name:    "zero max pings outstanding",
			mutate:  func(c *cfg) { c.MaxPingsOutstanding = 0 },
			wantErr: "nats.max_pings_outstanding",
		},
		{
			name:   "unlimited reconnects",
			mutate: func(c *cfg) { c.MaxReconnects = -1 },
		},
		{
			name:   "disabled reconnects",
			mutate: func(c *cfg) { c.MaxReconnects = 0 },
		},
		{
			name:    "invalid reconnects",
			mutate:  func(c *cfg) { c.MaxReconnects = -2 },
			wantErr: "nats.max_reconnects",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := baseConfig()
			test.mutate(c)

			err := c.validate()
			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("validate() = %v, want nil", err)

			case test.wantErr != "" && err == nil:
				t.Fatalf("validate() = nil, want error containing %q", test.wantErr)

			case test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr):
				t.Fatalf("validate() = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestConnectionOptionsDefaultsPreserveNativeBehavior(t *testing.T) {
	opts, err := baseConfig().connectionOptions("app-1")
	if err != nil {
		t.Fatalf("connectionOptions() = %v", err)
	}

	defaults := nats.GetDefaultOptions()
	applied := applyOptions(t, opts)

	if !applied.RetryOnFailedConnect {
		t.Error("RetryOnFailedConnect = false, want true")
	}

	if applied.Name != "app-1" {
		t.Errorf("Name = %q, want %q", applied.Name, "app-1")
	}

	if applied.Timeout != defaults.Timeout {
		t.Errorf("Timeout = %v, want %v", applied.Timeout, defaults.Timeout)
	}

	if applied.ReconnectWait != defaults.ReconnectWait {
		t.Errorf("ReconnectWait = %v, want %v", applied.ReconnectWait, defaults.ReconnectWait)
	}

	if applied.MaxReconnect != defaults.MaxReconnect {
		t.Errorf("MaxReconnect = %v, want %v", applied.MaxReconnect, defaults.MaxReconnect)
	}

	if applied.PingInterval != defaults.PingInterval {
		t.Errorf("PingInterval = %v, want %v", applied.PingInterval, defaults.PingInterval)
	}

	if applied.MaxPingsOut != defaults.MaxPingsOut {
		t.Errorf("MaxPingsOut = %v, want %v", applied.MaxPingsOut, defaults.MaxPingsOut)
	}

	if applied.DrainTimeout != defaults.DrainTimeout {
		t.Errorf("DrainTimeout = %v, want %v", applied.DrainTimeout, defaults.DrainTimeout)
	}

	if applied.ReconnectBufSize != defaults.ReconnectBufSize {
		t.Errorf("ReconnectBufSize = %v, want %v", applied.ReconnectBufSize, defaults.ReconnectBufSize)
	}

	if applied.NoEcho != defaults.NoEcho {
		t.Errorf("NoEcho = %v, want %v", applied.NoEcho, defaults.NoEcho)
	}

	if applied.Secure || applied.TLSConfig != nil {
		t.Error("TLS configured without TLS settings")
	}
}

func TestConnectionOptionsExplicitValues(t *testing.T) {
	c := baseConfig()
	c.Name = "configured"
	c.ConnectTimeout = 7 * time.Second
	c.ReconnectWait = 11 * time.Second
	c.MaxReconnects = -1
	c.PingInterval = 13 * time.Second
	c.MaxPingsOutstanding = 5
	c.DrainTimeout = 17 * time.Second

	applied := applyOptions(t, mustOptions(t, c, "fallback"))

	if applied.Name != "configured" {
		t.Errorf("Name = %q, want %q", applied.Name, "configured")
	}

	if applied.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s", applied.Timeout)
	}

	if applied.ReconnectWait != 11*time.Second {
		t.Errorf("ReconnectWait = %v, want 11s", applied.ReconnectWait)
	}

	if applied.MaxReconnect != -1 {
		t.Errorf("MaxReconnect = %v, want -1", applied.MaxReconnect)
	}

	if applied.PingInterval != 13*time.Second {
		t.Errorf("PingInterval = %v, want 13s", applied.PingInterval)
	}

	if applied.MaxPingsOut != 5 {
		t.Errorf("MaxPingsOut = %v, want 5", applied.MaxPingsOut)
	}

	if applied.DrainTimeout != 17*time.Second {
		t.Errorf("DrainTimeout = %v, want 17s", applied.DrainTimeout)
	}
}

func TestConnectionOptionsAuthenticationSelection(t *testing.T) {
	credsFile := writeCredentialsFile(t)

	tests := []struct {
		name   string
		mutate func(*cfg)
		assert func(*testing.T, nats.Options)
	}{
		{
			name:   "anonymous",
			mutate: func(c *cfg) {},
			assert: func(t *testing.T, o nats.Options) {
				if o.User != "" || o.Password != "" || o.Token != "" || o.UserJWT != nil {
					t.Error("unexpected authentication configured")
				}
			},
		},
		{
			name:   "username and password",
			mutate: func(c *cfg) { c.Username, c.Password = "user", "pass" },
			assert: func(t *testing.T, o nats.Options) {
				if o.User != "user" || o.Password != "pass" {
					t.Errorf("User/Password = %q/%q", o.User, o.Password)
				}

				if o.Token != "" || o.UserJWT != nil {
					t.Error("unexpected additional authentication configured")
				}
			},
		},
		{
			name:   "token only",
			mutate: func(c *cfg) { c.Token = "a-token" },
			assert: func(t *testing.T, o nats.Options) {
				if o.Token != "a-token" {
					t.Errorf("Token = %q, want %q", o.Token, "a-token")
				}

				if o.UserJWT != nil {
					t.Error("token only must not configure JWT authentication")
				}
			},
		},
		{
			name:   "token and secret",
			mutate: func(c *cfg) { c.Token, c.Secret = testJWT, testSeed },
			assert: func(t *testing.T, o nats.Options) {
				if o.Token != "" {
					t.Error("token+secret must not configure token authentication")
				}

				if o.UserJWT == nil || o.SignatureCB == nil {
					t.Error("token+secret must configure JWT/seed authentication")
				}
			},
		},
		{
			name:   "credentials file",
			mutate: func(c *cfg) { c.CredentialsFile = credsFile },
			assert: func(t *testing.T, o nats.Options) {
				if o.UserJWT == nil || o.SignatureCB == nil {
					t.Error("credentials file must configure JWT/seed authentication")
				}

				if o.Token != "" || o.User != "" {
					t.Error("unexpected additional authentication configured")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := baseConfig()
			test.mutate(c)

			test.assert(t, applyOptions(t, mustOptions(t, c, "app")))
		})
	}
}

func TestConnectionOptionsTLSFiles(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertificate(t, dir, "client")
	caFile, _ := writeCertificate(t, dir, "ca")

	t.Run("root ca only", func(t *testing.T) {
		c := baseConfig()
		c.RootCAFile = caFile

		applied := applyOptions(t, mustOptions(t, c, "app"))
		if !applied.Secure || applied.RootCAsCB == nil {
			t.Error("root CA did not enable verified TLS")
		}

		if applied.TLSCertCB != nil {
			t.Error("unexpected client certificate configured")
		}
	})

	t.Run("client certificate", func(t *testing.T) {
		c := baseConfig()
		c.RootCAFile = caFile
		c.ClientCertFile, c.ClientKeyFile = certFile, keyFile

		applied := applyOptions(t, mustOptions(t, c, "app"))
		if applied.TLSCertCB == nil || applied.RootCAsCB == nil {
			t.Error("mTLS was not configured")
		}

		if applied.TLSConfig == nil || applied.TLSConfig.InsecureSkipVerify {
			t.Error("certificate verification must remain enabled")
		}
	})

	t.Run("missing root ca file", func(t *testing.T) {
		c := baseConfig()
		c.RootCAFile = filepath.Join(dir, "missing.pem")

		opts, err := c.connectionOptions("app")
		if err != nil {
			t.Fatalf("connectionOptions() = %v", err)
		}

		natsOpts := nats.GetDefaultOptions()
		var applyErr error
		for _, opt := range opts {
			if err := opt(&natsOpts); err != nil {
				applyErr = err
			}
		}

		if applyErr == nil {
			t.Error("expected an error for a missing root CA file")
		}
	})
}

func TestModuleOptionsRegisterEventHandlers(t *testing.T) {
	m := New().(*module)
	opts, err := m.options("app", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("options() = %v", err)
	}

	applied := applyOptions(t, opts)

	handlers := map[string]bool{
		"ConnectedCB":         applied.ConnectedCB != nil,
		"DisconnectedErrCB":   applied.DisconnectedErrCB != nil,
		"ReconnectedCB":       applied.ReconnectedCB != nil,
		"ReconnectErrCB":      applied.ReconnectErrCB != nil,
		"ClosedCB":            applied.ClosedCB != nil,
		"DiscoveredServersCB": applied.DiscoveredServersCB != nil,
		"AsyncErrorCB":        applied.AsyncErrorCB != nil,
		"LameDuckModeHandler": applied.LameDuckModeHandler != nil,
	}

	for name, registered := range handlers {
		if !registered {
			t.Errorf("%s was not registered", name)
		}
	}

	if applied.DisconnectedCB != nil {
		t.Error("the deprecated disconnect handler must not be registered")
	}
}

func TestEventHandlerLogging(t *testing.T) {
	tests := []struct {
		name      string
		shutdown  bool
		invoke    func(nats.Options, *nats.Conn)
		wantLevel string
		wantMsg   string
		wantKeys  []string
	}{
		{
			name:      "connected",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.ConnectedCB(nc) },
			wantLevel: "INFO",
			wantMsg:   "NATS connected",
			wantKeys:  []string{"address", "url", "status"},
		},
		{
			name:      "disconnected",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.DisconnectedErrCB(nc, errors.New("boom")) },
			wantLevel: "WARN",
			wantMsg:   "NATS disconnected",
			wantKeys:  []string{"status", "reconnects", "error"},
		},
		{
			name:      "reconnected",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.ReconnectedCB(nc) },
			wantLevel: "INFO",
			wantMsg:   "NATS reconnected",
			wantKeys:  []string{"address", "status", "reconnects"},
		},
		{
			name:      "reconnect failed",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.ReconnectErrCB(nc, errors.New("boom")) },
			wantLevel: "WARN",
			wantMsg:   "NATS reconnect attempt failed",
			wantKeys:  []string{"status", "reconnects", "error"},
		},
		{
			name:      "expected close",
			shutdown:  true,
			invoke:    func(o nats.Options, nc *nats.Conn) { o.ClosedCB(nc) },
			wantLevel: "INFO",
			wantMsg:   "NATS connection closed",
			wantKeys:  []string{"reconnects"},
		},
		{
			name:      "unexpected close",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.ClosedCB(nc) },
			wantLevel: "ERROR",
			wantMsg:   "NATS connection closed unexpectedly",
			wantKeys:  []string{"reconnects", "error"},
		},
		{
			name:      "discovered servers",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.DiscoveredServersCB(nc) },
			wantLevel: "DEBUG",
			wantMsg:   "NATS discovered servers",
			wantKeys:  []string{"count", "servers"},
		},
		{
			name: "async error",
			invoke: func(o nats.Options, nc *nats.Conn) {
				o.AsyncErrorCB(nc, &nats.Subscription{Subject: "orders.>", Queue: "workers"}, errors.New("permissions violation"))
			},
			wantLevel: "ERROR",
			wantMsg:   "NATS asynchronous error",
			wantKeys:  []string{"status", "error", "subject", "queue"},
		},
		{
			name:      "lame duck mode",
			invoke:    func(o nats.Options, nc *nats.Conn) { o.LameDuckModeHandler(nc) },
			wantLevel: "WARN",
			wantMsg:   "NATS server entered lame duck mode",
			wantKeys:  []string{"address", "url"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			m := New().(*module)
			m.cfg.Username, m.cfg.Password = "secret-user", "super-secret"
			m.shutdown.Store(test.shutdown)

			applied := applyOptions(t, mustModuleOptions(t, m, "app", logger))
			test.invoke(applied, &nats.Conn{})

			record := map[string]any{}
			if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
				t.Fatalf("decode log record %q: %v", buf.String(), err)
			}

			if record["level"] != test.wantLevel {
				t.Errorf("level = %v, want %v", record["level"], test.wantLevel)
			}

			if record["msg"] != test.wantMsg {
				t.Errorf("msg = %v, want %v", record["msg"], test.wantMsg)
			}

			for _, key := range test.wantKeys {
				if _, ok := record[key]; !ok {
					t.Errorf("missing log field %q in %v", key, record)
				}
			}

			assertNoSecrets(t, buf.String())
		})
	}
}

func TestPreStopMarksShutdownBeforeClose(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	m := New().(*module)
	m.shutdown.Store(true)

	applied := applyOptions(t, mustModuleOptions(t, m, "app", logger))
	applied.ClosedCB(&nats.Conn{})

	if !strings.Contains(buf.String(), `"msg":"NATS connection closed"`) {
		t.Errorf("shutdown close was not logged as expected: %s", buf.String())
	}

	if strings.Contains(buf.String(), "ERROR") {
		t.Errorf("shutdown close must not be reported as a failure: %s", buf.String())
	}
}

func TestConnectImmediateAndRetry(t *testing.T) {
	t.Run("immediate", func(t *testing.T) {
		addr := startHandshakeServer(t)

		m := New().(*module)
		m.cfg.Addr = addr

		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		nc, err := nats.Connect(addr, mustModuleOptions(t, m, "app", logger)...)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}

		t.Cleanup(nc.Close)

		if status := nc.Status(); status != nats.CONNECTED {
			t.Fatalf("status = %v, want CONNECTED", status)
		}

		assertNoSecrets(t, buf.String())
	})

	t.Run("initial retry", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}

		addr := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatalf("close listener: %v", err)
		}

		m := New().(*module)
		m.cfg.Addr = addr
		m.cfg.ConnectTimeout = 100 * time.Millisecond
		m.cfg.ReconnectWait = 50 * time.Millisecond

		nc, err := nats.Connect(addr, mustModuleOptions(t, m, "app", slog.New(slog.DiscardHandler))...)
		if err != nil {
			t.Fatalf("connect with retry: %v", err)
		}

		if nc == nil {
			t.Fatal("connection is nil, it must be registered even while reconnecting")
		}

		t.Cleanup(nc.Close)

		if status := nc.Status(); status == nats.CONNECTED {
			t.Fatalf("status = %v, want a reconnecting connection", status)
		}
	})
}

func TestSafeURLRemovesCredentials(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "nats://127.0.0.1:4222", want: "nats://127.0.0.1:4222"},
		{in: "nats://user:pass@127.0.0.1:4222", want: "nats://127.0.0.1:4222"},
		{in: "nats://token@127.0.0.1:4222", want: "nats://127.0.0.1:4222"},
	}

	for _, test := range tests {
		if got := safeURL(test.in); got != test.want {
			t.Errorf("safeURL(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func mustOptions(t *testing.T, c *cfg, fallbackName string) []nats.Option {
	t.Helper()

	opts, err := c.connectionOptions(fallbackName)
	if err != nil {
		t.Fatalf("connectionOptions() = %v", err)
	}

	return opts
}

func mustModuleOptions(t *testing.T, m *module, fallbackName string, logger *slog.Logger) []nats.Option {
	t.Helper()

	opts, err := m.options(fallbackName, logger)
	if err != nil {
		t.Fatalf("options() = %v", err)
	}

	return opts
}

func assertNoSecrets(t *testing.T, contents string) {
	t.Helper()

	for _, secret := range []string{"super-secret", testSeed, "pass@"} {
		if strings.Contains(contents, secret) {
			t.Errorf("secret value leaked into %q", contents)
		}
	}
}

func writeCredentialsFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "user.creds")
	contents := "-----BEGIN NATS USER JWT-----\n" + testJWT +
		"\n------END NATS USER JWT------\n\n-----BEGIN USER NKEY SEED-----\n" + testSeed +
		"\n------END USER NKEY SEED------\n"

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}

	return path
}

func writeCertificate(t *testing.T, dir, name string) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	certFile = filepath.Join(dir, name+".pem")
	keyFile = filepath.Join(dir, name+"-key.pem")

	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}

	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certFile, keyFile
}

// startHandshakeServer starts a minimal NATS protocol fixture that completes
// the client handshake so connection behavior can be exercised without a real
// server.
func startHandshakeServer(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
	})

	info, err := json.Marshal(map[string]any{
		"server_id":   "test",
		"server_name": "test",
		"version":     "2.10.0",
		"proto":       1,
		"host":        "127.0.0.1",
		"port":        0,
		"headers":     true,
		"max_payload": 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("encode INFO: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go serveHandshake(ctx, conn, info)
		}
	}()

	return listener.Addr().String()
}

func serveHandshake(ctx context.Context, conn net.Conn, info []byte) {
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(append(append([]byte("INFO "), info...), '\r', '\n')); err != nil {
		return
	}

	reader := bufio.NewReader(conn)
	for {
		if ctx.Err() != nil {
			return
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		switch {
		case strings.HasPrefix(line, "PING"):
			if _, err := conn.Write([]byte("PONG\r\n")); err != nil {
				return
			}

		case strings.HasPrefix(line, "PONG"), strings.HasPrefix(line, "CONNECT"), strings.HasPrefix(line, "SUB"), strings.HasPrefix(line, "UNSUB"):
			// accepted without a response
		}
	}
}
