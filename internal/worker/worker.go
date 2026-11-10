package worker

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/config"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/executor"
	"github.com/tfenng/tasks/task2go/internal/params"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

type Worker struct {
	Cfg   *config.Config
	Store *store.Store
	Box   *security.SecretBox
	ID    string
}

func New(cfg *config.Config, st *store.Store, box *security.SecretBox) *Worker {
	return &Worker{Cfg: cfg, Store: st, Box: box, ID: uuid.NewString()}
}

func (w *Worker) Run(ctx context.Context) error {
	hostname, _ := os.Hostname()
	record := &store.Worker{
		ID: w.ID, Hostname: hostname, PID: os.Getpid(), Role: "worker",
		StartedAt: time.Now().UTC(),
	}
	if err := w.Store.RegisterWorker(ctx, w.Store.Pool, record); err != nil {
		return err
	}
	defer func() {
		_ = w.Store.StopWorker(context.Background(), w.Store.Pool, w.ID)
	}()
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go w.heartbeatWorker(heartbeatCtx)
	go w.reaper(heartbeatCtx)

	slots := make(chan struct{}, w.Cfg.WorkerConcurrency)
	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case slots <- struct{}{}:
		}
		claimed, err := w.claim(ctx)
		if err != nil {
			<-slots
			slog.Error("worker claim failed", "error", err)
			time.Sleep(seconds(w.Cfg.WorkerPollSeconds))
			continue
		}
		if claimed == nil {
			<-slots
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			case <-time.After(seconds(w.Cfg.WorkerPollSeconds)):
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			w.execute(context.Background(), claimed)
		}()
	}
}

func (w *Worker) claim(ctx context.Context) (*store.Claimed, error) {
	var claimed *store.Claimed
	err := w.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		claimed, err = w.Store.ClaimRun(ctx, tx, w.ID, w.Cfg.LeaseSeconds)
		return err
	})
	return claimed, err
}

func (w *Worker) execute(ctx context.Context, claimed *store.Claimed) {
	run := claimed.Run
	version := claimed.Version
	target := run.TargetSnapshot.Map()
	plain, err := params.DecryptParameters(version.ParameterSchema.Map(), run.Parameters.Map(), run.ID, w.Box)
	if err != nil {
		w.finish(claimed, executor.Result{
			Status: "failed", ErrorClass: strptr("UNKNOWN"), ErrorMessage: strptr("decrypt parameters failed"),
		})
		return
	}
	var credential map[string]any
	if credentialID, _ := target["credential_id"].(string); credentialID != "" {
		credential, err = core.CredentialValues(ctx, w.Store, w.Store.Pool, w.Box, credentialID)
		if err != nil {
			w.finish(claimed, executor.Result{
				Status: "failed", ErrorClass: strptr("TARGET_DISABLED"),
				ErrorMessage: strptr("execution target or credential disabled"),
			})
			return
		}
	}
	exec, err := executor.Create(version.ExecutorType)
	if err != nil {
		w.finish(claimed, executor.Result{
			Status: "failed", ErrorClass: strptr("UNKNOWN"), ErrorMessage: strptr(err.Error()),
		})
		return
	}
	stopLease := w.leaseHeartbeat(run.ID)
	defer stopLease()
	result := exec.Execute(ctx, executor.Context{
		RunID: run.ID, AttemptNo: claimed.Attempt.AttemptNo, Parameters: plain,
		Target: target, TimeoutSeconds: run.TimeoutSeconds,
		OutputLimitBytes:        w.Cfg.OutputLimitBytes,
		OutputCaptureLimitBytes: w.Cfg.OutputCaptureLimitBytes,
		OutputRoot:              w.Cfg.OutputRoot, DefaultShell: w.Cfg.DefaultShell,
		KnownHostsPath: w.Cfg.KnownHostsPath, Credential: credential,
		SecretValues: params.SecretValues(version.ParameterSchema.Map(), plain),
	})
	w.finish(claimed, result)
}

