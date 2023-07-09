package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"

	"github.com/portcullis/application"
	"github.com/renevo/bootstrap"
	"github.com/renevo/bootstrap/modules/spa"
)

//go:embed static/***
var content embed.FS

const (
	version = "0.0.0"
	name    = "spa"
)

func main() {
	var dfs fs.FS

	// using an env here, because there isn't currently a good way in the bootstrap to add a configuration before creating it
	if staticPath := os.Getenv("HTTP_STATIC_PATH"); staticPath != "" {
		dfs = os.DirFS(staticPath)
	} else {
		dfs, _ = fs.Sub(content, "static")
	}

	if err := bootstrap.HTTP(name, version, http.FS(dfs),
		application.WithModule("Single Page Application", spa.New("index.html", dfs)),
	); err != nil {
		panic(err)
	}
}
