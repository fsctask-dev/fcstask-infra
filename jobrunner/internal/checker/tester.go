package checker

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"jobrunner/internal/config"
	domainerrors "jobrunner/internal/domain/errors"
)

type GlobalPipelineVariables struct {
	RefDir       string   `json:"ref_dir"`
	RepoDir      string   `json:"repo_dir"`
	TempDir      string   `json:"temp_dir"`
	TaskNames    []string `json:"task_names"`
	TaskSubPaths []string `json:"task_sub_paths"`
}

type TaskPipelineVariables struct {
	TaskName         string  `json:"task_name"`
	TaskSubPath      string  `json:"task_sub_path"`
	TaskScorePercent float64 `json:"task_score_percent"`
	TaskPassed       bool    `json:"task_passed"`
}

type percentStep struct {
	Percent  float64
	Deadline time.Time
}

type taskRunResult struct {
	name   string
	failed bool
	err    error
}

type Tester struct {
	course              *Course
	testingConfig       *config.CheckerTestingConfig
	defaultParams       *config.CheckerParametersConfig
	registry            PluginRegistry
	globalPipeline      *PipelineRunner
	verbose             bool
	dryRun              bool
	taskToPercents      map[string][]percentStep
	deadlinesType       string
	interpolationWindow float64
	maxConcurrency      int
}

func NewTester(
	course *Course,
	checkerConfig *config.CheckerConfig,
	registry PluginRegistry,
	verbose bool,
	dryRun bool,
) (*Tester, error) {
	globalPipeline, err := NewPipelineRunner(checkerConfig.Testing.GlobalPipeline, registry, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to build global pipeline: %w", err)
	}

	window := 0
	if course.CourseConfig.Deadlines.Window != nil {
		window = *course.CourseConfig.Deadlines.Window
	}

	taskToPercents := make(map[string][]percentStep)
	for i := range course.CourseConfig.Deadlines.Schedule {
		group := &course.CourseConfig.Deadlines.Schedule[i]
		steps := buildPercentsForGroup(group)
		for _, task := range group.Tasks {
			taskToPercents[task.Task] = steps
		}
	}

	return &Tester{
		course:              course,
		testingConfig:       checkerConfig.Testing,
		defaultParams:       checkerConfig.DefaultParameters,
		registry:            registry,
		globalPipeline:      globalPipeline,
		verbose:             verbose,
		dryRun:              dryRun,
		taskToPercents:      taskToPercents,
		deadlinesType:       course.CourseConfig.Deadlines.Deadlines,
		interpolationWindow: float64(window) * 24 * 3600,
		maxConcurrency:      runtime.NumCPU(),
	}, nil
}

func (t *Tester) Run(tempDir string, tasks []FileSystemTask, report bool, timestamp *time.Time) error {
	if tasks == nil {
		enabled := true
		tasks = t.course.GetTasks(&enabled)
	}

	globalOutputs := make(map[string]any)

	globalVars := GlobalPipelineVariables{
		RefDir:       t.course.ReferenceRoot,
		RepoDir:      t.course.RepositoryRoot,
		TempDir:      tempDir,
		TaskNames:    collectTaskNames(tasks),
		TaskSubPaths: collectTaskSubPaths(tasks),
	}

	if t.globalPipeline.Len() > 0 {
		ctx := t.buildContext(globalVars, nil, globalOutputs, nil)
		result, err := t.globalPipeline.Run(ctx, t.dryRun)
		if err != nil {
			return err
		}
		if !result.Succeeded() {
			return domainerrors.NewTestingError("global pipeline failed")
		}
	}

	results := make(chan taskRunResult, len(tasks))
	sem := make(chan struct{}, t.maxConcurrency)

	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		task := task
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- t.runSingleTask(task, globalVars, globalOutputs, report, timestamp)
		}()
	}

	wg.Wait()
	close(results)

	var failedTasks []string
	for r := range results {
		if r.err != nil {
			return r.err
		}
		if r.failed {
			failedTasks = append(failedTasks, r.name)
		}
	}

	if len(failedTasks) > 0 {
		return domainerrors.NewTestingError(fmt.Sprintf("task pipelines failed: %v", failedTasks))
	}

	return nil
}

func (t *Tester) runSingleTask(
	task FileSystemTask,
	globalVars GlobalPipelineVariables,
	globalOutputs map[string]any,
	report bool,
	timestamp *time.Time,
) taskRunResult {
	taskOutputs := make(map[string]any, len(globalOutputs))
	for k, v := range globalOutputs {
		taskOutputs[k] = v
	}

	var ts time.Time
	if timestamp != nil {
		ts = *timestamp
	} else {
		ts = t.course.CourseConfig.Deadlines.GetNowWithTimezone()
	}

	scorePercent := t.GetTaskScorePercent(task.Name, ts)
	taskVars := TaskPipelineVariables{
		TaskName:         task.Name,
		TaskSubPath:      task.RelativePath,
		TaskScorePercent: scorePercent,
	}

	ctx := t.buildContext(globalVars, &taskVars, taskOutputs, task.Config.Parameters)

	taskRunner, err := t.getTaskPipelineRunner(task)
	if err != nil {
		return taskRunResult{name: task.Name, err: err}
	}

	taskResult, err := taskRunner.Run(ctx, t.dryRun)
	if err != nil {
		return taskRunResult{name: task.Name, err: err}
	}

	if !taskResult.Succeeded() {
		return taskRunResult{name: task.Name, failed: true}
	}

	taskVars.TaskPassed = true
	reportCtx := t.buildContext(globalVars, &taskVars, taskOutputs, task.Config.Parameters)

	reportRunner, err := t.getReportPipelineRunner(task)
	if err != nil {
		return taskRunResult{name: task.Name, err: err}
	}

	reportDryRun := t.dryRun || !report
	if _, err = reportRunner.Run(reportCtx, reportDryRun); err != nil {
		return taskRunResult{name: task.Name, err: err}
	}

	return taskRunResult{name: task.Name}
}

