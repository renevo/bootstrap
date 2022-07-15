package main

import (
	"embed"
	"net/http"

	"github.com/renevo/bootstrap"
)

//go:embed index.html
var content embed.FS

func main() {
	if err := bootstrap.HTTP("test", "0.0.0", http.FS(content)); err != nil {
		panic(err)
	}
}
