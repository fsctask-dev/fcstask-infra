package config

import (
	"fmt"
	"jobrunner/internal/domain/errors"
	"sort"
	"strings"
	"time"
)

const (
	DeadlineHard        string = "hard"
	DeadlineInterpolate string = "interpolate"
)

type DeadlineValue struct {
	Time     *time.Time     `yaml:"time,omitempty"`
	Duration *time.Duration `yaml:"duration,omitempty"`
}

type CourseSettingsConfig struct {
	CourseName    string `yaml:"course_name"`
	GitlabBaseURL string `yaml:"gitlab_base_url"`
	PublicRepo    string `yaml:"public_repo"`
	StudentsGroup string `yaml:"students_group"`
}

type CourseUIConfig struct {
	TaskURLTemplate string            `yaml:"task_url_template"`
	Links           map[string]string `yaml:"links"`
}

type CourseTaskConfig struct {
	Task      string `yaml:"task"`
	Enabled   bool   `yaml:"enabled"`
	Score     int    `yaml:"score"`
	MinScore  int    `yaml:"min_score"`
	Special   int    `yaml:"special"`
	IsBonus   bool   `yaml:"is_bonus"`
	IsLarge   bool   `yaml:"is_large"`
	IsSpecial bool   `yaml:"is_special"`
	URL       string `yaml:"url,omitempty"`
}

type CourseGroupConfig struct {
	Name    string                    `yaml:"group"`
	Enabled bool                      `yaml:"enabled"`
	Start   time.Time                 `yaml:"start"`
	Steps   map[float64]DeadlineValue `yaml:"steps"`
	End     DeadlineValue             `yaml:"end"`
	Tasks   []CourseTaskConfig        `yaml:"tasks"`
}

type CourseDeadlinesConfig struct {
	Timezone          string              `yaml:"timezone"`
	Deadlines         string              `yaml:"deadlines"`
	Window            *int                `yaml:"window,omitempty"`
	MaxSubmissions    *int                `yaml:"max_submissions,omitempty"`
	SubmissionPenalty float64             `yaml:"submission_penalty"`
	Schedule          []CourseGroupConfig `yaml:"schedule"`
}

type CourseConfig struct {
	Version   int                    `yaml:"version"`
	Settings  CourseSettingsConfig   `yaml:"settings"`
	UI        CourseUIConfig         `yaml:"ui"`
	Deadlines CourseDeadlinesConfig  `yaml:"deadlines"`
	Grades    map[string]interface{} `yaml:"grades,omitempty"`
}

func (g *CourseUIConfig) Validate() error {
	if g.TaskURLTemplate == "" {
		return nil
	}

	isValid := strings.HasPrefix(g.TaskURLTemplate, "http://") ||
		strings.HasPrefix(g.TaskURLTemplate, "https://")

	if !isValid {
		return errors.NewBadConfig("task_url_template must start with http:// or https://")
	}

	return nil
}

func (g *CourseGroupConfig) Validate() error {
	if g.Name == "" {
		return errors.NewBadConfig("group name is required")
	}

	if err := g.validateEnd(); err != nil {
		return err
	}

	if err := g.validateSteps(); err != nil {
		return err
	}

	return nil
}

func (g *CourseGroupConfig) SetDefaults() {
	if g.Steps == nil {
		g.Steps = make(map[float64]DeadlineValue)
	}
	if g.Tasks == nil {
		g.Tasks = []CourseTaskConfig{}
	}
}

func (g *CourseGroupConfig) GetPercentsBeforeDeadline() map[float64]time.Time {
	result := make(map[float64]time.Time)

	percentages := []float64{1.0}
	for p := range g.Steps {
		if p != 1.0 {
			percentages = append(percentages, p)
		}
	}
	sort.Float64s(percentages)

	values := make([]DeadlineValue, 0, len(percentages))
	for i, percent := range percentages {
		if i == len(percentages)-1 {
			values = append(values, g.End)
		} else {
			values = append(values, g.Steps[percent])
		}
	}

	for i, percent := range percentages {
		result[percent] = g.resolveToTime(values[i])
	}

	return result
}

