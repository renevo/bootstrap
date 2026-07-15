// Package otel provides an application module that configures OpenTelemetry
// metrics and tracing for a bootstrap application.
package otel

import (
	"context"
	"fmt"
	"time"

	"github.com/renevo/application"
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
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	_ application.Module      = (*module)(nil)
	_ application.PreStarter  = (*module)(nil)
	_ application.PostStopper = (*module)(nil)
)

type module struct {
	cfg            *cfg
	metricExporter *prometheus.Exporter
	spanProcessor  sdktrace.SpanProcessor
	traceExporter  *otlptrace.Exporter
}

type cfg struct {
	GRPC struct {
		Address string `setting:"address" description:"The address of the OTEL gRPC collector to send traces to"`
	}
}

// New returns an OpenTelemetry application module. It always registers a
// Prometheus metrics exporter and configures OTLP trace export when a gRPC
// collector address is set.
func New() application.Module {
	return &module{cfg: &cfg{}}
}

func (m *module) Initialize(ctx *application.Context) error {
	return ctx.Settings().Subset("otel").Bind(m.cfg)
}

func (m *module) Start(ctx *application.Context) error {
	return nil
}

func (m *module) PreStart(ctx *application.Context) error {
	app := application.FromContext(ctx)
	if app == nil {
		return nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(app.Name()),
			semconv.ServiceVersionKey.String(app.Version()),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize otel resource: %w", err)
	}

	// metrics
	metricExporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create otel prometheus exporter: %w", err)
	}
	m.metricExporter = metricExporter

	otel.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(m.metricExporter), metric.WithResource(res)))

	// tracing
	if m.cfg.GRPC.Address != "" {
		conn, err := grpc.NewClient(m.cfg.GRPC.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to create grpc collector connection: %w", err)
		}

		conn.Connect()
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
			if !conn.WaitForStateChange(connectCtx, state) {
				return fmt.Errorf("timed out connecting to %q grpc collector", m.cfg.GRPC.Address)
			}
		}

		traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return fmt.Errorf("failed to create grpc trace exporter: %w", err)
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

func (m *module) Stop(ctx *application.Context) error {
	return nil
}

func (m *module) PostStop(ctx *application.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second*5)
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
		_ = m.metricExporter.Shutdown(shutdownCtx)
	}
	return nil
}
