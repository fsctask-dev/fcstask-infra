package repo

import (
	"context"
	"jobrunner/internal/repo/postgres"
)

type JobRepository interface {
	Ping(ctx context.Context) (string, error)
}

type jobRepository struct {
	client postgres.Client
}

func NewJobRepository(client postgres.Client) JobRepository {
	return &jobRepository{client: client}
}

func (r *jobRepository) Ping(ctx context.Context) (string, error) {
	err := r.client.Ping(ctx)
	if err != nil {
		return "down", err
	}

	return "up", nil
}
