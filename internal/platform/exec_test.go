package platform

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIsAvailable(t *testing.T) {
	if !IsAvailable("go") {
		t.Fatalf("expected go binary to be available in test environment")
	}

	if IsAvailable("definitely-not-installed-scrcpytui-test-binary") {
		t.Fatalf("expected missing binary check to return false")
	}
}

func TestRunCommandSuccess(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := RunCommand(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", "success", "hello")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if out != "hello" {
		t.Fatalf("expected trimmed stdout %q, got %q", "hello", out)
	}
}

func TestRunCommandDetailedSuccess(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := RunCommandDetailed(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", "success", "hello")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", res.Stdout)
	}
	if strings.TrimSpace(res.Stderr) != "" {
		t.Fatalf("expected empty stderr, got %q", res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", res.ExitCode)
	}
	if res.TimedOut || res.Canceled {
		t.Fatalf("expected non-timeout non-cancel result: %+v", res)
	}
}

func TestRunCommandDetailedFailureCapturesStderrAndExitCode(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := RunCommandDetailed(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", "fail", "boom")
	if err == nil {
		t.Fatalf("expected error from failing helper command")
	}
	if strings.TrimSpace(res.Stderr) != "boom" {
		t.Fatalf("expected stderr %q, got %q", "boom", res.Stderr)
	}
	if res.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", res.ExitCode)
	}
}

func TestRunCommandDetailedTimeoutMetadata(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	res, err := RunCommandDetailed(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", "sleep", "2")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !res.TimedOut {
		t.Fatalf("expected timed_out metadata true: %+v", res)
	}
	if res.ExitCode != unknownExitCode {
		t.Fatalf("expected unknown exit code on timeout, got %d", res.ExitCode)
	}
}

func TestRunCommandCompatibilityErrorWithOutput(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := RunCommand(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", "fail", "boom")
	if err == nil {
		t.Fatalf("expected error from failing helper command")
	}
	if out != "" {
		t.Fatalf("expected empty output on failure, got %q", out)
	}
	if err.Error() != "boom" {
		t.Fatalf("expected stderr text wrapped as error %q, got %q", "boom", err.Error())
	}
}

func TestRunCommandErrorWithoutOutput(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := RunCommand(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", "silent-fail")
	if err == nil {
		t.Fatalf("expected error from failing helper command")
	}
	if out != "" {
		t.Fatalf("expected empty output on failure without stderr, got %q", out)
	}
	if strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("expected non-empty execution error")
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep == -1 || sep+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "missing helper mode")
		os.Exit(2)
	}

	mode := args[sep+1]
	switch mode {
	case "success":
		if sep+2 < len(args) {
			fmt.Println(args[sep+2])
		}
		os.Exit(0)
	case "fail":
		msg := "error"
		if sep+2 < len(args) {
			msg = args[sep+2]
		}
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(7)
	case "silent-fail":
		os.Exit(9)
	case "sleep":
		seconds := 1
		if sep+2 < len(args) {
			_, _ = fmt.Sscanf(args[sep+2], "%d", &seconds)
		}
		time.Sleep(time.Duration(seconds) * time.Second)
		fmt.Fprintln(os.Stdout, "woke")
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %s\n", mode)
		os.Exit(2)
	}
}
