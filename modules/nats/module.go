// Package nats provides an application module that connects to a NATS server
// and registers the connection with the application's IoC context.
package nats

import (
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/renevo/application"
	"github.com/renevo/ioc"
)

var (
	_ application.Module      = (*module)(nil)
	_ application.PreStarter  = (*module)(nil)
	_ application.PreStopper  = (*module)(nil)
	_ application.Initializer = (*module)(nil)
)

type module struct {
	cfg        *cfg
	client     *nats.Conn
	clientOpts []nats.Option
}

// New returns a NATS application module. The module remains inactive when no
// server address is configured.
func New() application.Module {
	m := &module{
		cfg: &cfg{},
		clientOpts: []nats.Option{
			nats.RetryOnFailedConnect(true), // always set
		},
	}

	return m
}

func (m *module) Initialize(ctx *application.Context) error {
	return ctx.Settings().Subset("nats").Bind(m.cfg)
}

func (m *module) PreStart(ctx *application.Context) error {
	if m.cfg.Addr == "" {
		return nil
	}

	app := ctx.Application()

	// TODO: make this way more configurable, but for now just set the name and token/secret if provided

	if m.cfg.Name != "" {
		m.clientOpts = append(m.clientOpts, nats.Name(m.cfg.Name))
	} else {
		m.clientOpts = append(m.clientOpts, nats.Name(fmt.Sprintf("%s-%d", app.Name(), os.Getpid())))
	}

	if m.cfg.Token != "" {
		if m.cfg.Secret != "" {
			m.clientOpts = append(m.clientOpts, nats.UserJWTAndSeed(m.cfg.Token, m.cfg.Secret))
		} else {
			m.clientOpts = append(m.clientOpts, nats.Token(m.cfg.Token))
		}
	}

	if m.cfg.CredentialsFile != "" {
		m.clientOpts = append(m.clientOpts, nats.UserCredentials(m.cfg.CredentialsFile))
	}

	logger := ctx.Logger()

	nc, err := nats.Connect(m.cfg.Addr, m.clientOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to nats server: %w", err)
	}

	m.client = nc
	ioc.RegisterToContext(ctx, m.client)

	logger.Info("Connected to NATS server", "address", m.client.ConnectedAddr())

	return nil
}

func (m *module) Start(ctx *application.Context) error {
	return nil
}

func (m *module) PreStop(ctx *application.Context) error {
	if m.client == nil {
		return nil
	}

	return m.client.Drain()
}

func (m *module) Stop(ctx *application.Context) error {
	return nil
}
