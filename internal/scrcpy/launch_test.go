package scrcpy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"screener/internal/core"
)

func TestPreviewExecutionParity(t *testing.T) {
	plan := BuildPlan(core.DefaultProfile(), core.DeviceCapabilitySnapshot{SDKInt: 34, SupportsH265: true, SupportsAudio: true})
	preview := Preview(plan)
	res, err := Execute(context.Background(), plan, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan != plan {
		t.Fatal("expected execution to reference same command plan object")
	}
	if preview == "" {
		t.Fatal("preview should not be empty")
	}
	if preview != strings.TrimSpace(plan.Binary+" "+strings.Join(plan.Args, " ")) {
		t.Fatal("preview must be generated from exact execution args")
	}
}

func TestExecuteBlockedWhenResolutionNotLaunchable(t *testing.T) {
	resolution := core.EffectiveProfileResolution{FinalArgs: []string{"--audio"}, Launchable: false}
	plan := BuildPlanFromResolution(resolution, "")
	if _, err := Execute(context.Background(), plan, true); err == nil {
		t.Fatal("expected blocked launch error")
	}
}

func TestExecutePropagatesStderrAndMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test script uses POSIX shell")
	}
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "fail-scrcpy.sh")
	script := "#!/bin/sh\necho launch failed: invalid argument 1>&2\nexit 7\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	plan := &CommandPlan{Binary: scriptPath, Args: []string{"--display-id", "7"}, Launchable: true}
	res, err := Execute(context.Background(), plan, false)
	if err == nil {
		t.Fatal("expected launch error")
	}
	if res.Plan != plan {
		t.Fatal("expected result plan reference to match input plan")
	}
	if res.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "launch failed: invalid argument") {
		t.Fatalf("expected stderr in execution result, got %q", res.Stderr)
	}
	if !strings.Contains(err.Error(), "stderr=launch failed: invalid argument") {
		t.Fatalf("expected stderr-rich error, got %v", err)
	}
}

func TestExecuteMarksCanceledMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test script uses POSIX shell")
	}
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "slow-scrcpy.sh")
	script := "#!/bin/sh\nsleep 2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	plan := &CommandPlan{Binary: scriptPath, Launchable: true}
	res, err := Execute(ctx, plan, false)
	if err == nil {
		t.Fatal("expected launch cancellation error")
	}
	if !res.Canceled {
		t.Fatalf("expected canceled result metadata, got: %+v", res)
	}
	if !strings.Contains(err.Error(), "canceled=true") {
		t.Fatalf("expected canceled error metadata, got: %v", err)
	}
}

// ── Preview nil plan ──────────────────────────────────────────────────────────

func TestPreviewNilPlan(t *testing.T) {
	got := Preview(nil)
	if got != "" {
		t.Fatalf("Preview(nil) = %q, want empty string", got)
	}
}

func TestPreviewWithSerial(t *testing.T) {
	plan := BuildPlanFromResolution(
		core.EffectiveProfileResolution{FinalArgs: []string{"--display-id", "0"}, Launchable: true},
		"DEVICE001",
	)
	preview := Preview(plan)
	if !strings.Contains(preview, "--serial=DEVICE001") {
		t.Fatalf("expected --serial in preview: %q", preview)
	}
	// Preview must equal binary + args joined
	expected := strings.TrimSpace(plan.Binary + " " + strings.Join(plan.Args, " "))
	if preview != expected {
		t.Fatalf("preview mismatch: %q != %q", preview, expected)
	}
}

func TestBuildPlanFromResolutionEmptySerial(t *testing.T) {
	res := core.EffectiveProfileResolution{FinalArgs: []string{"--display-id", "0"}, Launchable: true}
	plan := BuildPlanFromResolution(res, "")
	if strings.Contains(Preview(plan), "--serial=") {
		t.Fatalf("empty serial should not produce --serial flag: %q", Preview(plan))
	}
}

