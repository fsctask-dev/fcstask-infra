package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jobrunner/internal/config"
)


func setupTesterEnv(t *testing.T) (refDir, repoDir, tempDir string) {
	t.Helper()
	refDir = t.TempDir()
	repoDir = t.TempDir()
	tempDir = t.TempDir()

	taskRefDir := filepath.Join(refDir, "tasks", "task1")
	if err := os.MkdirAll(taskRefDir, 0o755); err != nil {
		t.Fatalf("mkdir refDir/tasks/task1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(taskRefDir, ".task.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write .task.yml: %v", err)
	}

	taskRepoDir := filepath.Join(repoDir, "tasks", "task1")
	if err := os.MkdirAll(taskRepoDir, 0o755); err != nil {
		t.Fatalf("mkdir repoDir/tasks/task1: %v", err)
	}

	return refDir, repoDir, tempDir
}


func simpleCourseConfig() *config.CourseConfig {
	far := 100 * 365 * 24 * time.Hour
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	return &config.CourseConfig{
		Version: 1,
		Settings: config.CourseSettingsConfig{
			CourseName:    "test",
			GitlabBaseURL: "https://gitlab.com",
			PublicRepo:    "public",
			StudentsGroup: "students",
		},
		Deadlines: config.CourseDeadlinesConfig{
			Timezone:  "UTC",
			Deadlines: config.DeadlineHard,
			Schedule: []config.CourseGroupConfig{
				{
					Name:    "group1",
					Enabled: true,
					Start:   start,
					End:     config.DeadlineValue{Duration: &far},
					Steps:   make(map[float64]config.DeadlineValue),
					Tasks:   []config.CourseTaskConfig{{Task: "task1", Enabled: true, Score: 10}},
				},
			},
		},
	}
}

func buildTesterFull(t *testing.T, refDir, repoDir string, courseCfg *config.CourseConfig, globalPipeline, tasksPipeline []config.PipelineStageConfig) *Tester {
	t.Helper()

	checkerCfg := &config.CheckerConfig{
		Version: 1,
		Testing: &config.CheckerTestingConfig{
			ChangesDetection: config.ChangesDetectionLastCommitChanges,
			GlobalPipeline:   globalPipeline,
			TasksPipeline:    tasksPipeline,
		},
	}
	checkerCfg.SetDefaults()

	course, err := NewCourse(courseCfg, repoDir, refDir, "main")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}

	tester, err := NewTester(course, checkerCfg, DefaultRegistry(), false, false)
	if err != nil {
		t.Fatalf("NewTester: %v", err)
	}
	return tester
}

func buildTester(t *testing.T, refDir, repoDir string, tasksPipeline []config.PipelineStageConfig) *Tester {
	t.Helper()
	return buildTesterFull(t, refDir, repoDir, simpleCourseConfig(), nil, tasksPipeline)
}

const taskOrigin = "${{ .global.repo_dir }}/tasks/task1"

func TestTester_RunScript_Passes(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)
	pipeline := []config.PipelineStageConfig{{
		Name: "run",
		Run:  "run_script",
		Args: map[string]any{
			"origin": taskOrigin,
			"script": "echo ok",
		},
	}}
	tester := buildTester(t, refDir, repoDir, pipeline)
	task := tester.course.PotentialTasks["task1"]

	if err := tester.Run(tempDir, []FileSystemTask{task}, false, nil); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestTester_RunScript_Fails(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)
	pipeline := []config.PipelineStageConfig{{
		Name: "run",
		Run:  "run_script",
		Args: map[string]any{
			"origin": taskOrigin,
			"script": "exit 1",
		},
	}}
	tester := buildTester(t, refDir, repoDir, pipeline)
	task := tester.course.PotentialTasks["task1"]

	if err := tester.Run(tempDir, []FileSystemTask{task}, false, nil); err == nil {
		t.Fatal("expected failure, got nil error")
	}
}

func TestTester_RunScript_Timeout(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)
	timeout := 0.1
	pipeline := []config.PipelineStageConfig{{
		Name: "run",
		Run:  "run_script",
		Args: map[string]any{
			"origin":  taskOrigin,
			"script":  "sleep 10",
			"timeout": timeout,
		},
	}}
	tester := buildTester(t, refDir, repoDir, pipeline)
	task := tester.course.PotentialTasks["task1"]

	if err := tester.Run(tempDir, []FileSystemTask{task}, false, nil); err == nil {
		t.Fatal("expected timeout failure, got nil error")
	}
}

func TestTester_RunTests_AllPass(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)

	const xml = `<testsuite tests="3" failures="0" errors="0">` +
		`<testcase name="t1"/><testcase name="t2"/><testcase name="t3"/>` +
		`</testsuite>`
	reportPath := filepath.Join(repoDir, "tasks", "task1", "report.xml")
	if err := os.WriteFile(reportPath, []byte(xml), 0o644); err != nil {
		t.Fatalf("write report.xml: %v", err)
	}

	pipeline := []config.PipelineStageConfig{{
		Name: "test",
		Run:  "run_tests",
		Args: map[string]any{
			"origin":      taskOrigin,
			"script":      "true",
			"report_file": "report.xml",
		},
	}}
	tester := buildTester(t, refDir, repoDir, pipeline)
	task := tester.course.PotentialTasks["task1"]

	if err := tester.Run(tempDir, []FileSystemTask{task}, false, nil); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}

