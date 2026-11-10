package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tfenng/tasks/task2go/internal/api"
	"github.com/tfenng/tasks/task2go/internal/config"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
	"github.com/tfenng/tasks/task2go/internal/worker"
)

func TestImmediateRunEndToEnd(t *testing.T) {
	dbURL := os.Getenv("TASK_CONTROL_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TASK_CONTROL_TEST_DATABASE_URL is not set")
	}
	scriptRoot := t.TempDir()
	outputRoot := t.TempDir()
	script := filepath.Join(scriptRoot, "hello.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"hello $1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASK_CONTROL_DATABASE_URL", dbURL)
	t.Setenv("TASK_CONTROL_SCRIPT_ROOT", scriptRoot)
	t.Setenv("TASK_CONTROL_OUTPUT_ROOT", outputRoot)
	t.Setenv("TASK_CONTROL_WORKER_POLL_SECONDS", "0.01")
	t.Setenv("TASK_CONTROL_WORKER_CONCURRENCY", "1")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `TRUNCATE users, task_definitions, task_definition_versions,
		task_definition_runtime, runs, attempts, schedules, workers, audit_events,
		sessions, api_tokens, credentials, execution_targets RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	hash, _ := security.HashPassword("adminpassword123")
	now := time.Now().UTC()
	admin := &store.User{
		ID: store.NewID(), Username: "admin", DisplayName: "Admin", PasswordHash: hash,
		Role: "admin", IsActive: true, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := st.CreateUser(ctx, st.Pool, admin); err != nil {
		t.Fatal(err)
	}
	box := security.NewSecretBox(cfg.SecretKeys, cfg.SecretActiveVersion)
	srv := api.NewServer(cfg, st, box, http.FS(os.DirFS(t.TempDir())), nil)
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go worker.New(cfg, st, box).Run(workerCtx)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	login := doJSON(t, client, httpServer.URL+"/api/v1/auth/login", "", map[string]any{
		"username": "admin", "password": "adminpassword123",
	})
	csrf, _ := login["csrf_token"].(string)
	if csrf == "" {
		t.Fatalf("login failed: %#v", login)
	}
	definition := doJSON(t, client, httpServer.URL+"/api/v1/task-definitions", csrf, map[string]any{
		"key": "hello-test", "name": "Hello Test",
		"version": map[string]any{
			"executor_type":   "local",
			"executor_config": map[string]any{"argv": []any{script}},
			"timeout_seconds": 30,
		},
	})
	definitionID, _ := definition["id"].(string)
	run := doJSON(t, client, httpServer.URL+"/api/v1/runs", csrf, map[string]any{
		"definition_id": definitionID, "arguments": []any{"world"}, "timeout_seconds": 5,
	})
	if run["timeout_seconds"] != float64(5) {
		t.Fatalf("run timeout_seconds: %#v", run["timeout_seconds"])
	}
	runID, _ := run["id"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		detail := doJSON(t, client, httpServer.URL+"/api/v1/runs/"+runID, "", nil)
		if detail["status"] == "succeeded" {
			attempts, _ := detail["attempts"].([]any)
			if len(attempts) != 1 {
				t.Fatalf("attempts: %#v", detail["attempts"])
			}
			attempt := attempts[0].(map[string]any)
			stdout, _ := attempt["stdout_excerpt"].(string)
			if stdout != "hello world\n" {
				t.Fatalf("stdout %q", stdout)
			}
			doDelete(t, client, httpServer.URL+"/api/v1/runs/"+runID, csrf)
			resp, err := client.Get(httpServer.URL + "/api/v1/runs/" + runID)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("deleted run still readable: %d", resp.StatusCode)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run did not finish: %#v", run)
}

func doDelete(t *testing.T, client *http.Client, url, csrf string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE %s: %d", url, resp.StatusCode)
	}
}

func doJSON(t *testing.T, client *http.Client, url, csrf string, body any) map[string]any {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(http.MethodGet, url, reader)
	if body != nil {
		req.Method = http.MethodPost
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: %d %#v", req.Method, url, resp.StatusCode, out)
	}
	return out
}
