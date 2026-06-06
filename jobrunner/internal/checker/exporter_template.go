package checker

import (
	"os"
	"regexp"
	"strings"
)

const (
	templateSuffix         = ".template"
	templateStartComment   = "SOLUTION BEGIN"
	templateEndComment     = "SOLUTION END"
	templateReplaceComment = "TODO: Your solution"
)

var templateCommentRegex = regexp.MustCompile(`(?s)SOLUTION BEGIN(.*?)SOLUTION END`)

func (e *Exporter) processTemplateComments(source, destination string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	result := templateCommentRegex.ReplaceAllString(string(content), templateReplaceComment)

	if err := os.WriteFile(destination, []byte(result), 0644); err != nil {
		return err
	}

	if info, err := os.Stat(source); err == nil {
		os.Chmod(destination, info.Mode())
	}

	return nil
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

func isTextFile(content string) bool {
	for _, r := range content {
		if r == 0 {
			return false
		}
	}
	return true
}
