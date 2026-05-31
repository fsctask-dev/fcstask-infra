package checker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"jobrunner/internal/config"
)

func TestIsTextFile(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"hello world", true},
		{"", true},
		{"line1\nline2\n", true},
		{"has\x00null", false},
	}
	for _, c := range cases {
		if got := isTextFile(c.input); got != c.want {
			t.Errorf("isTextFile(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestMatchesPattern(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"*.py", "solution.py", true},
		{"*.py", "solution.go", false},
		{".*", ".gitignore", true},
		{".*", "gitignore", false},
		{"*", "anything.txt", true},
		{"exact", "exact", true},
		{"exact", "notexact", false},
	}
	for _, c := range cases {
		if got := matchesPattern(c.pattern, c.path); got != c.want {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	root := t.TempDir()

	cases := []struct {
		patterns []string
		file     string
		want     bool
	}{
		{[]string{"*.py", "*.go"}, filepath.Join(root, "solution.py"), true},
		{[]string{"*.py", "*.go"}, filepath.Join(root, "readme.md"), false},
		{nil, filepath.Join(root, "anything"), false},
		{[]string{}, filepath.Join(root, "anything"), false},
		{[]string{".*"}, filepath.Join(root, ".gitignore"), true},
	}
	for _, c := range cases {
		if got := matchesAnyPattern(c.patterns, c.file, root); got != c.want {
			t.Errorf("matchesAnyPattern(%v, %q) = %v, want %v", c.patterns, c.file, got, c.want)
		}
	}
}

func TestMergeConfigs(t *testing.T) {
	e := &Exporter{}
	parent := &config.CheckerStructureConfig{
		IgnorePatterns:  []string{".git"},
		PublicPatterns:  []string{"*.md"},
		PrivatePatterns: []string{"*.py"},
	}

	t.Run("child overrides all fields", func(t *testing.T) {
		child := &config.CheckerStructureConfig{
			IgnorePatterns:  []string{"*.pyc"},
			PublicPatterns:  []string{"*.txt"},
			PrivatePatterns: []string{"*.go"},
		}
		got := e.mergeConfigs(parent, child)
		if got.IgnorePatterns[0] != "*.pyc" {
			t.Errorf("expected *.pyc, got %v", got.IgnorePatterns)
		}
		if got.PublicPatterns[0] != "*.txt" {
			t.Errorf("expected *.txt, got %v", got.PublicPatterns)
		}
		if got.PrivatePatterns[0] != "*.go" {
			t.Errorf("expected *.go, got %v", got.PrivatePatterns)
		}
	})

	t.Run("nil child fields keep parent values", func(t *testing.T) {
		child := &config.CheckerStructureConfig{}
		got := e.mergeConfigs(parent, child)
		if got.IgnorePatterns[0] != ".git" {
			t.Errorf("expected .git, got %v", got.IgnorePatterns)
		}
		if got.PublicPatterns[0] != "*.md" {
			t.Errorf("expected *.md, got %v", got.PublicPatterns)
		}
	})

	t.Run("parent is not mutated", func(t *testing.T) {
		child := &config.CheckerStructureConfig{PublicPatterns: []string{"*.txt"}}
		_ = e.mergeConfigs(parent, child)
		if parent.PublicPatterns[0] != "*.md" {
			t.Error("parent PublicPatterns was mutated")
		}
	})
}

func TestProcessTemplateComments_Single(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.py")
	dst := filepath.Join(dir, "dst.py")

	content := "def foo():\n    # SOLUTION BEGIN\n    return 42\n    # SOLUTION END\n"
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := &Exporter{}
	if err := e.processTemplateComments(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "def foo():\n    # TODO: Your solution\n" {
		t.Errorf("unexpected output:\n%s", got)
	}
}

func TestProcessTemplateComments_Multiple(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.py")
	dst := filepath.Join(dir, "dst.py")

	content := "SOLUTION BEGIN\npart1\nSOLUTION END\nSOLUTION BEGIN\npart2\nSOLUTION END\n"
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := &Exporter{}
	if err := e.processTemplateComments(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dst)
	expected := "TODO: Your solution\nTODO: Your solution\n"
	if string(got) != expected {
		t.Errorf("got:\n%q\nwant:\n%q", string(got), expected)
	}
}

func TestProcessTemplateComments_NoComments(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.py")
	dst := filepath.Join(dir, "dst.py")

	content := "def foo():\n    return 42\n"
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := &Exporter{}
	if err := e.processTemplateComments(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != content {
		t.Errorf("expected unchanged content, got:\n%s", got)
	}
}


func singleTaskCourseConfig() *config.CourseConfig {
	dur := 365 * 24 * time.Hour * 10
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
					Name:    "g1",
					Enabled: true,
					Start:   start,
					End:     config.DeadlineValue{Duration: &dur},
					Steps:   make(map[float64]config.DeadlineValue),
					Tasks: []config.CourseTaskConfig{
						{Task: "t1", Enabled: true, Score: 10},
					},
				},
			},
		},
	}
}

func buildExporter(t *testing.T, tree map[string]any, structCfg config.CheckerStructureConfig, exportCfg config.CheckerExportConfig) (*Exporter, string) {
	t.Helper()
	refRoot := t.TempDir()

	writeStructure(t, refRoot, map[string]any{
		"g1": map[string]any{
			".group.yml": "version: 1",
			"t1":         map[string]any{".task.yml": "version: 1"},
		},
	})
	writeStructure(t, refRoot, tree)

	course, err := NewCourse(singleTaskCourseConfig(), refRoot, refRoot, "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}

	e := NewExporter(course, structCfg, exportCfg, false, false)
	return e, refRoot
}

func simpleCfg() config.CheckerStructureConfig {
	return config.CheckerStructureConfig{
		IgnorePatterns:  []string{".git"},
		PublicPatterns:  []string{"*.md"},
		PrivatePatterns: []string{"*.private"},
	}
}


func TestValidateTask_SearchMode_Valid(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"src": map[string]any{
					"solution.py":          "def foo(): return 1",
					"solution.py.template": "# template",
				},
			},
		},
	}
	e, refRoot := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeSearch,
	})
	_ = refRoot
	task := e.course.PotentialTasks["t1"]
	if err := e.validateTask(task); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateTask_SearchMode_MissingOriginal(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py.template": "# template",
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeSearch,
	})
	task := e.course.PotentialTasks["t1"]
	if err := e.validateTask(task); err == nil {
		t.Error("expected error for template without original")
	}
}

