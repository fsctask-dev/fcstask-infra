package run

import (
	"fmt"
	"time"
)

type RunRequest struct {
    RunID     string `json:"run_id" validate:"required"`
    StudentID string `json:"student_id" validate:"required"`
    TaskID    string `json:"task_id" validate:"required"`
    
    Submission SubmissionRef `json:"submission" validate:"required"`
    
    TaskDigest string `json:"task_digest"`
    
    SubmittedAt time.Time   `json:"submitted_at"`
    RetryPolicy RetryPolicy `json:"retry_policy"`
    
    TimeoutOverride   int               `json:"timeout_override"`
    EnvironmentOverride map[string]string `json:"environment_override"`
}

type SubmissionRef struct {
    Type   SubmissionType `json:"type"`
    URL    string         `json:"url"`
    GitRepo string        `json:"git_repo"`
    GitRef  string        `json:"git_ref"`
    Inline  []byte        `json:"inline"`
}

type SubmissionType string

const (
    SubmissionURL    SubmissionType = "url"
    SubmissionGit    SubmissionType = "git"
    SubmissionInline SubmissionType = "inline"
)

type RetryPolicy struct {
    MaxAttempts int `json:"max_attempts"`
    DelaySecs   int `json:"delay_secs"`
}

func (r *RunRequest) Validate() error {
    if r.RunID == "" {
        return fmt.Errorf("run_id is required")
    }
    if r.StudentID == "" {
        return fmt.Errorf("student_id is required")
    }
    if r.TaskID == "" {
        return fmt.Errorf("task_id is required")
    }
    if r.Submission.Type == "" {
        return fmt.Errorf("submission.type is required")
    }
    
    switch r.Submission.Type {
    case SubmissionURL:
        if r.Submission.URL == "" {
            return fmt.Errorf("submission.url is required for type=url")
        }
    case SubmissionGit:
        if r.Submission.GitRepo == "" {
            return fmt.Errorf("submission.git_repo is required for type=git")
        }
    case SubmissionInline:
        if len(r.Submission.Inline) == 0 {
            return fmt.Errorf("submission.inline is required for type=inline")
        }
    default:
        return fmt.Errorf("invalid submission type: %s", r.Submission.Type)
    }
    
    if r.RetryPolicy.MaxAttempts < 0 {
        r.RetryPolicy.MaxAttempts = 0
    }
    if r.RetryPolicy.DelaySecs < 0 {
        r.RetryPolicy.DelaySecs = 0
    }
    
    return nil
}

func (r *RunRequest) WithDefaults() *RunRequest {
    if r.SubmittedAt.IsZero() {
        r.SubmittedAt = time.Now()
    }
    if r.RetryPolicy.MaxAttempts == 0 {
        r.RetryPolicy.MaxAttempts = 1
    }
    if r.RetryPolicy.DelaySecs == 0 {
        r.RetryPolicy.DelaySecs = 5
    }
    return r
}