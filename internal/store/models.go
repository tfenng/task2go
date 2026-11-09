package store

import "time"

type User struct {
	ID                 string     `db:"id"`
	Username           string     `db:"username"`
	DisplayName        string     `db:"display_name"`
	PasswordHash       string     `db:"password_hash"`
	Role               string     `db:"role"`
	IsActive           bool       `db:"is_active"`
	MustChangePassword bool       `db:"must_change_password"`
	FailedLoginCount   int        `db:"failed_login_count"`
	LockedUntil        *time.Time `db:"locked_until"`
	LastLoginAt        *time.Time `db:"last_login_at"`
	CreatedAt          time.Time  `db:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at"`
	Version            int        `db:"version"`
}

type Session struct {
	ID            string     `db:"id"`
	UserID        string     `db:"user_id"`
	TokenHash     []byte     `db:"token_hash"`
	CSRFTokenHash []byte     `db:"csrf_token_hash"`
	IPAddress     *string    `db:"ip_address"`
	UserAgent     *string    `db:"user_agent"`
	CreatedAt     time.Time  `db:"created_at"`
	LastSeenAt    time.Time  `db:"last_seen_at"`
	ExpiresAt     time.Time  `db:"expires_at"`
	RevokedAt     *time.Time `db:"revoked_at"`
}

type APIToken struct {
	ID          string     `db:"id"`
	UserID      string     `db:"user_id"`
	Name        string     `db:"name"`
	TokenPrefix string     `db:"token_prefix"`
	TokenHash   []byte     `db:"token_hash"`
	ExpiresAt   *time.Time `db:"expires_at"`
	LastUsedAt  *time.Time `db:"last_used_at"`
	RevokedAt   *time.Time `db:"revoked_at"`
	CreatedAt   time.Time  `db:"created_at"`
}

