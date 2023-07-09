package spa

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/gorilla/mux"
	"github.com/portcullis/application"
)

// New will return a Single Page Application module to serve the supplied index
func New(index string, fs fs.FS) application.Module {
	return &module{fs, index}
}

type module struct {
	fs    fs.FS
	index string
}

func (m *module) Start(ctx context.Context) error { return nil }
func (m *module) Stop(ctx context.Context) error  { return nil }
func (m *module) Route(ctx context.Context, router *mux.Router) error {
	if m.index == "" {
		m.index = "index.html"
	}

	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Restrict only to instances where the browser is looking for an HTML file
			if !strings.Contains(r.Header.Get("Accept"), "text/html") {
				next.ServeHTTP(w, r)
				return
			}

			is404 := false
			wrapper := httpsnoop.Wrap(w, httpsnoop.Hooks{
				WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
					return func(code int) {
						if code != http.StatusNotFound {
							next(code)
							return
						}

						index, err := m.fs.Open(m.index)
						if err != nil {
							next(code)
							return
						}
						defer func() { _ = index.Close() }()

						st, err := index.Stat()
						if err != nil {
							next(code)
							return
						}

						// if we can stat/read the file, we take over the http response code and write functions
						is404 = true

						// overwrite whatever was set previously
						w.Header().Set("content-type", "text/html; charset=utf-8")
						http.ServeContent(w, r, st.Name(), st.ModTime(), index.(io.ReadSeeker))
					}
				},
				Write: func(wf httpsnoop.WriteFunc) httpsnoop.WriteFunc {
					return func(p []byte) (int, error) {
						if !is404 {
							return wf(p)
						}

						// won't write anything as this is going to no longer be valid to call
						return 0, nil
					}
				},
			})

			next.ServeHTTP(wrapper, r)
		})
	})

	return nil
}