func (t *Tester) buildContext(
	globalVars GlobalPipelineVariables,
	taskVars *TaskPipelineVariables,
	outputs map[string]any,
	taskParams *config.CheckerParametersConfig,
) map[string]any {
	ctx := map[string]any{
		"global":     structToMap(globalVars),
		"outputs":    outputs,
		"parameters": mergeParameters(t.defaultParams, taskParams),
		"env":        envMap(),
	}
	if taskVars != nil {
		ctx["task"] = structToMap(*taskVars)
	}
	return ctx
}

func (t *Tester) getTaskPipelineRunner(task FileSystemTask) (*PipelineRunner, error) {
	pipeline := task.Config.TaskPipeline
	if len(pipeline) == 0 {
		pipeline = t.testingConfig.TasksPipeline
	}
	return NewPipelineRunner(pipeline, t.registry, t.verbose)
}

func (t *Tester) getReportPipelineRunner(task FileSystemTask) (*PipelineRunner, error) {
	pipeline := task.Config.ReportPipeline
	if len(pipeline) == 0 {
		pipeline = t.testingConfig.ReportPipeline
	}
	return NewPipelineRunner(pipeline, t.registry, t.verbose)
}

func (t *Tester) GetTaskScorePercent(taskName string, timestamp time.Time) float64 {
	steps, ok := t.taskToPercents[taskName]
	if !ok {
		return 1.0
	}

	prevPercent := 1.0
	prevTimestamp := timestamp

	for _, step := range steps {
		if !timestamp.After(step.Deadline) {
			if t.deadlinesType == config.DeadlineHard {
				return step.Percent
			}
			return calcInterpolatedPercent(step.Percent, prevPercent, timestamp, prevTimestamp, t.interpolationWindow)
		}
		prevPercent = step.Percent
		prevTimestamp = step.Deadline
	}
	return 0.0
}

func buildPercentsForGroup(group *config.CourseGroupConfig) []percentStep {
	type pair struct {
		percent  float64
		deadline time.Time
	}

	pairs := make([]pair, 0, len(group.Steps))
	for percent, dv := range group.Steps {
		pairs = append(pairs, pair{percent, resolveDeadline(group.Start, dv)})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].deadline.Before(pairs[j].deadline)
	})

	endTime := resolveDeadline(group.Start, group.End)

	steps := make([]percentStep, 0, len(pairs)+1)
	for i, p := range pairs {
		var percent float64
		if i == 0 {
			percent = 1.0
		} else {
			percent = pairs[i-1].percent
		}
		steps = append(steps, percentStep{percent, p.deadline})
	}

	lastPercent := 1.0
	if len(pairs) > 0 {
		lastPercent = pairs[len(pairs)-1].percent
	}
	steps = append(steps, percentStep{lastPercent, endTime})

	return steps
}

func resolveDeadline(start time.Time, dv config.DeadlineValue) time.Time {
	if dv.Time != nil {
		return *dv.Time
	}
	if dv.Duration != nil {
		return start.Add(*dv.Duration)
	}
	return start
}

func calcInterpolatedPercent(percent, prevPercent float64, timestamp, prevTimestamp time.Time, windowSecs float64) float64 {
	if windowSecs <= 0 {
		return percent
	}
	frac := timestamp.Sub(prevTimestamp).Seconds() / windowSecs
	if frac >= 1 {
		return percent
	}
	return prevPercent - frac*(prevPercent-percent)
}

func mergeParameters(defaults, override *config.CheckerParametersConfig) map[string]any {
	result := make(map[string]any)
	if defaults != nil {
		for k, v := range defaults.Parameters {
			result[k] = v
		}
	}
	if override != nil {
		for k, v := range override.Parameters {
			result[k] = v
		}
	}
	return result
}

func collectTaskNames(tasks []FileSystemTask) []string {
	names := make([]string, len(tasks))
	for i, task := range tasks {
		names[i] = task.Name
	}
	return names
}

func collectTaskSubPaths(tasks []FileSystemTask) []string {
	paths := make([]string, len(tasks))
	for i, task := range tasks {
		paths[i] = task.RelativePath
	}
	return paths
}

func envMap() map[string]string {
	result := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			result[k] = v
		}
	}
	return result
}