func TestBuildPlanFromResolutionSerialFirst(t *testing.T) {
	res := core.EffectiveProfileResolution{FinalArgs: []string{"--video-bit-rate", "8M"}, Launchable: true}
	plan := BuildPlanFromResolution(res, "USB123")
	if len(plan.Args) == 0 || plan.Args[0] != "--serial=USB123" {
		t.Fatalf("expected --serial as first arg: %v", plan.Args)
	}
}

func TestExecuteNilPlan(t *testing.T) {
	_, err := Execute(context.Background(), nil, true)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

// ── BuildPlan with non-simulated caps ──────────────────────────────────────────

func TestBuildPlanWithLiveSerial(t *testing.T) {
	caps := core.DeviceCapabilitySnapshot{
		Serial: "LIVEDEVICE001",
		SDKInt: 34,
		SupportsAudio: true,
		SupportsH265:  true,
	}
	plan := BuildPlan(core.DefaultProfile(), caps)
	if plan.Serial != "LIVEDEVICE001" {
		t.Fatalf("expected serial=LIVEDEVICE001, got %q", plan.Serial)
	}
	if !strings.HasPrefix(Preview(plan), "scrcpy --serial=LIVEDEVICE001 ") {
		t.Fatalf("expected --serial first in preview: %q", Preview(plan))
	}
}

func TestBuildPlanSimulatedCapsNoSerial(t *testing.T) {
	caps := core.DeviceCapabilitySnapshot{Serial: "simulated", SDKInt: 34}
	plan := BuildPlan(core.DefaultProfile(), caps)
	if plan.Serial != "" {
		t.Fatalf("simulated serial should normalize to empty, got %q", plan.Serial)
	}
	if strings.Contains(Preview(plan), "--serial=") {
		t.Fatalf("simulated caps should not produce --serial: %q", Preview(plan))
	}
}

func TestBuildPlanEmptySerialNoSerialFlag(t *testing.T) {
	caps := core.DeviceCapabilitySnapshot{Serial: "", SDKInt: 34}
	plan := BuildPlan(core.DefaultProfile(), caps)
	if strings.Contains(Preview(plan), "--serial=") {
		t.Fatalf("empty serial should not produce --serial: %q", Preview(plan))
	}
}

// ── buildExecutionError ────────────────────────────────────────────────────────

func TestBuildExecutionErrorWithAllFields(t *testing.T) {
	res := ExecutionResult{
		Stderr:   "fatal error",
		ExitCode: 5,
		TimedOut: true,
		Canceled: true,
	}
	err := buildExecutionError(fmt.Errorf("base error"), res)
	s := err.Error()
	if !strings.Contains(s, "stderr=fatal error") {
		t.Fatalf("expected stderr in error: %q", s)
	}
	if !strings.Contains(s, "exit_code=5") {
		t.Fatalf("expected exit_code in error: %q", s)
	}
	if !strings.Contains(s, "timed_out=true") {
		t.Fatalf("expected timed_out in error: %q", s)
	}
	if !strings.Contains(s, "canceled=true") {
		t.Fatalf("expected canceled in error: %q", s)
	}
}

func TestBuildExecutionErrorBaseOnly(t *testing.T) {
	// No stderr, no exit code — just the base error
	err := buildExecutionError(fmt.Errorf("base"), ExecutionResult{Stderr: "", ExitCode: 0})
	if err.Error() != "base" {
		t.Fatalf("expected plain base error: %q", err.Error())
	}
}

func TestBuildExecutionErrorStderrAlreadyInBase(t *testing.T) {
	// Stderr already contained in the base error → should not duplicate
	base := fmt.Errorf("fatal error")
	res := ExecutionResult{Stderr: "fatal error", ExitCode: 1}
	err := buildExecutionError(base, res)
	// The base already contains "fatal error", so no duplication
	s := err.Error()
	if strings.Count(s, "fatal error") > 1 {
		t.Fatalf("stderr should not be duplicated: %q", s)
	}
}
