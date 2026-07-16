package http

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type telemetry struct {
	staticRoute    *mux.Route
	allowedMethods []string
}

var cloudflareHeaders = map[string]attribute.Key{
	"Cf-Ray":           "cloudflare.tunnel.ray",
	"Cf-Ipcountry":     "cloudflare.tunnel.ipcountry",
	"Cf-Connecting-Ip": "cloudflare.tunnel.connecting_ip",
	"Cf-Warp-Tag-Id":   "cloudflare.tunnel.warp_tag_id",
}

func newTelemetry() *telemetry {
	return &telemetry{}
}

func (t *telemetry) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		for header, key := range cloudflareHeaders {
			if values := r.Header.Values(header); len(values) > 0 {
				span.SetAttributes(key.StringSlice(values))
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (t *telemetry) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.enrichRoute(r, mux.CurrentRoute(r))
		next.ServeHTTP(w, r)
	})
}

func (t *telemetry) configureFallbackHandlers(router *mux.Router) error {
	methods := make(map[string]struct{})
	if err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		routeMethods, err := route.GetMethods()
		if err != nil {
			return nil
		}
		for _, method := range routeMethods {
			if _, exists := methods[method]; !exists {
				methods[method] = struct{}{}
				t.allowedMethods = append(t.allowedMethods, method)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	notFoundHandler := router.NotFoundHandler
	if notFoundHandler == nil {
		notFoundHandler = http.NotFoundHandler()
	}
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace.SpanFromContext(r.Context()).SetName(r.Method + " 404")
		notFoundHandler.ServeHTTP(w, r)
	})

	methodNotAllowedHandler := router.MethodNotAllowedHandler
	if methodNotAllowedHandler == nil {
		methodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
		})
	}
	router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.enrichRoute(r, t.methodNotAllowedRoute(router, r))
		methodNotAllowedHandler.ServeHTTP(w, r)
	})

	return nil
}

func (t *telemetry) methodNotAllowedRoute(router *mux.Router, r *http.Request) *mux.Route {
	for _, method := range t.allowedMethods {
		if method == r.Method {
			continue
		}

		// A 405 has no CurrentRoute, so retry matching with registered methods
		// to recover the route template without executing its handler chain.
		probe := r.Clone(r.Context())
		probe.Method = method
		var match mux.RouteMatch
		if router.Match(probe, &match) && match.MatchErr == nil && match.Route != nil {
			return match.Route
		}
	}
	return nil
}

func (t *telemetry) enrichRoute(r *http.Request, route *mux.Route) {
	if route == nil {
		return
	}

	routeName := ""
	if route == t.staticRoute {
		routeName = "StaticFile"
	} else if template, err := route.GetPathTemplate(); err == nil {
		routeName = template
	}
	if routeName == "" {
		return
	}

	span := trace.SpanFromContext(r.Context())
	span.SetName(r.Method + " " + routeName)
	span.SetAttributes(attribute.String("http.route", routeName))

	if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
		labeler.Add(attribute.String("http.route", routeName))
	}
}
