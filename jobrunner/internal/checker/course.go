package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"jobrunner/internal/config"
	"jobrunner/internal/domain/errors"
	"gopkg.in/yaml.v3"
)

type FileSystemTask struct {
	Name         string
	RelativePath string
	Config       config.CheckerSubConfig
}

type FileSystemGroup struct {
	Name         string
	RelativePath string
	Config       config.CheckerSubConfig
	Tasks        []FileSystemTask
}

type Course struct {
	CourseConfig    *config.CourseConfig
	RepositoryRoot  string
	ReferenceRoot   string
	PotentialTasks  map[string]FileSystemTask
	PotentialGroups map[string]FileSystemGroup
	BranchName      string
}

const (
	TaskConfigName  = ".task.yml"
	GroupConfigName = ".group.yml"
)

func NewCourse(courseConfig *config.CourseConfig, repositoryRoot, referenceRoot string, branchName string) (*Course, error) {
	if courseConfig == nil {
		return nil, errors.NewBadConfig("course config is required")
	}

	if repositoryRoot == "" {
		return nil, errors.NewBadConfig("repository root is required")
	}

	if referenceRoot == "" {
		referenceRoot = repositoryRoot
	}

	c := &Course{
		CourseConfig:    courseConfig,
		RepositoryRoot:  repositoryRoot,
		ReferenceRoot:   referenceRoot,
		PotentialTasks:  make(map[string]FileSystemTask),
		PotentialGroups: make(map[string]FileSystemGroup),
		BranchName:      branchName,
	}

	groups, err := c.searchForGroupsByConfigs(referenceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to search for groups: %w", err)
	}
	for _, g := range groups {
		c.PotentialGroups[g.Name] = g
	}

	tasks, err := c.searchForTasksByConfigs(referenceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to search for tasks: %w", err)
	}
	for _, t := range tasks {
		c.PotentialTasks[t.Name] = t
	}

	return c, nil
}

func (c *Course) Validate() error {
	deadlineGroups := c.CourseConfig.Deadlines.GetGroups(nil, nil, nil)
	for _, g := range deadlineGroups {
		if _, ok := c.PotentialGroups[g.Name]; !ok {
			return errors.NewBadStructure(fmt.Sprintf("group %q not found in repository", g.Name))
		}
	}

	deadlineTasks := c.CourseConfig.Deadlines.GetTasks(nil, nil, nil, nil)
	for _, t := range deadlineTasks {
		if _, ok := c.PotentialTasks[t.Task]; !ok {
			return errors.NewBadStructure(fmt.Sprintf("task %q not found in repository", t.Task))
		}
	}

	return nil
}

func (c *Course) GetTasks(enabled *bool) []FileSystemTask {
	deadlineTasks := c.CourseConfig.Deadlines.GetTasks(enabled, nil, nil, nil)
	var result []FileSystemTask

	for _, dt := range deadlineTasks {
		if fsTask, ok := c.PotentialTasks[dt.Task]; ok {
			result = append(result, fsTask)
		}
	}

	return result
}

func (c *Course) GetGroups(enabled *bool) []FileSystemGroup {
	deadlineGroups := c.CourseConfig.Deadlines.GetGroups(enabled, nil, nil)
	var result []FileSystemGroup

	for _, dg := range deadlineGroups {
		if fsGroup, ok := c.PotentialGroups[dg.Name]; ok {
			result = append(result, fsGroup)
		}
	}

	return result
}

func (c *Course) DetectChanges(detectionType config.ChangesDetectionType) ([]FileSystemTask, error) {
	switch detectionType {
	case config.ChangesDetectionBranchName:
		return c.detectChangesByBranchName()
	case config.ChangesDetectionCommitMessage:
		return c.detectChangesByCommitMessage()
	case config.ChangesDetectionLastCommitChanges:
		return c.detectChangesByLastCommitChanges()
	case config.ChangesDetectionFilesChanged:
		return c.detectChangesByFilesChanged()
	default:
		return nil, errors.NewBadConfig(fmt.Sprintf("unknown changes detection type: %s", detectionType))
	}
}

func (c *Course) detectChangesByBranchName() ([]FileSystemTask, error) {
	branchName := c.BranchName
	if branchName == "" {
		return nil, errors.NewExportError("branch name not set")
	}

	if group, ok := c.PotentialGroups[branchName]; ok {
		return group.Tasks, nil
	}

	if task, ok := c.PotentialTasks[branchName]; ok {
		return []FileSystemTask{task}, nil
	}

	return nil, errors.NewExportError(fmt.Sprintf("no group or task matches branch name %q", branchName))
}

func (c *Course) detectChangesByCommitMessage() ([]FileSystemTask, error) {
	repo, err := git.PlainOpen(c.RepositoryRoot)
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to open git repo: %v", err))
	}

	head, err := repo.Head()
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get HEAD: %v", err))
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get commit: %v", err))
	}

	message := commit.Message

	for groupName, group := range c.PotentialGroups {
		if strings.Contains(message, groupName) {
			return group.Tasks, nil
		}
	}

	for taskName, task := range c.PotentialTasks {
		if strings.Contains(message, taskName) {
			return []FileSystemTask{task}, nil
		}
	}

	return nil, errors.NewExportError(fmt.Sprintf("no group or task matches commit message: %s", message))
}

func (c *Course) detectChangesByLastCommitChanges() ([]FileSystemTask, error) {
	repo, err := git.PlainOpen(c.RepositoryRoot)
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to open git repo: %v", err))
	}

	head, err := repo.Head()
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get HEAD: %v", err))
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get commit: %v", err))
	}

	if commit.NumParents() == 0 {
		return nil, errors.NewExportError("initial commit has no parent")
	}

	parentCommit, err := repo.CommitObject(commit.ParentHashes[0])
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get parent commit: %v", err))
	}

	patch, err := commit.Patch(parentCommit)
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get patch: %v", err))
	}

	return c.getTasksFromChangedFiles(patch.String())
}

