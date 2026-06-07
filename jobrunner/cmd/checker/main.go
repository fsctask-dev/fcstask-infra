package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"

	"jobrunner/internal/checker"
	"jobrunner/internal/config"
)

var (
	refDir        string
	repoDir       string
	courseConfig  string
	checkerConfig string
	branch        string
	verbose       bool
	dryRun        bool
)

func main() {
	root := &cobra.Command{
		Use:           "checker",
		Short:         "Automated checker for student assignments",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if refDir == "" {
				return fmt.Errorf("--ref-dir is required")
			}
			var err error
			if refDir, err = filepath.Abs(refDir); err != nil {
				return fmt.Errorf("resolve --ref-dir: %w", err)
			}
			if repoDir, err = filepath.Abs(repoDir); err != nil {
				return fmt.Errorf("resolve --repo-dir: %w", err)
			}
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&refDir, "ref-dir", "", "path to reference (private) repository (required)")
	pf.StringVar(&repoDir, "repo-dir", ".", "path to student repository (default: current directory)")
	pf.StringVar(&courseConfig, "course-config", "", "course.yaml path; default: <ref-dir>/course.yaml")
	pf.StringVar(&checkerConfig, "checker-config", "", "checker config path; default: <ref-dir>/.checker/config.yaml")
	pf.StringVar(&branch, "branch", "", "branch name for change detection; auto-detected from repo-dir if empty")
	pf.BoolVar(&verbose, "verbose", false, "verbose pipeline output")
	pf.BoolVar(&dryRun, "dry-run", false, "print resolved stages without executing plugins")

	root.AddCommand(gradeCmd(), checkCmd(), exportCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "checker: %v\n", err)
		os.Exit(1)
	}
}

func gradeCmd() *cobra.Command {
	var submitScore bool

	cmd := &cobra.Command{
		Use:   "grade",
		Short: "Detect changed tasks and run tests (student CI)",
		Long: `Detect tasks changed in the last commit and run the test pipeline.
With --submit-score, runs the report pipeline and submits scores.

Environment variables used by the report pipeline:
  GITLAB_USER_LOGIN   student username passed to report_score
  TESTER_TOKEN        authentication token for the server

Examples:
  checker grade --ref-dir /opt/shad
  checker grade --ref-dir /opt/shad --submit-score`,
		RunE: func(_ *cobra.Command, _ []string) error {
			course, checkerCfg, err := loadCourse()
			if err != nil {
				return err
			}

			tasks, err := course.DetectChanges(checkerCfg.Testing.ChangesDetection)
			if err != nil {
				return fmt.Errorf("detect changed tasks: %w", err)
			}
			if len(tasks) == 0 {
				fmt.Println("no changed tasks detected")
				return nil
			}

			return runTester(course, checkerCfg, tasks, submitScore)
		},
	}

	cmd.Flags().BoolVar(&submitScore, "submit-score", false,
		"run report pipeline and submit score after passing")
	return cmd
}

func checkCmd() *cobra.Command {
	var task string
	var all  bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run task tests without reporting (tutor CI or local)",
		Long: `Run tests for a specific task, all enabled tasks, or detect changes.
Does not submit scores.

Examples:
  checker check --ref-dir ./ref-repo --task hello_world --verbose
  checker check --ref-dir ./ref-repo --all
  checker check --ref-dir ./ref-repo --dry-run`,
		RunE: func(_ *cobra.Command, _ []string) error {
			course, checkerCfg, err := loadCourse()
			if err != nil {
				return err
			}

			var tasks []checker.FileSystemTask
			switch {
			case task != "":
				t, ok := course.PotentialTasks[task]
				if !ok {
					return fmt.Errorf("task %q not found; available: %v", task, availableTaskNames(course))
				}
				tasks = []checker.FileSystemTask{t}
			case all:
				enabled := true
				tasks = course.GetTasks(&enabled)
			default:
				tasks, err = course.DetectChanges(checkerCfg.Testing.ChangesDetection)
				if err != nil {
					return fmt.Errorf("detect changed tasks: %w", err)
				}
			}

			if len(tasks) == 0 {
				fmt.Println("no tasks to test")
				return nil
			}

			return runTester(course, checkerCfg, tasks, false)
		},
	}

	cmd.Flags().StringVar(&task, "task", "", "specific task name to run")
	cmd.Flags().BoolVar(&all, "all", false, "run all enabled tasks (default: detect changes)")
	return cmd
}

