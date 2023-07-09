package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"

	bhttp "github.com/renevo/bootstrap/modules/http"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/gorilla/mux"
	"github.com/portcullis/application"
	"github.com/renevo/bootstrap"
)

//go:embed static/***
var content embed.FS

const (
	version = "0.0.0"
	name    = "example"
)

func init() {
	// create an otel meter for this app
	meter := otel.Meter("github.com/renevo/bootstrap/example", metric.WithInstrumentationVersion(version))

	// create a gauge with a callback to gather the data
	_, _ = meter.Int64ObservableGauge(
		"test",
		metric.WithUnit("ms"),
		metric.WithInt64Callback(func(ctx context.Context, io metric.Int64Observer) error {
			usage := 8000
			io.Observe(int64(usage), metric.WithAttributes(attribute.Int("pid", os.Getpid())))
			return nil
		}),
	)
}

func main() {
	var dfs fs.FS

	// using an env here, because there isn't currently a good way in the bootstrap to add a configuration before creating it
	if staticPath := os.Getenv("HTTP_STATIC_PATH"); staticPath != "" {
		dfs = os.DirFS(staticPath)
	} else {
		dfs, _ = fs.Sub(content, "static")
	}

	if err := bootstrap.HTTP(name, version, http.FS(dfs),
		application.WithModule("Custom Routes", new(module)),
	); err != nil {
		panic(err)
	}
}

var _ bhttp.Routable = (*module)(nil)

type module struct{}

func (module) Start(ctx context.Context) error {
	return nil
}
func (module) Stop(ctx context.Context) error {
	return nil
}

func (m module) Route(ctx context.Context, router *mux.Router) error {
	router.HandleFunc("/example", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("This is example route"))
	})

	return nil
}
