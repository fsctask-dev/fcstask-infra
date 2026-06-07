package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"jobrunner/internal/config"
	domainerrors "jobrunner/internal/domain/errors"
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

// trackingPlugin считает сколько вызовов идёт одновременно.
// Используется для проверки параллелизма и соблюдения лимита конкурентности.
type trackingPlugin struct {
	mu      sync.Mutex
	current int
	maxSeen int
	total   int
	delay   time.Duration
}

func (p *trackingPlugin) Run(_ map[string]any, _ bool) (*PluginOutput, error) {
	p.mu.Lock()
	p.current++
	if p.current > p.maxSeen {
		p.maxSeen = p.current
	}
	p.total++
	p.mu.Unlock()

	time.Sleep(p.delay)

	p.mu.Lock()
	p.current--
	p.mu.Unlock()

	return &PluginOutput{Output: "ok", Percentage: 1.0}, nil
}

// setupNTaskEnv создаёт n задач в изолированных временных директориях.
func setupNTaskEnv(t *testing.T, n int) (refDir, repoDir, tempDir string) {
	t.Helper()
	refDir = t.TempDir()
	repoDir = t.TempDir()
	tempDir = t.TempDir()
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("task%d", i)
		if err := os.MkdirAll(filepath.Join(refDir, name), 0o755); err != nil {
			t.Fatalf("mkdir ref/%s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(refDir, name, ".task.yml"), []byte("version: 1\n"), 0o644); err != nil {
			t.Fatalf("write .task.yml: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(repoDir, name), 0o755); err != nil {
			t.Fatalf("mkdir repo/%s: %v", name, err)
		}
	}
	return
}

func nTaskCourseConfig(n int) *config.CourseConfig {
	far := 100 * 365 * 24 * time.Hour
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	tasks := make([]config.CourseTaskConfig, n)
	for i := 0; i < n; i++ {
		tasks[i] = config.CourseTaskConfig{Task: fmt.Sprintf("task%d", i+1), Enabled: true, Score: 10}
	}
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
					Tasks:   tasks,
				},
			},
		},
	}
}

func buildTesterWithRegistry(
	t *testing.T,
	refDir, repoDir string,
	courseCfg *config.CourseConfig,
	tasksPipeline []config.PipelineStageConfig,
	registry PluginRegistry,
) (*Tester, []FileSystemTask) {
	t.Helper()
	checkerCfg := &config.CheckerConfig{
		Version: 1,
		Testing: &config.CheckerTestingConfig{
			ChangesDetection: config.ChangesDetectionLastCommitChanges,
			TasksPipeline:    tasksPipeline,
		},
	}
	checkerCfg.SetDefaults()
	course, err := NewCourse(courseCfg, repoDir, refDir, "main")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	tester, err := NewTester(course, checkerCfg, registry, false, false)
	if err != nil {
		t.Fatalf("NewTester: %v", err)
	}
	enabled := true
	return tester, course.GetTasks(&enabled)
}