type Definition struct {
	ID              string     `db:"id"`
	Key             string     `db:"key"`
	Name            string     `db:"name"`
	Description     *string    `db:"description"`
	ActiveVersionID *string    `db:"active_version_id"`
	IsArchived      bool       `db:"is_archived"`
	ArchivedAt      *time.Time `db:"archived_at"`
	CreatedBy       string     `db:"created_by"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	Version         int        `db:"version"`
}

type DefinitionVersion struct {
	ID              string    `db:"id"`
	DefinitionID    string    `db:"definition_id"`
	VersionNo       int       `db:"version_no"`
	ExecutorType    string    `db:"executor_type"`
	ParameterSchema JSONB     `db:"parameter_schema"`
	ExecutorConfig  JSONB     `db:"executor_config"`
	TimeoutSeconds  int       `db:"timeout_seconds"`
	RetryPolicy     JSONB     `db:"retry_policy"`
	MaxConcurrency  int       `db:"max_concurrency"`
	Checksum        string    `db:"checksum"`
	CreatedBy       string    `db:"created_by"`
	CreatedAt       time.Time `db:"created_at"`
}

type DefinitionRuntime struct {
	DefinitionID   string    `db:"definition_id"`
	RunningCount   int       `db:"running_count"`
	MaxConcurrency int       `db:"max_concurrency"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type Credential struct {
	ID         string    `db:"id"`
	Name       string    `db:"name"`
	Kind       string    `db:"kind"`
	Ciphertext []byte    `db:"ciphertext"`
	KeyVersion int       `db:"key_version"`
	Metadata   JSONB     `db:"metadata"`
	IsActive   bool      `db:"is_active"`
	CreatedBy  string    `db:"created_by"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
	Version    int       `db:"version"`
}

type ExecutionTarget struct {
	ID           string    `db:"id"`
	Name         string    `db:"name"`
	Type         string    `db:"type"`
	Config       JSONB     `db:"config"`
	CredentialID *string   `db:"credential_id"`
	IsActive     bool      `db:"is_active"`
	CreatedBy    string    `db:"created_by"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	Version      int       `db:"version"`
}

type Schedule struct {
	ID                  string     `db:"id"`
	DefinitionID        string     `db:"definition_id"`
	DefinitionVersionID string     `db:"definition_version_id"`
	Name                string     `db:"name"`
	TriggerType         string     `db:"trigger_type"`
	FireAt              *time.Time `db:"fire_at"`
	CronExpr            *string    `db:"cron_expr"`
	IntervalSeconds     *int       `db:"interval_seconds"`
	Timezone            string     `db:"timezone"`
	Parameters          JSONB      `db:"parameters"`
	ParametersRedacted  JSONB      `db:"parameters_redacted"`
	OverlapPolicy       string     `db:"overlap_policy"`
	MisfireGraceSeconds int        `db:"misfire_grace_seconds"`
	Status              string     `db:"status"`
	NextFireAt          *time.Time `db:"next_fire_at"`
	LastFireAt          *time.Time `db:"last_fire_at"`
	CreatedBy           string     `db:"created_by"`
	CreatedAt           time.Time  `db:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
	Version             int        `db:"version"`
}

type Worker struct {
	ID              string     `db:"id"`
	Hostname        string     `db:"hostname"`
	PID             int        `db:"pid"`
	Role            string     `db:"role"`
	StartedAt       time.Time  `db:"started_at"`
	LastHeartbeatAt time.Time  `db:"last_heartbeat_at"`
	StoppedAt       *time.Time `db:"stopped_at"`
}

type Run struct {
	ID                  string     `db:"id"`
	DefinitionID        string     `db:"definition_id"`
	DefinitionVersionID string     `db:"definition_version_id"`
	TargetSnapshot      JSONB      `db:"target_snapshot"`
	ScheduleID          *string    `db:"schedule_id"`
	ScheduledFor        *time.Time `db:"scheduled_for"`
	TriggerType         string     `db:"trigger_type"`
	Status              string     `db:"status"`
	IdempotencyKey      *string    `db:"idempotency_key"`
	RequestHash         *string    `db:"request_hash"`
	Parameters          JSONB      `db:"parameters"`
	ParametersRedacted  JSONB      `db:"parameters_redacted"`
	QueuedAt            *time.Time `db:"queued_at"`
	StartedAt           *time.Time `db:"started_at"`
	FinishedAt          *time.Time `db:"finished_at"`
	NextRetryAt         *time.Time `db:"next_retry_at"`
	AttemptCount        int        `db:"attempt_count"`
	MaxAttempts         int        `db:"max_attempts"`
	WorkerID            *string    `db:"worker_id"`
	LeaseExpiresAt      *time.Time `db:"lease_expires_at"`
	ParentRunID         *string    `db:"parent_run_id"`
	CancelRequestedAt   *time.Time `db:"cancel_requested_at"`
	CancelRequestedBy   *string    `db:"cancel_requested_by"`
	ErrorCode           *string    `db:"error_code"`
	ErrorMessage        *string    `db:"error_message"`
	ResultSummary       JSONB      `db:"result_summary"`
	CreatedBy           string     `db:"created_by"`
	CreatedAt           time.Time  `db:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at"`
	Version             int        `db:"version"`
}

type Attempt struct {
	ID              string     `db:"id"`
	RunID           string     `db:"run_id"`
	AttemptNo       int        `db:"attempt_no"`
	Status          string     `db:"status"`
	ExecutorType    string     `db:"executor_type"`
	TargetID        *string    `db:"target_id"`
	WorkerID        *string    `db:"worker_id"`
	StartedAt       time.Time  `db:"started_at"`
	FinishedAt      *time.Time `db:"finished_at"`
	DurationMS      *int       `db:"duration_ms"`
	ExitCode        *int       `db:"exit_code"`
	HTTPStatus      *int       `db:"http_status"`
	ErrorClass      *string    `db:"error_class"`
	ErrorMessage    *string    `db:"error_message"`
	StdoutExcerpt   *string    `db:"stdout_excerpt"`
	StderrExcerpt   *string    `db:"stderr_excerpt"`
	ResponseExcerpt *string    `db:"response_excerpt"`
	Truncated       bool       `db:"truncated"`
	CreatedAt       time.Time  `db:"created_at"`
}

type AuditEvent struct {
	ID            int64     `db:"id"`
	ActorUserID   *string   `db:"actor_user_id"`
	Action        string    `db:"action"`
	ResourceType  string    `db:"resource_type"`
	ResourceID    *string   `db:"resource_id"`
	Outcome       string    `db:"outcome"`
	IPAddress     *string   `db:"ip_address"`
	UserAgent     *string   `db:"user_agent"`
	CorrelationID string    `db:"correlation_id"`
	Details       JSONB     `db:"details"`
	CreatedAt     time.Time `db:"created_at"`
}
