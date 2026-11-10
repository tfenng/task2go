import type {
  ApiTokenInfo,
  AuditEvent,
  Credential,
  Definition,
  Page,
  Run,
  RunStats,
  LiveOutput,
  Schedule,
  ScriptList,
  SessionInfo,
  Target,
  User,
} from "./types";

const API = "/api/v1";
const CSRF_KEY = "tc_csrf";

function cookieValue(name: string): string | null {
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : null;
}

function csrfToken(): string | null {
  return localStorage.getItem(CSRF_KEY) || cookieValue("tc_csrf");
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const method = (options.method || "GET").toUpperCase();
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    const token = csrfToken();
    if (token) headers.set("X-CSRF-Token", token);
  }
  const response = await fetch(`${API}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });
  if (response.status === 204) return undefined as T;
  const text = await response.text();
  const body = text ? JSON.parse(text) : null;
  if (!response.ok) {
    const error = new Error(body?.error?.message || response.statusText);
    (error as Error & { status?: number; body?: unknown }).status = response.status;
    (error as Error & { status?: number; body?: unknown }).body = body;
    throw error;
  }
  return body as T;
}

export async function login(username: string, password: string, remember = false): Promise<{ user: User; csrf_token: string }> {
  const result = await request<{ user: User; csrf_token: string }>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password, remember }),
  });
  localStorage.setItem(CSRF_KEY, result.csrf_token);
  return result;
}

export function me(): Promise<User> {
  return request<User>("/auth/me");
}

export async function logout(): Promise<void> {
  await request("/auth/logout", { method: "POST" });
  localStorage.removeItem(CSRF_KEY);
}

export function listDefinitions(archived?: boolean): Promise<Page<Definition>> {
  const query = new URLSearchParams({ page_size: "100" });
  if (archived !== undefined) query.set("archived", String(archived));
  return request<Page<Definition>>(`/task-definitions?${query.toString()}`);
}

export function createDefinition(payload: unknown): Promise<Definition> {
  return request<Definition>("/task-definitions", { method: "POST", body: JSON.stringify(payload) });
}

export function deleteDefinition(id: string, deleteRuns: boolean): Promise<{ archived: boolean; deleted_runs: number }> {
  return request<{ archived: boolean; deleted_runs: number }>(
    `/task-definitions/${id}?delete_runs=${deleteRuns}`,
    { method: "DELETE" },
  );
}

export function getDefinition(id: string): Promise<Definition> {
  return request<Definition>(`/task-definitions/${id}`);
}

export function updateDefinition(id: string, payload: unknown, version: number): Promise<Definition> {
  return request<Definition>(`/task-definitions/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
    headers: { "If-Match": String(version) },
  });
}

export function listRuns(): Promise<Page<Run>> {
  return request<Page<Run>>("/runs?page_size=100");
}

export function getRun(id: string): Promise<Run> {
  return request<Run>(`/runs/${id}`);
}

export function getRunLiveOutput(id: string, attemptNo?: number, maxBytes = 262144): Promise<LiveOutput> {
  const params = new URLSearchParams({ max_bytes: String(maxBytes) });
  if (attemptNo != null) params.set("attempt_no", String(attemptNo));
  return request<LiveOutput>(`/runs/${id}/live-output?${params.toString()}`);
}

export function createRun(payload: unknown): Promise<Run> {
  return request<Run>("/runs", { method: "POST", body: JSON.stringify(payload) });
}

export function cancelRun(id: string): Promise<Run> {
  return request<Run>(`/runs/${id}/cancel`, { method: "POST" });
}

export async function deleteRun(id: string): Promise<void> {
  await request<void>(`/runs/${id}`, { method: "DELETE" });
}

export function retryRun(id: string): Promise<Run> {
  return request<Run>(`/runs/${id}/retry`, { method: "POST" });
}

export function listSchedules(): Promise<Page<Schedule>> {
  return request<Page<Schedule>>("/schedules?page_size=100");
}

