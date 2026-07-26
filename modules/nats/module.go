// Package nats provides an application module that connects to a NATS server
// and registers the connection with the application's IoC context.
package nats

import (
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

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
	cfg      *cfg
	client   *nats.Conn
	shutdown atomic.Bool
}

// New returns a NATS application module. The module remains inactive when no
// server address is configured. The exposed connection tunings are initialized
// from the native client defaults.
func New() application.Module {
	defaults := nats.GetDefaultOptions()

	return &module{
		cfg: &cfg{
			ConnectTimeout:      defaults.Timeout,
			ReconnectWait:       defaults.ReconnectWait,
			MaxReconnects:       defaults.MaxReconnect,
			PingInterval:        defaults.PingInterval,
			MaxPingsOutstanding: defaults.MaxPingsOut,
			DrainTimeout:        defaults.DrainTimeout,
		},
	}
}

func (m *module) Initialize(ctx *application.Context) error {
	return ctx.Settings().Subset("nats").Bind(m.cfg)
}

// options builds the complete set of client options, including the event
// handlers that log connection lifecycle events.
func (m *module) options(fallbackName string, logger *slog.Logger) ([]nats.Option, error) {
	opts, err := m.cfg.connectionOptions(fallbackName)
	if err != nil {
		return nil, err
	}

	return append(opts, m.eventOptions(logger)...), nil
}

func (m *module) eventOptions(logger *slog.Logger) []nats.Option {
	return []nats.Option{
		nats.ConnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS connected",
				"address", nc.ConnectedAddr(),
				"url", safeURL(nc.ConnectedUrl()),
				"status", nc.Status().String(),
			)
		}),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Warn("NATS disconnected",
				"status", nc.Status().String(),
				"reconnects", nc.Stats().Reconnects,
				"error", err,
			)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected",
				"address", nc.ConnectedAddr(),
				"url", safeURL(nc.ConnectedUrl()),
				"status", nc.Status().String(),
				"reconnects", nc.Stats().Reconnects,
			)
		}),
		nats.ReconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Warn("NATS reconnect attempt failed",
				"status", nc.Status().String(),
				"reconnects", nc.Stats().Reconnects,
				"error", err,
			)
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			if m.shutdown.Load() {
				logger.Info("NATS connection closed",
					"reconnects", nc.Stats().Reconnects,
				)

				return
			}

			logger.Error("NATS connection closed unexpectedly",
				"reconnects", nc.Stats().Reconnects,
				"error", nc.LastError(),
			)
		}),
		nats.DiscoveredServersHandler(func(nc *nats.Conn) {
			servers := safeURLs(nc.DiscoveredServers())

			logger.Debug("NATS discovered servers",
				"count", len(servers),
				"servers", servers,
			)
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			attrs := []any{"status", nc.Status().String(), "error", err}
			if sub != nil {
				attrs = append(attrs, "subject", sub.Subject, "queue", sub.Queue)
			}

			logger.Error("NATS asynchronous error", attrs...)
		}),
		nats.LameDuckModeHandler(func(nc *nats.Conn) {
			logger.Warn("NATS server entered lame duck mode",
				"address", nc.ConnectedAddr(),
				"url", safeURL(nc.ConnectedUrl()),
			)
		}),
	}
}

func (m *module) PreStart(ctx *application.Context) error {
	if m.cfg.Addr == "" {
		return nil
	}

	app := ctx.Application()
	logger := ctx.Logger()

	opts, err := m.options(fmt.Sprintf("%s-%d", app.Name(), os.Getpid()), logger)
	if err != nil {
		return err
	}

	nc, err := nats.Connect(m.cfg.Addr, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to nats server: %w", err)
	}

	// the connection is registered even when the initial retry left it
	// reconnecting, so dependents always resolve the same client
	m.client = nc
	ioc.RegisterToContext(ctx, m.client)

	if status := nc.Status(); status != nats.CONNECTED {
		logger.Warn("NATS server connection pending", "status", status.String())
	}

	return nil
}

func (m *module) Start(ctx *application.Context) error {
	return nil
}

func (m *module) PreStop(ctx *application.Context) error {
	if m.client == nil {
		return nil
	}

	m.shutdown.Store(true)

	return m.client.Drain()
}

func (m *module) Stop(ctx *application.Context) error {
	return nil
}
