package handler

import (
	"bytes"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEchoHandlerReturnsSameBody(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/echo",
		bytes.NewBufferString("aboba"),
	)
	rec := httptest.NewRecorder()

	ctx := e.NewContext(req, rec)

	err := Echo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if string(body) != "aboba" {
		t.Fatalf("expected %q, got %q", "aboba", string(body))
	}
}
