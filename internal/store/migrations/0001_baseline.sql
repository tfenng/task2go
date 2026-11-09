-- Task Control baseline schema.
-- Idempotent so it can adopt a database created by Alembic at the current head.

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY,
    username varchar(64) NOT NULL,
    display_name varchar(128) NOT NULL,
    password_hash text NOT NULL,
    role varchar(16) NOT NULL DEFAULT 'viewer',
    is_active boolean NOT NULL DEFAULT true,
    must_change_password boolean NOT NULL DEFAULT false,
    failed_login_count integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT ck_users_role CHECK (role IN ('admin','operator','viewer'))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_username_lower ON users (lower(username));

CREATE TABLE IF NOT EXISTS workers (
    id uuid PRIMARY KEY,
    hostname varchar(255) NOT NULL,
    pid integer NOT NULL,
    role varchar(16) NOT NULL,
    started_at timestamptz NOT NULL,
    last_heartbeat_at timestamptz NOT NULL,
    stopped_at timestamptz,
    CONSTRAINT ck_workers_role CHECK (role IN ('worker','scheduler'))
);

CREATE TABLE IF NOT EXISTS audit_events (
    id bigserial PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action varchar(64) NOT NULL,
    resource_type varchar(32) NOT NULL,
    resource_id varchar(64),
    outcome varchar(16) NOT NULL DEFAULT 'success',
    ip_address varchar(64),
    user_agent varchar(512),
    correlation_id varchar(64) NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT ck_audit_events_outcome CHECK (outcome IN ('success','failure','denied'))
);
CREATE INDEX IF NOT EXISTS ix_audit_actor ON audit_events (actor_user_id, created_at);
CREATE INDEX IF NOT EXISTS ix_audit_resource ON audit_events (resource_type, resource_id, created_at);
CREATE INDEX IF NOT EXISTS ix_audit_time ON audit_events (created_at);

CREATE TABLE IF NOT EXISTS sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    csrf_token_hash bytea NOT NULL,
    ip_address varchar(64),
    user_agent varchar(512),
    created_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS ix_sessions_user ON sessions (user_id, revoked_at);

