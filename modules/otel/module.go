package otel

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/portcullis/application"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type module struct {
	metricExporter *prometheus.Exporter
	spanProcessor  sdktrace.SpanProcessor
	traceExporter  *otlptrace.Exporter

	cfg struct {
		Addr string `env:"OTEL_GRPC_ADDRESS"`
	}
}

// New returns an application module that will wire up the trace exporter as per configured in the environment
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

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(app.Name),
			semconv.ServiceVersionKey.String(app.Version),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed to initialize otel resource")
	}

	// metrics
	metricExporter, err := prometheus.New()
	if err != nil {
		return errors.Wrap(err, "failed to create otel prometheus exporter")
	}
	m.metricExporter = metricExporter

	otel.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(m.metricExporter), metric.WithResource(res)))

	// tracing
	if m.cfg.Addr != "" {
		conn, err := grpc.DialContext(ctx, m.cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())

		if err != nil {
			return errors.Wrapf(err, "failed to connect to %q grpc collector", m.cfg.Addr)
		}

		traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return errors.Wrapf(err, "failed to create grpc trace exporter")
		}
		m.traceExporter = traceExporter

		// Register the trace exporter with a TracerProvider, using a batch span processor to aggregate spans before export.
		m.spanProcessor = sdktrace.NewBatchSpanProcessor(m.traceExporter)
		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithResource(res),
			sdktrace.WithSpanProcessor(m.spanProcessor),
		)
		otel.SetTracerProvider(tracerProvider)

		// set global propagator to tracecontext (the default is no-op).
		otel.SetTextMapPropagator(propagation.TraceContext{})
	}

	return nil
}

func (m *module) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// shutdown span processor
	if m.spanProcessor != nil {
		_ = m.spanProcessor.ForceFlush(shutdownCtx)
		_ = m.spanProcessor.Shutdown(shutdownCtx)
	}

	// shutdown trace exporter
	if m.traceExporter != nil {
		_ = m.traceExporter.Shutdown(shutdownCtx)
	}

	// shutdown metric exporter
	if m.metricExporter != nil {
		_ = m.metricExporter.ForceFlush(shutdownCtx)
		_ = m.metricExporter.Shutdown(shutdownCtx)
	}
	return nil
}