func (w *Worker) finish(claimed *store.Claimed, result executor.Result) {
	ctx := context.Background()
	err := w.Store.WithTx(ctx, func(tx pgx.Tx) error {
		run, err := w.Store.GetRun(ctx, tx, claimed.Run.ID)
		if err != nil {
			return err
		}
		attempt, err := w.Store.GetAttempt(ctx, tx, claimed.Attempt.ID)
		if err != nil {
			return err
		}
		if run == nil || attempt == nil {
			return nil
		}
		version, _ := w.Store.GetVersion(ctx, tx, run.DefinitionVersionID)
		now := time.Now().UTC()
		attempt.Status = result.Status
		attempt.FinishedAt = &now
		duration := result.DurationMS
		attempt.DurationMS = &duration
		attempt.ExitCode = result.ExitCode
		attempt.HTTPStatus = result.HTTPStatus
		attempt.ErrorClass = result.ErrorClass
		attempt.ErrorMessage = truncate(result.ErrorMessage, 2000)
		attempt.StdoutExcerpt = result.Stdout
		attempt.StderrExcerpt = result.Stderr
		attempt.ResponseExcerpt = result.Response
		attempt.Truncated = result.Truncated
		if err := w.Store.SaveAttempt(ctx, tx, attempt); err != nil {
			return err
		}
		run.ErrorCode = result.ErrorClass
		run.ErrorMessage = truncate(result.ErrorMessage, 2000)
		run.ResultSummary = store.JSONB(map[string]any{
			"exit_code": result.ExitCode, "http_status": result.HTTPStatus,
			"duration_ms": duration, "truncated": result.Truncated,
		})
		var retry core.RetryPolicy
		if version != nil {
			retry = core.ParseRetryPolicy(version.RetryPolicy.Map())
		} else {
			retry = core.ParseRetryPolicy(nil)
		}
		switch {
		case result.Status == "succeeded":
			run.Status = "succeeded"
			run.FinishedAt = &now
		case run.CancelRequestedAt != nil:
			run.Status = "cancelled"
			run.FinishedAt = &now
		case retry.Retryable(stringValue(result.ErrorClass), result.ExitCode) && run.AttemptCount < run.MaxAttempts:
			run.Status = "retry_wait"
			next := now.Add(retry.Delay(run.AttemptCount))
			run.NextRetryAt = &next
			run.FinishedAt = nil
		default:
			run.Status = "failed"
			run.FinishedAt = &now
		}
		run.LeaseExpiresAt = nil
		run.Version++
		if err := w.Store.SaveRun(ctx, tx, run); err != nil {
			return err
		}
		if err := w.Store.DecrementRuntime(ctx, tx, run.DefinitionID); err != nil {
			return err
		}
		event := &store.AuditEvent{
			Action: "run." + run.Status, ResourceType: "task_run", ResourceID: &run.ID,
			CorrelationID: "-", Details: store.JSONB(map[string]any{
				"attempt": attempt.AttemptNo, "error_class": stringValue(result.ErrorClass),
			}),
		}
		if err := w.Store.InsertAudit(ctx, tx, event); err != nil {
			return err
		}
		slog.Info("run finished", "run_id", run.ID, "attempt", attempt.AttemptNo, "status", run.Status)
		return nil
	})
	if err != nil {
		slog.Error("finish run failed", "run_id", claimed.Run.ID, "error", err)
	}
}

func (w *Worker) leaseHeartbeat(runID string) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		interval := time.Duration(w.Cfg.LeaseSeconds) * time.Second / 3
		if interval < time.Second {
			interval = time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.Store.RenewLease(context.Background(), w.Store.Pool, runID, w.Cfg.LeaseSeconds); err != nil {
					slog.Warn("lease heartbeat failed", "run_id", runID, "error", err)
				}
			}
		}
	}()
	return cancel
}

func (w *Worker) heartbeatWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.Store.HeartbeatWorker(context.Background(), w.Store.Pool, w.ID)
		}
	}
}

func (w *Worker) reaper(ctx context.Context) {
	ticker := time.NewTicker(seconds(w.Cfg.WorkerPollSeconds))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var count int
			err := w.Store.WithTx(context.Background(), func(tx pgx.Tx) error {
				var err error
				count, err = w.Store.ReapExpired(context.Background(), tx)
				return err
			})
			if err != nil {
				slog.Error("reaper failed", "error", err)
			} else if count > 0 {
				slog.Warn("expired runs reaped", "count", count)
			}
		}
	}
}

func strptr(value string) *string { return &value }

func truncate(value *string, limit int) *string {
	if value == nil {
		return nil
	}
	runes := []rune(*value)
	if len(runes) > limit {
		v := string(runes[:limit])
		return &v
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func seconds(value float64) time.Duration {
	if value <= 0 {
		value = 1
	}
	return time.Duration(value * float64(time.Second))
}
