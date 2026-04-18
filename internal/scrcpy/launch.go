package scrcpy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/NamasteJasutin/screener/internal/core"
	"github.com/NamasteJasutin/screener/internal/platform"
)

type CommandPlan struct {
	Binary     string
	Serial     string // device serial injected as --serial=<serial>; empty = any device
	Args       []string
	Resolution core.EffectiveProfileResolution
	Launchable bool
}

type ExecutionResult struct {
	Plan     *CommandPlan
	Output   string
	Stderr   string
	ExitCode int
	TimedOut bool
	Canceled bool
}

func BuildPlan(profile core.ProfileDefinition, caps core.DeviceCapabilitySnapshot) *CommandPlan {
	resolved := core.ResolveEffectiveProfile(profile, caps)
	serial := caps.Serial
	if serial == "simulated" {
		serial = ""
	}
	return BuildPlanFromResolution(resolved, serial)
}

// BuildPlanFromResolution assembles a CommandPlan from an already-resolved
// profile. serial, when non-empty, is prepended as --serial=<serial> so that
// scrcpy targets the correct device when multiple are connected.
func BuildPlanFromResolution(resolved core.EffectiveProfileResolution, serial string) *CommandPlan {
	args := append([]string(nil), resolved.FinalArgs...)
	if serial != "" {
		args = append([]string{"--serial=" + serial}, args...)
	}
	return &CommandPlan{
		Binary:     "scrcpy",
		Serial:     serial,
		Args:       args,
		Resolution: resolved,
		Launchable: resolved.Launchable,
	}
}

func Preview(plan *CommandPlan) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.Binary + " " + strings.Join(plan.Args, " "))
}

func Execute(ctx context.Context, plan *CommandPlan, dryRun bool) (ExecutionResult, error) {
	if plan == nil {
		return ExecutionResult{}, errors.New("nil command plan")
	}
	if !plan.Launchable {
		return ExecutionResult{Plan: plan}, errors.New("launch blocked by resolution")
	}
	if dryRun {
		return ExecutionResult{Plan: plan}, nil
	}
	platformRes, err := platform.RunCommandDetailed(ctx, plan.Binary, plan.Args...)
	res := ExecutionResult{
		Plan:     plan,
		Output:   strings.TrimSpace(platformRes.Stdout),
		Stderr:   strings.TrimSpace(platformRes.Stderr),
		ExitCode: platformRes.ExitCode,
		TimedOut: platformRes.TimedOut,
		Canceled: platformRes.Canceled,
	}
	if err != nil {
		return res, buildExecutionError(err, res)
	}
	return res, nil
}

// Session is a running detached scrcpy process. Call Wait to block until exit.
type Session struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

// Wait blocks until the session process exits and returns its result.
func (s *Session) Wait() (ExecutionResult, error) {
	err := s.cmd.Wait()
	stderrStr := ""
	if s.stderr != nil {
		stderrStr = strings.TrimSpace(s.stderr.String())
	}
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	res := ExecutionResult{
		Stderr:   stderrStr,
		ExitCode: exitCode,
		Canceled: errors.Is(err, context.Canceled),
	}
	if err != nil {
		return res, buildExecutionError(err, res)
	}
	return res, nil
}

// ExecuteDetached starts scrcpy without waiting for it to finish.
// The caller must call Session.Wait() (via monitorSessionCmd) to reap the process.
func ExecuteDetached(ctx context.Context, plan *CommandPlan) (*Session, error) {
	if plan == nil {
		return nil, errors.New("nil command plan")
	}
	if !plan.Launchable {
		return nil, errors.New("launch blocked by resolution")
	}
	cmd := exec.CommandContext(ctx, plan.Binary, plan.Args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Session{cmd: cmd, stderr: &stderr}, nil
}

func buildExecutionError(runErr error, res ExecutionResult) error {
	parts := []string{runErr.Error()}
	if res.Stderr != "" && !strings.Contains(runErr.Error(), res.Stderr) {
		parts = append(parts, "stderr="+res.Stderr)
	}
	if res.ExitCode > 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", res.ExitCode))
	}
	if res.TimedOut {
		parts = append(parts, "timed_out=true")
	}
	if res.Canceled {
		parts = append(parts, "canceled=true")
	}
	return errors.New(strings.Join(parts, "; "))
}
