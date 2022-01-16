package http

import (
	"context"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/portcullis/application"
	"github.com/portcullis/logging"
)

type module struct {
	content  fs.FS
	listener net.Listener
	server   *http.Server

	cfg struct {
		HTTP struct {
			Addr            string        `config:"address,label" env:"HTTP_ADDRESS"`
			ReadTimeout     time.Duration `config:"read_timeout,optional"`
			WriteTimeout    time.Duration `config:"write_timeout,optional"`
			IdleTimeout     time.Duration `config:"idle_timeout,optional"`
			ShutdownTimeout time.Duration `config:"shutdown_timeout,optional"`
		} `config:"http,block"`
	}
}

// New creates a new HTTP server and serves up the specified optional file system at the root
func New(content fs.FS) application.Module {
	hm := &module{content: content}

	hm.cfg.HTTP.Addr = ":8080"
	hm.cfg.HTTP.IdleTimeout = 120 * time.Second
	hm.cfg.HTTP.ReadTimeout = 5 * time.Second
	hm.cfg.HTTP.WriteTimeout = 10 * time.Second
	hm.cfg.HTTP.ShutdownTimeout = 30 * time.Second

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
		Handler: router,
	}

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
		router.PathPrefix("/").Handler(http.FileServer(http.FS(m.content)))
	}

	// TODO: logging

	// TODO: metrics

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

	go func() {
		err := m.server.Serve(m.listener)

		// don't panic on server closed
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// don't panic on not being able to accept connections (dirty/hacky/works)
			if nopErr, ok := errors.Cause(err).(*net.OpError); ok && (strings.EqualFold(nopErr.Op, "accept") && strings.Contains(nopErr.Error(), "closed network connection")) {
				return
			}

			// app package needs an err handler `app.Error(err)` to do normalized shutdown instead of inline panics
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
