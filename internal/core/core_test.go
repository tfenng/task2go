package core

import (
	"testing"
	"time"
)

func TestComputeNextFire(t *testing.T) {
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 30
	next, err := ComputeNextFire("interval", nil, nil, &interval, "UTC", after)
	if err != nil || next == nil || !next.Equal(after.Add(30*time.Second)) {
		t.Fatalf("interval: %v %v", next, err)
	}
	cronExpr := "*/5 * * * *"
	next, err = ComputeNextFire("cron", nil, &cronExpr, nil, "Asia/Shanghai", after)
	if err != nil || next == nil || next.Before(after) {
		t.Fatalf("cron: %v %v", next, err)
	}
	fireAt := after.Add(time.Hour)
	next, err = ComputeNextFire("once", &fireAt, nil, nil, "UTC", after)
	if err != nil || next == nil || !next.Equal(fireAt) {
		t.Fatalf("once: %v %v", next, err)
	}
	next, err = ComputeNextFire("once", &fireAt, nil, nil, "UTC", fireAt)
	if err != nil || next != nil {
		t.Fatalf("past once: %v %v", next, err)
	}
}

func TestRetryPolicy(t *testing.T) {
	policy := ParseRetryPolicy(map[string]any{
		"max_attempts":            3,
		"initial_backoff_seconds": 10,
		"multiplier":              2,
		"max_backoff_seconds":     30,
		"retry_on_exit_codes":     []any{2},
		"retryable_error_classes": []any{"TIMEOUT"},
	})
	if !policy.Retryable("TIMEOUT", nil) {
		t.Fatal("TIMEOUT should retry")
	}
	code := 2
	if !policy.Retryable("EXIT_CODE", &code) {
		t.Fatal("exit code 2 should retry")
	}
	if policy.Delay(1) != 10*time.Second || policy.Delay(5) != 30*time.Second {
		t.Fatalf("unexpected delays: %v %v", policy.Delay(1), policy.Delay(5))
	}
}
