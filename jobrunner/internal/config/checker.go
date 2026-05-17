package config

import (
	"fmt"
	"strings"
)

type TParamType interface{}
type TTemplateType interface{}

type CheckerStructureConfig struct {
	IgnorePatterns  []string `yaml:"ignore_patterns,omitempty"`
	PrivatePatterns []string `yaml:"private_patterns,omitempty"`
	PublicPatterns  []string `yaml:"public_patterns,omitempty"`
}

type CheckerParametersConfig struct {
	Parameters map[string]TParamType `yaml:",inline"`
}

type CheckerExportTemplateType string

type CheckerExportConfig struct {
	Destination   string                    `yaml:"destination"`
	DefaultBranch string                    `yaml:"default_branch"`
	CommitMessage string                    `yaml:"commit_message"`
	Templates     CheckerExportTemplateType `yaml:"templates"`
}

const (
	TemplateTypeSearch         CheckerExportTemplateType = "search"
	TemplateTypeCreate         CheckerExportTemplateType = "create"
	TemplateTypeSearchOrCreate CheckerExportTemplateType = "search_or_create"
)

type PipelineStageConfig struct {
	Name           string                 `yaml:"name"`
	Run            string                 `yaml:"run"`
	Args           map[string]interface{} `yaml:"args,omitempty"`
	RunIf          interface{}            `yaml:"run_if,omitempty"`
	Fail           FailType               `yaml:"fail"`
	RegisterOutput *string                `yaml:"register_output,omitempty"`
}

type FailType string

const (
	FailFast     FailType = "fast"
	FailAfterAll FailType = "after_all"
	FailNever    FailType = "never"
)

type CheckerTestingConfig struct {
	ChangesDetection ChangesDetectionType  `yaml:"changes_detection"`
	SearchPlugins    []string              `yaml:"search_plugins,omitempty"`
	GlobalPipeline   []PipelineStageConfig `yaml:"global_pipeline,omitempty"`
	TasksPipeline    []PipelineStageConfig `yaml:"tasks_pipeline,omitempty"`
	ReportPipeline   []PipelineStageConfig `yaml:"report_pipeline,omitempty"`
}

type ChangesDetectionType string

const (
	ChangesDetectionBranchName        ChangesDetectionType = "branch_name"
	ChangesDetectionCommitMessage     ChangesDetectionType = "commit_message"
	ChangesDetectionLastCommitChanges ChangesDetectionType = "last_commit_changes"
	ChangesDetectionFilesChanged      ChangesDetectionType = "files_changed"
)

type CheckerConfig struct {
	Version           int                      `yaml:"version"`
	DefaultParameters *CheckerParametersConfig `yaml:"default_parameters,omitempty"`
	Structure         *CheckerStructureConfig  `yaml:"structure"`
	Export            *CheckerExportConfig     `yaml:"export"`
	Testing           *CheckerTestingConfig    `yaml:"testing"`
}

type CheckerSubConfig struct {
	Version        int                      `yaml:"version"`
	Structure      *CheckerStructureConfig  `yaml:"structure,omitempty"`
	Parameters     *CheckerParametersConfig `yaml:"parameters,omitempty"`
	TaskPipeline   []PipelineStageConfig    `yaml:"task_pipeline,omitempty"`
	ReportPipeline []PipelineStageConfig    `yaml:"report_pipeline,omitempty"`
}

type CheckerConfigBuilder struct {
	config *CheckerConfig
}

func (c *CheckerConfig) SetDefaults() {
	if c.Version == 0 {
		c.Version = 1
	}

	if c.DefaultParameters == nil {
		c.DefaultParameters = &CheckerParametersConfig{
			Parameters: make(map[string]TParamType),
		}
	}

	if c.Export != nil {
		if c.Export.DefaultBranch == "" {
			c.Export.DefaultBranch = "main"
		}
		if c.Export.CommitMessage == "" {
			c.Export.CommitMessage = "chore(auto): export new tasks"
		}
		if c.Export.Templates == "" {
			c.Export.Templates = TemplateTypeSearch
		}
	}

	if c.Testing != nil {
		if c.Testing.ChangesDetection == "" {
			c.Testing.ChangesDetection = ChangesDetectionLastCommitChanges
		}
		if c.Testing.SearchPlugins == nil {
			c.Testing.SearchPlugins = []string{}
		}
		if c.Testing.GlobalPipeline == nil {
			c.Testing.GlobalPipeline = []PipelineStageConfig{}
		}
		if c.Testing.TasksPipeline == nil {
			c.Testing.TasksPipeline = []PipelineStageConfig{}
		}
		if c.Testing.ReportPipeline == nil {
			c.Testing.ReportPipeline = []PipelineStageConfig{}
		}
	}
}