export function createSchedule(payload: unknown): Promise<Schedule> {
  return request<Schedule>("/schedules", { method: "POST", body: JSON.stringify(payload) });
}

export function scheduleAction(id: string, action: "pause" | "resume" | "cancel", version: number): Promise<Schedule> {
  return request<Schedule>(`/schedules/${id}/${action}`, {
    method: "POST",
    headers: { "If-Match": String(version) },
  });
}

export function deleteSchedule(id: string): Promise<void> {
  return request<void>(`/schedules/${id}`, { method: "DELETE" });
}

export function deleteAllSchedules(): Promise<{ deleted: number }> {
  return request<{ deleted: number }>("/schedules", { method: "DELETE" });
}

export function listTargets(): Promise<Page<Target>> {
  return request<Page<Target>>("/execution-targets?page_size=100");
}

export function createTarget(payload: unknown): Promise<Target> {
  return request<Target>("/execution-targets", { method: "POST", body: JSON.stringify(payload) });
}

export function updateTarget(id: string, payload: unknown, version: number): Promise<Target> {
  return request<Target>(`/execution-targets/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
    headers: { "If-Match": String(version) },
  });
}

export function deleteTarget(id: string, version: number): Promise<void> {
  return request<void>(`/execution-targets/${id}`, {
    method: "DELETE",
    headers: { "If-Match": String(version) },
  });
}

export function listCredentials(): Promise<Page<Credential>> {
  return request<Page<Credential>>("/credentials?page_size=100");
}

export function createCredential(payload: unknown): Promise<Credential> {
  return request<Credential>("/credentials", { method: "POST", body: JSON.stringify(payload) });
}

export function updateCredential(id: string, payload: unknown, version: number): Promise<Credential> {
  return request<Credential>(`/credentials/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
    headers: { "If-Match": String(version) },
  });
}

export function deleteCredential(id: string, version: number): Promise<void> {
  return request<void>(`/credentials/${id}`, {
    method: "DELETE",
    headers: { "If-Match": String(version) },
  });
}

export function getRunStats(groupBy: "day" | "definition" = "definition"): Promise<RunStats> {
  return request<RunStats>(`/stats/runs?group_by=${groupBy}`);
}

export function listUsers(): Promise<Page<User>> {
  return request<Page<User>>("/users?page_size=100");
}

export function createUser(payload: unknown): Promise<User> {
  return request<User>("/users", { method: "POST", body: JSON.stringify(payload) });
}

export function updateUser(id: string, payload: unknown, version: number): Promise<User> {
  return request<User>(`/users/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
    headers: { "If-Match": String(version) },
  });
}

export function resetUserPassword(id: string): Promise<{ temporary_password: string }> {
  return request<{ temporary_password: string }>(`/users/${id}/reset-password`, { method: "POST" });
}

export function changePassword(current_password: string, new_password: string): Promise<void> {
  return request<void>("/auth/password", {
    method: "POST",
    body: JSON.stringify({ current_password, new_password }),
  });
}

export function listSessions(): Promise<{ items: SessionInfo[] }> {
  return request<{ items: SessionInfo[] }>("/auth/sessions");
}

export function revokeSession(id: string): Promise<void> {
  return request<void>(`/auth/sessions/${id}`, { method: "DELETE" });
}

export function listApiTokens(): Promise<{ items: ApiTokenInfo[] }> {
  return request<{ items: ApiTokenInfo[] }>("/auth/tokens");
}

export function createApiToken(name: string): Promise<ApiTokenInfo & { token: string }> {
  return request<ApiTokenInfo & { token: string }>("/auth/tokens", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function revokeApiToken(id: string): Promise<void> {
  return request<void>(`/auth/tokens/${id}`, { method: "DELETE" });
}

export function listAuditEvents(): Promise<Page<AuditEvent>> {
  return request<Page<AuditEvent>>("/audit-events?page_size=100");
}

export function listScripts(path = ""): Promise<ScriptList> {
  return request<ScriptList>(`/scripts?path=${encodeURIComponent(path)}`);
}
