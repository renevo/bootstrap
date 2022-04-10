package otel

import (
	"context"

	"github.com/pkg/errors"
	"github.com/portcullis/application"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type module struct {
	cfg struct {
		Addr string `env:"OTEL_GRPC_ADDRESS"`
	}
}

func New() application.Module {
	return &module{}
}

func (m *module) Config() (interface{}, error) {
	return &m.cfg, nil
}

func (m *module) Start(ctx context.Context) error {
	app := application.FromContext(ctx)
	if app == nil {
		return nil
	}

	if m.cfg.Addr == "" {
		return nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(app.Name),
		),
	)

	if err != nil {
		return errors.Wrap(err, "failed to initialize otel resource")
	}

	conn, err := grpc.DialContext(ctx, m.cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())

	if err != nil {
		return errors.Wrapf(err, "failed to connect to %q grpc collector", m.cfg.Addr)
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return errors.Wrapf(err, "failed to create grpc trace exporter")
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return nil
}

func (m *module) Stop(ctx context.Context) error {
	return nil
}
