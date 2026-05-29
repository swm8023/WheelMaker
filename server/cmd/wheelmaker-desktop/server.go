package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
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
	serverErr := s.server.Close()
	listenerErr := s.ln.Close()
	if serverErr != nil && !errors.Is(serverErr, net.ErrClosed) {
		return serverErr
	}
	if listenerErr != nil && !errors.Is(listenerErr, net.ErrClosed) {
		return listenerErr
	}
	return nil
}

func startDesktopAssetServer(assets fs.FS) (*desktopAssetServer, error) {
	return startDesktopAssetServerWithWebSource(assets, newEmbeddedOnlyDesktopWebSourceRuntime())
}

func startDesktopAssetServerWithWebSource(assets fs.FS, webSource *desktopWebSourceRuntime) (*desktopAssetServer, error) {
	ln, err := net.Listen("tcp", desktopAssetListenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen desktop asset server on %s: %w", desktopAssetListenAddr, err)
	}
	handler := newDesktopAssetHandlerWithWebSource(assets, webSource)
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
	return newDesktopAssetHandlerWithWebSource(assets, newEmbeddedOnlyDesktopWebSourceRuntime())
}

func newDesktopAssetHandlerWithWebSource(assets fs.FS, webSource *desktopWebSourceRuntime) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := cleanAssetPath(r.URL.Path)
		if name == "" {
			name = "index.html"
		}
		if isRegistryWebSocketRoute(name) {
			http.NotFound(w, r)
			return
		}
		if webSource != nil && serveRemoteDesktopAsset(w, r, name, webSource) {
			return
		}
		if webSource != nil {
			webSource.SetActualSource(desktopWebSourceActualEmbedded)
		}
		serveEmbeddedDesktopAsset(w, r, assets, name)
	})
}

func serveRemoteDesktopAsset(w http.ResponseWriter, r *http.Request, name string, webSource *desktopWebSourceRuntime) bool {
	remoteBase, ok := webSource.remoteBaseURL()
	if !ok {
		return false
	}
	remoteURL, ok := buildDesktopRemoteAssetURL(remoteBase, name)
	if !ok {
		return false
	}
	if serveRemoteDesktopAssetURL(w, r, remoteURL, webSource) {
		return true
	}
	if isWorkspaceRoute(name) {
		indexURL, ok := buildDesktopRemoteAssetURL(remoteBase, "index.html")
		if ok && indexURL != remoteURL {
			return serveRemoteDesktopAssetURL(w, r, indexURL, webSource)
		}
	}
	return false
}

func serveRemoteDesktopAssetURL(w http.ResponseWriter, r *http.Request, remoteURL string, webSource *desktopWebSourceRuntime) bool {
	resp, err := webSource.httpClient().Get(remoteURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		copyDesktopRemoteHeaders(w.Header(), resp.Header)
		w.WriteHeader(http.StatusNotModified)
		webSource.SetActualSource(desktopWebSourceActualRemote)
		return true
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	copyDesktopRemoteHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
	webSource.SetActualSource(desktopWebSourceActualRemote)
	return true
}

func serveEmbeddedDesktopAsset(w http.ResponseWriter, r *http.Request, assets fs.FS, name string) {
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
	setDesktopAssetCacheControl(w.Header(), name)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func setDesktopAssetCacheControl(header http.Header, name string) {
	if value := desktopAssetCacheControl(name); value != "" {
		header.Set("Cache-Control", value)
	}
}

func desktopAssetCacheControl(name string) string {
	base := path.Base(name)
	switch base {
	case "index.html", "service-worker.js":
		return "no-cache, must-revalidate"
	case "runtime-config.js", "web-build.json":
		return "no-store"
	}
	if isImmutableDesktopAsset(base) {
		return "public, max-age=31536000, immutable"
	}
	return ""
}

func isImmutableDesktopAsset(base string) bool {
	ext := strings.ToLower(path.Ext(base))
	if strings.HasPrefix(base, "bundle.") && (ext == ".js" || ext == ".css") {
		return true
	}
	switch ext {
	case ".woff", ".woff2", ".ttf", ".eot", ".svg":
		return true
	default:
		return false
	}
}

func buildDesktopRemoteAssetURL(remoteBase string, name string) (string, bool) {
	parsed, err := url.Parse(remoteBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	remotePath := "/"
	if name != "index.html" {
		remotePath = "/" + strings.TrimPrefix(name, "/")
	}
	parsed.Path = remotePath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), true
}

func copyDesktopRemoteHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{
		"Cache-Control",
		"Content-Type",
		"ETag",
		"Last-Modified",
	} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
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

func isRegistryWebSocketRoute(name string) bool {
	return name == "ws"
}
