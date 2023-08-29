package bootstrap

import (
	"context"
	"flag"
	"log/slog"
	gohttp "net/http"
	"os"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/portcullis/application"
	"github.com/renevo/bootstrap/modules/env"
	"github.com/renevo/bootstrap/modules/http"
	"github.com/renevo/bootstrap/modules/otel"
)

// HTTP bootstraps a new http application and runs it
func HTTP(name, version string, content gohttp.FileSystem, opts ...application.Option) error {
	// initialize flags before constructing modules to allow them to register config
	// see if any flags have been added to the default flagset
	var hasFlags bool
	flag.VisitAll(func(*flag.Flag) {
		hasFlags = true
	})

	if !hasFlags {
		flag.CommandLine = flag.NewFlagSet(name, flag.ExitOnError)
	}

	// global application flags
	cfgFile := flag.CommandLine.String("config", "", "Application configuration file")
	debugFlag := flag.CommandLine.Bool("debug", false, "Enable application debug logging output")
	jsonFlag := flag.CommandLine.Bool("json", false, "Enable JSON logging output")
	noColorFlag := flag.CommandLine.Bool("no-color", false, "Disable colorized output on text")

	// parse them
	if !flag.CommandLine.Parsed() {
		if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
			return err
		}
	}

	// logger setup
	var logLeveler slog.LevelVar
	var logHandler slog.Handler
	logOutput := os.Stdout

	switch {
	case *jsonFlag:
		logHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &logLeveler})
	case *noColorFlag:
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: &logLeveler})
	default:
		logHandler = tint.NewHandler(colorable.NewColorable(logOutput), &tint.Options{
			Level:   &logLeveler,
			NoColor: !isatty.IsTerminal(logOutput.Fd()),
		})
	}

	if *debugFlag {
		logLeveler.Set(slog.LevelDebug)
	}

	// initialize the application
	app := &application.Application{
		Name:       name,
		Version:    version,
		Controller: &application.Controller{},
		Logger:     slog.New(logHandler),
	}

	app.Controller.Add("Environment", env.New("", map[string]string{}))
	app.Controller.Add("Telemetry", otel.New())
	app.Controller.Add("HTTP", http.New(content))

	// if we have a configuration file, then pass it in to get parsed/processed
	if *cfgFile != "" {
		application.WithConfigFile(*cfgFile)(app)
	}

	// process the provided app options
	for _, opt := range opts {
		opt(app)
	}

	// add the application name to all logs on this logger
	app.Logger = app.Logger.With("app", name)
	slog.SetDefault(app.Logger)

	// run the app
	return app.Run(context.Background())
}
