package checker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"jobrunner/internal/config"
	"jobrunner/internal/domain/errors"
)

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
		if err == nil && relPath != "." {
			for _, ignorePath := range opts.extraIgnorePaths {
				if relPath == ignorePath {
					return nil
				}
			}
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
		if entry.Name() == ".git" {
			continue
		}

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
		entries, _ := os.ReadDir(root)
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

			trimmed := strings.TrimSpace(string(content))
			if strings.HasPrefix(trimmed, templateStartComment) && strings.HasSuffix(trimmed, templateEndComment) {
				excludePaths = append(excludePaths, entry.Name())
			}
		}
	}

	return excludePaths, nil
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
	defer func() { _ = src.Close() }()

	dst, err := os.Create(destination)
	if err != nil {
		return err
	}

	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}

	if err = dst.Close(); err != nil {
		return err
	}

	if info, err := os.Stat(source); err == nil {
		_ = os.Chmod(destination, info.Mode())
	}

	return nil
}
