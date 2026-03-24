package backup_ops

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ScriptSpec struct {
	Command string
	Args    []string
	Env     []string
	WorkDir string
	Timeout time.Duration
}

type ScriptResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

type ScriptRunner interface {
	Run(ctx context.Context, spec ScriptSpec) (*ScriptResult, error)
}

type OSScriptRunner struct{}

func NewOSScriptRunner() *OSScriptRunner {
	return &OSScriptRunner{}
}

func (r *OSScriptRunner) Run(ctx context.Context, spec ScriptSpec) (*ScriptResult, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("script command is required")
	}

	runCtx := ctx
	cancel := func() {}
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, spec.Command, spec.Args...)
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(cmd.Environ(), spec.Env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	err := cmd.Run()
	dur := time.Since(startedAt)

	res := &ScriptResult{
		ExitCode: 0,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: dur,
	}

	if err == nil {
		return res, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
		return res, fmt.Errorf("script failed with exit code %d: %w", res.ExitCode, err)
	}
	if runCtx.Err() != nil {
		return res, fmt.Errorf("script timeout/canceled: %w", runCtx.Err())
	}
	return res, fmt.Errorf("script execution error: %w", err)
}