func runTester(course *checker.Course, checkerCfg *config.CheckerConfig, tasks []checker.FileSystemTask, report bool) error {
	tester, err := checker.NewTester(course, checkerCfg, checker.DefaultRegistry(), verbose, dryRun)
	if err != nil {
		return fmt.Errorf("build tester: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "checker-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	now := time.Now()
	if err := tester.Run(tempDir, tasks, report, &now); err != nil {
		return err
	}

	fmt.Println("OK")
	return nil
}

func loadCourse() (*checker.Course, *config.CheckerConfig, error) {
	resolve := func(explicit, rel string) string {
		if explicit != "" {
			return explicit
		}
		return filepath.Join(refDir, rel)
	}

	courseCfg, err := config.LoadConfig[config.CourseConfig, *config.CourseConfig](resolve(courseConfig, "course.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("load course config: %w", err)
	}

	checkerCfg, err := config.LoadConfig[config.CheckerConfig, *config.CheckerConfig](resolve(checkerConfig, ".checker/config.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("load checker config: %w", err)
	}
	checkerCfg.SetDefaults()

	branchName := branch
	if branchName == "" {
		branchName = currentBranch(repoDir)
	}

	course, err := checker.NewCourse(courseCfg, repoDir, refDir, branchName)
	if err != nil {
		return nil, nil, fmt.Errorf("build course: %w", err)
	}

	return course, checkerCfg, nil
}

func currentBranch(dir string) string {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	return head.Name().Short()
}


func availableTaskNames(course *checker.Course) []string {
	names := make([]string, 0, len(course.PotentialTasks))
	for n := range course.PotentialTasks {
		names = append(names, n)
	}
	return names
}

func exportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export tasks from ref repo (tutor operation)",
	}
	cmd.AddCommand(exportPublicCmd(), exportTestingCmd(), exportPrivateCmd())
	return cmd
}

func exportPublicCmd() *cobra.Command {
	var target string
	var commit bool

	cmd := &cobra.Command{
		Use:   "public",
		Short: "Export public student repo, stripping solutions and applying templates",
		Long: `Copy enabled tasks from ref repo to target directory.
Solution blocks (SOLUTION BEGIN / SOLUTION END) are stripped.
.template files replace the originals.

Examples:
  checker export public --ref-dir ./ref --target ./student-repo
  checker export public --ref-dir ./ref --target ./student-repo --commit`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if target == "" {
				return fmt.Errorf("--target is required")
			}
			course, checkerCfg, err := loadCourse()
			if err != nil {
				return err
			}
			targetAbs, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolve --target: %w", err)
			}
			exp := checker.NewExporter(course, *checkerCfg.Structure, *checkerCfg.Export, verbose, dryRun)
			if err := exp.Validate(); err != nil {
				return fmt.Errorf("export validation: %w", err)
			}
			if err := exp.ExportPublic(targetAbs, commit, checkerCfg.Export.CommitMessage); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "destination directory (required)")
	cmd.Flags().BoolVar(&commit, "commit", false, "commit and push to remote after export")
	return cmd
}

func exportTestingCmd() *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "testing",
		Short: "Merge ref and student repos into target for local testing",
		RunE: func(_ *cobra.Command, _ []string) error {
			if target == "" {
				return fmt.Errorf("--target is required")
			}
			course, checkerCfg, err := loadCourse()
			if err != nil {
				return err
			}
			targetAbs, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolve --target: %w", err)
			}
			exp := checker.NewExporter(course, *checkerCfg.Structure, *checkerCfg.Export, verbose, dryRun)
			if err := exp.ExportForTesting(targetAbs); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "destination directory (required)")
	return cmd
}

func exportPrivateCmd() *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "private",
		Short: "Export clean copy of ref repo (enabled tasks only)",
		RunE: func(_ *cobra.Command, _ []string) error {
			if target == "" {
				return fmt.Errorf("--target is required")
			}
			course, checkerCfg, err := loadCourse()
			if err != nil {
				return err
			}
			targetAbs, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolve --target: %w", err)
			}
			exp := checker.NewExporter(course, *checkerCfg.Structure, *checkerCfg.Export, verbose, dryRun)
			if err := exp.ExportPrivate(targetAbs); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "destination directory (required)")
	return cmd
}
