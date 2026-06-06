package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"jobrunner/internal/config"
	"jobrunner/internal/domain/errors"
)

type Exporter struct {
	course          *Course
	structureConfig config.CheckerStructureConfig
	exportConfig    config.CheckerExportConfig
	repositoryRoot  string
	referenceRoot   string
	subConfigFiles  map[string]*config.CheckerStructureConfig
	verbose         bool
	dryRun          bool
}

func NewExporter(
	course *Course,
	structureConfig config.CheckerStructureConfig,
	exportConfig config.CheckerExportConfig,
	verbose bool,
	dryRun bool,
) *Exporter {
	e := &Exporter{
		course:          course,
		structureConfig: structureConfig,
		exportConfig:    exportConfig,
		repositoryRoot:  course.RepositoryRoot,
		referenceRoot:   course.ReferenceRoot,
		subConfigFiles:  make(map[string]*config.CheckerStructureConfig),
		verbose:         verbose,
		dryRun:          dryRun,
	}

	for _, group := range course.GetGroups(nil) {
		if group.Config.Structure != nil {
			relPath := filepath.FromSlash(group.RelativePath)
			e.subConfigFiles[relPath] = group.Config.Structure
		}
	}

	for _, task := range course.GetTasks(nil) {
		if task.Config.Structure != nil {
			relPath := filepath.FromSlash(task.RelativePath)
			e.subConfigFiles[relPath] = task.Config.Structure
		}
	}

	return e
}

func (e *Exporter) Validate() error {
	if err := e.course.Validate(); err != nil {
		return err
	}

	enabledTasks := e.course.GetTasks(ptrBool(true))
	for _, task := range enabledTasks {
		if err := e.validateTask(task); err != nil {
			return err
		}
	}

	return nil
}

func (e *Exporter) validateTask(task FileSystemTask) error {
	taskFolder := filepath.Join(e.referenceRoot, task.RelativePath)

	hasTemplateFiles := false
	hasValidTemplateFiles := false
	hasTemplateComments := false
	hasValidTemplateComments := false

	if err := filepath.Walk(taskFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, templateSuffix) {
			return nil
		}
		hasTemplateFiles = true
		originalPath := strings.TrimSuffix(path, templateSuffix)
		if _, statErr := os.Stat(originalPath); statErr != nil {
			return errors.NewBadStructure(fmt.Sprintf(
				"template file %s does not have original file %s",
				path, originalPath,
			))
		}
		hasValidTemplateFiles = true
		return nil
	}); err != nil {
		return err
	}

	if err := filepath.Walk(taskFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		fileStr := string(content)
		if !isTextFile(fileStr) {
			return nil
		}

		if strings.Contains(fileStr, templateStartComment) || strings.Contains(fileStr, templateEndComment) {
			hasTemplateComments = true

			startCount := strings.Count(fileStr, templateStartComment)
			endCount := strings.Count(fileStr, templateEndComment)
			if startCount != endCount {
				return errors.NewBadStructure(fmt.Sprintf(
					"task %s has invalid template comments in %s: mismatched begin/end counts",
					task.Name, path,
				))
			}

			for _, match := range templateCommentRegex.FindAllStringSubmatchIndex(fileStr, -1) {
				begin := match[2]
				end := match[3]
				between := fileStr[begin:end]
				if strings.Contains(between, templateStartComment) || strings.Contains(between, templateEndComment) {
					return errors.NewBadStructure(fmt.Sprintf(
						"task %s has nested template comments in %s",
						task.Name, path,
					))
				}
			}

			hasValidTemplateComments = true
		}

		return nil
	}); err != nil {
		return err
	}

	templateType := e.exportConfig.Templates
	switch templateType {
	case config.TemplateTypeSearch:
		if hasTemplateComments {
			return errors.NewBadStructure(fmt.Sprintf(
				"templating set to %s but task %s has template comments",
				templateType, task.Name,
			))
		}
		if !hasValidTemplateFiles {
			return errors.NewBadStructure(fmt.Sprintf(
				"task %s does not have .template file/folder (required for %s mode)",
				task.Name, templateType,
			))
		}
	case config.TemplateTypeCreate:
		if hasTemplateFiles {
			return errors.NewBadStructure(fmt.Sprintf(
				"templating set to %s but task %s has .template file/folder",
				templateType, task.Name,
			))
		}
		if !hasValidTemplateComments {
			return errors.NewBadStructure(fmt.Sprintf(
				"task %s does not have template comments (required for %s mode)",
				task.Name, templateType,
			))
		}
	case config.TemplateTypeSearchOrCreate:
		if hasTemplateFiles && hasTemplateComments {
			return errors.NewBadStructure(fmt.Sprintf(
				"task %s cannot use both .template files and template comments",
				task.Name,
			))
		}
		if !hasValidTemplateFiles && !hasValidTemplateComments {
			return errors.NewBadStructure(fmt.Sprintf(
				"task %s must have either .template files or template comments",
				task.Name,
			))
		}
	}

	return nil
}

