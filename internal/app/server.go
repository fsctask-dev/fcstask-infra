package app

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"time"

	"fcstask-infra/internal/api"
	"fcstask-infra/internal/server"
)

type App struct {
	echo            *echo.Echo
	shutdownTimeout time.Duration
}

func New(
	host string,
	port int,
	shutdownTimeout time.Duration,
) *App {
	e := echo.New()
	apiServer := &server.Server{}

	api.RegisterHandlers(e, apiServer)

	addr := fmt.Sprintf("%s:%d", host, port)

	e.HideBanner = true
	e.Server.Addr = addr

	return &App{
		echo:            e,
		shutdownTimeout: shutdownTimeout,
	}
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		errCh <- a.echo.Start(a.echo.Server.Addr)
	}()

	select {
	case err := <-errCh:
		return err

	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			a.shutdownTimeout,
		)
		defer cancel()

		return a.echo.Shutdown(shutdownCtx)
	}
}
