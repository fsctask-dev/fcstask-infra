package checker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"jobrunner/internal/config"
	"jobrunner/internal/domain/errors"
)

const (
	templateSuffix         = ".template"
	templateStartComment   = "SOLUTION BEGIN"
	templateEndComment     = "SOLUTION END"
	templateReplaceComment = "TODO: Your solution"
)

var templateCommentRegex = regexp.MustCompile(`(?s)SOLUTION BEGIN(.*?)SOLUTION END`)

func ptr[T any](v T) *T { return &v }

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

	enabledTasks := e.course.GetTasks(ptr(true))
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

	err := filepath.Walk(taskFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), templateSuffix) {
			return nil
		}
		hasTemplateFiles = true
		originalPath := strings.TrimSuffix(path, templateSuffix)
		if _, err := os.Stat(originalPath); err != nil {
			return errors.NewBadStructure(fmt.Sprintf(
				"template file %s does not have original file %s",
				path, originalPath,
			))
		}
		hasValidTemplateFiles = true
		return nil
	})
	if err != nil {
		return err
	}

	err = filepath.Walk(taskFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
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
	})
	if err != nil {
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

func (e *Exporter) disabledPaths() []string {
	seen := make(map[string]bool)
	for _, group := range e.course.GetGroups(ptr(false)) {
		seen[group.RelativePath] = true
	}
	for _, task := range e.course.GetTasks(ptr(false)) {
		seen[task.RelativePath] = true
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	return paths
}

func (e *Exporter) ExportPublic(target string, commit bool, commitMessage string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to create target directory: %v", err))
	}

	disabledPaths := e.disabledPaths()

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

func (e *Exporter) ExportPrivate(target string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to create target directory: %v", err))
	}

	disabledPaths := e.disabledPaths()

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

type copyFilesOptions struct {
	root             string
	destination      string
	cfg              config.CheckerStructureConfig
	copyPublic       bool
	copyPrivate      bool
	copyOther        bool
	fillTemplates    bool
	extraIgnorePaths []string
	globalRoot       string
	globalDest       string
}

func (e *Exporter) copyFilesWithConfig(opts copyFilesOptions) error {
	if opts.globalRoot == "" {
		opts.globalRoot = opts.root
	}
	if opts.globalDest == "" {
		opts.globalDest = opts.destination
	}

	if opts.extraIgnorePaths != nil {
		relPath, err := filepath.Rel(opts.globalRoot, opts.root)
		if err == nil && relPath != "." && slices.Contains(opts.extraIgnorePaths, relPath) {
			return nil
		}
	}

	excludePaths, err := e.searchForExcludeDueToTemplates(opts.root, !opts.fillTemplates)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(opts.root)
	if err != nil {
		return errors.NewExportError(fmt.Sprintf("failed to read directory %s: %v", opts.root, err))
	}

	for _, entry := range entries {
		path := filepath.Join(opts.root, entry.Name())
		pathDestination := filepath.Join(opts.destination, entry.Name())

		if slices.Contains(excludePaths, entry.Name()) {
			continue
		}

		if matchesAnyPattern(opts.cfg.IgnorePatterns, path, opts.globalRoot) {
			continue
		}

		isPublic := matchesAnyPattern(opts.cfg.PublicPatterns, path, opts.globalRoot)
		if !opts.copyPublic && isPublic {
			continue
		}

		isPrivate := !isPublic && matchesAnyPattern(opts.cfg.PrivatePatterns, path, opts.globalRoot)
		if !opts.copyPrivate && isPrivate {
			continue
		}

		if !entry.IsDir() && !isPublic && !isPrivate && !opts.copyOther {
			continue
		}

		if entry.IsDir() {
			if isPublic || isPrivate {
				finalDest := pathDestination
				if opts.fillTemplates && strings.HasSuffix(entry.Name(), templateSuffix) {
					finalDest = filepath.Join(opts.destination, strings.TrimSuffix(entry.Name(), templateSuffix))
				}

				if err := e.copyFilesWithConfig(copyFilesOptions{
					root:             path,
					destination:      finalDest,
					cfg:              opts.cfg,
					copyPublic:       true,
					copyPrivate:      true,
					copyOther:        true,
					fillTemplates:    opts.fillTemplates,
					extraIgnorePaths: opts.extraIgnorePaths,
					globalRoot:       opts.globalRoot,
					globalDest:       opts.globalDest,
				}); err != nil {
					return err
				}
				continue
			}

			subCfg := opts.cfg
			relPath, _ := filepath.Rel(opts.globalRoot, path)
			if subConfigCfg, ok := e.subConfigFiles[relPath]; ok {
				subCfg = e.mergeConfigs(&opts.cfg, subConfigCfg)
			}

			finalDest := pathDestination
			if opts.fillTemplates && strings.HasSuffix(entry.Name(), templateSuffix) {
				finalDest = filepath.Join(opts.destination, strings.TrimSuffix(entry.Name(), templateSuffix))
			}

			if err := e.copyFilesWithConfig(copyFilesOptions{
				root:             path,
				destination:      finalDest,
				cfg:              subCfg,
				copyPublic:       opts.copyPublic,
				copyPrivate:      opts.copyPrivate,
				copyOther:        opts.copyOther,
				fillTemplates:    opts.fillTemplates,
				extraIgnorePaths: opts.extraIgnorePaths,
				globalRoot:       opts.globalRoot,
				globalDest:       opts.globalDest,
			}); err != nil {
				return err
			}
		} else {
			finalDest := pathDestination
			if opts.fillTemplates && strings.HasSuffix(entry.Name(), templateSuffix) {
				finalDest = filepath.Join(opts.destination, strings.TrimSuffix(entry.Name(), templateSuffix))
			}

			if err := os.MkdirAll(filepath.Dir(finalDest), 0755); err != nil {
				return errors.NewExportError(fmt.Sprintf("failed to create directory: %v", err))
			}

			if opts.fillTemplates && hasTemplateComments(path) {
				if err := e.processTemplateComments(path, finalDest); err != nil {
					return err
				}
			} else {
				if err := copyFile(path, finalDest); err != nil {
					return errors.NewExportError(fmt.Sprintf("failed to copy file %s: %v", path, err))
				}
			}
		}
	}

	return nil
}

func (e *Exporter) searchForExcludeDueToTemplates(root string, ignoreTemplates bool) ([]string, error) {
	var excludePaths []string

	if e.exportConfig.Templates == config.TemplateTypeSearch || e.exportConfig.Templates == config.TemplateTypeSearchOrCreate {
		matches, _ := filepath.Glob(filepath.Join(root, "*"+templateSuffix))
		for _, match := range matches {
			name := filepath.Base(match)
			if ignoreTemplates {
				excludePaths = append(excludePaths, name)
			} else {
				excludePaths = append(excludePaths, strings.TrimSuffix(name, templateSuffix))
			}
		}
	}

	if e.exportConfig.Templates == config.TemplateTypeCreate || e.exportConfig.Templates == config.TemplateTypeSearchOrCreate {
		entries, err := os.ReadDir(root)

		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if !isTextFile(string(content)) {
				continue
			}

			fileStr := string(content)
			trimmed := strings.TrimSpace(fileStr)
			if strings.HasPrefix(trimmed, templateStartComment) && strings.HasSuffix(trimmed, templateEndComment) {
				excludePaths = append(excludePaths, entry.Name())
			}
		}
	}

	return excludePaths, nil
}

func (e *Exporter) processTemplateComments(source, destination string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	fileStr := string(content)
	result := templateCommentRegex.ReplaceAllString(fileStr, templateReplaceComment)

	if err := os.WriteFile(destination, []byte(result), 0644); err != nil {
		return err
	}

	if info, err := os.Stat(source); err == nil {
		if err := os.Chmod(destination, info.Mode()); err != nil {
			return errors.NewExportError(fmt.Sprintf("failed to set permissions on %s: %v", destination, err))
		}
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

	if err := repo.Push(&git.PushOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
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

func isTextFile(content string) bool {
	for _, r := range content {
		if r == 0 {
			return false
		}
	}
	return true
}

func hasTemplateComments(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	if !isTextFile(string(content)) {
		return false
	}

	fileStr := string(content)
	return strings.Contains(fileStr, templateStartComment) &&
		strings.Contains(fileStr, templateEndComment)
}

func matchesAnyPattern(patterns []string, filePath, globalRoot string) bool {
	if len(patterns) == 0 {
		return false
	}

	relPath, _ := filepath.Rel(globalRoot, filePath)
	baseName := filepath.Base(filePath)

	for _, pattern := range patterns {
		if matchesPattern(pattern, baseName) || matchesPattern(pattern, relPath) {
			return true
		}
	}

	return false
}

func matchesPattern(pattern, path string) bool {
	matched, _ := filepath.Match(pattern, path)
	return matched
}

func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	if info, err := os.Stat(source); err == nil {
		if err := os.Chmod(destination, info.Mode()); err != nil {
			return errors.NewExportError(fmt.Sprintf("failed to set permissions on %s: %v", destination, err))
		}
	}

	return nil
}
