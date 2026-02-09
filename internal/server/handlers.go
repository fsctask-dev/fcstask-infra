package server

import (
	"fcstask-infra/internal/server/handler"
	"github.com/labstack/echo/v4"
)

func (s *Server) PostV1Echo(ctx echo.Context) error {
	return handler.Echo(ctx)
}