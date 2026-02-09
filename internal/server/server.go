package server

import (
	"context"
	"errors"
	"net/http"
)

type IServer interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

type Server struct {
	httpServer *http.Server
}

func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.httpServer.Shutdown(context.Background())
	}()

	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
