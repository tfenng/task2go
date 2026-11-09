export interface User {
  id: string;
  username: string;
  display_name: string;
  role: "admin" | "operator" | "viewer";
  is_active: boolean;
  must_change_password: boolean;
  version: number;
}

export interface ParameterSchema {
  type?: string;
  properties?: Record<
    string,
    { type?: string; title?: string; default?: unknown; "x-secret"?: boolean; writeOnly?: boolean }
  >;
  required?: string[];
  [key: string]: unknown;
}

export interface DefinitionVersion {
  id: string;
  definition_id: string;
  version_no: number;
  executor_type: string;
  parameter_schema: ParameterSchema;
  executor_config: {
    argv?: string[];
    interpreter?: string;
    cwd?: string;
    env?: Record<string, string>;
    target_id?: string;
    method?: string;
    path?: string;
    url?: string;
    command?: string;
  };
  timeout_seconds: number;
  retry_policy: { max_attempts: number };
  max_concurrency: number;
}

export interface Definition {
  id: string;
  key: string;
  name: string;
  description: string | null;
  is_archived: boolean;
  active_version: DefinitionVersion | null;
  version: number;
}

export interface Attempt {
  id: string;
  attempt_no: number;
  status: string;
  executor_type: string;
  started_at: string;
  finished_at: string | null;
  duration_ms: number | null;
  exit_code: number | null;
  error_class: string | null;
  error_message: string | null;
  stdout_excerpt: string | null;
  stderr_excerpt: string | null;
  truncated: boolean;
}

export interface Run {
  id: string;
  definition_id: string;
  definition_version_id: string;
  schedule_id: string | null;
  scheduled_for: string | null;
  trigger_type: string;
  status: string;
  arguments?: string[];
  parameters: Record<string, unknown>;
  attempt_count: number;
  max_attempts: number;
  queued_at: string | null;
  started_at: string | null;
  finished_at: string | null;
  next_retry_at: string | null;
  parent_run_id: string | null;
  error_code: string | null;
  error_message: string | null;
  result_summary: Record<string, unknown> | null;
  created_at: string;
  definition_key?: string | null;
  definition_name?: string | null;
  definition_version_no?: number | null;
  executor_type?: string | null;
  executor_config?: {
    argv?: string[];
    command?: string;
    method?: string;
    path?: string;
    url?: string;
    target_id?: string;
  } | null;
  parameter_schema?: Record<string, unknown> | null;
  timeout_seconds?: number | null;
  attempts?: Attempt[];
}

export interface LiveOutput {
  run_id: string;
  attempt_no: number;
  status: string;
  running: boolean;
  stdout: string;
  stderr: string;
  stdout_bytes: number;
  stderr_bytes: number;
  stdout_truncated: boolean;
  stderr_truncated: boolean;
}

export interface Schedule {
  id: string;
  definition_id: string;
  definition_version_id: string;
  name: string;
  trigger_type: "once" | "cron" | "interval";
  fire_at: string | null;
  cron_expr: string | null;
  interval_seconds: number | null;
  timezone: string;
  arguments?: string[];
  parameters: Record<string, unknown>;
  overlap_policy: "skip" | "queue";
  misfire_grace_seconds: number;
  status: "active" | "paused" | "completed" | "cancelled";
  next_fire_at: string | null;
  last_fire_at: string | null;
  version: number;
}

export interface Target {
  id: string;
  name: string;
  type: "local" | "ssh" | "http";
  config: Record<string, unknown>;
  credential_id: string | null;
  is_active: boolean;
  version: number;
}

export interface Credential {
  id: string;
  name: string;
  kind: "password" | "ssh_key" | "http_bearer" | "http_basic";
  metadata: Record<string, unknown>;
  is_active: boolean;
  version: number;
}

export interface StatsBucket {
  key: string;
  total: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  skipped: number;
  missed: number;
  success_rate: number;
  avg_duration_ms: number | null;
  p95_duration_ms: number | null;
}

export interface RunStats {
  total: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  skipped: number;
  missed: number;
  retrying: number;
  success_rate: number;
  avg_duration_ms: number | null;
  p95_duration_ms: number | null;
  buckets: StatsBucket[];
}

export interface SessionInfo {
  id: string;
  ip_address: string | null;
  user_agent: string | null;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
}

export interface ApiTokenInfo {
  id: string;
  name: string;
  token_prefix: string;
  expires_at: string | null;
  last_used_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface AuditEvent {
  id: number;
  actor_user_id: string | null;
  actor_username?: string | null;
  action: string;
  resource_type: string;
  resource_id: string | null;
  outcome: string;
  correlation_id: string;
  details: Record<string, unknown>;
  created_at: string;
}

export interface ScriptEntry {
  name: string;
  path: string;
  type: "file" | "dir";
  size: number | null;
}

export interface ScriptList {
  root: string;
  path: string;
  parent: string | null;
  items: ScriptEntry[];
}

export interface Page<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}
