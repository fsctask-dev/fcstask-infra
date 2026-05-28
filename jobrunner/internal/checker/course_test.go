package checker

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"jobrunner/internal/config"
)

func newTestCourseConfig() *config.CourseConfig {
	dur := 365 * 24 * time.Hour * 10
	start := time.Date(2020, 10, 10, 0, 0, 0, 0, time.UTC)

	grp := func(name string, enabled bool, tasks []config.CourseTaskConfig) config.CourseGroupConfig {
		return config.CourseGroupConfig{
			Name:    name,
			Enabled: enabled,
			Start:   start,
			End:     config.DeadlineValue{Duration: &dur},
			Steps:   make(map[float64]config.DeadlineValue),
			Tasks:   tasks,
		}
	}
	task := func(name string) config.CourseTaskConfig {
		return config.CourseTaskConfig{Task: name, Enabled: true, Score: 10}
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
				grp("group1", true, []config.CourseTaskConfig{task("task1_1"), task("task1_2")}),
				grp("group2", false, []config.CourseTaskConfig{task("task2_1"), task("task2_2"), task("task2_3")}),
				grp("group3", true, nil),
				grp("group4", true, []config.CourseTaskConfig{task("task4_1")}),
			},
		},
	}
}

var testFileStructure = map[string]any{
	"group1": map[string]any{
		".group.yml": "",
		"task1_1":    map[string]any{".task.yml": "version: 1", "file1": "", "file2": ""},
		"task1_2":    map[string]any{".task.yml": "", "file1": ""},
		"random_folder": map[string]any{"a": ""},
	},
	"group2": map[string]any{
		".group.yml":    "version: 1",
		"task2_1":       map[string]any{".task.yml": ""},
		"task2_2":       map[string]any{".task.yml": "version: 1"},
		"task2_3":       map[string]any{".task.yml": ""},
		"random_folder": map[string]any{"b": ""},
	},
	"group3": map[string]any{".group.yml": ""},
	"group4": map[string]any{
		".group.yml": "",
		"task4_1":    map[string]any{".task.yml": "version: 1"},
	},
	"random_folder": map[string]any{"c": ""},
}

func setupDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeStructure(t, root, testFileStructure)
	return root
}

func writeStructure(t *testing.T, base string, structure map[string]any) {
	t.Helper()
	for name, content := range structure {
		path := filepath.Join(base, name)
		switch v := content.(type) {
		case map[string]any:
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatalf("mkdir %s: %v", path, err)
			}
			writeStructure(t, path, v)
		case string:
			if err := os.WriteFile(path, []byte(v), 0644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
	}
}

func initGitRepo(t *testing.T, root string) *gogit.Repository {
	t.Helper()
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}
	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := w.Add("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := w.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("initial commit: %v", err)
	}
	return repo
}

func addCommit(t *testing.T, repo *gogit.Repository, root string, files map[string]string, message string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := w.Add("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := w.Commit(message, &gogit.CommitOptions{
		Author:            &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
		AllowEmptyCommits: true,
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func taskNames(tasks []FileSystemTask) []string {
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
	}
	sort.Strings(names)
	return names
}

func groupNames(groups []FileSystemGroup) []string {
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.Name
	}
	sort.Strings(names)
	return names
}

func TestNewCourse(t *testing.T) {
	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	if c.RepositoryRoot != root {
		t.Errorf("RepositoryRoot = %q, want %q", c.RepositoryRoot, root)
	}
	if c.ReferenceRoot != root {
		t.Errorf("ReferenceRoot should default to RepositoryRoot, got %q", c.ReferenceRoot)
	}
}

func TestNewCourseNilConfig(t *testing.T) {
	_, err := NewCourse(nil, "/some/path", "", "")
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewCourseEmptyRoot(t *testing.T) {
	_, err := NewCourse(newTestCourseConfig(), "", "", "")
	if err == nil {
		t.Fatal("expected error for empty repository root")
	}
}

func TestNewCourseInvalidTaskConfig(t *testing.T) {
	root := setupDir(t)
	bad := filepath.Join(root, "group1", "task1_1", TaskConfigName)
	if err := os.WriteFile(bad, []byte("bad: yaml: {{{"), 0644); err != nil {
		t.Fatalf("write bad config: %v", err)
	}
	_, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err == nil {
		t.Fatal("expected error for malformed .task.yml")
	}
}

func TestCourseDiscoversGroups(t *testing.T) {
	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}

	if got := len(c.PotentialGroups); got != 4 {
		t.Errorf("PotentialGroups = %d, want 4", got)
	}
	for _, g := range c.PotentialGroups {
		if _, err := os.Stat(filepath.Join(root, g.RelativePath)); err != nil {
			t.Errorf("group %q: path %q not found on disk", g.Name, g.RelativePath)
		}
	}
}

func TestCourseDiscoversTasks(t *testing.T) {
	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}

	if got := len(c.PotentialTasks); got != 6 {
		t.Errorf("PotentialTasks = %d, want 6", got)
	}
	for _, task := range c.PotentialTasks {
		if _, err := os.Stat(filepath.Join(root, task.RelativePath)); err != nil {
			t.Errorf("task %q: path %q not found on disk", task.Name, task.RelativePath)
		}
	}
}