func (c *CheckerConfig) Validate() error {
	if err := c.validateVersion(); err != nil {
		return err
	}

	if c.Structure == nil {
		return fmt.Errorf("structure is required")
	}

	if err := c.Structure.Validate(); err != nil {
		return fmt.Errorf("structure validation failed: %w", err)
	}

	if c.Export == nil {
		return fmt.Errorf("export is required")
	}

	if err := c.Export.Validate(); err != nil {
		return fmt.Errorf("export validation failed: %w", err)
	}

	if c.Testing == nil {
		return fmt.Errorf("testing is required")
	}

	if err := c.Testing.Validate(); err != nil {
		return fmt.Errorf("testing validation failed: %w", err)
	}

	return nil
}

func (c *CheckerConfig) validateVersion() error {
	if c.Version != 1 {
		return fmt.Errorf("only version 1 is supported for CheckerConfig, got %d", c.Version)
	}
	return nil
}

func (s *CheckerStructureConfig) Validate() error {
	allPatterns := append(s.IgnorePatterns, s.PrivatePatterns...)
	allPatterns = append(allPatterns, s.PublicPatterns...)

	for _, pattern := range allPatterns {
		if strings.Contains(pattern, "**") {
			return fmt.Errorf("pattern '%s' contains '**' which is not allowed", pattern)
		}
	}

	return nil
}

func (e *CheckerExportConfig) Validate() error {
	if e.Destination == "" {
		return fmt.Errorf("destination is required")
	}

	if !strings.HasPrefix(e.Destination, "http://") &&
		!strings.HasPrefix(e.Destination, "https://") &&
		!strings.HasPrefix(e.Destination, "git@") &&
		!strings.HasPrefix(e.Destination, "ssh://") {
		return fmt.Errorf("destination must be a valid URL (http://, https://, git@, ssh://)")
	}

	switch e.Templates {
	case TemplateTypeSearch, TemplateTypeCreate, TemplateTypeSearchOrCreate:
		return nil
	default:
		return fmt.Errorf("invalid template type: %s, must be one of: search, create, search_or_create", e.Templates)
	}
}

func (t *CheckerTestingConfig) Validate() error {
	switch t.ChangesDetection {
	case ChangesDetectionBranchName, ChangesDetectionCommitMessage,
		ChangesDetectionLastCommitChanges, ChangesDetectionFilesChanged:
	default:
		return fmt.Errorf("invalid changes_detection type: %s", t.ChangesDetection)
	}

	if err := validatePipeline(t.GlobalPipeline); err != nil {
		return fmt.Errorf("global_pipeline validation failed: %w", err)
	}

	if err := validatePipeline(t.TasksPipeline); err != nil {
		return fmt.Errorf("tasks_pipeline validation failed: %w", err)
	}

	if err := validatePipeline(t.ReportPipeline); err != nil {
		return fmt.Errorf("report_pipeline validation failed: %w", err)
	}

	return nil
}

func validatePipeline(pipeline []PipelineStageConfig) error {
	stageNames := make(map[string]bool)

	for i, stage := range pipeline {
		if stage.Name == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}

		if stageNames[stage.Name] {
			return fmt.Errorf("duplicate stage name: %s", stage.Name)
		}
		stageNames[stage.Name] = true

		if stage.Run == "" {
			return fmt.Errorf("stage %s: run command is required", stage.Name)
		}

		switch stage.Fail {
		case FailFast, FailAfterAll, FailNever:
		case "":
			stage.Fail = FailFast
		default:
			return fmt.Errorf("stage %s: invalid fail type: %s", stage.Name, stage.Fail)
		}
	}

	return nil
}

func (s *CheckerSubConfig) SetDefaults() {
	if s.Version == 0 {
		s.Version = 1
	}

	if s.TaskPipeline == nil {
		s.TaskPipeline = []PipelineStageConfig{}
	}

	if s.ReportPipeline == nil {
		s.ReportPipeline = []PipelineStageConfig{}
	}
}

func (s *CheckerSubConfig) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("only version 1 is supported for CheckerSubConfig, got %d", s.Version)
	}

	if s.Structure != nil {
		if err := s.Structure.Validate(); err != nil {
			return fmt.Errorf("structure validation failed: %w", err)
		}
	}

	if err := validatePipeline(s.TaskPipeline); err != nil {
		return fmt.Errorf("task_pipeline validation failed: %w", err)
	}

	if err := validatePipeline(s.ReportPipeline); err != nil {
		return fmt.Errorf("report_pipeline validation failed: %w", err)
	}

	return nil
}
