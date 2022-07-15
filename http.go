package bootstrap

import (
	"context"
	"flag"
	"os"

	gohttp "net/http"

	"github.com/portcullis/application"
	"github.com/portcullis/application/modules/logging"
	"github.com/renevo/bootstrap/modules/env"
	"github.com/renevo/bootstrap/modules/http"
	"github.com/renevo/bootstrap/modules/otel"
)

// HTTP bootstraps a new http application and runs it
func HTTP(name, version string, content gohttp.FileSystem, opts ...application.Option) error {
	app := &application.Application{
		Name:       name,
		Version:    version,
		Controller: &application.Controller{},
	}

	// initialize flags before constructing modules to allow them to register config
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	flag.CommandLine = fs

	// in built modules
	app.Controller.Add("Logging", logging.New())
	app.Controller.Add("Environment", env.New("", map[string]string{}))
	app.Controller.Add("Telemetry", otel.New())
	app.Controller.Add("HTTP", http.New(content))

	// global application flags
	cfgFile := fs.String("config", "", "Application configuration file")

	// parse them
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	// if we have a configuration file, then pass it in to get parsed/processed
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
