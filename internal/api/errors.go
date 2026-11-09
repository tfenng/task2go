package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tfenng/tasks/task2go/internal/core"
	"github.com/tfenng/tasks/task2go/internal/params"
)

type apiError struct {
	Status     int
	Code       string
	Message    string
	Details    []params.FieldError
	RetryAfter int
}

func (e *apiError) Error() string { return e.Message }

func notFound(message string) *apiError {
	return &apiError{Status: 404, Code: "NOT_FOUND", Message: message}
}
func forbidden(message string) *apiError {
	return &apiError{Status: 403, Code: "FORBIDDEN", Message: message}
}
func conflict(message string) *apiError {
	return &apiError{Status: 409, Code: "CONFLICT_VERSION", Message: message}
}
func invalidState(message string) *apiError {
	return &apiError{Status: 409, Code: "INVALID_STATE", Message: message}
}
func validationErr(message string, details ...params.FieldError) *apiError {
	return &apiError{Status: 422, Code: "VALIDATION_ERROR", Message: message, Details: details}
}
func authRequired() *apiError {
	return &apiError{Status: 401, Code: "AUTH_REQUIRED", Message: "authentication required"}
}
func invalidCredentials(message string) *apiError {
	return &apiError{Status: 401, Code: "AUTH_INVALID_CREDENTIALS", Message: message}
}
func internalError(message string) *apiError {
	return &apiError{Status: 500, Code: "INTERNAL_ERROR", Message: message}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value != nil {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(value)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err *apiError) {
	if err.Status == 0 {
		err.Status = 500
	}
	if err.Details == nil {
		err.Details = []params.FieldError{}
	}
	if err.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(err.RetryAfter))
	}
	writeJSON(w, err.Status, map[string]any{
		"error": map[string]any{
			"code":           err.Code,
			"message":        err.Message,
			"details":        err.Details,
			"correlation_id": correlationID(r),
		},
	})
}

func parseJSON(r *http.Request, value any) *apiError {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4*1024*1024))
	dec.UseNumber()
	if err := dec.Decode(value); err != nil {
		return validationErr("request validation failed", params.FieldError{
			Field: "body", Code: "invalid", Message: err.Error(),
		})
	}
	return nil
}

func pathID(r *http.Request, name string) (string, *apiError) {
	raw := r.PathValue(name)
	if _, err := uuid.Parse(raw); err != nil {
		return "", validationErr("request validation failed", params.FieldError{
			Field: name, Code: "uuid_parsing", Message: "invalid UUID",
		})
	}
	return raw, nil
}

func ifMatch(r *http.Request) (int, *apiError) {
	raw := r.Header.Get("If-Match")
	if raw == "" {
		return 0, &apiError{Status: 428, Code: "PRECONDITION_REQUIRED", Message: "If-Match header is required"}
	}
	value, err := strconv.Atoi(strings.Trim(strings.TrimSpace(raw), `"`))
	if err != nil {
		return 0, validationErr("If-Match must be a version number")
	}
	if value < 1 {
		return 0, validationErr("If-Match must be positive")
	}
	return value, nil
}

type page struct {
	Page       int
	PageSize   int
	Limit      int
	Offset     int
	Total      int
	TotalPages int
}

func parsePage(r *http.Request) (page, *apiError) {
	p := page{Page: 1, PageSize: 20}
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return p, validationErr("request validation failed", params.FieldError{Field: "page", Code: "greater_than_equal", Message: "Input should be greater than or equal to 1"})
		}
		p.Page = n
	}
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return p, validationErr("request validation failed", params.FieldError{Field: "page_size", Code: "less_than_equal", Message: "Input should be between 1 and 100"})
		}
		p.PageSize = n
	}
	p.Limit = p.PageSize
	p.Offset = (p.Page - 1) * p.PageSize
	return p, nil
}

func (p *page) setTotal(total int) {
	p.Total = total
	p.TotalPages = (total + p.PageSize - 1) / p.PageSize
}

func parseOptionalBool(r *http.Request, name string) (*bool, *apiError) {
	raw, ok := r.URL.Query()[name]
	if !ok || len(raw) == 0 || raw[0] == "" {
		return nil, nil
	}
	switch strings.ToLower(raw[0]) {
	case "1", "true", "yes", "on":
		v := true
		return &v, nil
	case "0", "false", "no", "off":
		v := false
		return &v, nil
	default:
		return nil, validationErr("request validation failed", params.FieldError{Field: name, Code: "bool_parsing", Message: "Input should be a valid boolean"})
	}
}

func parseOptionalString(r *http.Request, name string) *string {
	if raw := r.URL.Query().Get(name); raw != "" {
		return &raw
	}
	return nil
}

func parseIntDefault(r *http.Request, name string, def int) (int, *apiError) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, validationErr("request validation failed", params.FieldError{Field: name, Code: "int_parsing", Message: "Input should be a valid integer"})
	}
	return n, nil
}

func parseOptionalTime(r *http.Request, name string) (*time.Time, *apiError) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		value, err = time.Parse("2006-01-02", raw)
	}
	if err != nil {
		return nil, validationErr("request validation failed", params.FieldError{Field: name, Code: "datetime_parsing", Message: "Input should be a valid datetime"})
	}
	value = value.UTC()
	return &value, nil
}

func strPtr(v string) *string        { return &v }
func boolPtr(v bool) *bool           { return &v }
func timePtr(v time.Time) *time.Time { return &v }
func optionalPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func asAPIError(err error) *apiError {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae
	}
	switch {
	case errors.Is(err, core.ErrNotFound):
		return notFound(err.Error())
	case errors.Is(err, core.ErrTargetDisabled):
		return &apiError{Status: 409, Code: "TARGET_DISABLED", Message: "execution target or credential disabled"}
	case errors.Is(err, core.ErrConflict):
		return conflict(err.Error())
	case errors.Is(err, core.ErrValidation):
		return validationErr(err.Error())
	}
	return internalError("internal server error")
}

func badRequest(message string) *apiError {
	return &apiError{Status: 400, Code: "VALIDATION_ERROR", Message: message}
}
