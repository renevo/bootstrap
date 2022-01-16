package env

import (
	"context"

	"github.com/caarlos0/env/v6"
	"github.com/pkg/errors"
	"github.com/portcullis/application"
)

type module struct {
	opts env.Options
}

// New creates a new application.Module to load configuration values from the environment
func New(prefix string, seed map[string]string) application.Module {
	m := &module{
		opts: env.Options{
			Prefix:      prefix,
			Environment: seed,
		},
	}

	return m
}

func (m *module) Initialize(ctx context.Context) (context.Context, error) {
	app := application.FromContext(ctx)
	if app == nil {
		return ctx, nil
	}

	var initErr error

	opts := env.Options{}

	app.Controller.Range(func(name string, m application.Module) bool {
		if c, ok := m.(application.Configurable); ok {
			cfg, err := c.Config()
			if err != nil {
				initErr = errors.Wrapf(err, "failed to config for module %q", name)
				return false
			}

			if err := env.Parse(cfg, opts); err != nil {
				initErr = errors.Wrapf(err, "failed to parse environment for module %q", name)
				return false
			}
		}
		return true
	})

	return ctx, initErr
}

func (m *module) Start(ctx context.Context) error {
	return nil
}

func (m *module) Stop(ctx context.Context) error {
	return nil
}