func (c *Course) detectChangesByFilesChanged() ([]FileSystemTask, error) {
	repo, err := git.PlainOpen(c.RepositoryRoot)
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to open git repo: %v", err))
	}

	head, err := repo.Head()
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get HEAD: %v", err))
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get commit: %v", err))
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, errors.NewExportError(fmt.Sprintf("failed to get tree: %v", err))
	}

	var patch string
	if commit.NumParents() > 0 {
		parentCommit, err := repo.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return nil, errors.NewExportError(fmt.Sprintf("failed to get parent commit: %v", err))
		}

		p, err := commit.Patch(parentCommit)
		if err != nil {
			return nil, errors.NewExportError(fmt.Sprintf("failed to get patch: %v", err))
		}
		patch = p.String()
	}

	_ = tree
	return c.getTasksFromChangedFiles(patch)
}

func (c *Course) getTasksFromChangedFiles(patch string) ([]FileSystemTask, error) {
	changedTasks := make(map[string]FileSystemTask)

	for taskName, task := range c.PotentialTasks {
		if strings.Contains(patch, task.RelativePath) {
			changedTasks[taskName] = task
		}
	}

	if len(changedTasks) == 0 {
		return nil, errors.NewExportError("no tasks found in changed files")
	}

	result := make([]FileSystemTask, 0, len(changedTasks))
	for _, task := range changedTasks {
		result = append(result, task)
	}

	return result, nil
}

func (c *Course) searchForTasksByConfigs(root string) ([]FileSystemTask, error) {
	var tasks []FileSystemTask

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == TaskConfigName {
			parentDir := filepath.Dir(path)
			taskName := filepath.Base(parentDir)
			relativePath, relErr := filepath.Rel(root, parentDir)
			if relErr != nil {
				return fmt.Errorf("failed to get relative path: %w", relErr)
			}

			var subConfig config.CheckerSubConfig
			configData, readErr := os.ReadFile(path)
			if readErr != nil {
				return errors.NewBadStructure(fmt.Sprintf("failed to read task config %s: %v", path, readErr))
			}

			if len(configData) > 0 {
				if yamlErr := yaml.Unmarshal(configData, &subConfig); yamlErr != nil {
					return errors.NewBadStructure(fmt.Sprintf("failed to parse task config %s: %v", path, yamlErr))
				}
			}

			subConfig.SetDefaults()

			if validateErr := subConfig.Validate(); validateErr != nil {
				return validateErr
			}

			tasks = append(tasks, FileSystemTask{
				Name:         taskName,
				RelativePath: relativePath,
				Config:       subConfig,
			})
		}

		return nil
	})

	return tasks, err
}

func (c *Course)  searchForGroupsByConfigs(root string) ([]FileSystemGroup, error) {
	var groups []FileSystemGroup
	groupPaths := make(map[string]string)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == GroupConfigName {
			parentDir := filepath.Dir(path)
			groupName := filepath.Base(parentDir)
			groupPaths[groupName] = parentDir
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	for groupName, groupPath := range groupPaths {
		var groupConfig config.CheckerSubConfig
		configPath := filepath.Join(groupPath, GroupConfigName)
		configData, readErr := os.ReadFile(configPath)
		if readErr != nil {
			return nil, errors.NewBadStructure(fmt.Sprintf("failed to read group config %s: %v", configPath, readErr))
		}

		if len(configData) > 0 {
			if yamlErr := yaml.Unmarshal(configData, &groupConfig); yamlErr != nil {
				return nil, errors.NewBadStructure(fmt.Sprintf("failed to parse group config %s: %v", configPath, yamlErr))
			}
		}

		groupConfig.SetDefaults()

		if validateErr := groupConfig.Validate(); validateErr != nil {
			return nil, validateErr
		}

		relativePath, relErr := filepath.Rel(root, groupPath)
		if relErr != nil {
			return nil, fmt.Errorf("failed to get relative path for group: %w", relErr)
		}

		var groupTasks []FileSystemTask
		err := filepath.Walk(groupPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && info.Name() == TaskConfigName {
				parentDir := filepath.Dir(path)
				taskName := filepath.Base(parentDir)
				taskRelativePath, relErr := filepath.Rel(root, parentDir)
				if relErr != nil {
					return errors.NewBadStructure(fmt.Sprintf("failed to get relative path: %v", relErr))
				}

				var taskConfig config.CheckerSubConfig
				taskConfigData, readErr := os.ReadFile(path)
				if readErr != nil {
					return errors.NewBadStructure(fmt.Sprintf("failed to read task config %s: %v", path, readErr))
				}

				if len(taskConfigData) > 0 {
					if yamlErr := yaml.Unmarshal(taskConfigData, &taskConfig); yamlErr != nil {
						return errors.NewBadStructure(fmt.Sprintf("failed to parse task config %s: %v", path, yamlErr))
					}
				}

				taskConfig.SetDefaults()

				if validateErr := taskConfig.Validate(); validateErr != nil {
					return validateErr
				}

				groupTasks = append(groupTasks, FileSystemTask{
					Name:         taskName,
					RelativePath: taskRelativePath,
					Config:       taskConfig,
				})
			}

			return nil
		})

		if err != nil {
			return nil, err
		}

		groups = append(groups, FileSystemGroup{
			Name:         groupName,
			RelativePath: relativePath,
			Config:       groupConfig,
			Tasks:        groupTasks,
		})
	}

	return groups, nil
}
