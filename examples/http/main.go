package main

import (
	"embed"

	"github.com/renevo/bootstrap"
)

//go:embed index.html
var content embed.FS

func main() {
	if err := bootstrap.HTTP("test", "0.0.0", content); err != nil {
		panic(err)
	}
}
