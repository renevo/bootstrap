package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/portcullis/application"
	"github.com/portcullis/config"
	"github.com/portcullis/logging"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
)

type module struct {
	content  http.FileSystem
	listener net.Listener
	server   *http.Server

	cfg struct {
		HTTP struct {
			Addr            string        `config:"address,label" env:"HTTP_ADDRESS" flag:"address" setting:"Address" description:"The address to listen for the http server"`
			ReadTimeout     time.Duration `config:"read_timeout,optional"`
			WriteTimeout    time.Duration `config:"write_timeout,optional"`
			IdleTimeout     time.Duration `config:"idle_timeout,optional"`
			ShutdownTimeout time.Duration `config:"shutdown_timeout,optional"`
		} `config:"http,block"`
	}
}

// New creates a new HTTP server and serves up the specified optional file system at the root
func New(content http.FileSystem) application.Module {
	hm := &module{content: content}

	hm.cfg.HTTP.Addr = ":8080"
	hm.cfg.HTTP.IdleTimeout = 120 * time.Second
	hm.cfg.HTTP.ReadTimeout = 5 * time.Second
	hm.cfg.HTTP.WriteTimeout = 10 * time.Second
	hm.cfg.HTTP.ShutdownTimeout = 30 * time.Second

	config.Subset("HTTP").Bind(&hm.cfg.HTTP)

	return hm
}

func (m *module) Config() (interface{}, error) {
	return &m.cfg, nil
}

func (m *module) Start(ctx context.Context) error {
	router := mux.NewRouter()

	// setup http server
	m.server = &http.Server{
		ReadTimeout:  m.cfg.HTTP.ReadTimeout,
		WriteTimeout: m.cfg.HTTP.WriteTimeout,
		IdleTimeout:  m.cfg.HTTP.IdleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		Handler: handlers.CombinedLoggingHandler(os.Stderr, handlers.ProxyHeaders(router)),
	}

	// panic handling
	router.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))

	// default headers
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", "RenEvo/1.0")

			next.ServeHTTP(w, r)
		})
	})

	// health check endpoint
	router.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// if io.FS is set, serve the static files
	if m.content != nil {
		router.PathPrefix("/").Handler(http.FileServer(m.content))
	}

	// tracing
	// TODO: add cloud flare headers as span attributes
	// Cf-Ray (ID)
	// Cf-Ipcountry
	// Cf-Connecting-Ip
	// Cf-Warp-Tag-Id
	router.Use(otelmux.Middleware(application.FromContext(ctx).Name))

	// listener
	// TODO: support unix://
	listener, err := net.Listen("tcp", m.cfg.HTTP.Addr)
	if err != nil {
		return errors.Wrapf(err, "failed to listen on %q", m.cfg.HTTP.Addr)
	}

	if tcpListener, ok := listener.(*net.TCPListener); ok {
		m.listener = tcpKeepAliveListener{tcpListener}
	} else {
		m.listener = listener
	}

	logging.FromContext(ctx).Info("HTTP Server Listening: http://%s", m.listener.Addr().String())

	go func() {
		err := m.server.Serve(m.listener)

		// don't panic on server closed
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// don't panic on not being able to accept connections (dirty/hacky/works)
			if nopErr, ok := errors.Cause(err).(*net.OpError); ok && (strings.EqualFold(nopErr.Op, "accept") && strings.Contains(nopErr.Error(), "closed network connection")) {
				return
			}

			app := application.FromContext(ctx)
			if app != nil {
				app.Exit(errors.Wrap(err, "http server failed to serve"))
				return
			}

			// can't gracefully shutdown, so just die
			logging.Fatal("HTTP Serve Failure: %v", err)
		}
	}()

	return nil
}

func (m *module) Stop(ctx context.Context) error {
	// no more new connections
	if m.listener != nil {
		m.listener.Close()
		m.listener = nil
	}

	// stop http server
	if m.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), m.cfg.HTTP.ShutdownTimeout)
		defer cancel()

		_ = m.server.Shutdown(shutdownCtx)
		_ = m.server.Close()

		m.server = nil
	}

	return nil
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted connections. It's used so dead TCP connections (e.g. closing laptop mid-download) eventually go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}

	_ = tc.SetKeepAlive(true)
	_ = tc.SetKeepAlivePeriod(3 * time.Minute)

	return tc, nil
}