func (g *CourseGroupConfig) GetCurrentPercentMultiplier(now time.Time) float64 {
	deadlines := g.GetPercentsBeforeDeadline()

	percentages := make([]float64, 0, len(deadlines))
	for p := range deadlines {
		percentages = append(percentages, p)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(percentages)))

	for _, percent := range percentages {
		deadline := deadlines[percent]
		if now.Before(deadline) || now.Equal(deadline) {
			return percent
		}
	}

	return 0.0
}

func (g *CourseGroupConfig) ReplaceTimezone(tz *time.Location) {
	g.Start = g.Start.In(tz)

	if g.End.Time != nil {
		updated := g.End.Time.In(tz)
		g.End.Time = &updated
	}

	for percent, step := range g.Steps {
		if step.Time != nil {
			updated := step.Time.In(tz)
			g.Steps[percent] = DeadlineValue{Time: &updated}
		}
	}
}

func (g *CourseGroupConfig) AddTask(task CourseTaskConfig) *CourseGroupConfig {
	g.Tasks = append(g.Tasks, task)
	return g
}

func (g *CourseGroupConfig) AddStep(percent float64, value DeadlineValue) *CourseGroupConfig {
	if g.Steps == nil {
		g.Steps = make(map[float64]DeadlineValue)
	}
	g.Steps[percent] = value
	return g
}

func (g *CourseGroupConfig) SetEnd(value DeadlineValue) *CourseGroupConfig {
	g.End = value
	return g
}

func (g *CourseGroupConfig) validateEnd() error {
	if g.End.Duration != nil && *g.End.Duration < 0 {
		return errors.NewBadConfig(fmt.Sprintf("end timedelta <%v> should be positive", *g.End.Duration))
	}

	if g.End.Time != nil && g.End.Time.Before(g.Start) {
		return errors.NewBadConfig(fmt.Sprintf("end datetime <%v> should be after the start <%v>", g.End.Time, g.Start))
	}

	return nil
}

func (g *CourseGroupConfig) validateSteps() error {
	var lastStep interface{} = g.Start

	for _, step := range g.Steps {
		if step.Duration != nil && *step.Duration < 0 {
			return errors.NewBadConfig(fmt.Sprintf("step timedelta <%v> should be positive", *step.Duration))
		}

		if step.Time != nil && (step.Time.Before(g.Start) || step.Time.Equal(g.Start)) {
			return errors.NewBadConfig(fmt.Sprintf("step datetime <%v> should be after the start <%v>", step.Time, g.Start))
		}

		stepDate := g.resolveToTime(step)

		lastStepDate, err := g.resolveToTimeValue(lastStep)

		if err != nil {
			return err
		}

		if stepDate.Before(lastStepDate) || stepDate.Equal(lastStepDate) {
			return errors.NewBadConfig(fmt.Sprintf("step datetime/timedelta <%v> should be after the last step <%v>",
				deadlineValueToString(step), deadlineValueToString(lastStep)))
		}

		lastStep = g.getStepValue(step)
	}

	return nil
}

func (g *CourseGroupConfig) resolveToTime(dv DeadlineValue) time.Time {
	if dv.Time != nil {
		return *dv.Time
	}

	if dv.Duration != nil {
		return g.Start.Add(*dv.Duration)
	}

	return g.Start
}

func (g *CourseGroupConfig) resolveToTimeValue(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil

	case time.Duration:
		return g.Start.Add(v), nil

	case DeadlineValue:
		return g.resolveToTime(v), nil

	case *DeadlineValue:
		if v != nil {
			return g.resolveToTime(*v), nil
		}

	default:
		return time.Time{}, errors.NewBadConfig("invalid type")
	}

	return g.Start, nil
}

func (g *CourseGroupConfig) getStepValue(step DeadlineValue) interface{} {
	if step.Time != nil {
		return *step.Time
	}
	if step.Duration != nil {
		return *step.Duration
	}
	return g.Start
}

func (c *CourseDeadlinesConfig) SetDefaults() {
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	if c.Deadlines == "" {
		c.Deadlines = DeadlineHard
	}
	if c.Schedule == nil {
		c.Schedule = []CourseGroupConfig{}
	}
}

