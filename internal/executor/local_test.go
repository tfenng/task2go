package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalExecutor(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hello.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"hello $1\"\necho err >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := Local{}.Execute(context.Background(), Context{
		RunID: "run-1", AttemptNo: 1, Parameters: map[string]any{"__arguments__": []any{"world"}},
		Target:         map[string]any{"executor": map[string]any{"argv": []any{script}}},
		TimeoutSeconds: 5, OutputLimitBytes: 1024, OutputCaptureLimitBytes: 1024,
		OutputRoot: t.TempDir(), DefaultShell: "/bin/sh",
	})
	if result.Status != "succeeded" {
		t.Fatalf("status=%s error=%v", result.Status, result.ErrorMessage)
	}
	if result.Stdout == nil || !strings.Contains(*result.Stdout, "hello world") {
		t.Fatalf("stdout=%v", result.Stdout)
	}
	if result.Stderr == nil || !strings.Contains(*result.Stderr, "err") {
		t.Fatalf("stderr=%v", result.Stderr)
	}
}

func TestLocalExecutorTimeout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := Local{}.Execute(context.Background(), Context{
		RunID: "run-2", AttemptNo: 1, Parameters: map[string]any{},
		Target:         map[string]any{"executor": map[string]any{"argv": []any{script}}},
		TimeoutSeconds: 1, OutputLimitBytes: 1024, OutputCaptureLimitBytes: 1024,
		OutputRoot: t.TempDir(), DefaultShell: "/bin/sh",
	})
	if result.Status != "timed_out" || result.ErrorClass == nil || *result.ErrorClass != "TIMEOUT" {
		t.Fatalf("status=%s class=%v", result.Status, result.ErrorClass)
	}
}

func TestShellJoin(t *testing.T) {
	if got := ShellJoin([]string{"a", "hello world", "it's"}); got != `a 'hello world' 'it'\''s'` {
		t.Fatalf("got %q", got)
	}
}
