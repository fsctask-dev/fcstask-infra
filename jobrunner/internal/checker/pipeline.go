package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"jobrunner/internal/config"
	domainerrors "jobrunner/internal/domain/errors"
)

type PipelineStageResult struct {
	Name        string  `json:"name"`
	Failed      bool    `json:"failed"`
	Skipped     bool    `json:"skipped"`
	Percentage  float64 `json:"percentage"`
	ElapsedTime float64 `json:"elapsed_time"`
	Output      string  `json:"output"`
}

type PipelineResult struct {
	Failed       bool
	StageResults []PipelineStageResult
}

func (r *PipelineResult) Succeeded() bool { return !r.Failed }

type ParametersResolver struct{}

func (r *ParametersResolver) Resolve(value any, ctx map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		return r.resolveString(v, ctx)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			resolved, err := r.Resolve(item, ctx)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			resolved, err := r.Resolve(val, ctx)
			if err != nil {
				return nil, err
			}
			result[k] = resolved
		}
		return result, nil
	default:
		return value, nil
	}
}

func (r *ParametersResolver) ResolveBool(value any, ctx map[string]any) (bool, error) {
	if b, ok := value.(bool); ok {
		return b, nil
	}
	resolved, err := r.Resolve(value, ctx)
	if err != nil {
		return false, err
	}
	s, ok := resolved.(string)
	if !ok {
		return false, domainerrors.NewBadConfig(fmt.Sprintf("run_if must resolve to bool, got %T", resolved))
	}
	b, err := strconv.ParseBool(strings.TrimSpace(s))
	if err != nil {
		return false, domainerrors.NewBadConfig(fmt.Sprintf("run_if resolved to %q, expected true or false", s))
	}
	return b, nil
}

func (r *ParametersResolver) resolveString(tmpl string, ctx map[string]any) (any, error) {
	if !strings.Contains(tmpl, "${{") {
		return tmpl, nil
	}
	t, err := template.New("").Delims("${{", "}}").Parse(tmpl)
	if err != nil {
		return nil, domainerrors.NewBadConfig(fmt.Sprintf("invalid template %q: %v", tmpl, err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return nil, domainerrors.NewBadConfig(fmt.Sprintf("failed to render template %q: %v", tmpl, err))
	}
	return buf.String(), nil
}

type PipelineRunner struct {
	pipeline []config.PipelineStageConfig
	registry PluginRegistry
	verbose  bool
	resolver ParametersResolver
}

func NewPipelineRunner(pipeline []config.PipelineStageConfig, registry PluginRegistry, verbose bool) (*PipelineRunner, error) {
	for _, stage := range pipeline {
		if _, ok := registry[stage.Run]; !ok {
			return nil, domainerrors.NewBadConfig(fmt.Sprintf("unknown plugin %q in stage %q", stage.Run, stage.Name))
		}
	}
	return &PipelineRunner{pipeline: pipeline, registry: registry, verbose: verbose}, nil
}

func (r *PipelineRunner) Len() int { return len(r.pipeline) }

func (r *PipelineRunner) Run(ctx map[string]any, dryRun bool) (*PipelineResult, error) {
	var stageResults []PipelineStageResult
	pipelinePassed := true
	skipTheRest := false

	for _, stage := range r.pipeline {
		resolvedArgs, err := r.resolveArgs(stage.Args, ctx)
		if err != nil {
			return nil, err
		}

		runIf := true
		if stage.RunIf != nil {
			runIf, err = r.resolver.ResolveBool(stage.RunIf, ctx)
			if err != nil {
				return nil, err
			}
		}

		if !r.verbose && (skipTheRest || !runIf) {
			continue
		}

		if skipTheRest {
			stageResults = append(stageResults, PipelineStageResult{Name: stage.Name, Skipped: true})
			if r.verbose {
				fmt.Printf("  -- %s (skipped)\n", stage.Name)
			}
			continue
		}

		if !runIf {
			stageResults = append(stageResults, PipelineStageResult{Name: stage.Name, Skipped: true})
			if r.verbose {
				fmt.Printf("  -- %s (run_if=false)\n", stage.Name)
			}
			continue
		}

		plugin := r.registry[stage.Run]

		if dryRun {
			result := PipelineStageResult{Name: stage.Name, Percentage: 1.0}
			stageResults = append(stageResults, result)
			if stage.RegisterOutput != nil {
				registerOutput(ctx, *stage.RegisterOutput, result)
			}
			if r.verbose {
				fmt.Printf("  ** %s (dry-run)\n", stage.Name)
			}
			continue
		}

		if r.verbose {
			fmt.Printf("  >> %s\n", stage.Name)
		}

		start := time.Now()
		output, pluginErr := plugin.Run(resolvedArgs, r.verbose)
		elapsed := time.Since(start).Seconds()

		if pluginErr != nil {
			allowPartial, _ := resolvedArgs["partially_scored"].(bool)

			var outStr string
			var percentage float64
			if pef, ok := pluginErr.(*domainerrors.PluginExecutionFailed); ok {
				outStr = pef.Output
				percentage = pef.Percentage
			}

			if r.verbose {
				printVerboseOutput(outStr)
				fmt.Printf("  FAIL %s (%.2fs)\n", stage.Name, elapsed)
			}

			result := PipelineStageResult{
				Name:        stage.Name,
				Failed:      !allowPartial,
				Output:      outStr,
				Percentage:  percentage,
				ElapsedTime: elapsed,
			}
			stageResults = append(stageResults, result)
			if stage.RegisterOutput != nil {
				registerOutput(ctx, *stage.RegisterOutput, result)
			}

			if !allowPartial {
				failType := stage.Fail
				if failType == "" {
					failType = config.FailFast
				}
				switch failType {
				case config.FailFast:
					skipTheRest = true
					pipelinePassed = false
				case config.FailAfterAll:
					pipelinePassed = false
				case config.FailNever:
				}
			}
		} else {
			if r.verbose {
				printVerboseOutput(output.Output)
				fmt.Printf("  OK   %s (%.2fs)\n", stage.Name, elapsed)
			}

			result := PipelineStageResult{
				Name:        stage.Name,
				Output:      output.Output,
				Percentage:  output.Percentage,
				ElapsedTime: elapsed,
			}
			stageResults = append(stageResults, result)
			if stage.RegisterOutput != nil {
				registerOutput(ctx, *stage.RegisterOutput, result)
			}
		}
	}

	return &PipelineResult{Failed: !pipelinePassed, StageResults: stageResults}, nil
}

func (r *PipelineRunner) resolveArgs(args map[string]any, ctx map[string]any) (map[string]any, error) {
	if args == nil {
		return map[string]any{}, nil
	}
	resolved, err := r.resolver.Resolve(args, ctx)
	if err != nil {
		return nil, err
	}
	result, ok := resolved.(map[string]any)
	if !ok {
		return nil, domainerrors.NewBadConfig("args resolved to non-map type")
	}
	return result, nil
}

func printVerboseOutput(output string) {
	if output == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		fmt.Printf("     %s\n", line)
	}
}

func registerOutput(ctx map[string]any, name string, result PipelineStageResult) {
	outputs, ok := ctx["outputs"].(map[string]any)
	if !ok {
		outputs = make(map[string]any)
		ctx["outputs"] = outputs
	}
	outputs[name] = structToMap(result)
}

func structToMap(v any) map[string]any {
	data, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}