func (c *CourseDeadlinesConfig) Validate() error {
	if err := c.validateMaxSubmissions(); err != nil {
		return err
	}

	if err := c.validateSubmissionPenalty(); err != nil {
		return err
	}

	if err := c.validateTimezone(); err != nil {
		return err
	}

	if err := c.validateUniqueNames(); err != nil {
		return err
	}

	if err := c.validateWindow(); err != nil {
		return err
	}

	if err := c.validateInterpolateDeadlines(); err != nil {
		return err
	}

	return nil
}

func (c *CourseDeadlinesConfig) GetNowWithTimezone() time.Time {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Now().In(loc)
}

func (c *CourseDeadlinesConfig) FindTask(taskName string) (*CourseGroupConfig, *CourseTaskConfig, error) {
	for i := range c.Schedule {
		group := &c.Schedule[i]
		for j := range group.Tasks {
			task := &group.Tasks[j]
			if task.Task == taskName {
				return group, task, nil
			}
		}
	}
	return nil, nil, errors.NewBadConfig(fmt.Sprintf("task %s not found", taskName))
}

func (c *CourseDeadlinesConfig) GetGroups(enabled *bool, started *bool, now *time.Time) []CourseGroupConfig {
	currentTime := now
	if currentTime == nil {
		t := c.GetNowWithTimezone()
		currentTime = &t
	}

	groups := make([]CourseGroupConfig, 0, len(c.Schedule))

	for _, group := range c.Schedule {
		if enabled != nil && group.Enabled != *enabled {
			continue
		}

		if started != nil {
			if *started && group.Start.After(*currentTime) {
				continue
			}
			if !*started && !group.Start.After(*currentTime) {
				continue
			}
		}

		groups = append(groups, group)
	}

	return groups
}

func (c *CourseDeadlinesConfig) GetTasks(enabled *bool, started *bool, isBonus *bool, now *time.Time) []CourseTaskConfig {
	currentTime := now
	if currentTime == nil {
		t := c.GetNowWithTimezone()
		currentTime = &t
	}

	groups := c.GetGroups(nil, started, currentTime)
	tasks := make([]CourseTaskConfig, 0)
	extraTasks := make([]CourseTaskConfig, 0)

	for _, group := range groups {
		for _, task := range group.Tasks {
			if enabled != nil {
				if *enabled {
					if group.Enabled && task.Enabled {
						tasks = append(tasks, task)
					}
				} else {
					if !group.Enabled {
						extraTasks = append(extraTasks, task)
					} else if !task.Enabled {
						tasks = append(tasks, task)
					}
				}
			} else {
				tasks = append(tasks, task)
			}
		}
	}

	if enabled != nil && !*enabled {
		tasksMap := make(map[string]bool)
		for _, task := range tasks {
			tasksMap[task.Task] = true
		}
		for _, task := range extraTasks {
			if !tasksMap[task.Task] {
				tasks = append(tasks, task)
			}
		}
	}

	if isBonus != nil {
		filtered := make([]CourseTaskConfig, 0)
		for _, task := range tasks {
			if task.IsBonus == *isBonus {
				filtered = append(filtered, task)
			}
		}
		tasks = filtered
	}

	return tasks
}

func (c *CourseDeadlinesConfig) MaxScore(started *bool, now *time.Time) int {
	enabledTrue := true
	isBonusFalse := false

	tasks := c.GetTasks(&enabledTrue, started, &isBonusFalse, now)

	total := 0
	for _, task := range tasks {
		total += task.Score
	}
	return total
}

func (c *CourseDeadlinesConfig) MaxScoreStarted() int {
	startedTrue := true
	return c.MaxScore(&startedTrue, nil)
}

func (c *CourseDeadlinesConfig) ReplaceTimezone() error {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return errors.NewBadConfig(fmt.Sprintf("failed to load timezone %s: %v", c.Timezone, err))
	}

	for i := range c.Schedule {
		c.Schedule[i].ReplaceTimezone(loc)
	}

	return nil
}

func (c *CourseDeadlinesConfig) validateMaxSubmissions() error {
	if c.MaxSubmissions != nil && *c.MaxSubmissions <= 0 {
		return errors.NewBadConfig(fmt.Sprintf("max_submissions should be positive, got %d", *c.MaxSubmissions))
	}
	return nil
}