func TestTester_RunTests_AllFail(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)

	const xml = `<testsuite tests="3" failures="2" errors="0">` +
		`<testcase name="t1"/>` +
		`<testcase name="t2"><failure message="wrong"/></testcase>` +
		`<testcase name="t3"><failure message="wrong"/></testcase>` +
		`</testsuite>`
	reportPath := filepath.Join(repoDir, "tasks", "task1", "report.xml")
	if err := os.WriteFile(reportPath, []byte(xml), 0o644); err != nil {
		t.Fatalf("write report.xml: %v", err)
	}

	pipeline := []config.PipelineStageConfig{{
		Name: "test",
		Run:  "run_tests",
		Args: map[string]any{
			"origin":      taskOrigin,
			"script":      "true",
			"report_file": "report.xml",
		},
	}}
	tester := buildTester(t, refDir, repoDir, pipeline)
	task := tester.course.PotentialTasks["task1"]

	if err := tester.Run(tempDir, []FileSystemTask{task}, false, nil); err == nil {
		t.Fatal("expected failure from failing tests, got nil error")
	}
}

func TestTester_RunTests_PartialScore(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)

	const xml = `<testsuite tests="4" failures="2" errors="0">` +
		`<testcase name="t1"/><testcase name="t2"/>` +
		`<testcase name="t3"><failure message="wrong"/></testcase>` +
		`<testcase name="t4"><failure message="wrong"/></testcase>` +
		`</testsuite>`
	reportPath := filepath.Join(repoDir, "tasks", "task1", "report.xml")
	if err := os.WriteFile(reportPath, []byte(xml), 0o644); err != nil {
		t.Fatalf("write report.xml: %v", err)
	}

	pipeline := []config.PipelineStageConfig{{
		Name: "test",
		Run:  "run_tests",
		Args: map[string]any{
			"origin":           taskOrigin,
			"script":           "true",
			"report_file":      "report.xml",
			"partially_scored": true,
		},
	}}
	tester := buildTester(t, refDir, repoDir, pipeline)
	task := tester.course.PotentialTasks["task1"]

	if err := tester.Run(tempDir, []FileSystemTask{task}, false, nil); err != nil {
		t.Fatalf("expected pass with partial score, got: %v", err)
	}
}


func TestTester_GlobalPipeline_FailStopsExecution(t *testing.T) {
	refDir, repoDir, tempDir := setupTesterEnv(t)

	globalPipeline := []config.PipelineStageConfig{{
		Name: "global-fail",
		Run:  "run_script",
		Args: map[string]any{
			"origin": "${{ .global.ref_dir }}",
			"script": "exit 1",
		},
	}}
	tasksPipeline := []config.PipelineStageConfig{{
		Name: "task-run",
		Run:  "run_script",
		Args: map[string]any{
			"origin": taskOrigin,
			"script": "echo should_not_run",
		},
	}}
	tester := buildTesterFull(t, refDir, repoDir, simpleCourseConfig(), globalPipeline, tasksPipeline)
	task := tester.course.PotentialTasks["task1"]

	err := tester.Run(tempDir, []FileSystemTask{task}, false, nil)
	if err == nil {
		t.Fatal("expected error from failing global pipeline, got nil")
	}
	if !strings.Contains(err.Error(), "global pipeline failed") {
		t.Errorf("expected 'global pipeline failed' in error, got: %v", err)
	}
}


func TestTester_Deadline(t *testing.T) {
	refDir, repoDir, _ := setupTesterEnv(t)

	deadline := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	courseCfg := &config.CourseConfig{
		Version: 1,
		Settings: config.CourseSettingsConfig{
			CourseName:    "test",
			GitlabBaseURL: "https://gitlab.com",
			PublicRepo:    "public",
			StudentsGroup: "students",
		},
		Deadlines: config.CourseDeadlinesConfig{
			Timezone:  "UTC",
			Deadlines: config.DeadlineHard,
			Schedule: []config.CourseGroupConfig{
				{
					Name:    "group1",
					Enabled: true,
					Start:   start,
					End:     config.DeadlineValue{Time: &deadline},
					Steps:   make(map[float64]config.DeadlineValue),
					Tasks:   []config.CourseTaskConfig{{Task: "task1", Enabled: true, Score: 10}},
				},
			},
		},
	}
	tester := buildTesterFull(t, refDir, repoDir, courseCfg, nil, nil)

	before := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := tester.GetTaskScorePercent("task1", before); got != 1.0 {
		t.Errorf("before deadline: got %v, want 1.0", got)
	}

	after := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := tester.GetTaskScorePercent("task1", after); got != 0.0 {
		t.Errorf("after deadline: got %v, want 0.0", got)
	}

	if got := tester.GetTaskScorePercent("nonexistent_task", before); got != 1.0 {
		t.Errorf("unknown task: got %v, want 1.0 (no deadline constraint)", got)
	}
}
