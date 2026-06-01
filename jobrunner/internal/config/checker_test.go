package config

import (
	"testing"

	"jobrunner/internal/domain/errors"
)

func validCheckerConfig() CheckerConfig {
	return CheckerConfig{
		Version: 1,
		Structure: &CheckerStructureConfig{
			PublicPatterns: []string{"*"},
		},
		Export: &CheckerExportConfig{
			Destination: "https://gitlab.com/org/repo",
			Templates:   TemplateTypeSearch,
		},
		Testing: &CheckerTestingConfig{
			ChangesDetection: ChangesDetectionBranchName,
		},
	}
}

func stage(name, run string, fail FailType) PipelineStageConfig {
	return PipelineStageConfig{Name: name, Run: run, Fail: fail}
}

func TestCheckerConfigSetDefaults_Version(t *testing.T) {
	c := CheckerConfig{}
	c.SetDefaults()
	if c.Version != 1 {
		t.Errorf("expected version 1, got %d", c.Version)
	}
}

func TestCheckerConfigSetDefaults_DoesNotOverwriteVersion(t *testing.T) {
	c := CheckerConfig{Version: 1}
	c.SetDefaults()
	if c.Version != 1 {
		t.Errorf("expected version 1, got %d", c.Version)
	}
}

func TestCheckerConfigSetDefaults_DefaultParameters(t *testing.T) {
	c := CheckerConfig{}
	c.SetDefaults()
	if c.DefaultParameters == nil {
		t.Fatal("expected DefaultParameters to be set")
	}
	if c.DefaultParameters.Parameters == nil {
		t.Error("expected Parameters map to be initialized")
	}
}

func TestCheckerConfigSetDefaults_ExportDefaults(t *testing.T) {
	c := CheckerConfig{
		Export: &CheckerExportConfig{Destination: "https://example.com"},
	}
	c.SetDefaults()
	if c.Export.DefaultBranch != "main" {
		t.Errorf("expected DefaultBranch=main, got %q", c.Export.DefaultBranch)
	}
	if c.Export.CommitMessage != "chore(auto): export new tasks" {
		t.Errorf("unexpected CommitMessage: %q", c.Export.CommitMessage)
	}
	if c.Export.Templates != TemplateTypeSearch {
		t.Errorf("expected templates=search, got %q", c.Export.Templates)
	}
}

func TestCheckerConfigSetDefaults_ExportDoesNotOverwrite(t *testing.T) {
	c := CheckerConfig{
		Export: &CheckerExportConfig{
			Destination:   "https://example.com",
			DefaultBranch: "develop",
			CommitMessage: "custom message",
			Templates:     TemplateTypeCreate,
		},
	}
	c.SetDefaults()
	if c.Export.DefaultBranch != "develop" {
		t.Errorf("expected develop, got %q", c.Export.DefaultBranch)
	}
	if c.Export.CommitMessage != "custom message" {
		t.Errorf("expected custom message, got %q", c.Export.CommitMessage)
	}
	if c.Export.Templates != TemplateTypeCreate {
		t.Errorf("expected create, got %q", c.Export.Templates)
	}
}

func TestCheckerConfigSetDefaults_NilExportSkipped(t *testing.T) {
	c := CheckerConfig{}
	c.SetDefaults()
	if c.Export != nil {
		t.Error("expected Export to remain nil when not set")
	}
}

func TestCheckerConfigSetDefaults_TestingDefaults(t *testing.T) {
	c := CheckerConfig{Testing: &CheckerTestingConfig{}}
	c.SetDefaults()
	if c.Testing.ChangesDetection != ChangesDetectionLastCommitChanges {
		t.Errorf("expected last_commit_changes, got %q", c.Testing.ChangesDetection)
	}
	if c.Testing.SearchPlugins == nil {
		t.Error("expected SearchPlugins to be initialized")
	}
	if c.Testing.GlobalPipeline == nil {
		t.Error("expected GlobalPipeline to be initialized")
	}
	if c.Testing.TasksPipeline == nil {
		t.Error("expected TasksPipeline to be initialized")
	}
	if c.Testing.ReportPipeline == nil {
		t.Error("expected ReportPipeline to be initialized")
	}
}

