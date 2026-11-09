package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/executor"
	"github.com/tfenng/tasks/task2go/internal/params"
	"github.com/tfenng/tasks/task2go/internal/store"
)

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	p, apiErr := parsePage(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	definitionID := parseOptionalString(r, "definition_id")
	if definitionID != nil && !validUUID(*definitionID) {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	items, total, err := s.Store.ListRuns(r.Context(), s.Store.Pool, p.Limit, p.Offset, definitionID, parseOptionalString(r, "status"))
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	out := make([]RunOut, 0, len(items))
	for i := range items {
		out = append(out, runOut(&items[i]))
	}
	writeJSON(w, 200, RunList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body RunCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if !validUUID(body.DefinitionID) {
		writeError(w, r, validationErr("request validation failed", params.FieldError{Field: "definition_id", Code: "uuid_parsing", Message: "invalid UUID"}))
		return
	}
	if body.DefinitionVersionID != nil && !validUUID(*body.DefinitionVersionID) {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.Trigger.Type != "" && body.Trigger.Type != "immediate" {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if len(body.Parameters) > 0 {
		data, _ := json.Marshal(body.Parameters)
		if len(data) > params.MaxParameterBytes {
			writeError(w, r, validationErr("parameters too large"))
			return
		}
	}
	idempotency := r.Header.Get("Idempotency-Key")
	ctx := r.Context()
	var created *store.Run
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		definition, err := s.Store.GetDefinition(ctx, tx, body.DefinitionID)
		if err != nil {
			return err
		}
		if definition == nil {
			return notFound("definition not found")
		}
		if definition.IsArchived {
			return &apiError{Status: 409, Code: "DEFINITION_DISABLED", Message: "definition is archived"}
		}
		version, err := s.Store.ActiveVersion(ctx, tx, definition, body.DefinitionVersionID)
		if err != nil {
			return err
		}
		if version == nil {
			return notFound("definition version not found")
		}
		schema := version.ParameterSchema.Map()
		var parameters map[string]any
		var redacted map[string]any
		if len(body.Arguments) > 0 {
			parameters = map[string]any{"__arguments__": stringsToAny(body.Arguments)}
			redacted = parameters
		} else {
			parameters = params.ApplyDefaults(schema, body.Parameters)
			if err := params.ValidateParameters(ctx, schema, parameters); err != nil {
				if ve, ok := err.(*params.ValidationError); ok {
					return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid parameters", Details: ve.Errors}
				}
				return err
			}
			redacted = params.Redact(schema, parameters)
		}
		requestHash := core.RequestHash(body.DefinitionID, body.Arguments, parameters)
		if idempotency != "" {
			existing, err := s.Store.FindRunByIdempotency(ctx, tx, auth.User.ID, idempotency)
			if err != nil {
				return err
			}
			if existing != nil {
				if existing.RequestHash != nil && *existing.RequestHash == requestHash {
					created = existing
					return nil
				}
				return conflict("idempotency key reused with a different payload")
			}
		}
		runID := store.NewID()
		var encrypted map[string]any
		if len(body.Arguments) > 0 {
			encrypted = parameters
			redacted = parameters
		} else {
			encrypted, err = params.EncryptParameters(schema, parameters, runID, s.Box)
			if err != nil {
				return err
			}
			redacted = params.Redact(schema, parameters)
		}
		target, err := core.BuildTargetSnapshot(ctx, s.Store, tx, version.ExecutorConfig.Map())
		if err != nil {
			return err
		}
		retry := core.ParseRetryPolicy(version.RetryPolicy.Map())
		now := time.Now().UTC()
		var keyPtr *string
		if idempotency != "" {
			keyPtr = &idempotency
		}
		run := &store.Run{
			ID: runID, DefinitionID: definition.ID, DefinitionVersionID: version.ID,
			TargetSnapshot: rawMap(target), TriggerType: "immediate", Status: "queued",
			IdempotencyKey: keyPtr, RequestHash: &requestHash,
			Parameters: rawMap(encrypted), ParametersRedacted: rawMap(redacted),
			QueuedAt: &now, AttemptCount: 0, MaxAttempts: retry.MaxAttempts,
			CreatedBy: auth.User.ID, CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		if err := s.Store.CreateRun(ctx, tx, run); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "run.create", "task_run", run.ID,
			map[string]any{"definition_id": definition.ID, "version": version.VersionNo}, r)
		created = run
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, runOut(created))
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "run_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	run, err := s.Store.GetRun(ctx, s.Store.Pool, id)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if run == nil {
		writeError(w, r, notFound("run not found"))
		return
	}
	base := runOut(run)
	definition, _ := s.Store.GetDefinition(ctx, s.Store.Pool, run.DefinitionID)
	version, _ := s.Store.GetVersion(ctx, s.Store.Pool, run.DefinitionVersionID)
	attempts, _ := s.Store.AttemptsFor(ctx, s.Store.Pool, run.ID)
	detail := RunDetail{RunOut: base, Attempts: make([]AttemptOut, 0, len(attempts))}
	if definition != nil {
		detail.DefinitionKey = &definition.Key
		detail.DefinitionName = &definition.Name
	}
	if version != nil {
		detail.DefinitionVersionNo = &version.VersionNo
		detail.ExecutorType = &version.ExecutorType
		detail.ExecutorConfig = version.ExecutorConfig.Map()
		detail.ParameterSchema = version.ParameterSchema.Map()
		detail.TimeoutSeconds = &version.TimeoutSeconds
	}
	for _, attempt := range attempts {
		detail.Attempts = append(detail.Attempts, attemptOut(attempt))
	}
	writeJSON(w, 200, detail)
}

func (s *Server) getLiveOutput(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "run_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	attemptNo, apiErr := parseIntDefault(r, "attempt_no", 0)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	maxBytes, apiErr := parseIntDefault(r, "max_bytes", s.Config.LiveOutputMaxBytes)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	if maxBytes < 1024 || maxBytes > 4*1024*1024 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	ctx := r.Context()
	run, err := s.Store.GetRun(ctx, s.Store.Pool, id)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if run == nil {
		writeError(w, r, notFound("run not found"))
		return
	}
	attempts, err := s.Store.AttemptsFor(ctx, s.Store.Pool, run.ID)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if len(attempts) == 0 {
		writeError(w, r, notFound("run has no attempts yet"))
		return
	}
	var attempt *store.Attempt
	if attemptNo == 0 {
		attempt = &attempts[len(attempts)-1]
	} else {
		for i := range attempts {
			if attempts[i].AttemptNo == attemptNo {
				attempt = &attempts[i]
				break
			}
		}
	}
	if attempt == nil {
		writeError(w, r, notFound("attempt not found"))
		return
	}
	dir := executor.AttemptDir(s.Config.OutputRoot, run.ID, attempt.AttemptNo)
	stdout, stdoutBytes, stdoutTruncated := executor.ReadTail(dir+"/"+executor.StdoutName, maxBytes)
	stderr, stderrBytes, stderrTruncated := executor.ReadTail(dir+"/"+executor.StderrName, maxBytes)
	if stdout == "" && attempt.StdoutExcerpt != nil {
		stdout, stdoutBytes, stdoutTruncated = executor.TailText(*attempt.StdoutExcerpt, maxBytes)
	}
	if stderr == "" && attempt.StderrExcerpt != nil {
		stderr, stderrBytes, stderrTruncated = executor.TailText(*attempt.StderrExcerpt, maxBytes)
	}
	writeJSON(w, 200, RunLiveOutput{
		RunID: run.ID, AttemptNo: attempt.AttemptNo, Status: attempt.Status,
		Running: attempt.Status == "running", Stdout: stdout, Stderr: stderr,
		StdoutBytes: stdoutBytes, StderrBytes: stderrBytes,
		StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated,
	})
}

func (s *Server) retryRun(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "run_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	idempotency := r.Header.Get("Idempotency-Key")
	ctx := r.Context()
	var created *store.Run
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		run, err := s.Store.GetRun(ctx, tx, id)
		if err != nil {
			return err
		}
		if run == nil {
			return notFound("run not found")
		}
		switch run.Status {
		case "succeeded", "failed", "cancelled":
		default:
			return invalidState("run is not terminal")
		}
		if idempotency != "" {
			existing, err := s.Store.FindRunByIdempotency(ctx, tx, auth.User.ID, idempotency)
			if err != nil {
				return err
			}
			if existing != nil {
				created = existing
				return nil
			}
		}
		version, err := s.Store.GetVersion(ctx, tx, run.DefinitionVersionID)
		if err != nil {
			return err
		}
		if version == nil {
			return notFound("definition version not found")
		}
		plain, err := params.DecryptParameters(version.ParameterSchema.Map(), run.Parameters.Map(), run.ID, s.Box)
		if err != nil {
			return err
		}
		runID := store.NewID()
		encrypted, err := params.EncryptParameters(version.ParameterSchema.Map(), plain, runID, s.Box)
		if err != nil {
			return err
		}
		retry := core.ParseRetryPolicy(version.RetryPolicy.Map())
		now := time.Now().UTC()
		var keyPtr *string
		if idempotency != "" {
			keyPtr = &idempotency
		}
		newRun := &store.Run{
			ID: runID, DefinitionID: run.DefinitionID, DefinitionVersionID: run.DefinitionVersionID,
			TargetSnapshot: run.TargetSnapshot, TriggerType: "immediate", Status: "queued",
			IdempotencyKey: keyPtr, Parameters: rawMap(encrypted),
			ParametersRedacted: run.ParametersRedacted, QueuedAt: &now, AttemptCount: 0,
			MaxAttempts: retry.MaxAttempts, ParentRunID: &run.ID, CreatedBy: auth.User.ID,
			CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		if err := s.Store.CreateRun(ctx, tx, newRun); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "run.retry", "task_run", newRun.ID,
			map[string]any{"parent_run_id": run.ID}, r)
		created = newRun
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, runOut(created))
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "run_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	var updated *store.Run
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		run, err := s.Store.GetRun(ctx, tx, id)
		if err != nil {
			return err
		}
		if run == nil {
			return notFound("run not found")
		}
		switch run.Status {
		case "succeeded", "failed", "cancelled", "skipped", "missed":
			return invalidState("run is already terminal")
		}
		now := time.Now().UTC()
		switch run.Status {
		case "scheduled", "queued", "retry_wait":
			run.Status = "cancelled"
			run.FinishedAt = &now
		default:
			run.CancelRequestedAt = &now
			run.CancelRequestedBy = &auth.User.ID
		}
		run.Version++
		if err := s.Store.SaveRun(ctx, tx, run); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "run.cancel", "task_run", run.ID, nil, r)
		updated = run
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, runOut(updated))
}

func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}
