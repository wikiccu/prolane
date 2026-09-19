package main_test

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRootInvocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	binary := filepath.Join(t.TempDir(), "prolane")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Prolane: %v\n%s", err, output)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run Prolane: %v\nstderr: %s", err, &stderr)
	}

	const want = "Prolane\nProve every software change before it reaches production.\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}
