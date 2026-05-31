package config

import (
	"testing"
	"time"

	"jobrunner/internal/domain/errors"
)

var (
	jan1 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	jan2 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	jan3 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	jan4 = time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	jan5 = time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	jan6 = time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC)
)

func absDeadline(t time.Time) DeadlineValue    { return DeadlineValue{Time: &t} }
func durDeadline(d time.Duration) DeadlineValue { return DeadlineValue{Duration: &d} }

func ptr[T any](v T) *T { return &v }

func newGroup(name string, enabled bool, start time.Time, steps map[float64]DeadlineValue, end DeadlineValue, tasks []CourseTaskConfig) CourseGroupConfig {
	if steps == nil {
		steps = make(map[float64]DeadlineValue)
	}
	return CourseGroupConfig{
		Name:    name,
		Enabled: enabled,
		Start:   start,
		Steps:   steps,
		End:     end,
		Tasks:   tasks,
	}
}


func TestGroupGetPercentsBeforeDeadline_NoSteps(t *testing.T) {
	g := newGroup("g", true, jan1, nil, absDeadline(jan5), nil)

	got := g.GetPercentsBeforeDeadline()

	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if !got[1.0].Equal(jan5) {
		t.Errorf("expected 1.0 = jan5, got %v", got[1.0])
	}
}

func TestGroupGetPercentsBeforeDeadline_WithAbsoluteSteps(t *testing.T) {
	g := newGroup("g", true, jan1,
		map[float64]DeadlineValue{
			0.9: absDeadline(jan2),
			0.5: absDeadline(jan3),
		},
		absDeadline(jan5), nil)

	got := g.GetPercentsBeforeDeadline()

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if !got[0.5].Equal(jan3) {
		t.Errorf("0.5: expected jan3, got %v", got[0.5])
	}
	if !got[0.9].Equal(jan2) {
		t.Errorf("0.9: expected jan2, got %v", got[0.9])
	}
	if !got[1.0].Equal(jan5) {
		t.Errorf("1.0: expected jan5, got %v", got[1.0])
	}
}

func TestGroupGetPercentsBeforeDeadline_WithDurationSteps(t *testing.T) {
	g := newGroup("g", true, jan1,
		map[float64]DeadlineValue{
			0.5: durDeadline(2 * 24 * time.Hour),
		},
		durDeadline(4*24*time.Hour), nil)

	got := g.GetPercentsBeforeDeadline()

	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if !got[0.5].Equal(jan3) {
		t.Errorf("0.5: expected jan3, got %v", got[0.5])
	}
	if !got[1.0].Equal(jan5) {
		t.Errorf("1.0: expected jan5, got %v", got[1.0])
	}
}


func TestGroupGetCurrentPercentMultiplier_NoSteps(t *testing.T) {
	g := newGroup("g", true, jan1, nil, absDeadline(jan5), nil)

	cases := []struct {
		now  time.Time
		want float64
	}{
		{jan1, 1.0},
		{jan3, 1.0},
		{jan5, 1.0},
		{jan6, 0.0},
	}
	for _, c := range cases {
		got := g.GetCurrentPercentMultiplier(c.now)
		if got != c.want {
			t.Errorf("now=%v: want %.1f, got %.1f", c.now, c.want, got)
		}
	}
}

func TestGroupGetCurrentPercentMultiplier_WithSteps(t *testing.T) {
	g := newGroup("g", true, jan1,
		map[float64]DeadlineValue{
			0.9: absDeadline(jan2),
			0.5: absDeadline(jan3),
		},
		absDeadline(jan5), nil)

	cases := []struct {
		now  time.Time
		want float64
	}{
		{jan1, 1.0},
		{jan2, 1.0},
		{jan3, 1.0},
		{jan4, 1.0},
		{jan5, 1.0},
		{jan6, 0.0},
	}
	for _, c := range cases {
		got := g.GetCurrentPercentMultiplier(c.now)
		if got != c.want {
			t.Errorf("now=%v: want %.1f, got %.1f", c.now, c.want, got)
		}
	}
}


func newTestSchedule() CourseDeadlinesConfig {
	past := jan1
	future := jan6

	return CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineHard,
		Schedule: []CourseGroupConfig{
			newGroup("group1", true, past, nil, absDeadline(jan5),
				[]CourseTaskConfig{
					{Task: "task1a", Enabled: true, Score: 10, IsBonus: false},
					{Task: "task1b", Enabled: false, Score: 5, IsBonus: false},
					{Task: "task1c", Enabled: true, Score: 0, IsBonus: true},
				}),
			newGroup("group2", false, past, nil, absDeadline(jan5),
				[]CourseTaskConfig{
					{Task: "task2a", Enabled: true, Score: 20, IsBonus: false},
				}),
			newGroup("group3", true, future, nil, absDeadline(jan5),
				[]CourseTaskConfig{
					{Task: "task3a", Enabled: true, Score: 15, IsBonus: false},
				}),
			newGroup("group4", false, future, nil, absDeadline(jan5), nil),
		},
	}
}


