package main

import (
	"embed"
	"flag"
	"io/fs"
	"net/http"
	"os"

	"github.com/renevo/bootstrap"
)

//go:embed static/***
var content embed.FS

func main() {
	flag.Parse()

	var hfs http.FileSystem
	if staticPath := os.Getenv("HTTP_STATIC_PATH"); staticPath != "" {
		hfs = http.Dir(staticPath)
	} else {
		sfs, _ := fs.Sub(content, "static")
		hfs = http.FS(sfs)
	}

	if err := bootstrap.HTTP("test", "0.0.0", hfs); err != nil {
		panic(err)
	}
}
