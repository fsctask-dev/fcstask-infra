package checker

import (
	"encoding/json"
	"fmt"

	"jobrunner/internal/domain/errors"
)

type PluginOutput struct {
	Output     string
	Percentage float64
}

type Plugin interface {
	Run(args map[string]any, verbose bool) (*PluginOutput, error)
}

type PluginRegistry map[string]Plugin

func DefaultRegistry() PluginRegistry {
	return PluginRegistry{
		"run_script":   &RunScriptPlugin{},
		"run_tests":    &RunTestsPlugin{},
		"report_score": &ReportScorePlugin{},
	}
}

func parseArgs[T any](args map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(args)
	if err != nil {
		return result, errors.NewBadConfig(fmt.Sprintf("failed to marshal args: %v", err))
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, errors.NewBadConfig(fmt.Sprintf("invalid args: %v", err))
	}
	return result, nil
}