func (e *Exporter) ExportPublic(target string, commit bool, commitMessage string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to create target directory: %v", err))
	}

	disabledTasks := make(map[string]bool)
	for _, group := range e.course.GetGroups(ptrBool(false)) {
		disabledTasks[group.RelativePath] = true
	}
	for _, task := range e.course.GetTasks(ptrBool(false)) {
		disabledTasks[task.RelativePath] = true
	}

	var disabledPaths []string
	for path := range disabledTasks {
		disabledPaths = append(disabledPaths, path)
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:             e.referenceRoot,
		destination:      target,
		cfg:              e.structureConfig,
		copyPublic:       true,
		copyPrivate:      false,
		copyOther:        true,
		fillTemplates:    true,
		extraIgnorePaths: disabledPaths,
		globalRoot:       e.referenceRoot,
		globalDest:       target,
	}); err != nil {
		return err
	}

	if commit && !e.dryRun {
		if err := e.commitAndPushRepo(target, commitMessage); err != nil {
			return err
		}
	}

	return nil
}

func (e *Exporter) ExportForTesting(target string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to create target directory: %v", err))
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:          e.repositoryRoot,
		destination:   target,
		cfg:           e.structureConfig,
		copyPublic:    false,
		copyPrivate:   false,
		copyOther:     true,
		fillTemplates: false,
		globalRoot:    e.repositoryRoot,
		globalDest:    target,
	}); err != nil {
		return err
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:          e.referenceRoot,
		destination:   target,
		cfg:           e.structureConfig,
		copyPublic:    true,
		copyPrivate:   true,
		copyOther:     false,
		fillTemplates: false,
		globalRoot:    e.referenceRoot,
		globalDest:    target,
	}); err != nil {
		return err
	}

	return nil
}

func (e *Exporter) ExportForContribution(target string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to create target directory: %v", err))
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:          e.repositoryRoot,
		destination:   target,
		cfg:           e.structureConfig,
		copyPublic:    true,
		copyPrivate:   false,
		copyOther:     true,
		fillTemplates: false,
		globalRoot:    e.repositoryRoot,
		globalDest:    target,
	}); err != nil {
		return err
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:          e.referenceRoot,
		destination:   target,
		cfg:           e.structureConfig,
		copyPublic:    false,
		copyPrivate:   true,
		copyOther:     true,
		fillTemplates: false,
		globalRoot:    e.referenceRoot,
		globalDest:    target,
	}); err != nil {
		return err
	}

	return nil
}

func (e *Exporter) ExportPrivate(target string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to create target directory: %v", err))
	}

	disabledTasks := make(map[string]bool)
	for _, group := range e.course.GetGroups(ptrBool(false)) {
		disabledTasks[group.RelativePath] = true
	}
	for _, task := range e.course.GetTasks(ptrBool(false)) {
		disabledTasks[task.RelativePath] = true
	}

	var disabledPaths []string
	for path := range disabledTasks {
		disabledPaths = append(disabledPaths, path)
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:             e.referenceRoot,
		destination:      target,
		cfg:              e.structureConfig,
		copyPublic:       false,
		copyPrivate:      false,
		copyOther:        true,
		fillTemplates:    true,
		extraIgnorePaths: disabledPaths,
		globalRoot:       e.referenceRoot,
		globalDest:       target,
	}); err != nil {
		return err
	}

	if err := e.copyFilesWithConfig(copyFilesOptions{
		root:             e.referenceRoot,
		destination:      target,
		cfg:              e.structureConfig,
		copyPublic:       true,
		copyPrivate:      true,
		copyOther:        false,
		fillTemplates:    false,
		extraIgnorePaths: disabledPaths,
		globalRoot:       e.referenceRoot,
		globalDest:       target,
	}); err != nil {
		return err
	}

	return nil
}

func (e *Exporter) commitAndPushRepo(repoDir, message string) error {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to open git repository: %v", err))
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to get worktree: %v", err))
	}

	if _, err := worktree.Add("."); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to add files: %v", err))
	}

	_, err = worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Exporter",
			Email: "exporter@example.com",
		},
	})
	if err != nil {
		if err == git.ErrEmptyCommit {
			return nil
		}
		return errors.NewExportError(fmt.Sprintf("failed to commit: %v", err))
	}

	if err := repo.Push(&git.PushOptions{}); err != nil && err.Error() != "already up-to-date" {
		return errors.NewExportError(fmt.Sprintf("failed to push: %v", err))
	}

	return nil
}

func (e *Exporter) mergeConfigs(parent *config.CheckerStructureConfig, child *config.CheckerStructureConfig) config.CheckerStructureConfig {
	result := *parent
	if child.IgnorePatterns != nil {
		result.IgnorePatterns = child.IgnorePatterns
	}
	if child.PrivatePatterns != nil {
		result.PrivatePatterns = child.PrivatePatterns
	}
	if child.PublicPatterns != nil {
		result.PublicPatterns = child.PublicPatterns
	}
	return result
}

func ptrBool(v bool) *bool {
	return &v
}