func TestCourseValidate(t *testing.T) {
	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestCourseValidateMissingTask(t *testing.T) {
	root := setupDir(t)
	if err := os.RemoveAll(filepath.Join(root, "group1", "task1_1")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate to fail for missing task folder")
	}
}

func TestCourseValidateMissingGroup(t *testing.T) {
	root := setupDir(t)
	if err := os.RemoveAll(filepath.Join(root, "group3")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate to fail for missing group folder")
	}
}


func TestCourseGetGroups(t *testing.T) {
	cases := []struct {
		enabled   *bool
		wantNames []string
	}{
		{nil, []string{"group1", "group2", "group3", "group4"}},
		{ptrBool(true), []string{"group1", "group3", "group4"}},
		{ptrBool(false), []string{"group2"}},
	}

	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}

	for _, tc := range cases {
		groups := c.GetGroups(tc.enabled)
		got := groupNames(groups)
		want := append([]string{}, tc.wantNames...)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("GetGroups(%v) = %v, want %v", tc.enabled, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("GetGroups(%v)[%d] = %q, want %q", tc.enabled, i, got[i], want[i])
			}
		}
	}
}


func TestCourseGetTasks(t *testing.T) {
	cases := []struct {
		enabled   *bool
		wantNames []string
	}{
		{nil, []string{"task1_1", "task1_2", "task2_1", "task2_2", "task2_3", "task4_1"}},
		{ptrBool(true), []string{"task1_1", "task1_2", "task4_1"}},
		{ptrBool(false), []string{"task2_1", "task2_2", "task2_3"}},
	}

	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}

	for _, tc := range cases {
		tasks := c.GetTasks(tc.enabled)
		got := taskNames(tasks)
		want := append([]string{}, tc.wantNames...)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("GetTasks(%v) = %v, want %v", tc.enabled, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("GetTasks(%v)[%d] = %q, want %q", tc.enabled, i, got[i], want[i])
			}
		}
	}
}


func TestCourseDetectChangesNotARepo(t *testing.T) {
	root := setupDir(t)
	c, err := NewCourse(newTestCourseConfig(), root, "", "main")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	if _, err := c.DetectChanges(config.ChangesDetectionCommitMessage); err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestCourseDetectChangesByBranchName(t *testing.T) {
	cases := []struct {
		branch    string
		wantTasks []string
		wantErr   bool
	}{
		{"task1_1", []string{"task1_1"}, false},
		{"task1_2", []string{"task1_2"}, false},
		{"task4_1", []string{"task4_1"}, false},
		{"group1", []string{"task1_1", "task1_2"}, false},
		{"group4", []string{"task4_1"}, false},
		{"group3", []string{}, false},
		{"", nil, true},
		{"nonexistent", nil, true},
	}

	root := setupDir(t)

	for _, tc := range cases {
		t.Run("branch="+tc.branch, func(t *testing.T) {
			c, err := NewCourse(newTestCourseConfig(), root, "", tc.branch)
			if err != nil {
				t.Fatalf("NewCourse: %v", err)
			}
			tasks, err := c.DetectChanges(config.ChangesDetectionBranchName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got tasks %v", taskNames(tasks))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := taskNames(tasks)
			want := append([]string{}, tc.wantTasks...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Errorf("tasks = %v, want %v", got, want)
				return
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("tasks[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestCourseDetectChangesByCommitMessage(t *testing.T) {
	cases := []struct {
		message   string
		files     map[string]string
		wantTasks []string
		wantErr   bool
	}{
		{"task1_1 done", map[string]string{"group1/task1_1/file1": "x"}, []string{"task1_1"}, false},
		{"fix task1_2", nil, []string{"task1_2"}, false},
		{"group4 update", nil, []string{"task4_1"}, false},
		{"no match here", nil, nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			root := setupDir(t)
			repo := initGitRepo(t, root)
			addCommit(t, repo, root, tc.files, tc.message)

			c, err := NewCourse(newTestCourseConfig(), root, "", "")
			if err != nil {
				t.Fatalf("NewCourse: %v", err)
			}
			tasks, err := c.DetectChanges(config.ChangesDetectionCommitMessage)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got tasks %v", taskNames(tasks))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := taskNames(tasks)
			want := append([]string{}, tc.wantTasks...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Errorf("tasks = %v, want %v", got, want)
				return
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("tasks[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestCourseDetectChangesByLastCommitChanges(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		wantTasks []string
		wantErr   bool
	}{
		{
			"single task file",
			map[string]string{"group1/task1_1/new.txt": "x"},
			[]string{"task1_1"}, false,
		},
		{
			"two tasks",
			map[string]string{"group1/task1_1/a.txt": "x", "group4/task4_1/b.txt": "x"},
			[]string{"task1_1", "task4_1"}, false,
		},
		{
			"file outside any task",
			map[string]string{"random_folder/x.txt": "x"},
			nil, true,
		},
		{
			"empty commit",
			nil,
			nil, true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupDir(t)
			repo := initGitRepo(t, root)
			addCommit(t, repo, root, tc.files, "test commit")

			c, err := NewCourse(newTestCourseConfig(), root, "", "")
			if err != nil {
				t.Fatalf("NewCourse: %v", err)
			}
			tasks, err := c.DetectChanges(config.ChangesDetectionLastCommitChanges)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got tasks %v", taskNames(tasks))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := taskNames(tasks)
			want := append([]string{}, tc.wantTasks...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Errorf("tasks = %v, want %v", got, want)
				return
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("tasks[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}

func TestCourseDetectChangesByLastCommitInitialCommit(t *testing.T) {
	root := setupDir(t)
	initGitRepo(t, root)

	c, err := NewCourse(newTestCourseConfig(), root, "", "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	if _, err := c.DetectChanges(config.ChangesDetectionLastCommitChanges); err == nil {
		t.Fatal("expected error for initial commit with no parent")
	}
}
