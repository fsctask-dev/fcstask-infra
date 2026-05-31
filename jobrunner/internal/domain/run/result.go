package run

import "time"

type RunStatus string

const (
	StatusQueued     RunStatus = "queued"
	StatusRunning    RunStatus = "running"
	StatusPassed     RunStatus = "passed"
	StatusFailed     RunStatus = "failed"
	StatusBuildError RunStatus = "build_error"
	StatusTimeout    RunStatus = "timeout"
	StatusInfraError RunStatus = "infra_error"
	StatusCancelled  RunStatus = "cancelled"
)

func (s RunStatus) String() string { return string(s) }

func (s RunStatus) IsFinal() bool {
	switch s {
	case StatusPassed, StatusFailed, StatusBuildError, StatusTimeout, StatusInfraError, StatusCancelled:
		return true
	}
	return false
}

type RunResult struct {
	RunID     string    `json:"run_id"`
	StudentID string    `json:"student_id"`
	TaskID    string    `json:"task_id"`
	Status    RunStatus `json:"status"`
	Score     float64   `json:"score"`
	Stdout    string    `json:"stdout"`
	Stderr    string    `json:"stderr"`
	Details   RunDetails    `json:"details"`
	Stages    []StageResult `json:"stages,omitempty"`
}

type RunDetails struct {
	TaskName    string    `json:"task_name"`
	TaskDigest  string    `json:"task_digest"`
	SubmittedAt time.Time `json:"submitted_at"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	WorkerID    string    `json:"worker_id,omitempty"`
	Attempt     int       `json:"attempt"`
}

type StageResult struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	DurationMs        int64   `json:"duration_ms"`
	Output            string  `json:"output,omitempty"`
	ScoreContribution float64 `json:"score_contribution,omitempty"`
	Error             string  `json:"error,omitempty"`
}

func (r *RunResult) Finalize() {
	if r.Status.IsFinal() {
		return
	}
	if len(r.Stages) == 0 {
		if r.Status == "" {
			r.Status = StatusFailed
		}
		return
	}

	allPassed := true
	anyFailed := false
	for _, stage := range r.Stages {
		switch stage.Status {
		case "failed":
			anyFailed = true
			allPassed = false
		case "passed", "skipped":
		default:
			allPassed = false
		}
	}

	if allPassed {
		r.Status = StatusPassed
	} else if anyFailed && r.Status != StatusBuildError {
		r.Status = StatusFailed
	}

	if r.Score == 0 {
		for _, stage := range r.Stages {
			r.Score += stage.ScoreContribution
		}
	}

	const maxLogSize = 10 * 1024 * 1024
	if len(r.Stdout) > maxLogSize {
		r.Stdout = r.Stdout[:maxLogSize] + "\n... [truncated]"
	}
	if len(r.Stderr) > maxLogSize {
		r.Stderr = r.Stderr[:maxLogSize] + "\n... [truncated]"
	}
}

func (r *RunResult) ElapsedSeconds() float64 {
	if r.Details.StartedAt.IsZero() || r.Details.FinishedAt.IsZero() {
		return 0
	}
	return r.Details.FinishedAt.Sub(r.Details.StartedAt).Seconds()
}

func (r *RunResult) AddStage(stage StageResult) {
	r.Stages = append(r.Stages, stage)
	r.Score += stage.ScoreContribution
}
