// internal/server/server.go
package server

import (
	"context"
	"fmt"
	"jobrunner/internal/config"
	"jobrunner/internal/controller"

	"github.com/labstack/echo/v4"
)

type Server struct {
    echo   *echo.Echo
    config *config.Config
}

func New(cfg *config.Config, jobController *controller.JobController) *Server {
    e := echo.New()

    e.GET("/ping", jobController.Ping)
    
    return &Server{
        echo:   e,
        config: cfg,
    }
}

func (s *Server) Start() error {
    addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
    return s.echo.Start(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}