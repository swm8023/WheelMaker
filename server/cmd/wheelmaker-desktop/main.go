package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "WheelMakerDesktop: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	assets, err := embeddedAssets()
	if err != nil {
		return err
	}
	return runDesktopAppWithWebSource(assets, newWebView2Launcher(), newDefaultDesktopWebSourceRuntime())
}