func (c *CourseDeadlinesConfig) validateSubmissionPenalty() error {
	if c.SubmissionPenalty < 0 {
		return errors.NewBadConfig(fmt.Sprintf("submission_penalty should be non-negative, got %f", c.SubmissionPenalty))
	}
	return nil
}

func (c *CourseDeadlinesConfig) validateTimezone() error {
	if c.Timezone == "" {
		return nil
	}
	_, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return errors.NewBadConfig(fmt.Sprintf("invalid timezone: %s, error: %v", c.Timezone, err))
	}
	return nil
}

func (c *CourseDeadlinesConfig) validateUniqueNames() error {
	groupNames := make(map[string]bool)
	taskNames := make(map[string]bool)

	for _, group := range c.Schedule {
		if groupNames[group.Name] {
			return errors.NewBadConfig(fmt.Sprintf("duplicate group name: %s", group.Name))
		}
		groupNames[group.Name] = true

		for _, task := range group.Tasks {
			if taskNames[task.Task] {
				return errors.NewBadConfig(fmt.Sprintf("duplicate task name: %s", task.Task))
			}
			taskNames[task.Task] = true
		}
	}

	return nil
}

func (c *CourseDeadlinesConfig) validateWindow() error {
	if c.Window != nil && *c.Window <= 0 {
		return errors.NewBadConfig(fmt.Sprintf("window should be positive, got %d", *c.Window))
	}

	if c.Window != nil && c.Deadlines != DeadlineInterpolate {
		return errors.NewBadConfig("window can be applied only with interpolate deadline type")
	}

	return nil
}

func (c *CourseDeadlinesConfig) validateInterpolateDeadlines() error {
	if c.Deadlines != DeadlineInterpolate {
		return nil
	}

	windowDays := 0
	if c.Window != nil {
		windowDays = *c.Window
	}

	for _, group := range c.Schedule {
		deadlines := group.GetPercentsBeforeDeadline()

		dates := make([]time.Time, 0, len(deadlines))
		for _, date := range deadlines {
			dates = append(dates, date)
		}
		sort.Slice(dates, func(i, j int) bool {
			return dates[i].Before(dates[j])
		})

		for i := 0; i < len(dates)-1; i++ {
			left := dates[i]
			right := dates[i+1]

			if left.Add(time.Duration(windowDays) * 24 * time.Hour).After(right) {
				return errors.NewBadConfig(fmt.Sprintf("window is too large for group %s: interval between %v and %v is too small",
					group.Name, left, right))
			}
		}
	}

	return nil
}

func (s *CourseSettingsConfig) SetDefaults() {
	if s.GitlabBaseURL == "" {
		s.GitlabBaseURL = "https://gitlab.com"
	}
	if s.PublicRepo == "" {
		s.PublicRepo = "public-course"
	}
	if s.StudentsGroup == "" {
		s.StudentsGroup = "students"
	}
}

func (s *CourseSettingsConfig) Validate() error {
	if s.CourseName == "" {
		return errors.NewBadConfig("course name is required")
	}
	if s.GitlabBaseURL == "" {
		return errors.NewBadConfig("gitlab base URL is required")
	}
	if s.PublicRepo == "" {
		return errors.NewBadConfig("public repo is required")
	}
	if s.StudentsGroup == "" {
		return errors.NewBadConfig("students group is required")
	}
	return nil
}

func (c *CourseConfig) SetDefaults() {
	if c.Version == 0 {
		c.Version = 1
	}

	c.Settings.SetDefaults()
	c.Deadlines.SetDefaults()
}

func (c *CourseConfig) Validate() error {
	if err := c.validateVersion(); err != nil {
		return err
	}

	if err := c.Settings.Validate(); err != nil {
		return err
	}

	if err := c.UI.Validate(); err != nil {
		return err
	}

	if err := c.Deadlines.Validate(); err != nil {
		return err
	}

	return nil
}

func (c *CourseConfig) validateVersion() error {
	if c.Version != 1 {
		return errors.NewBadConfig(fmt.Sprintf("only version 1 is supported for CourseConfig, got %d", c.Version))
	}
	return nil
}
