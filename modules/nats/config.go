package nats

import (
	"fmt"
	"net/url"
	"time"

	"github.com/nats-io/nats.go"
)

type cfg struct {
	Name            string `setting:"name" description:"The name of the nats connection"`
	Addr            string `setting:"address" description:"The address to connect to the nats server"`
	Username        string `setting:"username" description:"The user name to use for user/password authentication"`
	Password        string `setting:"password" description:"The password to use for user/password authentication" mask:"true"`
	Token           string `setting:"token" description:"The token to use for authentication" mask:"true"`
	Secret          string `setting:"secret" description:"The token secret, when set, the client will use JWT/seed auth" mask:"true"`
	CredentialsFile string `setting:"credentials_file" description:"The path to the credentials file"`

	ConnectTimeout      time.Duration `setting:"connect_timeout" description:"The timeout for establishing a connection to the nats server"`
	ReconnectWait       time.Duration `setting:"reconnect_wait" description:"The time to wait between reconnect attempts to a server"`
	MaxReconnects       int           `setting:"max_reconnects" description:"The number of reconnect attempts per server, -1 for unlimited, 0 to disable"`
	PingInterval        time.Duration `setting:"ping_interval" description:"The interval at which pings are sent to the nats server"`
	MaxPingsOutstanding int           `setting:"max_pings_outstanding" description:"The number of unanswered pings before the connection is considered stale"`
	DrainTimeout        time.Duration `setting:"drain_timeout" description:"The timeout for draining the connection on shutdown"`

	RootCAFile     string `setting:"root_ca_file" description:"The path to the PEM encoded root certificate authority file used to verify the server"`
	ClientCertFile string `setting:"client_cert_file" description:"The path to the PEM encoded client certificate file used for mTLS"`
	ClientKeyFile  string `setting:"client_key_file" description:"The path to the PEM encoded client private key file used for mTLS"`
}

// validate reports configuration errors by setting name. It never includes
// configured secret values in the returned error.
func (c *cfg) validate() error {
	if (c.Username == "") != (c.Password == "") {
		return fmt.Errorf("nats: %q and %q must both be set", "nats.username", "nats.password")
	}

	if c.Secret != "" && c.Token == "" {
		return fmt.Errorf("nats: %q requires %q to be set", "nats.secret", "nats.token")
	}

	modes := 0
	if c.Username != "" {
		modes++
	}

	if c.Token != "" {
		modes++
	}

	if c.CredentialsFile != "" {
		modes++
	}

	if modes > 1 {
		return fmt.Errorf("nats: only one of %q, %q or %q may be configured", "nats.username", "nats.token", "nats.credentials_file")
	}

	if (c.ClientCertFile == "") != (c.ClientKeyFile == "") {
		return fmt.Errorf("nats: %q and %q must both be set", "nats.client_cert_file", "nats.client_key_file")
	}

	if c.ConnectTimeout <= 0 {
		return fmt.Errorf("nats: %q must be greater than zero", "nats.connect_timeout")
	}

	if c.ReconnectWait <= 0 {
		return fmt.Errorf("nats: %q must be greater than zero", "nats.reconnect_wait")
	}

	if c.PingInterval <= 0 {
		return fmt.Errorf("nats: %q must be greater than zero", "nats.ping_interval")
	}

	if c.DrainTimeout <= 0 {
		return fmt.Errorf("nats: %q must be greater than zero", "nats.drain_timeout")
	}

	if c.MaxPingsOutstanding <= 0 {
		return fmt.Errorf("nats: %q must be greater than zero", "nats.max_pings_outstanding")
	}

	if c.MaxReconnects < -1 {
		return fmt.Errorf("nats: %q must be -1 or greater", "nats.max_reconnects")
	}

	return nil
}

// connectionOptions builds the connection, authentication and TLS options for
// the configuration. The supplied name is used when no connection name is
// configured.
func (c *cfg) connectionOptions(fallbackName string) ([]nats.Option, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	name := c.Name
	if name == "" {
		name = fallbackName
	}

	opts := []nats.Option{
		nats.RetryOnFailedConnect(true), // always set
		nats.Name(name),
		nats.Timeout(c.ConnectTimeout),
		nats.ReconnectWait(c.ReconnectWait),
		nats.MaxReconnects(c.MaxReconnects),
		nats.PingInterval(c.PingInterval),
		nats.MaxPingsOutstanding(c.MaxPingsOutstanding),
		nats.DrainTimeout(c.DrainTimeout),
	}

	switch {
	case c.Username != "":
		opts = append(opts, nats.UserInfo(c.Username, c.Password))

	case c.Token != "" && c.Secret != "":
		opts = append(opts, nats.UserJWTAndSeed(c.Token, c.Secret))

	case c.Token != "":
		opts = append(opts, nats.Token(c.Token))

	case c.CredentialsFile != "":
		opts = append(opts, nats.UserCredentials(c.CredentialsFile))
	}

	if c.RootCAFile != "" {
		opts = append(opts, nats.RootCAs(c.RootCAFile))
	}

	if c.ClientCertFile != "" {
		opts = append(opts, nats.ClientCert(c.ClientCertFile, c.ClientKeyFile))
	}

	return opts, nil
}

// safeURL removes any user information from a server URL so that credentials
// are never written to logs.
func safeURL(raw string) string {
	if raw == "" {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	if parsed.User == nil {
		return raw
	}

	parsed.User = nil

	return parsed.String()
}

func safeURLs(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}

	safe := make([]string, 0, len(raw))
	for _, u := range raw {
		safe = append(safe, safeURL(u))
	}

	return safe
}
