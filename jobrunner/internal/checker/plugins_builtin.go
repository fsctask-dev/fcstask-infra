package checker

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"jobrunner/internal/domain/errors"
)

type RunScriptPlugin struct{}

type runScriptArgs struct {
	Origin        string            `json:"origin"`
	Script        any               `json:"script"`
	Timeout       *float64          `json:"timeout,omitempty"`
	EnvAdditional map[string]string `json:"env_additional,omitempty"`
	EnvWhitelist  []string          `json:"env_whitelist,omitempty"`
	Input         *string           `json:"input,omitempty"`
}

func (p *RunScriptPlugin) Run(args map[string]any, verbose bool) (*PluginOutput, error) {
	a, err := parseArgs[runScriptArgs](args)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if a.Timeout != nil {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(float64(time.Second)**a.Timeout))
		defer cancel()
	}

	cmd, err := buildCommand(ctx, a.Script)
	if err != nil {
		return nil, err
	}
	cmd.Dir = a.Origin
	cmd.Env = buildEnv(a.EnvWhitelist, a.EnvAdditional)

	if a.Input != nil {
		f, err := os.Open(*a.Input)
		if err != nil {
			return nil, errors.NewBadConfig(fmt.Sprintf("failed to open input file: %v", err))
		}
		defer func() { _ = f.Close() }()
		cmd.Stdin = f
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	output := buf.String()

	if runErr != nil {
		msg := fmt.Sprintf("script failed: %v", runErr)
		if ctx.Err() == context.DeadlineExceeded {
			msg = fmt.Sprintf("script timed out after %.1fs", *a.Timeout)
		}
		return nil, errors.NewPluginExecutionFailed(msg, output, 0)
	}

	return &PluginOutput{Output: output, Percentage: 1.0}, nil
}

type RunTestsPlugin struct{}

type runTestsArgs struct {
	Origin     string   `json:"origin"`
	Script     string   `json:"script"`
	ReportFile string   `json:"report_file"`
	Timeout    *float64 `json:"timeout,omitempty"`
}

func (p *RunTestsPlugin) Run(args map[string]any, verbose bool) (*PluginOutput, error) {
	a, err := parseArgs[runTestsArgs](args)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if a.Timeout != nil {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(float64(time.Second)**a.Timeout))
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", a.Script)
	cmd.Dir = a.Origin
	cmd.Env = os.Environ()

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	output := buf.String()

	reportPath := filepath.Join(a.Origin, a.ReportFile)
	xmlData, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		msg := fmt.Sprintf("no report file at %s", a.ReportFile)
		if runErr != nil {
			msg = fmt.Sprintf("script failed and %s", msg)
		}
		return nil, errors.NewPluginExecutionFailed(msg, output, 0)
	}

	total, failed, parseErr := parseJUnitXML(xmlData)
	if parseErr != nil {
		return nil, errors.NewPluginExecutionFailed(
			fmt.Sprintf("failed to parse JUnit XML: %v", parseErr),
			output, 0,
		)
	}

	passed := total - failed
	var percentage float64
	if total > 0 {
		percentage = float64(passed) / float64(total)
	}

	summary := fmt.Sprintf("%spassed: %d/%d", output, passed, total)

	if failed > 0 || runErr != nil {
		return nil, errors.NewPluginExecutionFailed(
			fmt.Sprintf("%d/%d tests failed", failed, total),
			summary,
			percentage,
		)
	}

	return &PluginOutput{Output: summary, Percentage: percentage}, nil
}

func parseJUnitXML(data []byte) (total, failed int, err error) {
	var suites struct {
		XMLName xml.Name `xml:"testsuites"`
		Suites  []struct {
			Tests    int `xml:"tests,attr"`
			Failures int `xml:"failures,attr"`
			Errors   int `xml:"errors,attr"`
		} `xml:"testsuite"`
	}
	if xmlErr := xml.Unmarshal(data, &suites); xmlErr == nil && len(suites.Suites) > 0 {
		for _, s := range suites.Suites {
			total += s.Tests
			failed += s.Failures + s.Errors
		}
		return
	}

	var suite struct {
		XMLName  xml.Name `xml:"testsuite"`
		Tests    int      `xml:"tests,attr"`
		Failures int      `xml:"failures,attr"`
		Errors   int      `xml:"errors,attr"`
	}
	if xmlErr := xml.Unmarshal(data, &suite); xmlErr != nil {
		err = xmlErr
		return
	}
	total = suite.Tests
	failed = suite.Failures + suite.Errors
	return
}

type ReportScorePlugin struct{}

type reportScorePluginArgs struct {
	Username    string  `json:"username"`
	TaskName    string  `json:"task_name"`
	ReportURL   string  `json:"report_url"`
	ReportToken string  `json:"report_token"`
	SendTime    *string `json:"send_time,omitempty"`
}

func (p *ReportScorePlugin) Run(args map[string]any, verbose bool) (*PluginOutput, error) {
	a, err := parseArgs[reportScorePluginArgs](args)
	if err != nil {
		return nil, err
	}

	sendTime := time.Now().Format("2006-01-02 15:04:05-0700")
	if a.SendTime != nil {
		sendTime = *a.SendTime
	}

	form := url.Values{}
	form.Set("token", a.ReportToken)
	form.Set("task", a.TaskName)
	form.Set("username", a.Username)
	form.Set("submit_time", sendTime)
	form.Set("status", "pass")

	resp, err := http.PostForm(a.ReportURL, form)
	if err != nil {
		return nil, errors.NewPluginExecutionFailed(
			fmt.Sprintf("failed to report status: %v", err), "", 0,
		)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, errors.NewPluginExecutionFailed(
			fmt.Sprintf("%d: %s", resp.StatusCode, string(body)), "", 0,
		)
	}

	return &PluginOutput{
		Output:     fmt.Sprintf("reported task=%s user=%s status=pass", a.TaskName, a.Username),
		Percentage: 1.0,
	}, nil
}

func buildCommand(ctx context.Context, script any) (*exec.Cmd, error) {
	switch s := script.(type) {
	case string:
		return exec.CommandContext(ctx, "sh", "-c", s), nil
	case []any:
		parts := make([]string, len(s))
		for i, v := range s {
			parts[i] = fmt.Sprintf("%v", v)
		}
		if len(parts) == 0 {
			return nil, errors.NewBadConfig("script array must not be empty")
		}
		return exec.CommandContext(ctx, parts[0], parts[1:]...), nil
	default:
		return nil, errors.NewBadConfig(fmt.Sprintf("script must be a string or array, got %T", script))
	}
}

func buildEnv(whitelist []string, additional map[string]string) []string {
	var base []string
	if whitelist != nil {
		for _, k := range whitelist {
			if v, ok := os.LookupEnv(k); ok {
				base = append(base, k+"="+v)
			}
		}
	} else {
		base = os.Environ()
	}
	for k, v := range additional {
		base = append(base, k+"="+v)
	}
	return base
}