func TestDeadlinesGetGroups_NoFilter(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	groups := c.GetGroups(nil, nil, &now)
	if len(groups) != 4 {
		t.Errorf("expected 4 groups, got %d", len(groups))
	}
}

func TestDeadlinesGetGroups_FilterEnabled(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	enabled := c.GetGroups(ptr(true), nil, &now)
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled groups, got %d", len(enabled))
	}
	for _, g := range enabled {
		if !g.Enabled {
			t.Errorf("group %s should be enabled", g.Name)
		}
	}

	disabled := c.GetGroups(ptr(false), nil, &now)
	if len(disabled) != 2 {
		t.Errorf("expected 2 disabled groups, got %d", len(disabled))
	}
	for _, g := range disabled {
		if g.Enabled {
			t.Errorf("group %s should be disabled", g.Name)
		}
	}
}

func TestDeadlinesGetGroups_FilterStarted(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	started := c.GetGroups(nil, ptr(true), &now)
	if len(started) != 2 {
		t.Errorf("expected 2 started groups, got %d", len(started))
	}
	for _, g := range started {
		if g.Start.After(now) {
			t.Errorf("group %s should be started (start=%v, now=%v)", g.Name, g.Start, now)
		}
	}

	notStarted := c.GetGroups(nil, ptr(false), &now)
	if len(notStarted) != 2 {
		t.Errorf("expected 2 not-started groups, got %d", len(notStarted))
	}
	for _, g := range notStarted {
		if !g.Start.After(now) {
			t.Errorf("group %s should not be started (start=%v, now=%v)", g.Name, g.Start, now)
		}
	}
}

func TestDeadlinesGetGroups_FilterEnabledAndStarted(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	groups := c.GetGroups(ptr(true), ptr(true), &now)
	if len(groups) != 1 {
		t.Errorf("expected 1 (enabled+started), got %d", len(groups))
	}
	if groups[0].Name != "group1" {
		t.Errorf("expected group1, got %s", groups[0].Name)
	}
}


func TestDeadlinesGetTasks_NoFilter(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	tasks := c.GetTasks(nil, nil, nil, &now)
	if len(tasks) != 5 {
		t.Errorf("expected 5 tasks, got %d: %v", len(tasks), taskNames(tasks))
	}
}

func TestDeadlinesGetTasks_FilterEnabled(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	tasks := c.GetTasks(ptr(true), nil, nil, &now)
	names := taskNames(tasks)
	want := map[string]bool{"task1a": true, "task1c": true, "task3a": true}
	if len(tasks) != 3 {
		t.Errorf("expected 3 enabled tasks, got %d: %v", len(tasks), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected task %s", n)
		}
	}
}

func TestDeadlinesGetTasks_FilterDisabled(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	tasks := c.GetTasks(ptr(false), nil, nil, &now)
	names := taskNames(tasks)
	want := map[string]bool{"task1b": true, "task2a": true}
	if len(tasks) != 2 {
		t.Errorf("expected 2 disabled tasks, got %d: %v", len(tasks), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected task %s", n)
		}
	}
}

func TestDeadlinesGetTasks_FilterIsBonus(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	bonus := c.GetTasks(ptr(true), nil, ptr(true), &now)
	bonusNames := taskNames(bonus)
	if len(bonus) != 1 || bonusNames[0] != "task1c" {
		t.Errorf("expected [task1c] bonus tasks, got %v", bonusNames)
	}

	nonBonus := c.GetTasks(ptr(true), nil, ptr(false), &now)
	nonBonusNames := taskNames(nonBonus)
	want := map[string]bool{"task1a": true, "task3a": true}
	if len(nonBonus) != 2 {
		t.Errorf("expected 2 non-bonus tasks, got %d: %v", len(nonBonus), nonBonusNames)
	}
	for _, n := range nonBonusNames {
		if !want[n] {
			t.Errorf("unexpected non-bonus task %s", n)
		}
	}
}

func TestDeadlinesGetTasks_FilterStarted(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	tasks := c.GetTasks(ptr(true), ptr(true), nil, &now)
	names := taskNames(tasks)

	if len(tasks) != 2 {
		t.Errorf("expected 2 started+enabled tasks, got %d: %v", len(tasks), names)
	}
}


func TestDeadlinesFindTask_Found(t *testing.T) {
	c := newTestSchedule()

	group, task, err := c.FindTask("task1a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.Name != "group1" {
		t.Errorf("expected group1, got %s", group.Name)
	}
	if task.Task != "task1a" {
		t.Errorf("expected task1a, got %s", task.Task)
	}
}

func TestDeadlinesFindTask_NotFound(t *testing.T) {
	c := newTestSchedule()

	_, _, err := c.FindTask("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig error, got %T: %v", err, err)
	}
}


func TestDeadlinesMaxScore_AllGroups(t *testing.T) {
	c := newTestSchedule()
	now := jan3

	score := c.MaxScore(nil, &now)
	if score != 25 {
		t.Errorf("expected 25, got %d", score)
	}
}

