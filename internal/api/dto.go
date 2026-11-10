package api

import (
	"encoding/json"
	"time"

	"github.com/tfenng/tasks/task2go/internal/store"
)

type UserOut struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"display_name"`
	Role               string     `json:"role"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Version            int        `json:"version"`
}

func userOut(u *store.User) UserOut {
	return UserOut{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role,
		IsActive: u.IsActive, MustChangePassword: u.MustChangePassword,
		LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		Version: u.Version,
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

type LoginResponse struct {
	User      UserOut `json:"user"`
	CSRFToken string  `json:"csrf_token"`
}

type UserCreate struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

type UserUpdate struct {
	DisplayName *string `json:"display_name"`
	Role        *string `json:"role"`
	IsActive    *bool   `json:"is_active"`
}

type UserList struct {
	Items      []UserOut `json:"items"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	Total      int       `json:"total"`
	TotalPages int       `json:"total_pages"`
}

type PasswordChange struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type PasswordResetResponse struct {
	TemporaryPassword string `json:"temporary_password"`
}

type SessionOut struct {
	ID         string    `json:"id"`
	IPAddress  *string   `json:"ip_address"`
	UserAgent  *string   `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SessionList struct {
	Items []SessionOut `json:"items"`
}

type APITokenOut struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type APITokenCreate struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type APITokenCreated struct {
	APITokenOut
	Token string `json:"token"`
}

type APITokenList struct {
	Items []APITokenOut `json:"items"`
}

type RetryPolicyInput struct {
	MaxAttempts           int      `json:"max_attempts"`
	InitialBackoffSeconds int      `json:"initial_backoff_seconds"`
	Multiplier            float64  `json:"multiplier"`
	MaxBackoffSeconds     int      `json:"max_backoff_seconds"`
	RetryOnExitCodes      []int    `json:"retry_on_exit_codes"`
	RetryableErrorClasses []string `json:"retryable_error_classes"`
}

func (r RetryPolicyInput) Map() map[string]any {
	maxAttempts := r.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 1
	}
	initial := r.InitialBackoffSeconds
	if initial == 0 {
		initial = 30
	}
	multiplier := r.Multiplier
	if multiplier == 0 {
		multiplier = 2
	}
	maxBackoff := r.MaxBackoffSeconds
	if maxBackoff == 0 {
		maxBackoff = 600
	}
	classes := r.RetryableErrorClasses
	if len(classes) == 0 {
		classes = []string{"TIMEOUT", "CONNECTION", "HTTP_429", "HTTP_5XX", "WORKER_LOST"}
	}
	codes := r.RetryOnExitCodes
	if codes == nil {
		codes = []int{}
	}
	return map[string]any{
		"max_attempts":            maxAttempts,
		"initial_backoff_seconds": initial,
		"multiplier":              multiplier,
		"max_backoff_seconds":     maxBackoff,
		"retry_on_exit_codes":     codes,
		"retryable_error_classes": classes,
	}
}

type DefinitionVersionInput struct {
	ExecutorType    string           `json:"executor_type"`
	ParameterSchema map[string]any   `json:"parameter_schema"`
	ExecutorConfig  map[string]any   `json:"executor_config"`
	TimeoutSeconds  int              `json:"timeout_seconds"`
	RetryPolicy     RetryPolicyInput `json:"retry_policy"`
	MaxConcurrency  int              `json:"max_concurrency"`
}

type DefinitionCreate struct {
	Key         string                 `json:"key"`
	Name        string                 `json:"name"`
	Description *string                `json:"description"`
	Version     DefinitionVersionInput `json:"version"`
}

type DefinitionUpdate struct {
	Name        *string                 `json:"name"`
	Description *string                 `json:"description"`
	Version     *DefinitionVersionInput `json:"version"`
}

type VersionOut struct {
	ID              string         `json:"id"`
	DefinitionID    string         `json:"definition_id"`
	VersionNo       int            `json:"version_no"`
	ExecutorType    string         `json:"executor_type"`
	ParameterSchema map[string]any `json:"parameter_schema"`
	ExecutorConfig  map[string]any `json:"executor_config"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	RetryPolicy     map[string]any `json:"retry_policy"`
	MaxConcurrency  int            `json:"max_concurrency"`
	Checksum        string         `json:"checksum"`
	CreatedAt       time.Time      `json:"created_at"`
}

func versionOut(v *store.DefinitionVersion) *VersionOut {
	if v == nil {
		return nil
	}
	return &VersionOut{
		ID: v.ID, DefinitionID: v.DefinitionID, VersionNo: v.VersionNo,
		ExecutorType: v.ExecutorType, ParameterSchema: v.ParameterSchema.Map(),
		ExecutorConfig: v.ExecutorConfig.Map(), TimeoutSeconds: v.TimeoutSeconds,
		RetryPolicy: v.RetryPolicy.Map(), MaxConcurrency: v.MaxConcurrency,
		Checksum: v.Checksum, CreatedAt: v.CreatedAt,
	}
}

type DefinitionOut struct {
	ID            string      `json:"id"`
	Key           string      `json:"key"`
	Name          string      `json:"name"`
	Description   *string     `json:"description"`
	IsArchived    bool        `json:"is_archived"`
	ActiveVersion *VersionOut `json:"active_version"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Version       int         `json:"version"`
}

type DefinitionList struct {
	Items      []DefinitionOut `json:"items"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

type DefinitionDeleteResult struct {
	Archived    bool `json:"archived"`
	DeletedRuns int  `json:"deleted_runs"`
}

type TriggerInput struct {
	Type string `json:"type"`
}

type RunCreate struct {
	DefinitionID        string         `json:"definition_id"`
	DefinitionVersionID *string        `json:"definition_version_id"`
	Arguments           []string       `json:"arguments"`
	Parameters          map[string]any `json:"parameters"`
	TimeoutSeconds      *int           `json:"timeout_seconds"`
	Trigger             TriggerInput   `json:"trigger"`
}

type RunOut struct {
	ID                  string         `json:"id"`
	DefinitionID        string         `json:"definition_id"`
	DefinitionVersionID string         `json:"definition_version_id"`
	ScheduleID          *string        `json:"schedule_id"`
	ScheduledFor        *time.Time     `json:"scheduled_for"`
	TriggerType         string         `json:"trigger_type"`
	Status              string         `json:"status"`
	Arguments           []string       `json:"arguments"`
	Parameters          map[string]any `json:"parameters"`
	AttemptCount        int            `json:"attempt_count"`
	MaxAttempts         int            `json:"max_attempts"`
	TimeoutSeconds      int            `json:"timeout_seconds"`
	QueuedAt            *time.Time     `json:"queued_at"`
	StartedAt           *time.Time     `json:"started_at"`
	FinishedAt          *time.Time     `json:"finished_at"`
	NextRetryAt         *time.Time     `json:"next_retry_at"`
	ParentRunID         *string        `json:"parent_run_id"`
	ErrorCode           *string        `json:"error_code"`
	ErrorMessage        *string        `json:"error_message"`
	ResultSummary       map[string]any `json:"result_summary"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	Version             int            `json:"version"`
}

func argumentsFrom(parameters store.JSONB) []string {
	raw, ok := parameters.Map()["__arguments__"]
	if !ok {
		return []string{}
	}
	list, ok := raw.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func runOut(r *store.Run) RunOut {
	var result map[string]any
	if r.ResultSummary != nil {
		result = r.ResultSummary.Map()
	}
	return RunOut{
		ID: r.ID, DefinitionID: r.DefinitionID, DefinitionVersionID: r.DefinitionVersionID,
		ScheduleID: r.ScheduleID, ScheduledFor: r.ScheduledFor, TriggerType: r.TriggerType,
		Status: r.Status, Arguments: argumentsFrom(r.ParametersRedacted),
		Parameters: r.ParametersRedacted.Map(), AttemptCount: r.AttemptCount,
		MaxAttempts: r.MaxAttempts, TimeoutSeconds: r.TimeoutSeconds,
		QueuedAt: r.QueuedAt, StartedAt: r.StartedAt,
		FinishedAt: r.FinishedAt, NextRetryAt: r.NextRetryAt, ParentRunID: r.ParentRunID,
		ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, ResultSummary: result,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
	}
}

type AttemptOut struct {
	ID              string     `json:"id"`
	RunID           string     `json:"run_id"`
	AttemptNo       int        `json:"attempt_no"`
	Status          string     `json:"status"`
	ExecutorType    string     `json:"executor_type"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	DurationMS      *int       `json:"duration_ms"`
	ExitCode        *int       `json:"exit_code"`
	HTTPStatus      *int       `json:"http_status"`
	ErrorClass      *string    `json:"error_class"`
	ErrorMessage    *string    `json:"error_message"`
	StdoutExcerpt   *string    `json:"stdout_excerpt"`
	StderrExcerpt   *string    `json:"stderr_excerpt"`
	ResponseExcerpt *string    `json:"response_excerpt"`
	Truncated       bool       `json:"truncated"`
}

func attemptOut(a store.Attempt) AttemptOut {
	return AttemptOut{
		ID: a.ID, RunID: a.RunID, AttemptNo: a.AttemptNo, Status: a.Status,
		ExecutorType: a.ExecutorType, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
		DurationMS: a.DurationMS, ExitCode: a.ExitCode, HTTPStatus: a.HTTPStatus,
		ErrorClass: a.ErrorClass, ErrorMessage: a.ErrorMessage,
		StdoutExcerpt: a.StdoutExcerpt, StderrExcerpt: a.StderrExcerpt,
		ResponseExcerpt: a.ResponseExcerpt, Truncated: a.Truncated,
	}
}

type RunDetail struct {
	RunOut
	DefinitionKey       *string        `json:"definition_key"`
	DefinitionName      *string        `json:"definition_name"`
	DefinitionVersionNo *int           `json:"definition_version_no"`
	ExecutorType        *string        `json:"executor_type"`
	ExecutorConfig      map[string]any `json:"executor_config"`
	ParameterSchema     map[string]any `json:"parameter_schema"`
	Attempts            []AttemptOut   `json:"attempts"`
}

type RunLiveOutput struct {
	RunID           string `json:"run_id"`
	AttemptNo       int    `json:"attempt_no"`
	Status          string `json:"status"`
	Running         bool   `json:"running"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutBytes     int    `json:"stdout_bytes"`
	StderrBytes     int    `json:"stderr_bytes"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
}

type RunList struct {
	Items      []RunOut `json:"items"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	Total      int      `json:"total"`
	TotalPages int      `json:"total_pages"`
}

type ScheduledTrigger struct {
	Type            string     `json:"type"`
	FireAt          *time.Time `json:"fire_at"`
	CronExpr        *string    `json:"cron_expr"`
	IntervalSeconds *int       `json:"interval_seconds"`
}

type ScheduleCreate struct {
	DefinitionID        string           `json:"definition_id"`
	DefinitionVersionID string           `json:"definition_version_id"`
	Name                string           `json:"name"`
	Trigger             ScheduledTrigger `json:"trigger"`
	Timezone            string           `json:"timezone"`
	Arguments           []string         `json:"arguments"`
	Parameters          map[string]any   `json:"parameters"`
	OverlapPolicy       string           `json:"overlap_policy"`
	MisfireGraceSeconds *int             `json:"misfire_grace_seconds"`
}

type ScheduleUpdate struct {
	Name                *string        `json:"name"`
	Parameters          map[string]any `json:"parameters"`
	OverlapPolicy       *string        `json:"overlap_policy"`
	MisfireGraceSeconds *int           `json:"misfire_grace_seconds"`
}

type ScheduleOut struct {
	ID                  string         `json:"id"`
	DefinitionID        string         `json:"definition_id"`
	DefinitionVersionID string         `json:"definition_version_id"`
	Name                string         `json:"name"`
	TriggerType         string         `json:"trigger_type"`
	FireAt              *time.Time     `json:"fire_at"`
	CronExpr            *string        `json:"cron_expr"`
	IntervalSeconds     *int           `json:"interval_seconds"`
	Timezone            string         `json:"timezone"`
	Arguments           []string       `json:"arguments"`
	Parameters          map[string]any `json:"parameters"`
	OverlapPolicy       string         `json:"overlap_policy"`
	MisfireGraceSeconds int            `json:"misfire_grace_seconds"`
	Status              string         `json:"status"`
	NextFireAt          *time.Time     `json:"next_fire_at"`
	LastFireAt          *time.Time     `json:"last_fire_at"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	Version             int            `json:"version"`
}

func scheduleOut(s *store.Schedule) ScheduleOut {
	return ScheduleOut{
		ID: s.ID, DefinitionID: s.DefinitionID, DefinitionVersionID: s.DefinitionVersionID,
		Name: s.Name, TriggerType: s.TriggerType, FireAt: s.FireAt, CronExpr: s.CronExpr,
		IntervalSeconds: s.IntervalSeconds, Timezone: s.Timezone,
		Arguments: argumentsFrom(s.ParametersRedacted), Parameters: s.ParametersRedacted.Map(),
		OverlapPolicy: s.OverlapPolicy, MisfireGraceSeconds: s.MisfireGraceSeconds,
		Status: s.Status, NextFireAt: s.NextFireAt, LastFireAt: s.LastFireAt,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Version: s.Version,
	}
}

type ScheduleList struct {
	Items      []ScheduleOut `json:"items"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	Total      int           `json:"total"`
	TotalPages int           `json:"total_pages"`
}

type ScheduleDeleteResult struct {
	Deleted int `json:"deleted"`
}

type CredentialOut struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Metadata  map[string]any `json:"metadata"`
	IsActive  bool           `json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Version   int            `json:"version"`
}

type CredentialCreate struct {
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Secret   string         `json:"secret"`
	Metadata map[string]any `json:"metadata"`
}

type CredentialUpdate struct {
	Name     *string        `json:"name"`
	Secret   *string        `json:"secret"`
	Metadata map[string]any `json:"metadata"`
	IsActive *bool          `json:"is_active"`
}

type CredentialList struct {
	Items      []CredentialOut `json:"items"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

type TargetOut struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Config       map[string]any `json:"config"`
	CredentialID *string        `json:"credential_id"`
	IsActive     bool           `json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Version      int            `json:"version"`
}

type TargetCreate struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Config       map[string]any `json:"config"`
	CredentialID *string        `json:"credential_id"`
}

type TargetUpdate struct {
	Name            *string        `json:"name"`
	Config          map[string]any `json:"config"`
	CredentialID    *string        `json:"credential_id"`
	CredentialIDSet bool           `json:"-"`
	IsActive        *bool          `json:"is_active"`
}

type TargetList struct {
	Items      []TargetOut `json:"items"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Total      int         `json:"total"`
	TotalPages int         `json:"total_pages"`
}

type StatsBucket struct {
	Key           string   `json:"key"`
	Total         int      `json:"total"`
	Succeeded     int      `json:"succeeded"`
	Failed        int      `json:"failed"`
	Cancelled     int      `json:"cancelled"`
	Skipped       int      `json:"skipped"`
	Missed        int      `json:"missed"`
	SuccessRate   float64  `json:"success_rate"`
	AvgDurationMS *float64 `json:"avg_duration_ms"`
	P95DurationMS *float64 `json:"p95_duration_ms"`
}

type RunStats struct {
	Total         int           `json:"total"`
	Succeeded     int           `json:"succeeded"`
	Failed        int           `json:"failed"`
	Cancelled     int           `json:"cancelled"`
	Skipped       int           `json:"skipped"`
	Missed        int           `json:"missed"`
	Retrying      int           `json:"retrying"`
	SuccessRate   float64       `json:"success_rate"`
	AvgDurationMS *float64      `json:"avg_duration_ms"`
	P95DurationMS *float64      `json:"p95_duration_ms"`
	Buckets       []StatsBucket `json:"buckets"`
}

type AttemptStats struct {
	Total         int           `json:"total"`
	Succeeded     int           `json:"succeeded"`
	Failed        int           `json:"failed"`
	TimedOut      int           `json:"timed_out"`
	Cancelled     int           `json:"cancelled"`
	SuccessRate   float64       `json:"success_rate"`
	AvgDurationMS *float64      `json:"avg_duration_ms"`
	Buckets       []StatsBucket `json:"buckets"`
}

type AuditEventOut struct {
	ID            int64          `json:"id"`
	ActorUserID   *string        `json:"actor_user_id"`
	ActorUsername *string        `json:"actor_username"`
	Action        string         `json:"action"`
	ResourceType  string         `json:"resource_type"`
	ResourceID    *string        `json:"resource_id"`
	Outcome       string         `json:"outcome"`
	IPAddress     *string        `json:"ip_address"`
	UserAgent     *string        `json:"user_agent"`
	CorrelationID string         `json:"correlation_id"`
	Details       map[string]any `json:"details"`
	CreatedAt     time.Time      `json:"created_at"`
}

type AuditEventList struct {
	Items      []AuditEventOut `json:"items"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

type ScriptEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size *int   `json:"size"`
}

type ScriptList struct {
	Root   string        `json:"root"`
	Path   string        `json:"path"`
	Parent *string       `json:"parent"`
	Items  []ScriptEntry `json:"items"`
}

type HealthResponse struct {
	Status string                       `json:"status"`
	Checks map[string]map[string]string `json:"checks"`
}

func rawMap(value map[string]any) store.JSONB {
	if value == nil {
		return store.JSONB{}
	}
	return store.JSONB(value)
}

func jsonRaw(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
