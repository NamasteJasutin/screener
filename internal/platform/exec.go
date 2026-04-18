package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const unknownExitCode = -1

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	Canceled bool
}

func IsAvailable(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

func RunCommandDetailed(ctx context.Context, binary string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	ctxErr := ctx.Err()
	result := CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: unknownExitCode,
		TimedOut: errors.Is(err, context.DeadlineExceeded) || errors.Is(ctxErr, context.DeadlineExceeded),
		Canceled: errors.Is(err, context.Canceled) || errors.Is(ctxErr, context.Canceled),
	}

	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}

	return result, err
}

func RunCommand(ctx context.Context, binary string, args ...string) (string, error) {
	res, err := RunCommandDetailed(ctx, binary, args...)
	stdout := strings.TrimSpace(res.Stdout)
	stderr := strings.TrimSpace(res.Stderr)
	if err != nil {
		text := stderr
		if text == "" {
			text = stdout
		}
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", text)
	}
	return stdout, nil
}
