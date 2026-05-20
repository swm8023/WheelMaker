package main

import (
	"embed"
	"io/fs"
)

//go:embed all:webroot
var embeddedWebRoot embed.FS

func embeddedAssets() (fs.FS, error) {
	return fs.Sub(embeddedWebRoot, "webroot")
}
