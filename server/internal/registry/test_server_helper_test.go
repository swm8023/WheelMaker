package registry

import (
	"net"
	"net/http"
)

type testHTTPServer struct {
	addr string
	srv  *http.Server
	ln   net.Listener
}

func startHTTPServer(handler http.Handler) (*testHTTPServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	return &testHTTPServer{
		addr: ln.Addr().String(),
		srv:  srv,
		ln:   ln,
	}, nil
}

func (s *testHTTPServer) close() error {
	_ = s.srv.Close()
	return s.ln.Close()
}
