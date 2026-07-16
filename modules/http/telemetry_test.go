package http

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTelemetryRouteTemplate(t *testing.T) {
	handler, recorder, reader := newTelemetryTestHandler(t, func(router *mux.Router, _ *telemetry) {
		router.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}).Methods(http.MethodGet)
	})

	for _, path := range []string{"/users/one", "/users/two"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s returned %d, want %d", path, response.Code, http.StatusNoContent)
		}
	}

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	for _, span := range spans {
		assertSpanRoute(t, span, "GET /users/{id}", "/users/{id}")
	}
	if count := metricRouteCount(t, reader, "/users/{id}"); count != 2 {
		t.Errorf("route metric count = %d, want 2", count)
	}
}

func TestTelemetryStaticFile(t *testing.T) {
	handler, recorder, reader := newTelemetryTestHandler(t, func(router *mux.Router, telemetry *telemetry) {
		content := http.FS(fstest.MapFS{
			"one.txt": {Data: []byte("one")},
			"two.txt": {Data: []byte("two")},
		})
		telemetry.staticRoute = router.PathPrefix("/").Handler(http.FileServer(content))
	})

	for _, path := range []string{"/one.txt", "/two.txt"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d, want %d", path, response.Code, http.StatusOK)
		}
	}

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	for _, span := range spans {
		assertSpanRoute(t, span, "GET StaticFile", "StaticFile")
	}
	if count := metricRouteCount(t, reader, "StaticFile"); count != 2 {
		t.Errorf("static file metric count = %d, want 2", count)
	}
}

func TestTelemetryNotFoundHandler(t *testing.T) {
	for _, test := range []struct {
		name   string
		custom bool
	}{
		{name: "default"},
		{name: "custom", custom: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			customCalled := false
			handler, recorder, _ := newTelemetryTestHandler(t, func(router *mux.Router, _ *telemetry) {
				if test.custom {
					router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						customCalled = true
						w.WriteHeader(http.StatusNotFound)
					})
				}
			})

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			if customCalled != test.custom {
				t.Errorf("custom handler called = %t, want %t", customCalled, test.custom)
			}

			span := onlyEndedSpan(t, recorder)
			if span.Name() != "GET 404" {
				t.Errorf("span name = %q, want %q", span.Name(), "GET 404")
			}
			if _, ok := spanAttribute(span, "http.route"); ok {
				t.Error("404 span unexpectedly has http.route")
			}
		})
	}
}

func TestTelemetryMethodNotAllowedHandler(t *testing.T) {
	for _, test := range []struct {
		name   string
		custom bool
	}{
		{name: "default"},
		{name: "custom", custom: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			customCalled := false
			handler, recorder, reader := newTelemetryTestHandler(t, func(router *mux.Router, _ *telemetry) {
				router.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}).Methods(http.MethodGet)
				if test.custom {
					router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						customCalled = true
						w.WriteHeader(http.StatusMethodNotAllowed)
					})
				}
			})

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/users/one", nil))
			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
			}
			if customCalled != test.custom {
				t.Errorf("custom handler called = %t, want %t", customCalled, test.custom)
			}

			assertSpanRoute(t, onlyEndedSpan(t, recorder), "POST /users/{id}", "/users/{id}")
			if count := metricRouteCount(t, reader, "/users/{id}"); count != 1 {
				t.Errorf("route metric count = %d, want 1", count)
			}
		})
	}
}

func TestTelemetryHandlerCloudflareHeaders(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	request.Header.Add("Cf-Ray", "ray-one")
	request.Header.Add("Cf-Ray", "ray-two")
	request.Header.Set("Cf-Ipcountry", "US")
	request.Header.Set("Cf-Connecting-Ip", "203.0.113.10")
	request.Header.Set("Cf-Warp-Tag-Id", "warp-tag")

	telemetry := newTelemetry()
	handler := otelhttp.NewHandler(
		telemetry.handler(http.NotFoundHandler()),
		"test",
		otelhttp.WithTracerProvider(provider),
	)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}

	attributes := make(map[attribute.Key]attribute.Value)
	for _, attr := range spans[0].Attributes() {
		attributes[attr.Key] = attr.Value
	}

	want := map[attribute.Key][]string{
		"cloudflare.tunnel.ray":           {"ray-one", "ray-two"},
		"cloudflare.tunnel.ipcountry":     {"US"},
		"cloudflare.tunnel.connecting_ip": {"203.0.113.10"},
		"cloudflare.tunnel.warp_tag_id":   {"warp-tag"},
	}
	for key, values := range want {
		got, ok := attributes[key]
		if !ok {
			t.Errorf("missing span attribute %q", key)
			continue
		}
		if got.Type() != attribute.STRINGSLICE || !slices.Equal(got.AsStringSlice(), values) {
			t.Errorf("attribute %q = %v, want %v", key, got.AsInterface(), values)
		}
	}
}

func TestTelemetryHandlerOmitsMissingCloudflareHeaders(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	telemetry := newTelemetry()
	handler := otelhttp.NewHandler(
		telemetry.handler(http.NotFoundHandler()),
		"test",
		otelhttp.WithTracerProvider(provider),
	)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}

	for _, attr := range spans[0].Attributes() {
		for _, key := range cloudflareHeaders {
			if attr.Key == key {
				t.Errorf("unexpected empty Cloudflare attribute %q", attr.Key)
			}
		}
	}
}

func newTelemetryTestHandler(
	t *testing.T,
	setup func(*mux.Router, *telemetry),
) (http.Handler, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()

	telemetry := newTelemetry()
	router := mux.NewRouter()
	router.Use(telemetry.middleware)
	setup(router, telemetry)
	if err := telemetry.configureFallbackHandlers(router); err != nil {
		t.Fatalf("configure fallback handlers: %v", err)
	}

	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { _ = traceProvider.Shutdown(t.Context()) })

	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(t.Context()) })

	handler := otelhttp.NewHandler(
		telemetry.handler(router),
		"test",
		otelhttp.WithTracerProvider(traceProvider),
		otelhttp.WithMeterProvider(meterProvider),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method
		}),
	)
	return handler, spanRecorder, metricReader
}

func onlyEndedSpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	return spans[0]
}

func assertSpanRoute(t *testing.T, span sdktrace.ReadOnlySpan, name, route string) {
	t.Helper()
	if span.Name() != name {
		t.Errorf("span name = %q, want %q", span.Name(), name)
	}
	value, ok := spanAttribute(span, "http.route")
	if !ok || value.AsString() != route {
		t.Errorf("http.route = %v, want %q", value.AsInterface(), route)
	}
}

func spanAttribute(span sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, attr := range span.Attributes() {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

func metricRouteCount(t *testing.T, reader *sdkmetric.ManualReader, route string) uint64 {
	t.Helper()
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &metrics); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "http.server.request.duration" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("request duration has type %T, want histogram", metric.Data)
			}
			for _, point := range histogram.DataPoints {
				value, ok := point.Attributes.Value("http.route")
				if ok && value.AsString() == route {
					return point.Count
				}
			}
		}
	}
	return 0
}
