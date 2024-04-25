package nats

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/portcullis/application"
	"github.com/renevo/ioc"
)

var (
	_ application.Module       = (*module)(nil)
	_ application.Configurable = (*module)(nil)
	_ application.Initializer  = (*module)(nil)
)

type module struct {
	cfg        *cfg
	client     *nats.Conn
	clientOpts []nats.Option
}

func New() application.Module {
	m := &module{
		cfg: &cfg{
			NATS: &natsConfig{},
		},
		clientOpts: []nats.Option{
			nats.RetryOnFailedConnect(true), // always set
		},
	}

	return m
}

func (m *module) Config() (any, error) {
	return m.cfg, nil
}

func (m *module) Initialize(ctx context.Context) (context.Context, error) {
	if m.cfg.NATS.Addr == "" {
		return ctx, nil
	}

	if m.cfg.NATS.Name != "" {
		m.clientOpts = append(m.clientOpts, nats.Name(m.cfg.NATS.Name))
	} else {
		m.clientOpts = append(m.clientOpts, nats.Name(fmt.Sprintf("%s-%d", application.FromContext(ctx).Name, os.Getpid())))
	}

	if m.cfg.NATS.Token != "" {
		if m.cfg.NATS.Secret != "" {
			m.clientOpts = append(m.clientOpts, nats.UserJWTAndSeed(m.cfg.NATS.Token, m.cfg.NATS.Secret))
		} else {
			m.clientOpts = append(m.clientOpts, nats.Token(m.cfg.NATS.Token))
		}
	}

	if m.cfg.NATS.CredentialsFile != "" {
		m.clientOpts = append(m.clientOpts, nats.UserCredentials(m.cfg.NATS.CredentialsFile))
	}

	log := slog.With("module", "nats")

	nc, err := nats.Connect(m.cfg.NATS.Addr, m.clientOpts...)
	if err != nil {
		return ctx, fmt.Errorf("failed to connect to nats server: %w", err)
	}

	m.client = nc
	ioc.RegisterToContext(ctx, m.client)

	log.Info("Connected to NATS server", "address", m.client.ConnectedAddr())

	return ctx, nil
}

func (m *module) Start(ctx context.Context) error {
	return nil
}

func (m *module) Stop(ctx context.Context) error {
	if m.client == nil {
		return nil
	}

	return m.client.Drain()
}
