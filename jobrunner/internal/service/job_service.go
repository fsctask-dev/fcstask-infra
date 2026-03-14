package service

import (
	"context"
	"fmt"
	"jobrunner/internal/repo"
)

type JobService interface {
	Ping() PingResponse
}

type jobService struct {
	jobRepo repo.JobRepository
}

type PingResponse struct {
	Message string
}

func NewJobService(jobRepo repo.JobRepository) JobService {
	return &jobService{
		jobRepo: jobRepo,
	}
}

func (s *jobService) Ping() PingResponse {
	ctx := context.Background()
	msg, _ := s.jobRepo.Ping(ctx)

	return PingResponse{
		Message: fmt.Sprintf("database %s", msg),
	}
}
