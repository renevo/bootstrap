package http

import (
	"context"

	"github.com/gorilla/mux"
)

// Routable is implemented by application modules that register HTTP routes or
// middleware with the shared Gorilla Mux router.
type Routable interface {
	// Route registers handlers or middleware during the HTTP module's Start
	// phase. Returning an error prevents the application from starting.
	Route(ctx context.Context, router *mux.Router) error
}