func TestCheckerConfigSetDefaults_NilTestingSkipped(t *testing.T) {
	c := CheckerConfig{}
	c.SetDefaults()
	if c.Testing != nil {
		t.Error("expected Testing to remain nil when not set")
	}
}


func TestCheckerConfigValidate_Valid(t *testing.T) {
	c := validCheckerConfig()
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckerConfigValidate_WrongVersion(t *testing.T) {
	c := validCheckerConfig()
	c.Version = 2
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestCheckerConfigValidate_MissingStructure(t *testing.T) {
	c := validCheckerConfig()
	c.Structure = nil
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for missing structure")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestCheckerConfigValidate_MissingExport(t *testing.T) {
	c := validCheckerConfig()
	c.Export = nil
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for missing export")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestCheckerConfigValidate_MissingTesting(t *testing.T) {
	c := validCheckerConfig()
	c.Testing = nil
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for missing testing")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}


func TestStructureValidate_Valid(t *testing.T) {
	s := CheckerStructureConfig{
		IgnorePatterns:  []string{".git", "*.pyc"},
		PublicPatterns:  []string{"*"},
		PrivatePatterns: []string{".*"},
	}
	if err := s.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStructureValidate_GlobStarInIgnore(t *testing.T) {
	s := CheckerStructureConfig{IgnorePatterns: []string{"**/*.pyc"}}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for ** in ignore pattern")
	}
	if !errors.IsBadStructure(err) {
		t.Errorf("expected BadStructure, got %T", err)
	}
}

func TestStructureValidate_GlobStarInPublic(t *testing.T) {
	s := CheckerStructureConfig{PublicPatterns: []string{"src/**"}}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for ** in public pattern")
	}
	if !errors.IsBadStructure(err) {
		t.Errorf("expected BadStructure, got %T", err)
	}
}

func TestStructureValidate_GlobStarInPrivate(t *testing.T) {
	s := CheckerStructureConfig{PrivatePatterns: []string{"**"}}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for ** in private pattern")
	}
	if !errors.IsBadStructure(err) {
		t.Errorf("expected BadStructure, got %T", err)
	}
}


func TestExportValidate_HTTPSDestination(t *testing.T) {
	e := CheckerExportConfig{Destination: "https://gitlab.com/org/repo", Templates: TemplateTypeSearch}
	if err := e.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExportValidate_GitAtDestination(t *testing.T) {
	e := CheckerExportConfig{Destination: "git@gitlab.com:org/repo.git", Templates: TemplateTypeCreate}
	if err := e.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExportValidate_SSHDestination(t *testing.T) {
	e := CheckerExportConfig{Destination: "ssh://git@gitlab.com/org/repo.git", Templates: TemplateTypeSearchOrCreate}
	if err := e.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExportValidate_EmptyDestination(t *testing.T) {
	e := CheckerExportConfig{Templates: TemplateTypeSearch}
	err := e.Validate()
	if err == nil {
		t.Fatal("expected error for empty destination")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestExportValidate_InvalidDestination(t *testing.T) {
	e := CheckerExportConfig{Destination: "ftp://example.com", Templates: TemplateTypeSearch}
	err := e.Validate()
	if err == nil {
		t.Fatal("expected error for invalid destination scheme")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestExportValidate_InvalidTemplateType(t *testing.T) {
	e := CheckerExportConfig{Destination: "https://gitlab.com/org/repo", Templates: "unknown"}
	err := e.Validate()
	if err == nil {
		t.Fatal("expected error for invalid template type")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestExportValidate_EmptyTemplateType(t *testing.T) {
	e := CheckerExportConfig{Destination: "https://gitlab.com/org/repo"}
	err := e.Validate()
	if err == nil {
		t.Fatal("expected error for empty template type")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}


func TestTestingValidate_AllChangesDetectionTypes(t *testing.T) {
	types := []ChangesDetectionType{
		ChangesDetectionBranchName,
		ChangesDetectionCommitMessage,
		ChangesDetectionLastCommitChanges,
		ChangesDetectionFilesChanged,
	}
	for _, cd := range types {
		cfg := CheckerTestingConfig{ChangesDetection: cd}
		if err := cfg.Validate(); err != nil {
			t.Errorf("changes_detection=%q: unexpected error: %v", cd, err)
		}
	}
}

func TestTestingValidate_InvalidChangesDetection(t *testing.T) {
	cfg := CheckerTestingConfig{ChangesDetection: "unknown"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid changes_detection")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestTestingValidate_WithValidPipelines(t *testing.T) {
	cfg := CheckerTestingConfig{
		ChangesDetection: ChangesDetectionBranchName,
		GlobalPipeline:   []PipelineStageConfig{stage("build", "make build", FailFast)},
		TasksPipeline:    []PipelineStageConfig{stage("test", "make test", FailAfterAll)},
		ReportPipeline:   []PipelineStageConfig{stage("report", "make report", FailNever)},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}


func TestValidatePipeline_Empty(t *testing.T) {
	if err := validatePipeline(nil); err != nil {
		t.Errorf("unexpected error for nil pipeline: %v", err)
	}
	if err := validatePipeline([]PipelineStageConfig{}); err != nil {
		t.Errorf("unexpected error for empty pipeline: %v", err)
	}
}

func TestValidatePipeline_Valid(t *testing.T) {
	p := []PipelineStageConfig{
		stage("stage1", "run_script", FailFast),
		stage("stage2", "run_tests", FailAfterAll),
	}
	if err := validatePipeline(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidatePipeline_MissingName(t *testing.T) {
	p := []PipelineStageConfig{{Run: "run_script"}}
	err := validatePipeline(p)
	if err == nil {
		t.Fatal("expected error for missing stage name")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestValidatePipeline_MissingRun(t *testing.T) {
	p := []PipelineStageConfig{{Name: "stage1"}}
	err := validatePipeline(p)
	if err == nil {
		t.Fatal("expected error for missing run command")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestValidatePipeline_DuplicateName(t *testing.T) {
	p := []PipelineStageConfig{
		stage("build", "make build", FailFast),
		stage("build", "make test", FailFast),
	}
	err := validatePipeline(p)
	if err == nil {
		t.Fatal("expected error for duplicate stage name")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestValidatePipeline_InvalidFailType(t *testing.T) {
	p := []PipelineStageConfig{{Name: "s", Run: "cmd", Fail: "unknown"}}
	err := validatePipeline(p)
	if err == nil {
		t.Fatal("expected error for invalid fail type")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestValidatePipeline_EmptyFailType(t *testing.T) {
	p := []PipelineStageConfig{{Name: "s", Run: "cmd"}}
	if err := validatePipeline(p); err != nil {
		t.Errorf("unexpected error for empty fail type (should default): %v", err)
	}
}


func TestCheckerSubConfigSetDefaults(t *testing.T) {
	s := CheckerSubConfig{}
	s.SetDefaults()
	if s.Version != 1 {
		t.Errorf("expected version 1, got %d", s.Version)
	}
	if s.TaskPipeline == nil {
		t.Error("expected TaskPipeline to be initialized")
	}
	if s.ReportPipeline == nil {
		t.Error("expected ReportPipeline to be initialized")
	}
}

func TestCheckerSubConfigValidate_Valid(t *testing.T) {
	s := CheckerSubConfig{
		Version:      1,
		TaskPipeline: []PipelineStageConfig{stage("test", "run_tests", FailFast)},
	}
	if err := s.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckerSubConfigValidate_WrongVersion(t *testing.T) {
	s := CheckerSubConfig{Version: 2}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestCheckerSubConfigValidate_InvalidStructure(t *testing.T) {
	s := CheckerSubConfig{
		Version:   1,
		Structure: &CheckerStructureConfig{PublicPatterns: []string{"**"}},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for invalid structure")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestCheckerSubConfigValidate_InvalidTaskPipeline(t *testing.T) {
	s := CheckerSubConfig{
		Version:      1,
		TaskPipeline: []PipelineStageConfig{{Name: "s1"}, {Name: "s1", Run: "cmd"}},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for invalid task_pipeline")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}

func TestCheckerSubConfigValidate_InvalidReportPipeline(t *testing.T) {
	s := CheckerSubConfig{
		Version:        1,
		ReportPipeline: []PipelineStageConfig{{Name: "report"}},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error: report pipeline stage missing run")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T", err)
	}
}
