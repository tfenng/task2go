package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/config"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/params"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

type Scheduler struct {
	Cfg   *config.Config
	Store *store.Store
	Box   *security.SecretBox
}

func New(cfg *config.Config, st *store.Store, box *security.SecretBox) *Scheduler {
	return &Scheduler{Cfg: cfg, Store: st, Box: box}
}

func (s *Scheduler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := s.RunOnce(ctx); err != nil {
			slog.Error("scheduler tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	conn, err := s.Store.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('task_control_scheduler'))`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('task_control_scheduler'))`)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	schedules, err := s.Store.ListDueSchedules(ctx, tx, now, 100)
	if err != nil {
		return err
	}
	for i := range schedules {
		schedule := &schedules[i]
		scheduledFor := schedule.NextFireAt
		if scheduledFor == nil {
			continue
		}
		exists, err := s.Store.ScheduleOccurrenceExists(ctx, tx, schedule.ID, *scheduledFor)
		if err != nil {
			return err
		}
		if !exists {
			age := now.Sub(*scheduledFor).Seconds()
			active, err := s.Store.ScheduleHasActiveRun(ctx, tx, schedule.ID)
			if err != nil {
				return err
			}
			status := "queued"
			reason := ""
			switch {
			case age > float64(schedule.MisfireGraceSeconds):
				status = "missed"
				reason = "outside misfire grace window"
			case active && schedule.OverlapPolicy == "skip":
				status = "skipped"
				reason = "previous run still active"
			}
			if err := s.createRunFromSchedule(ctx, tx, schedule, *scheduledFor, status, reason); err != nil {
				return err
			}
		}
		schedule.LastFireAt = scheduledFor
		if schedule.TriggerType == "once" {
			schedule.Status = "completed"
			schedule.NextFireAt = nil
		} else {
			next, err := core.ComputeNextFire(schedule.TriggerType, schedule.FireAt, schedule.CronExpr,
				schedule.IntervalSeconds, schedule.Timezone, *scheduledFor)
			if err != nil {
				return err
			}
			schedule.NextFireAt = next
			if next == nil {
				schedule.Status = "completed"
			}
		}
		schedule.Version++
		if err := s.Store.SaveSchedule(ctx, tx, schedule); err != nil {
			return err
		}
	}
	if _, err := s.Store.PromoteRetryWait(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Scheduler) createRunFromSchedule(ctx context.Context, tx pgx.Tx, schedule *store.Schedule, scheduledFor time.Time, status, reason string) error {
	version, err := s.Store.GetVersion(ctx, tx, schedule.DefinitionVersionID)
	if err != nil {
		return err
	}
	if version == nil {
		return nil
	}
	plain, err := params.DecryptParameters(version.ParameterSchema.Map(), schedule.Parameters.Map(), "schedule:"+schedule.ID, s.Box)
	if err != nil {
		return err
	}
	runID := store.NewID()
	encrypted, err := params.EncryptParameters(version.ParameterSchema.Map(), plain, runID, s.Box)
	if err != nil {
		return err
	}
	target, err := core.BuildTargetSnapshot(ctx, s.Store, tx, version.ExecutorConfig.Map())
	if err != nil {
		return err
	}
	retry := core.ParseRetryPolicy(version.RetryPolicy.Map())
	now := time.Now().UTC()
	run := &store.Run{
		ID: runID, DefinitionID: schedule.DefinitionID, DefinitionVersionID: schedule.DefinitionVersionID,
		TargetSnapshot: rawMap(target), ScheduleID: &schedule.ID, ScheduledFor: &scheduledFor,
		TriggerType: schedule.TriggerType, Status: status, Parameters: rawMap(encrypted),
		ParametersRedacted: schedule.ParametersRedacted, AttemptCount: 0,
		MaxAttempts: retry.MaxAttempts, CreatedBy: schedule.CreatedBy,
		CreatedAt: now, UpdatedAt: now, Version: 1, TimeoutSeconds: version.TimeoutSeconds,
	}
	if status == "queued" {
		run.QueuedAt = &now
	}
	if status == "skipped" || status == "missed" || status == "failed" {
		run.FinishedAt = &now
	}
	if status == "skipped" {
		run.ErrorCode = strptr("SCHEDULE_OVERLAP")
	}
	if reason != "" {
		run.ErrorMessage = &reason
	}
	if err := s.Store.CreateRun(ctx, tx, run); err != nil {
		return err
	}
	event := &store.AuditEvent{
		Action: "run.create", ResourceType: "task_run", ResourceID: &run.ID,
		CorrelationID: "-", Details: store.JSONB(map[string]any{
			"schedule_id": schedule.ID, "status": status, "scheduled_for": scheduledFor.Format(time.RFC3339),
		}),
	}
	if err := s.Store.InsertAudit(ctx, tx, event); err != nil {
		return err
	}
	slog.Info("scheduled run created", "run_id", run.ID, "schedule_id", schedule.ID, "status", status)
	return nil
}

func rawMap(value map[string]any) store.JSONB {
	if value == nil {
		return store.JSONB{}
	}
	return store.JSONB(value)
}

func strptr(value string) *string { return &value }
