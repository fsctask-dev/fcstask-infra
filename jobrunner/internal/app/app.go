package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"jobrunner/internal/config"
	"jobrunner/internal/controller"
	"jobrunner/internal/repo"
	"jobrunner/internal/repo/postgres"
	"jobrunner/internal/server"
	"jobrunner/internal/service"
)

type App struct {
	cfg        *config.Config
	db         *postgres.Client
	jobRepo    repo.JobRepository
	jobService service.JobService
	controller *controller.JobController
	server     *server.Server
}

func New(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	db, err := postgres.NewClient(
		context.Background(),
		&postgres.Config{
			Host:        cfg.Database.Host,
			Port:        cfg.Database.Port,
			User:        cfg.Database.User,
			Password:    cfg.Database.Password,
			DBName:      cfg.Database.DBName,
			MaxConns:    int32(cfg.Database.MaxConns),
			ConnTimeout: cfg.Database.ConnTimeout,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	jobRepo := repo.NewJobRepository(*db)
	jobService := service.NewJobService(jobRepo)
	controller := controller.NewJobController(jobService)
	server := server.New(cfg, controller)

	return &App{
		cfg:        cfg,
		db:         db,
		jobRepo:    jobRepo,
		jobService: jobService,
		controller: controller,
		server:     server,
	}, nil
}

func (a *App) Run() error {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("job runner started")

	go func() {
		if err := a.server.Start(); err != nil {
			if err != http.ErrServerClosed {
				fmt.Printf("server error: %v\n", err)
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("shutting down...")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), a.cfg.Server.ShutdownTimeout)
	defer stopCancel()

	if err := a.server.Shutdown(stopCtx); err != nil {
		fmt.Printf("server shutdown error: %v\n", err)
	}

	fmt.Println("shutdown complete")
	return nil
}
