package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

// Keep the desktop asset origin stable so Web storage survives app restarts.
const desktopAssetListenAddr = "127.0.0.1:9632"

type desktopAssetServer struct {
	server *http.Server
	ln     net.Listener
	url    string
}

func (s *desktopAssetServer) URL() string {
	return s.url
}

func (s *desktopAssetServer) Close() error {
	return s.server.Close()
}

func startDesktopAssetServer(assets fs.FS) (*desktopAssetServer, error) {
	ln, err := net.Listen("tcp", desktopAssetListenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen desktop asset server on %s: %w", desktopAssetListenAddr, err)
	}
	handler := newDesktopAssetHandler(assets)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	out := &desktopAssetServer{
		server: srv,
		ln:     ln,
		url:    "http://" + ln.Addr().String() + "/",
	}
	go func() {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			return
		}
	}()
	return out, nil
}

func newDesktopAssetHandler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := cleanAssetPath(r.URL.Path)
		if name == "" {
			name = "index.html"
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			if isWorkspaceRoute(name) {
				data, err = fs.ReadFile(assets, "index.html")
				name = "index.html"
			}
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" && name == "index.html" {
			contentType = "text/html; charset=utf-8"
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

func cleanAssetPath(raw string) string {
	clean := path.Clean("/" + raw)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." {
		return ""
	}
	return clean
}

func isWorkspaceRoute(name string) bool {
	return path.Ext(name) == ""
}