// TestTester_Parallel_RunsConcurrently проверяет, что задачи выполняются параллельно:
// при n задачах и maxConcurrency=n плагин должен вызываться минимум дважды одновременно.
func TestTester_Parallel_RunsConcurrently(t *testing.T) {
	const n = 4
	refDir, repoDir, tempDir := setupNTaskEnv(t, n)

	plugin := &trackingPlugin{delay: 100 * time.Millisecond}
	registry := DefaultRegistry()
	registry["track"] = plugin

	pipeline := []config.PipelineStageConfig{{Name: "run", Run: "track"}}
	tester, tasks := buildTesterWithRegistry(t, refDir, repoDir, nTaskCourseConfig(n), pipeline, registry)
	tester.maxConcurrency = n

	if err := tester.Run(tempDir, tasks, false, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plugin.total != n {
		t.Errorf("expected %d tasks to run, got %d", n, plugin.total)
	}
	if plugin.maxSeen < 2 {
		t.Errorf("expected concurrent execution (maxSeen >= 2), got maxSeen=%d", plugin.maxSeen)
	}
}

// TestTester_Parallel_ConcurrencyLimitRespected проверяет, что семафор ограничивает
// количество одновременно выполняющихся задач значением maxConcurrency.
func TestTester_Parallel_ConcurrencyLimitRespected(t *testing.T) {
	const n, limit = 6, 2
	refDir, repoDir, tempDir := setupNTaskEnv(t, n)

	plugin := &trackingPlugin{delay: 30 * time.Millisecond}
	registry := DefaultRegistry()
	registry["track"] = plugin

	pipeline := []config.PipelineStageConfig{{Name: "run", Run: "track"}}
	tester, tasks := buildTesterWithRegistry(t, refDir, repoDir, nTaskCourseConfig(n), pipeline, registry)
	tester.maxConcurrency = limit

	if err := tester.Run(tempDir, tasks, false, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plugin.total != n {
		t.Errorf("expected all %d tasks to complete, got %d", n, plugin.total)
	}
	if plugin.maxSeen > limit {
		t.Errorf("concurrency limit %d exceeded: max concurrent executions was %d", limit, plugin.maxSeen)
	}
}

// TestTester_Parallel_FatalErrorDoesNotCancelSiblings проверяет, что фатальная ошибка
// в одной задаче (например, неизвестный плагин) не отменяет остальные горутины —
// task1 и task3 должны завершиться несмотря на ошибку task2.
func TestTester_Parallel_FatalErrorDoesNotCancelSiblings(t *testing.T) {
	const n = 3
	refDir, repoDir, tempDir := setupNTaskEnv(t, n)

	plugin := &trackingPlugin{delay: 50 * time.Millisecond}
	registry := DefaultRegistry()
	registry["track"] = plugin

	trackPipeline := []config.PipelineStageConfig{{Name: "run", Run: "track"}}
	// неизвестный плагин вызовет ошибку при построении PipelineRunner
	badPipeline := []config.PipelineStageConfig{{Name: "run", Run: "nonexistent_plugin"}}

	tester, tasks := buildTesterWithRegistry(t, refDir, repoDir, nTaskCourseConfig(n), nil, registry)
	tester.maxConcurrency = n

	for i := range tasks {
		if tasks[i].Name == "task2" {
			tasks[i].Config.TaskPipeline = badPipeline
		} else {
			tasks[i].Config.TaskPipeline = trackPipeline
		}
	}

	err := tester.Run(tempDir, tasks, false, nil)
	if err == nil {
		t.Fatal("expected error from bad plugin in task2, got nil")
	}

	// task1 и task3 должны завершиться несмотря на ошибку task2
	const want = n - 1
	if plugin.total != want {
		t.Errorf("expected %d tasks to complete despite task2 error, got %d", want, plugin.total)
	}
}

// TestTester_Parallel_OutputsIsolatedBetweenTasks проверяет через детектор гонок,
// что горутины не делят общий outputs map. Запускать с флагом -race.
func TestTester_Parallel_OutputsIsolatedBetweenTasks(t *testing.T) {
	const n = 4
	refDir, repoDir, tempDir := setupNTaskEnv(t, n)

	plugin := &trackingPlugin{delay: 10 * time.Millisecond}
	registry := DefaultRegistry()
	registry["track"] = plugin

	outputKey := "stage_result"
	pipeline := []config.PipelineStageConfig{{
		Name:           "run",
		Run:            "track",
		RegisterOutput: &outputKey,
	}}

	tester, tasks := buildTesterWithRegistry(t, refDir, repoDir, nTaskCourseConfig(n), pipeline, registry)
	tester.maxConcurrency = n

	if err := tester.Run(tempDir, tasks, false, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plugin.total != n {
		t.Errorf("expected %d tasks, got %d", n, plugin.total)
	}
}

// TestTester_Parallel_FailedPipelineCollectsAllResults проверяет, что при провале
// нескольких задач все их имена попадают в итоговую ошибку.
func TestTester_Parallel_FailedPipelineCollectsAllResults(t *testing.T) {
	const n = 4
	refDir, repoDir, tempDir := setupNTaskEnv(t, n)

	registry := DefaultRegistry()
	// плагин всегда возвращает PluginExecutionFailed — задача "провалена", не фатальная ошибка
	registry["always_fail"] = &alwaysFailPlugin{}

	pipeline := []config.PipelineStageConfig{{Name: "run", Run: "always_fail"}}
	tester, tasks := buildTesterWithRegistry(t, refDir, repoDir, nTaskCourseConfig(n), pipeline, registry)
	tester.maxConcurrency = n

	err := tester.Run(tempDir, tasks, false, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("task%d", i)
		if !strings.Contains(err.Error(), name) {
			t.Errorf("expected task %q in error message, got: %v", name, err)
		}
	}
}

type alwaysFailPlugin struct{}

func (p *alwaysFailPlugin) Run(_ map[string]any, _ bool) (*PluginOutput, error) {
	return nil, domainerrors.NewPluginExecutionFailed("intentional failure", "", 0)
}
