package api

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/executor"
	"github.com/tfenng/tasks/task2go/internal/params"
	"github.com/tfenng/tasks/task2go/internal/store"
)

var definitionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,99}$`)

func (s *Server) serializeDefinition(ctx context.Context, q store.Querier, d *store.Definition) DefinitionOut {
	var active *store.DefinitionVersion
	if d.ActiveVersionID != nil {
		active, _ = s.Store.GetVersion(ctx, q, *d.ActiveVersionID)
	}
	return DefinitionOut{
		ID: d.ID, Key: d.Key, Name: d.Name, Description: d.Description,
		IsArchived: d.IsArchived, ActiveVersion: versionOut(active),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Version: d.Version,
	}
}

func validateVersionInput(input *DefinitionVersionInput) *apiError {
	if input.ExecutorType == "" {
		input.ExecutorType = "local"
	}
	if input.ExecutorType != "local" && input.ExecutorType != "ssh" {
		return validationErr("request validation failed", params.FieldError{Field: "version.executor_type", Code: "literal_error", Message: "Input should be 'local' or 'ssh'"})
	}
	if input.ParameterSchema == nil {
		input.ParameterSchema = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": true}
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 21600
	}
	if input.TimeoutSeconds < 1 || input.TimeoutSeconds > 86400 {
		return validationErr("request validation failed", params.FieldError{Field: "version.timeout_seconds", Code: "range", Message: "Input should be between 1 and 86400"})
	}
	if input.MaxConcurrency == 0 {
		input.MaxConcurrency = 1
	}
	if input.MaxConcurrency < 1 || input.MaxConcurrency > 100 {
		return validationErr("request validation failed", params.FieldError{Field: "version.max_concurrency", Code: "range", Message: "Input should be between 1 and 100"})
	}
	rp := input.RetryPolicy
	if rp.MaxAttempts == 0 {
		rp.MaxAttempts = 1
	}
	if rp.MaxAttempts < 1 || rp.MaxAttempts > 10 {
		return validationErr("request validation failed", params.FieldError{Field: "version.retry_policy.max_attempts", Code: "range", Message: "Input should be between 1 and 10"})
	}
	if rp.InitialBackoffSeconds < 0 || rp.InitialBackoffSeconds > 3600 {
		return validationErr("request validation failed", params.FieldError{Field: "version.retry_policy.initial_backoff_seconds", Code: "range", Message: "Input should be between 1 and 3600"})
	}
	if rp.Multiplier < 0 || rp.Multiplier > 10 {
		return validationErr("request validation failed", params.FieldError{Field: "version.retry_policy.multiplier", Code: "range", Message: "Input should be between 1 and 10"})
	}
	if rp.MaxBackoffSeconds < 0 || rp.MaxBackoffSeconds > 86400 {
		return validationErr("request validation failed", params.FieldError{Field: "version.retry_policy.max_backoff_seconds", Code: "range", Message: "Input should be between 1 and 86400"})
	}
	input.RetryPolicy = rp
	if err := params.ValidateSchema(context.Background(), input.ParameterSchema); err != nil {
		if ve, ok := err.(*params.ValidationError); ok {
			return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid parameter schema", Details: ve.Errors}
		}
		return validationErr("invalid parameter schema")
	}
	if err := core.ValidateExecutorConfig(input.ExecutorType, input.ExecutorConfig); err != nil {
		return validationErr(err.Error())
	}
	return nil
}

func versionPayload(input *DefinitionVersionInput) map[string]any {
	return map[string]any{
		"executor_type":    input.ExecutorType,
		"parameter_schema": input.ParameterSchema,
		"executor_config":  input.ExecutorConfig,
		"timeout_seconds":  input.TimeoutSeconds,
		"retry_policy":     input.RetryPolicy.Map(),
		"max_concurrency":  input.MaxConcurrency,
	}
}

func (s *Server) listDefinitions(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	p, apiErr := parsePage(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	archived, apiErr := parseOptionalBool(r, "archived")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	items, total, err := s.Store.ListDefinitions(ctx, s.Store.Pool, p.Limit, p.Offset, parseOptionalString(r, "q"), archived)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	out := make([]DefinitionOut, 0, len(items))
	for i := range items {
		out = append(out, s.serializeDefinition(ctx, s.Store.Pool, &items[i]))
	}
	writeJSON(w, 200, DefinitionList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func (s *Server) createDefinition(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body DefinitionCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if !definitionKeyPattern.MatchString(body.Key) {
		writeError(w, r, validationErr("request validation failed", params.FieldError{Field: "key", Code: "string_pattern_mismatch", Message: "String should match pattern"}))
		return
	}
	if len(body.Name) < 1 || len(body.Name) > 160 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.Description != nil && len(*body.Description) > 2000 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if apiErr := validateVersionInput(&body.Version); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	var created *store.Definition
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		existing, err := s.Store.GetDefinitionByKey(ctx, tx, body.Key)
		if err != nil {
			return err
		}
		if existing != nil {
			return conflict("definition key already exists")
		}
		now := time.Now().UTC()
		definition := &store.Definition{
			ID: store.NewID(), Key: body.Key, Name: body.Name, Description: body.Description,
			CreatedBy: auth.User.ID, CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		if err := s.Store.CreateDefinition(ctx, tx, definition); err != nil {
			return err
		}
		payload := versionPayload(&body.Version)
		version := &store.DefinitionVersion{
			ID: store.NewID(), DefinitionID: definition.ID, VersionNo: 1,
			ExecutorType: body.Version.ExecutorType, ParameterSchema: rawMap(body.Version.ParameterSchema),
			ExecutorConfig: rawMap(body.Version.ExecutorConfig), TimeoutSeconds: body.Version.TimeoutSeconds,
			RetryPolicy: rawMap(body.Version.RetryPolicy.Map()), MaxConcurrency: body.Version.MaxConcurrency,
			Checksum: core.Checksum(payload), CreatedBy: auth.User.ID, CreatedAt: now,
		}
		if err := s.Store.CreateVersion(ctx, tx, version); err != nil {
			return err
		}
		definition.ActiveVersionID = &version.ID
		if err := s.Store.SaveDefinition(ctx, tx, definition); err != nil {
			return err
		}
		runtime := &store.DefinitionRuntime{
			DefinitionID: definition.ID, RunningCount: 0,
			MaxConcurrency: version.MaxConcurrency, UpdatedAt: now,
		}
		if err := s.Store.CreateRuntime(ctx, tx, runtime); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "definition.create", "task_definition", definition.ID,
			map[string]any{"key": definition.Key, "version": 1}, r)
		created = definition
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, s.serializeDefinition(ctx, s.Store.Pool, created))
}

func (s *Server) getDefinition(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "definition_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	definition, err := s.Store.GetDefinition(ctx, s.Store.Pool, id)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if definition == nil {
		writeError(w, r, notFound("definition not found"))
		return
	}
	writeJSON(w, 200, s.serializeDefinition(ctx, s.Store.Pool, definition))
}

func (s *Server) updateDefinition(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "definition_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body DefinitionUpdate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if body.Version != nil {
		if apiErr := validateVersionInput(body.Version); apiErr != nil {
			writeError(w, r, apiErr)
			return
		}
	}
	if body.Name != nil && (len(*body.Name) < 1 || len(*body.Name) > 160) {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.Description != nil && len(*body.Description) > 2000 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	ctx := r.Context()
	var updated *store.Definition
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		definition, err := s.Store.GetDefinition(ctx, tx, id)
		if err != nil {
			return err
		}
		if definition == nil {
			return notFound("definition not found")
		}
		if definition.Version != expected {
			return conflict("definition version conflict")
		}
		if body.Name != nil {
			definition.Name = *body.Name
		}
		if body.Description != nil {
			definition.Description = body.Description
		}
		if body.Version != nil {
			last, err := s.Store.MaxVersionNo(ctx, tx, definition.ID)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			payload := versionPayload(body.Version)
			version := &store.DefinitionVersion{
				ID: store.NewID(), DefinitionID: definition.ID, VersionNo: last + 1,
				ExecutorType: body.Version.ExecutorType, ParameterSchema: rawMap(body.Version.ParameterSchema),
				ExecutorConfig: rawMap(body.Version.ExecutorConfig), TimeoutSeconds: body.Version.TimeoutSeconds,
				RetryPolicy: rawMap(body.Version.RetryPolicy.Map()), MaxConcurrency: body.Version.MaxConcurrency,
				Checksum: core.Checksum(payload), CreatedBy: auth.User.ID, CreatedAt: now,
			}
			if err := s.Store.CreateVersion(ctx, tx, version); err != nil {
				return err
			}
			definition.ActiveVersionID = &version.ID
			runtime, err := s.Store.GetRuntime(ctx, tx, definition.ID)
			if err != nil {
				return err
			}
			if runtime != nil {
				if version.MaxConcurrency < runtime.RunningCount {
					return conflict("max_concurrency cannot be lower than running_count")
				}
				runtime.MaxConcurrency = version.MaxConcurrency
				if err := s.Store.SaveRuntime(ctx, tx, runtime); err != nil {
					return err
				}
			}
		}
		definition.Version++
		if err := s.Store.SaveDefinition(ctx, tx, definition); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "definition.update", "task_definition", definition.ID, nil, r)
		updated = definition
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, s.serializeDefinition(ctx, s.Store.Pool, updated))
}

func (s *Server) setDefinitionEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "definition_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	var updated *store.Definition
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		definition, err := s.Store.GetDefinition(ctx, tx, id)
		if err != nil {
			return err
		}
		if definition == nil {
			return notFound("definition not found")
		}
		if definition.Version != expected {
			return conflict("definition version conflict")
		}
		if definition.IsArchived {
			return &apiError{Status: 409, Code: "DEFINITION_DISABLED", Message: "definition is archived"}
		}
		if enabled {
			definition.Version++
			if err := s.Store.SaveDefinition(ctx, tx, definition); err != nil {
				return err
			}
			s.audit(ctx, tx, auth.User, "definition.enable", "task_definition", definition.ID, nil, r)
		} else {
			cancelled, err := s.Store.CancelScheduledRunsForDefinition(ctx, tx, definition.ID, "DEFINITION_DISABLED", "definition disabled")
			if err != nil {
				return err
			}
			paused, err := s.Store.PauseActiveSchedulesForDefinition(ctx, tx, definition.ID)
			if err != nil {
				return err
			}
			definition.Version++
			if err := s.Store.SaveDefinition(ctx, tx, definition); err != nil {
				return err
			}
			s.audit(ctx, tx, auth.User, "definition.disable", "task_definition", definition.ID,
				map[string]any{"cancelled_scheduled_runs": cancelled, "paused_schedules": paused}, r)
		}
		updated = definition
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, s.serializeDefinition(ctx, s.Store.Pool, updated))
}

func (s *Server) enableDefinition(w http.ResponseWriter, r *http.Request) {
	s.setDefinitionEnabled(w, r, true)
}

func (s *Server) disableDefinition(w http.ResponseWriter, r *http.Request) {
	s.setDefinitionEnabled(w, r, false)
}

func (s *Server) archiveDefinition(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "definition_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	var updated *store.Definition
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		definition, err := s.Store.GetDefinition(ctx, tx, id)
		if err != nil {
			return err
		}
		if definition == nil {
			return notFound("definition not found")
		}
		if definition.Version != expected {
			return conflict("definition version conflict")
		}
		if definition.IsArchived {
			updated = definition
			return nil
		}
		cancelled, err := s.Store.CancelPendingRunsForDefinition(ctx, tx, definition.ID, "DEFINITION_ARCHIVED", "definition archived")
		if err != nil {
			return err
		}
		if _, err := s.Store.CancelSchedulesForDefinition(ctx, tx, definition.ID); err != nil {
			return err
		}
		now := time.Now().UTC()
		definition.IsArchived = true
		definition.ArchivedAt = &now
		definition.Version++
		if err := s.Store.SaveDefinition(ctx, tx, definition); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "definition.archive", "task_definition", definition.ID,
			map[string]any{"cancelled_runs": cancelled}, r)
		updated = definition
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, s.serializeDefinition(ctx, s.Store.Pool, updated))
}

func (s *Server) deleteDefinition(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "definition_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	deleteRunsValue, apiErr := parseOptionalBool(r, "delete_runs")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	deleteRuns := deleteRunsValue != nil && *deleteRunsValue
	ctx := r.Context()
	deletedRuns := 0
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		definition, err := s.Store.GetDefinition(ctx, tx, id)
		if err != nil {
			return err
		}
		if definition == nil {
			return notFound("definition not found")
		}
		running, err := s.Store.CountRunningRunsForDefinition(ctx, tx, definition.ID)
		if err != nil {
			return err
		}
		if running > 0 {
			return conflict("definition has running tasks; cancel them first")
		}
		if !definition.IsArchived {
			now := time.Now().UTC()
			if _, err := s.Store.CancelPendingRunsForDefinition(ctx, tx, definition.ID, "DEFINITION_ARCHIVED", "definition archived"); err != nil {
				return err
			}
			if _, err := s.Store.CancelSchedulesForDefinition(ctx, tx, definition.ID); err != nil {
				return err
			}
			definition.IsArchived = true
			definition.ArchivedAt = &now
			definition.Version++
			if err := s.Store.SaveDefinition(ctx, tx, definition); err != nil {
				return err
			}
			s.audit(ctx, tx, auth.User, "definition.archive", "task_definition", definition.ID, nil, r)
		}
		if deleteRuns {
			ids, err := s.Store.DeleteRunsForDefinition(ctx, tx, definition.ID)
			if err != nil {
				return err
			}
			deletedRuns = len(ids)
			for _, runID := range ids {
				executor.CleanupRunOutput(s.Config.OutputRoot, runID)
			}
		}
		s.audit(ctx, tx, auth.User, "definition.delete", "task_definition", definition.ID,
			map[string]any{"archived": true, "deleted_runs": deletedRuns}, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, DefinitionDeleteResult{Archived: true, DeletedRuns: deletedRuns})
}
