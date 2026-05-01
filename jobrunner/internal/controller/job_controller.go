package controller

import (
	"jobrunner/internal/service"
	"net/http"

	"github.com/labstack/echo/v4"
)

type JobController struct {
	jobService service.JobService
}

func NewJobController(jobService service.JobService) *JobController {
	return &JobController{
		jobService: jobService,
	}
}

func (c *JobController) Ping(ctx echo.Context) error {
	response := c.jobService.Ping()
	return ctx.JSON(http.StatusOK, response)
}