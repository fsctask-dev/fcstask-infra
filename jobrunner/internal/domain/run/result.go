package run

import (
    "encoding/json"
    "time"
)

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

func (s RunStatus) String() string {
    return string(s)
}

func (s RunStatus) IsFinal() bool {
    switch s {
    case StatusPassed, StatusFailed, StatusBuildError, StatusTimeout, StatusInfraError, StatusCancelled:
        return true
    }
    return false
}

type RunResult struct {
    RunID     string `json:"run_id" db:"run_id"`
    StudentID string `json:"student_id" db:"student_id"`
    TaskID    string `json:"task_id" db:"task_id"`
    
    Status RunStatus `json:"status" db:"status"`
    Score  float64   `json:"score" db:"score"`
    
    Stdout string `json:"stdout" db:"stdout"`
    Stderr string `json:"stderr" db:"stderr"`
    
    ExecutionTimeSeconds float64 `json:"execution_time_seconds" db:"execution_time_seconds"`
    
    Details RunDetails `json:"details" db:"details_json"`
    
    Stages []StageResult `json:"stages,omitempty" db:"stages_json"`
    
    Metrics *RunMetrics `json:"metrics,omitempty" db:"metrics_json"`
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

type RunMetrics struct {
    MaxMemoryMB    int64   `json:"max_memory_mb,omitempty"`
    MaxCPUPercent  float64 `json:"max_cpu_percent,omitempty"`
    NetworkIOBytes int64   `json:"network_io_bytes,omitempty"`
    DiskReadBytes  int64   `json:"disk_read_bytes,omitempty"`
    DiskWriteBytes int64   `json:"disk_write_bytes,omitempty"`
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
        case "passed":
        case "skipped":
        default:
            allPassed = false
        }
    }
    
    if allPassed {
        r.Status = StatusPassed
    } else if anyFailed && r.Status != StatusBuildError {
        r.Status = StatusFailed
    }
    
    if r.Score == 0 && len(r.Stages) > 0 {
        totalScore := 0.0
        for _, stage := range r.Stages {
            totalScore += stage.ScoreContribution
        }
        r.Score = totalScore
    }
    
    const maxLogSize = 10 * 1024 * 1024
    if len(r.Stdout) > maxLogSize {
        r.Stdout = r.Stdout[:maxLogSize] + "\n... [truncated]"
    }
    if len(r.Stderr) > maxLogSize {
        r.Stderr = r.Stderr[:maxLogSize] + "\n... [truncated]"
    }
}

func (r *RunResult) ScoreValue() float64 {
    if r.Status != StatusPassed && r.Score > 0 {
        return 0
    }
    return r.Score
}

func (r *RunResult) ElapsedSeconds() float64 {
    if r.Details.StartedAt.IsZero() || r.Details.FinishedAt.IsZero() {
        return r.ExecutionTimeSeconds
    }
    return r.Details.FinishedAt.Sub(r.Details.StartedAt).Seconds()
}

func (r *RunResult) ToJSON() ([]byte, error) {
    return json.Marshal(r)
}

func (r *RunResult) FromJSON(data []byte) error {
    return json.Unmarshal(data, r)
}

func (r *RunResult) AddStage(stage StageResult) {
    r.Stages = append(r.Stages, stage)
    r.Score += stage.ScoreContribution
}