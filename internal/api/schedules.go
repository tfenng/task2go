package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/params"
	"github.com/tfenng/tasks/task2go/internal/store"
)

func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := s.Store.ListSchedules(r.Context(), s.Store.Pool, p.Limit, p.Offset, definitionID, parseOptionalString(r, "status"))
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	out := make([]ScheduleOut, 0, len(items))
	for i := range items {
		out = append(out, scheduleOut(&items[i]))
	}
	writeJSON(w, 200, ScheduleList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body ScheduleCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if !validUUID(body.DefinitionID) || !validUUID(body.DefinitionVersionID) {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if len(body.Name) < 1 || len(body.Name) > 120 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.Timezone == "" {
		body.Timezone = "UTC"
	}
	if body.OverlapPolicy == "" {
		body.OverlapPolicy = "skip"
	}
	if body.OverlapPolicy != "skip" && body.OverlapPolicy != "queue" {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	graceSeconds := 3600
	if body.MisfireGraceSeconds != nil {
		if *body.MisfireGraceSeconds < 0 {
			writeError(w, r, validationErr("request validation failed"))
			return
		}
		graceSeconds = *body.MisfireGraceSeconds
	}
	if body.Trigger.Type != "once" && body.Trigger.Type != "cron" && body.Trigger.Type != "interval" {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	ctx := r.Context()
	var created *store.Schedule
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
		version, err := s.Store.ActiveVersion(ctx, tx, definition, &body.DefinitionVersionID)
		if err != nil {
			return err
		}
		if version == nil {
			return notFound("definition version not found")
		}
		scheduleID := store.NewID()
		var encrypted, redacted map[string]any
		if len(body.Arguments) > 0 {
			encrypted = map[string]any{"__arguments__": stringsToAny(body.Arguments)}
			redacted = encrypted
		} else {
			parameters := params.ApplyDefaults(version.ParameterSchema.Map(), body.Parameters)
			if err := params.ValidateParameters(ctx, version.ParameterSchema.Map(), parameters); err != nil {
				if ve, ok := err.(*params.ValidationError); ok {
					return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid parameters", Details: ve.Errors}
				}
				return err
			}
			encrypted, err = params.EncryptParameters(version.ParameterSchema.Map(), parameters, "schedule:"+scheduleID, s.Box)
			if err != nil {
				return err
			}
			redacted = params.Redact(version.ParameterSchema.Map(), parameters)
		}
		now := time.Now().UTC()
		var nextFire *time.Time
		if body.Trigger.Type == "once" {
			if body.Trigger.FireAt != nil && body.Trigger.FireAt.After(now) {
				nextFire = body.Trigger.FireAt
			} else {
				nextFire = &now
			}
		} else {
			nextFire, err = core.ComputeNextFire(body.Trigger.Type, body.Trigger.FireAt, body.Trigger.CronExpr, body.Trigger.IntervalSeconds, body.Timezone, now)
			if err != nil {
				return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: err.Error()}
			}
		}
		schedule := &store.Schedule{
			ID: scheduleID, DefinitionID: definition.ID, DefinitionVersionID: version.ID,
			Name: body.Name, TriggerType: body.Trigger.Type, FireAt: body.Trigger.FireAt,
			CronExpr: body.Trigger.CronExpr, IntervalSeconds: body.Trigger.IntervalSeconds,
			Timezone: body.Timezone, Parameters: rawMap(encrypted), ParametersRedacted: rawMap(redacted),
			OverlapPolicy: body.OverlapPolicy, MisfireGraceSeconds: graceSeconds,
			Status: "active", NextFireAt: nextFire, CreatedBy: auth.User.ID,
			CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		if err := s.Store.CreateSchedule(ctx, tx, schedule); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "schedule.create", "schedule", schedule.ID,
			map[string]any{"definition_id": definition.ID}, r)
		created = schedule
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, scheduleOut(created))
}

func (s *Server) deleteAllSchedules(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	deleted := 0
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		deleted, err = s.Store.DeleteAllSchedules(ctx, tx)
		if err != nil {
			return err
		}
		if deleted > 0 {
			s.audit(ctx, tx, auth.User, "schedule.delete_all", "schedule", "",
				map[string]any{"deleted": deleted}, r)
		}
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, ScheduleDeleteResult{Deleted: deleted})
}

func (s *Server) getSchedule(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "schedule_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	schedule, err := s.Store.GetSchedule(r.Context(), s.Store.Pool, id)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if schedule == nil {
		writeError(w, r, notFound("schedule not found"))
		return
	}
	writeJSON(w, 200, scheduleOut(schedule))
}

