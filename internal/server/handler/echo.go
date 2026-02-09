package handler

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

func Echo(ctx echo.Context) error {
	req := ctx.Request()
	res := ctx.Response()

	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read body")
	}

	res.Header().Set(echo.HeaderContentType, "application/octet-stream")
	res.WriteHeader(http.StatusOK)
	_, _ = res.Write(body)

	return nil
}