func TestDeadlinesMaxScore_StartedOnly(t *testing.T) {
	c := newTestSchedule()
	now := jan3


	score := c.MaxScore(ptr(true), &now)
	if score != 10 {
		t.Errorf("expected 10, got %d", score)
	}
}

func TestDeadlinesMaxScoreStarted(t *testing.T) {
	c := newTestSchedule()
	score := c.MaxScoreStarted()
	if score < 0 {
		t.Errorf("expected non-negative score, got %d", score)
	}
}


func TestDeadlinesValidate_Valid(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineHard,
		Schedule: []CourseGroupConfig{
			newGroup("group1", true, jan1, nil, absDeadline(jan5),
				[]CourseTaskConfig{{Task: "task1", Enabled: true}}),
		},
	}

	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeadlinesValidate_DuplicateGroupNames(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineHard,
		Schedule: []CourseGroupConfig{
			newGroup("group1", true, jan1, nil, absDeadline(jan5), nil),
			newGroup("group1", true, jan2, nil, absDeadline(jan5), nil),
		},
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate group names")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_DuplicateTaskNames(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineHard,
		Schedule: []CourseGroupConfig{
			newGroup("group1", true, jan1, nil, absDeadline(jan5),
				[]CourseTaskConfig{{Task: "task1"}, {Task: "task1"}}),
		},
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate task names")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_InvalidTimezone(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "Not/ATimezone",
		Deadlines: DeadlineHard,
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_WindowWithHardDeadline(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineHard,
		Window:    ptr(7),
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error: window requires interpolate deadline")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_NegativeWindow(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineInterpolate,
		Window:    ptr(-1),
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for non-positive window")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_WindowWithInterpolate_Valid(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineInterpolate,
		Window:    ptr(1),
		Schedule: []CourseGroupConfig{
			newGroup("group1", true, jan1,
				map[float64]DeadlineValue{0.5: absDeadline(jan3)},
				absDeadline(jan5), nil),
		},
	}

	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeadlinesValidate_WindowWithInterpolate_TooSmallInterval(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:  "UTC",
		Deadlines: DeadlineInterpolate,
		Window:    ptr(3),
		Schedule: []CourseGroupConfig{
			newGroup("group1", true, jan1,
				map[float64]DeadlineValue{0.5: absDeadline(jan3)},
				absDeadline(jan4), nil),
		},
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error: window too large for interval")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_NegativeMaxSubmissions(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:       "UTC",
		Deadlines:      DeadlineHard,
		MaxSubmissions: ptr(0),
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for non-positive max_submissions")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestDeadlinesValidate_NegativeSubmissionPenalty(t *testing.T) {
	c := CourseDeadlinesConfig{
		Timezone:          "UTC",
		Deadlines:         DeadlineHard,
		SubmissionPenalty: -0.1,
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for negative submission_penalty")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}


func TestCourseUIConfigValidate_Empty(t *testing.T) {
	u := CourseUIConfig{}
	if err := u.Validate(); err != nil {
		t.Errorf("unexpected error for empty UI config: %v", err)
	}
}

func TestCourseUIConfigValidate_ValidURL(t *testing.T) {
	u := CourseUIConfig{TaskURLTemplate: "https://example.com/$GROUP/$TASK"}
	if err := u.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCourseUIConfigValidate_InvalidURL(t *testing.T) {
	u := CourseUIConfig{TaskURLTemplate: "ftp://example.com"}
	err := u.Validate()
	if err == nil {
		t.Fatal("expected error for non-http URL")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}


func TestCourseConfigValidate_Valid(t *testing.T) {
	c := CourseConfig{
		Version: 1,
		Settings: CourseSettingsConfig{
			CourseName:    "test-course",
			GitlabBaseURL: "https://gitlab.com",
			PublicRepo:    "public-course",
			StudentsGroup: "students",
		},
		Deadlines: CourseDeadlinesConfig{
			Timezone:  "UTC",
			Deadlines: DeadlineHard,
		},
	}

	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCourseConfigValidate_MissingCourseName(t *testing.T) {
	c := CourseConfig{
		Version: 1,
		Settings: CourseSettingsConfig{
			GitlabBaseURL: "https://gitlab.com",
			PublicRepo:    "public-course",
			StudentsGroup: "students",
		},
		Deadlines: CourseDeadlinesConfig{
			Timezone:  "UTC",
			Deadlines: DeadlineHard,
		},
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for missing course name")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}

func TestCourseConfigValidate_WrongVersion(t *testing.T) {
	c := CourseConfig{
		Version: 2,
		Settings: CourseSettingsConfig{
			CourseName:    "test",
			GitlabBaseURL: "https://gitlab.com",
			PublicRepo:    "public",
			StudentsGroup: "students",
		},
	}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.IsBadConfig(err) {
		t.Errorf("expected BadConfig, got %T: %v", err, err)
	}
}


func taskNames(tasks []CourseTaskConfig) []string {
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Task
	}
	return names
}