CREATE TABLE IF NOT EXISTS task_definitions (
    id uuid PRIMARY KEY,
    key varchar(100) NOT NULL UNIQUE,
    name varchar(160) NOT NULL,
    description text,
    active_version_id uuid,
    is_archived boolean NOT NULL DEFAULT false,
    archived_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT ck_task_definitions_archive_state CHECK (is_archived = (archived_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS task_definition_versions (
    id uuid PRIMARY KEY,
    definition_id uuid NOT NULL REFERENCES task_definitions(id) ON DELETE RESTRICT,
    version_no integer NOT NULL,
    executor_type varchar(16) NOT NULL,
    parameter_schema jsonb NOT NULL,
    executor_config jsonb NOT NULL,
    timeout_seconds integer NOT NULL,
    retry_policy jsonb NOT NULL,
    max_concurrency integer NOT NULL DEFAULT 1,
    checksum varchar(64) NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    CONSTRAINT uq_definition_version UNIQUE (definition_id, version_no),
    CONSTRAINT ck_task_definition_versions_executor_type CHECK (executor_type IN ('local','ssh','http')),
    CONSTRAINT ck_task_definition_versions_timeout CHECK (timeout_seconds BETWEEN 1 AND 86400),
    CONSTRAINT ck_task_definition_versions_concurrency CHECK (max_concurrency BETWEEN 1 AND 100)
);

CREATE TABLE IF NOT EXISTS task_definition_runtime (
    definition_id uuid PRIMARY KEY REFERENCES task_definitions(id) ON DELETE CASCADE,
    running_count integer NOT NULL DEFAULT 0,
    max_concurrency integer NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL,
    CONSTRAINT ck_task_definition_runtime_running_count CHECK (running_count >= 0),
    CONSTRAINT ck_task_definition_runtime_concurrency CHECK (max_concurrency BETWEEN 1 AND 100)
);

CREATE TABLE IF NOT EXISTS runs (
    id uuid PRIMARY KEY,
    definition_id uuid NOT NULL REFERENCES task_definitions(id) ON DELETE RESTRICT,
    definition_version_id uuid NOT NULL REFERENCES task_definition_versions(id) ON DELETE RESTRICT,
    target_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    schedule_id uuid,
    scheduled_for timestamptz,
    trigger_type varchar(16) NOT NULL DEFAULT 'immediate',
    status varchar(16) NOT NULL DEFAULT 'queued',
    idempotency_key varchar(128),
    request_hash varchar(64),
    parameters jsonb NOT NULL,
    parameters_redacted jsonb NOT NULL,
    queued_at timestamptz,
    started_at timestamptz,
    finished_at timestamptz,
    next_retry_at timestamptz,
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 1,
    worker_id uuid REFERENCES workers(id) ON DELETE SET NULL,
    lease_expires_at timestamptz,
    parent_run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
    cancel_requested_at timestamptz,
    cancel_requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
    error_code varchar(64),
    error_message text,
    result_summary jsonb,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT ck_runs_status CHECK (status IN ('scheduled','queued','running','retry_wait','succeeded','failed','cancelled','skipped','missed')),
    CONSTRAINT ck_runs_attempt_count CHECK (attempt_count >= 0),
    CONSTRAINT ck_runs_attempt_limit CHECK (attempt_count <= max_attempts)
);
CREATE INDEX IF NOT EXISTS ix_runs_definition_time ON runs (definition_id, created_at);
CREATE INDEX IF NOT EXISTS ix_runs_lease ON runs (lease_expires_at) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS ix_runs_queue ON runs (queued_at, id) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS ix_runs_retention ON runs (created_at);
CREATE INDEX IF NOT EXISTS ix_runs_status_time ON runs (status, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_runs_idempotency ON runs (created_by, idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS attempts (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    attempt_no integer NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'running',
    executor_type varchar(16) NOT NULL,
    target_id uuid,
    worker_id uuid REFERENCES workers(id) ON DELETE SET NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    duration_ms integer,
    exit_code integer,
    http_status integer,
    error_class varchar(64),
    error_message text,
    stdout_excerpt text,
    stderr_excerpt text,
    response_excerpt text,
    truncated boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    CONSTRAINT uq_attempt_no UNIQUE (run_id, attempt_no),
    CONSTRAINT ck_attempts_status CHECK (status IN ('running','succeeded','failed','timed_out','cancelled'))
);
CREATE INDEX IF NOT EXISTS ix_attempt_created ON attempts (created_at);
CREATE INDEX IF NOT EXISTS ix_attempt_run ON attempts (run_id, attempt_no);

CREATE TABLE IF NOT EXISTS api_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    token_prefix varchar(12) NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_api_tokens_user ON api_tokens (user_id, revoked_at);

CREATE TABLE IF NOT EXISTS credentials (
    id uuid PRIMARY KEY,
    name varchar(120) NOT NULL UNIQUE,
    kind varchar(16) NOT NULL,
    ciphertext bytea NOT NULL,
    key_version integer NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_active boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT ck_credentials_kind CHECK (kind IN ('password','ssh_key','http_bearer','http_basic'))
);

CREATE TABLE IF NOT EXISTS execution_targets (
    id uuid PRIMARY KEY,
    name varchar(120) NOT NULL UNIQUE,
    type varchar(16) NOT NULL,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    credential_id uuid REFERENCES credentials(id) ON DELETE SET NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT ck_execution_targets_type CHECK (type IN ('local','ssh','http'))
);

CREATE TABLE IF NOT EXISTS schedules (
    id uuid PRIMARY KEY,
    definition_id uuid NOT NULL REFERENCES task_definitions(id) ON DELETE RESTRICT,
    definition_version_id uuid NOT NULL REFERENCES task_definition_versions(id) ON DELETE RESTRICT,
    name varchar(120) NOT NULL,
    trigger_type varchar(16) NOT NULL,
    fire_at timestamptz,
    cron_expr varchar(128),
    interval_seconds integer,
    timezone varchar(64) NOT NULL DEFAULT 'UTC',
    parameters jsonb NOT NULL,
    parameters_redacted jsonb NOT NULL,
    overlap_policy varchar(16) NOT NULL DEFAULT 'skip',
    misfire_grace_seconds integer NOT NULL DEFAULT 3600,
    status varchar(16) NOT NULL DEFAULT 'active',
    next_fire_at timestamptz,
    last_fire_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT ck_schedules_trigger_type CHECK (trigger_type IN ('once','cron','interval')),
    CONSTRAINT ck_schedules_status CHECK (status IN ('active','paused','completed','cancelled')),
    CONSTRAINT ck_schedules_overlap CHECK (overlap_policy IN ('skip','queue')),
    CONSTRAINT ck_schedules_trigger_config CHECK (
        trigger_type = 'once'
        OR (trigger_type = 'cron' AND cron_expr IS NOT NULL)
        OR (trigger_type = 'interval' AND interval_seconds IS NOT NULL)
    ),
    CONSTRAINT ck_schedules_active_next_fire CHECK (status <> 'active' OR next_fire_at IS NOT NULL),
    CONSTRAINT ck_schedules_interval_min CHECK (interval_seconds IS NULL OR interval_seconds >= 10),
    CONSTRAINT ck_schedules_grace CHECK (misfire_grace_seconds >= 0)
);
CREATE INDEX IF NOT EXISTS ix_schedules_definition ON schedules (definition_id, created_at);
CREATE INDEX IF NOT EXISTS ix_schedules_due ON schedules (next_fire_at) WHERE status = 'active';

-- Columns added after the initial Alembic revision.
ALTER TABLE runs ADD COLUMN IF NOT EXISTS schedule_id uuid;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS scheduled_for timestamptz;
CREATE INDEX IF NOT EXISTS ix_runs_schedule_time ON runs (schedule_id, scheduled_for);
CREATE UNIQUE INDEX IF NOT EXISTS uq_runs_schedule_occurrence ON runs (schedule_id, scheduled_for) WHERE schedule_id IS NOT NULL;

-- Conditional constraints for databases created by older revisions.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_task_definitions_active_version_id'
          AND conrelid = 'task_definitions'::regclass
    ) THEN
        ALTER TABLE task_definitions
            ADD CONSTRAINT fk_task_definitions_active_version_id
            FOREIGN KEY (active_version_id) REFERENCES task_definition_versions(id) ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_runs_schedule_id_schedules'
          AND conrelid = 'runs'::regclass
    ) THEN
        ALTER TABLE runs
            ADD CONSTRAINT fk_runs_schedule_id_schedules
            FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE SET NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'schedules' AND relkind = 'r') THEN
        ALTER TABLE schedules DROP CONSTRAINT IF EXISTS trigger_config;
        ALTER TABLE schedules DROP CONSTRAINT IF EXISTS ck_schedules_trigger_config;
        ALTER TABLE schedules
            ADD CONSTRAINT ck_schedules_trigger_config CHECK (
                trigger_type = 'once'
                OR (trigger_type = 'cron' AND cron_expr IS NOT NULL)
                OR (trigger_type = 'interval' AND interval_seconds IS NOT NULL)
            );
    END IF;
END $$;

-- Keep a head marker so a rollback to the Python/Alembic implementation is possible.
CREATE TABLE IF NOT EXISTS alembic_version (
    version_num varchar(32) PRIMARY KEY
);
INSERT INTO alembic_version(version_num)
VALUES ('9f1c2d3e4a5b')
ON CONFLICT DO NOTHING;
