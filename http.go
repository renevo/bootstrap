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
	"github.com/renevo/application"
	"github.com/renevo/bootstrap/modules/http"
	"github.com/renevo/bootstrap/modules/nats"
	"github.com/renevo/bootstrap/modules/otel"
	"github.com/renevo/config"
	"github.com/renevo/ioc"
)

// HTTP creates and runs an HTTP application with the standard telemetry, NATS,
// and HTTP modules. Content may be nil when the application does not serve
// static files. Additional options are applied after the standard options.
//
// HTTP reads command-line flags from flag.CommandLine and configuration from
// the environment. When the -config flag is set, the named configuration file
// is loaded before the environment, allowing environment values to override
// file values. The application runs until it receives a termination signal or
// encounters an error.
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
	generateConfig := flag.CommandLine.Bool("generate-config", false, "Generate a default configuration file and exit")

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
		logHandler = tint.NewTextHandler(colorable.NewColorable(logOutput), &tint.Options{
			Level:   &logLeveler,
			NoColor: !isatty.IsTerminal(logOutput.Fd()),
		})
	}

	if *debugFlag {
		logLeveler.Set(slog.LevelDebug)
	}

	logger := slog.New(logHandler).With("version", version)

	bootstrapOpts := []application.Option{
		application.WithLogger(logger),
	}

	var configSources []config.Source

	// if we have a configuration file, then pass it in to get parsed/processed
	if *cfgFile != "" {
		configSources = []config.Source{application.ConfigFileSource(*cfgFile), config.EnvironmentSource("")}
	} else {
		configSources = []config.Source{config.EnvironmentSource("")}
	}

	bootstrapOpts = append(bootstrapOpts,
		application.WithConfigSources(configSources...),
		application.WithModule("Telemetry", otel.New()),
		application.WithModule("NATS", nats.New()),
		application.WithModule("HTTP", http.New(content)),
	)

	// create a new context with the ioc container
	ctx := ioc.WithContext(context.Background(), &ioc.Container{})

	app, err := application.New(name, version, append(bootstrapOpts, opts...)...)
	if err != nil {
		return err
	}

	slog.SetDefault(app.Logger())

	if *generateConfig {
		if err := app.WriteConfigTemplate(ctx, os.Stdout); err != nil {
			return err
		}

		return nil
	}

	// run the app
	return app.Run(ctx, application.WithSignals())
}
