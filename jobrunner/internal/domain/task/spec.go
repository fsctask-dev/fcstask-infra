package task

import (
    "fmt"
    "regexp"
    "strings"
    "time"
    
    "github.com/go-playground/validator/v10"
)

type TaskSpec struct {
    Version int `yaml:"version" json:"version" validate:"required,eq=1"`
    
    TaskName string     `yaml:"task_name" json:"task_name" validate:"required,taskname"`
    Title    string     `yaml:"title" json:"title"`
    Language Language   `yaml:"language" json:"language" validate:"required,oneof=python java go cpp rust"`
    Deadline *time.Time `yaml:"deadline" json:"deadline"`
    
    Constraints Constraints `yaml:"constraints" json:"constraints"`
    
    Entrypoint     string            `yaml:"entrypoint" json:"entrypoint" validate:"required"`
    TestFilesPaths []string          `yaml:"test_files_paths" json:"test_files_paths"`
    Environment    map[string]string `yaml:"environment" json:"environment"`
    Dependencies   []string          `yaml:"dependencies" json:"dependencies"`
    
    Stages []TestStage `yaml:"stages" json:"stages"`
    
    FailFast bool `yaml:"fail_fast" json:"fail_fast"`
    
    Reporting ReportingSpec `yaml:"reporting" json:"reporting"`
    
    Manytask *ManytaskMetadata `yaml:"manytask" json:"manytask"`
}

type Language string

const (
    Python     Language = "python"
    Java       Language = "java"
    Go         Language = "go"
    CPP        Language = "cpp"
    JavaScript Language = "javascript"
    Rust       Language = "rust"
)

type Constraints struct {
    TimeoutSeconds int     `yaml:"timeout_seconds" json:"timeout_seconds" validate:"min=1,max=300"`
    MemoryLimitMB  int     `yaml:"memory_limit_mb" json:"memory_limit_mb" validate:"min=64,max=4096"`
    CPULimitCores  float64 `yaml:"cpu_limit_cores" json:"cpu_limit_cores" validate:"min=0.1,max=8"`
}

type TestStage struct {
    ID          string            `yaml:"id" json:"id" validate:"required"`
    Name        string            `yaml:"name" json:"name"`
    Run         string            `yaml:"run" json:"run" validate:"required"`
    TimeoutSecs int               `yaml:"timeout_secs" json:"timeout_secs"`
    Weight      float64           `yaml:"weight" json:"weight" validate:"min=0,max=100"`
    DependsOn   []string          `yaml:"depends_on" json:"depends_on"`
    RunIf       string            `yaml:"run_if" json:"run_if"` // условие (passed/failed/skipped)
    Env         map[string]string `yaml:"env" json:"env"`
}

type ReportingSpec struct {
    ScoreWeight      float64 `yaml:"score_weight" json:"score_weight" validate:"min=0,max=1000"`
    PassingThreshold float64 `yaml:"passing_threshold" json:"passing_threshold" validate:"min=0,max=100"`
}

type ManytaskMetadata struct {
    CourseID string `yaml:"course_id" json:"course_id"`
    GroupID  string `yaml:"group_id" json:"group_id"`
}

var validate *validator.Validate

func init() {
    validate = validator.New()
    
    validate.RegisterValidation("taskname", func(fl validator.FieldLevel) bool {
        return regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(fl.Field().String())
    })
}

func (t *TaskSpec) Validate() error {
    if err := validate.Struct(t); err != nil {
        return fmt.Errorf("task validation failed: %w", err)
    }
    
    if t.Constraints.TimeoutSeconds == 0 {
        t.Constraints.TimeoutSeconds = 30
    }
    if t.Constraints.MemoryLimitMB == 0 {
        t.Constraints.MemoryLimitMB = 512
    }
    if t.Constraints.CPULimitCores == 0 {
        t.Constraints.CPULimitCores = 1.0
    }
    if t.Reporting.ScoreWeight == 0 {
        t.Reporting.ScoreWeight = 100
    }
    if t.Reporting.PassingThreshold == 0 {
        t.Reporting.PassingThreshold = 60
    }
    
    seen := make(map[string]bool)
    for _, stage := range t.Stages {
        if seen[stage.ID] {
            return fmt.Errorf("duplicate stage id: %s", stage.ID)
        }
        seen[stage.ID] = true
    }
    
    return nil
}

func (t *TaskSpec) StageByID(id string) (TestStage, bool) {
    for _, stage := range t.Stages {
        if stage.ID == id {
            return stage, true
        }
    }
    return TestStage{}, false
}

func (t *TaskSpec) GetEffectiveEntrypoint(workspacePath string) string {
    entrypoint := t.Entrypoint
    entrypoint = strings.ReplaceAll(entrypoint, "$WORKSPACE", workspacePath)
    for k, v := range t.Environment {
        entrypoint = strings.ReplaceAll(entrypoint, "$"+k, v)
    }
    return entrypoint
}

func (t *TaskSpec) MaxScore() float64 {
    if len(t.Stages) > 0 {
        total := 0.0
        for _, stage := range t.Stages {
            total += stage.Weight
        }
        return total
    }
    return t.Reporting.ScoreWeight
}