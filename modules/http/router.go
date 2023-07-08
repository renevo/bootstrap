package http

import (
	"context"

	"github.com/gorilla/mux"
)

// Routable modules support gorilla mux routes being handled
type Routable interface {
	// Route will be called in the Start phase of the application bootstrap to add handlers/middleware for the supplied mux.Router
	Route(ctx context.Context, router *mux.Router) error
}
