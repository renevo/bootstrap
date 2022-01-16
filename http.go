package bootstrap

import (
	"context"
	"flag"
	"io/fs"
	"os"

	"github.com/portcullis/application"
	"github.com/portcullis/application/modules/logging"
	"github.com/renevo/bootstrap/modules/env"
	"github.com/renevo/bootstrap/modules/http"
)

// HTTP bootstraps a new http application and runs it
func HTTP(name, version string, content fs.FS, opts ...application.Option) error {
	app := &application.Application{
		Name:       name,
		Version:    version,
		Controller: &application.Controller{},
	}

	// in built modules
	app.Controller.Add("Logging", logging.New())
	app.Controller.Add("Environment", env.New("", map[string]string{}))
	app.Controller.Add("HTTP", http.New(content))

	// flag handling
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	cfgFile := fs.String("config", "", "Application configuration file")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *cfgFile != "" {
		application.WithConfigFile(*cfgFile)(app)
	}

	// process the provided app options
	for _, opt := range opts {
		opt(app)
	}

	// run the app
	return app.Run(context.Background())
}
