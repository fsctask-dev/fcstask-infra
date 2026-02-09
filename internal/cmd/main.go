package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fcstask-infra/internal/app"
	"fcstask-infra/internal/config"
)

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	app := app.New(
		cfg.Server.Host,
		cfg.Server.Port,
		cfg.Server.ShutdownTimeout,
	)

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