func (s *Server) updateSchedule(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "schedule_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body ScheduleUpdate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	ctx := r.Context()
	var updated *store.Schedule
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		schedule, err := s.Store.GetSchedule(ctx, tx, id)
		if err != nil {
			return err
		}
		if schedule == nil {
			return notFound("schedule not found")
		}
		if schedule.Version != expected {
			return conflict("schedule version conflict")
		}
		if body.Name != nil {
			if len(*body.Name) < 1 || len(*body.Name) > 120 {
				return validationErr("request validation failed")
			}
			schedule.Name = *body.Name
		}
		if body.OverlapPolicy != nil {
			if *body.OverlapPolicy != "skip" && *body.OverlapPolicy != "queue" {
				return validationErr("request validation failed")
			}
			schedule.OverlapPolicy = *body.OverlapPolicy
		}
		if body.MisfireGraceSeconds != nil {
			if *body.MisfireGraceSeconds < 0 {
				return validationErr("request validation failed")
			}
			schedule.MisfireGraceSeconds = *body.MisfireGraceSeconds
		}
		if body.Parameters != nil {
			version, err := s.Store.GetVersion(ctx, tx, schedule.DefinitionVersionID)
			if err != nil {
				return err
			}
			if version == nil {
				return notFound("definition version not found")
			}
			parameters := params.ApplyDefaults(version.ParameterSchema.Map(), body.Parameters)
			if err := params.ValidateParameters(ctx, version.ParameterSchema.Map(), parameters); err != nil {
				if ve, ok := err.(*params.ValidationError); ok {
					return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid parameters", Details: ve.Errors}
				}
				return err
			}
			encrypted, err := params.EncryptParameters(version.ParameterSchema.Map(), parameters, "schedule:"+schedule.ID, s.Box)
			if err != nil {
				return err
			}
			schedule.Parameters = rawMap(encrypted)
			schedule.ParametersRedacted = rawMap(params.Redact(version.ParameterSchema.Map(), parameters))
		}
		schedule.Version++
		if err := s.Store.SaveSchedule(ctx, tx, schedule); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "schedule.update", "schedule", schedule.ID, nil, r)
		updated = schedule
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, scheduleOut(updated))
}

func (s *Server) scheduleAction(w http.ResponseWriter, r *http.Request, action string) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "schedule_id")
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
	var updated *store.Schedule
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		schedule, err := s.Store.GetSchedule(ctx, tx, id)
		if err != nil {
			return err
		}
		if schedule == nil {
			return notFound("schedule not found")
		}
		if schedule.Version != expected {
			return conflict("schedule version conflict")
		}
		switch action {
		case "pause":
			if schedule.Status != "active" {
				return invalidState("schedule is not active")
			}
			schedule.Status = "paused"
			schedule.NextFireAt = nil
		case "resume":
			if schedule.Status != "paused" {
				return invalidState("schedule is not paused")
			}
			next, err := core.ComputeNextFire(schedule.TriggerType, schedule.FireAt, schedule.CronExpr, schedule.IntervalSeconds, schedule.Timezone, time.Now().UTC())
			if err != nil {
				return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: err.Error()}
			}
			if next == nil {
				return invalidState("schedule has no future fire time")
			}
			schedule.Status = "active"
			schedule.NextFireAt = next
		case "cancel":
			if schedule.Status == "completed" || schedule.Status == "cancelled" {
				return invalidState("schedule is already terminal")
			}
			schedule.Status = "cancelled"
			schedule.NextFireAt = nil
		}
		schedule.Version++
		if err := s.Store.SaveSchedule(ctx, tx, schedule); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "schedule."+action, "schedule", schedule.ID, nil, r)
		updated = schedule
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, scheduleOut(updated))
}

func (s *Server) pauseSchedule(w http.ResponseWriter, r *http.Request) {
	s.scheduleAction(w, r, "pause")
}
func (s *Server) resumeSchedule(w http.ResponseWriter, r *http.Request) {
	s.scheduleAction(w, r, "resume")
}
func (s *Server) cancelSchedule(w http.ResponseWriter, r *http.Request) {
	s.scheduleAction(w, r, "cancel")
}

func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin", "operator")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "schedule_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		schedule, err := s.Store.GetSchedule(ctx, tx, id)
		if err != nil {
			return err
		}
		if schedule == nil {
			return notFound("schedule not found")
		}
		s.audit(ctx, tx, auth.User, "schedule.delete", "schedule", schedule.ID,
			map[string]any{"name": schedule.Name, "definition_id": schedule.DefinitionID}, r)
		return s.Store.DeleteSchedule(ctx, tx, id)
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