func TestValidateTask_SearchMode_NoTemplateFile(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py": "def foo(): return 1",
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeSearch,
	})
	task := e.course.PotentialTasks["t1"]
	if err := e.validateTask(task); err == nil {
		t.Error("expected error: search mode requires .template file")
	}
}

func TestValidateTask_CreateMode_Valid(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py": "def foo():\n    # SOLUTION BEGIN\n    return 1\n    # SOLUTION END\n",
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	task := e.course.PotentialTasks["t1"]
	if err := e.validateTask(task); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateTask_CreateMode_MismatchedComments(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py": "# SOLUTION BEGIN\ncode\n",
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	task := e.course.PotentialTasks["t1"]
	if err := e.validateTask(task); err == nil {
		t.Error("expected error for mismatched BEGIN/END")
	}
}

func TestValidateTask_CreateMode_TemplateFileForbidden(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py": "# SOLUTION BEGIN\ncode\n# SOLUTION END\n",
				"src": map[string]any{
					"solution.py.template": "# template",
				},
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	task := e.course.PotentialTasks["t1"]
	if err := e.validateTask(task); err == nil {
		t.Error("expected error: create mode forbids .template files")
	}
}


func filesInDir(t *testing.T, root string) map[string]bool {
	t.Helper()
	result := make(map[string]bool)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		result[rel] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return result
}

func TestExportPublic_CopiesPublicAndOther(t *testing.T) {
	tree := map[string]any{
		"readme.md": "public file",
		"data.txt":  "other file",
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	target := t.TempDir()
	if err := e.ExportPublic(target, false, ""); err != nil {
		t.Fatalf("ExportPublic: %v", err)
	}

	files := filesInDir(t, target)
	if !files["readme.md"] {
		t.Error("expected readme.md (public) to be exported")
	}
	if !files["data.txt"] {
		t.Error("expected data.txt (other) to be exported")
	}
}

func TestExportPublic_SkipsPrivate(t *testing.T) {
	tree := map[string]any{
		"readme.md":    "public",
		"secret.private": "private",
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	target := t.TempDir()
	if err := e.ExportPublic(target, false, ""); err != nil {
		t.Fatalf("ExportPublic: %v", err)
	}

	files := filesInDir(t, target)
	if files["secret.private"] {
		t.Error("private file should not be exported")
	}
	if !files["readme.md"] {
		t.Error("public file should be exported")
	}
}

func TestExportPublic_SkipsIgnored(t *testing.T) {
	tree := map[string]any{
		"readme.md":  "public",
		".git":       "ignored",
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	target := t.TempDir()
	if err := e.ExportPublic(target, false, ""); err != nil {
		t.Fatalf("ExportPublic: %v", err)
	}

	files := filesInDir(t, target)
	if files[".git"] {
		t.Error("ignored file should not be exported")
	}
}

func TestExportPublic_FillsTemplateComments(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py": "def f():\n    # SOLUTION BEGIN\n    return 1\n    # SOLUTION END\n",
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	})
	target := t.TempDir()
	if err := e.ExportPublic(target, false, ""); err != nil {
		t.Fatalf("ExportPublic: %v", err)
	}

	exported := filepath.Join(target, "g1", "t1", "solution.py")
	content, err := os.ReadFile(exported)
	if err != nil {
		t.Fatalf("solution.py not found in export: %v", err)
	}
	if string(content) != "def f():\n    # TODO: Your solution\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestExportPublic_TemplateSearchMode_ReplacesOriginalWithTemplate(t *testing.T) {
	tree := map[string]any{
		"g1": map[string]any{
			"t1": map[string]any{
				"solution.py":          "original content",
				"solution.py.template": "template content",
			},
		},
	}
	e, _ := buildExporter(t, tree, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeSearch,
	})
	target := t.TempDir()
	if err := e.ExportPublic(target, false, ""); err != nil {
		t.Fatalf("ExportPublic: %v", err)
	}

	files := filesInDir(t, target)
	exported := filepath.Join(target, "g1", "t1", "solution.py")
	if !files[filepath.Join("g1", "t1", "solution.py")] {
		t.Fatal("expected solution.py to exist (from template)")
	}
	if files[filepath.Join("g1", "t1", "solution.py.template")] {
		t.Error("solution.py.template should not appear in export")
	}
	content, _ := os.ReadFile(exported)
	if string(content) != "template content" {
		t.Errorf("expected template content, got %q", content)
	}
}

func TestExportForTesting_CopiesOtherFromRepoAndPublicPrivateFromRef(t *testing.T) {
	repoRoot := t.TempDir()
	refRoot := t.TempDir()

	writeStructure(t, refRoot, map[string]any{
		"g1": map[string]any{
			".group.yml": "version: 1",
			"t1":         map[string]any{".task.yml": "version: 1"},
		},
		"readme.md":    "public from ref",
		"private.private": "private from ref",
	})
	writeStructure(t, repoRoot, map[string]any{
		"g1": map[string]any{
			".group.yml": "version: 1",
			"t1":         map[string]any{".task.yml": "version: 1"},
		},
		"student.txt": "student work",
	})

	course, err := NewCourse(singleTaskCourseConfig(), repoRoot, refRoot, "")
	if err != nil {
		t.Fatalf("NewCourse: %v", err)
	}
	e := NewExporter(course, simpleCfg(), config.CheckerExportConfig{
		Destination: "https://example.com",
		Templates:   config.TemplateTypeCreate,
	}, false, false)

	target := t.TempDir()
	if err := e.ExportForTesting(target); err != nil {
		t.Fatalf("ExportForTesting: %v", err)
	}

	files := filesInDir(t, target)
	if !files["student.txt"] {
		t.Error("expected student.txt (other from repo) in test export")
	}
	if !files["readme.md"] {
		t.Error("expected readme.md (public from ref) in test export")
	}
	if !files["private.private"] {
		t.Error("expected private.private (private from ref) in test export")
	}
}
