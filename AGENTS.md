# Repository Guidelines

Single Go binary that serves the HTTP API, worker, scheduler, and the embedded React console for a task-control system. Runtime dependency is PostgreSQL only; the binary itself needs no Python/Node/Redis.

## Project Overview

- Manages **task definitions** with immutable versions, JSON-Schema parameters, and one of three executors (`local`, `ssh`, `http`).
- Executes runs manually or via **schedules** (`once` / `cron` / `interval`), with concurrency limits, leases, retries, cancellation, and audit events.
- Stores execution targets and credentials (AES-GCM encrypted), users with roles `admin` / `operator` / `viewer`, sessions, and API tokens.
- Hard compatibility target (`README.md`): preserve the Python backend's `/api/v1` paths, JSON shapes, status codes, cookies, CSRF, `If-Match`, idempotency keys, and error envelope. Frontend source is a copy of the existing app and should only be rebuilt, not redesigned.
- Also acts as a database takeover: one idempotent baseline migration compatible with an existing Alembic head (`9f1c2d3e4a5b`).

## Architecture & Data Flow

One process, three optional roles. `runServe` (`cmd/task-control/main.go`) starts any of `api`, `worker`, `scheduler`; `--role` accepts a comma list (`--role api,worker`), defaults to `all`, and an unknown/empty role starts nothing. All roles share one `*store.Store` pgx pool and one `*config.Config`.

```mermaid
flowchart LR
  UI[React console<br/>go:embed dist] --> API[internal/api<br/>ServeMux + middleware]
  API --> ST[(PostgreSQL)]
  SCH[internal/scheduler<br/>1s tick, advisory lock] --> ST
  WK[internal/worker<br/>claim loop + reaper] --> ST
  WK --> EX[internal/executor<br/>local / ssh / http]
  EX --> FS[output files<br/>stdout/stderr]
  API -. live-output .-> FS
```

**Request path.** `correlation` → `securityHeaders` → `cors` → `recoverer` → `ServeMux` (Go 1.22 `"METHOD /path/{id}"` patterns) → handler. Handlers do auth/role checks (`requireAuth`, `requireCSRF`), optimistic concurrency (`If-Match` raw integer), then a `store.WithTx` transaction that also writes the audit event, then `writeJSON`.

**Queue path.**
1. `POST /runs` validates params against the pinned definition version's schema, encrypts secret values, writes a redacted copy for reads, and inserts `runs` with `status='queued'` plus a `task_definition_versions` FK. `Idempotency-Key` is unique per `(created_by, key)` and the request hash is compared. The effective timeout is resolved here — optional request `timeout_seconds` (1–86400) overrides the version value — and snapshotted into `runs.timeout_seconds`; the worker reads only the run value.
2. Worker `claim` runs in one transaction (`internal/store/runs.go:127`): `SELECT … FOR UPDATE OF r SKIP LOCKED` joined with `task_definition_runtime`, guarded by `running_count < max_concurrency`; then sets `running`, lease, increments the runtime counter, and creates an `attempts` row.
3. `Worker.execute` loads the encrypted params, decrypts, builds an `executor.Context`, and runs the executor. A lease heartbeat renews the lease every `LeaseSeconds/3`; `reaper` reclaims expired leases.
4. `Worker.finish` (same transaction) saves the attempt, decides terminal status or `retry_wait` via `core.RetryPolicy.Delay`, clears the lease, decrements `running_count`, and appends an audit event.
5. Scheduler `RunOnce` takes `pg_try_advisory_lock(hashtext('task_control_scheduler'))`, lists due schedules (also `SKIP LOCKED`), dedupes by `(schedule_id, scheduled_for)`, marks `missed` outside `misfire_grace_seconds`, applies `overlap_policy` (`skip`/`queue`), creates the run, advances `next_fire_at`, and calls `PromoteRetryWait` to move due `retry_wait` runs back to `queued`.

**Layering.** `cmd` → `api` / `worker` / `scheduler` → `core`, `params`, `executor`, `security` → `store`. Only `store` issues SQL; `api` uses store methods and touches pgx only for the readiness `Ping`. `core` holds pure decision logic over `store` models. `executor`, `security`, `config`, `logging`, `web` are leaf packages. Nothing imports `api`; `worker`/`scheduler` are only used from `cmd` and tests.

Store reads return `(nil, nil)` for missing rows (`One`/`scanRow` swallow `pgx.ErrNoRows`), so always nil-check before use. List queries use `pgx.RowToStructByName`; **single-row queries (`One`, `scanInto`) scan positionally** in struct-field order, so never use `SELECT *` / `RETURNING *` for a table whose physical column order may differ from the struct (see `runColumns` in `internal/store/runs.go`, needed for Alembic-created databases).

**Execution.** `executor.Create(type)` returns `local`, `ssh`, or `http`. All render `${param}` placeholders, redact known secret values from output, and truncate captured output (`OutputLimitBytes` for excerpts, `OutputCaptureLimitBytes` for capture files). Local runs in a new process group with `TASK_RUN_ID` and `TASK_PARAM_*` env vars; `__arguments__` params are appended to argv.

## Key Directories

| Path | Purpose |
|---|---|
| `cmd/task-control/` | `main.go`: CLI dispatch, role wiring, admin/password/retention/migrate/healthcheck commands |
| `internal/api/` | Routes, middleware, auth/CSRF/rate limit, DTOs, error mapping, embedded OpenAPI |
| `internal/store/` | pgx pool, transactions, one file per resource, embedded migrations |
| `internal/core/` | Next-fire computation, retry policy/delay, target snapshots, validation errors |
| `internal/executor/` | `local`, `ssh`, `http` executors, output pump/truncation |
| `internal/params/` | JSON Schema subset validation, defaults, secret detection, redaction, secret extraction |
| `internal/security/` | Argon2id passwords, AES-GCM key ring, session/token hashing, TTLs |
| `internal/worker/` | Claim loop, concurrency slots, lease heartbeat, reaper, finish |
| `internal/scheduler/` | Due-schedule tick, misfire/overlap handling, `retry_wait` promotion |
| `internal/config/` | Env loading and defaulting (`TASK_CONTROL_*`) |
| `internal/web/` | `//go:embed all:dist`; `.gitkeep` is required even without a frontend build |
| `frontend/src/` | React 18 + Vite + Ant Design console; `pages/`, `components/`, `lib/`, `api.ts`, `types.ts` |
| `scripts/` | `build.sh`, `build-frontend.sh` |

## Development Commands

```sh
make build          # frontend (pnpm or Docker node) + Go binary -> dist/task-control
make frontend       # scripts/build-frontend.sh only
SKIP_FRONTEND=1 ./scripts/build.sh   # API-only binary
make test           # go test ./...
make vet            # gofmt -l . && go vet ./...
make run            # go run ./cmd/task-control serve
```

`scripts/build.sh` requires `internal/web/dist/index.html`; it builds with `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"`.

```sh
# local run (env required, see .env.example)
export TASK_CONTROL_DATABASE_URL='postgresql+psycopg://user:pass@127.0.0.1:5432/task_control'
export TASK_CONTROL_SECRET_KEYS='{"1":"<32-byte-base64>"}'
./dist/task-control serve --role api
./dist/task-control serve --role api,worker   # split roles across processes

./dist/task-control create-admin --username admin
./dist/task-control reset-password --username admin
./dist/task-control migrate
./dist/task-control retention
./dist/task-control healthcheck
./dist/task-control version

docker compose up -d --build
docker compose exec app task-control create-admin --username admin
```

Frontend dev server: `cd frontend && pnpm dev` on port 5173; Vite proxies `/api` to `127.0.0.1:8000`, matching the default CORS origin.

## Code Conventions & Common Patterns

- **stdlib first.** `net/http` `ServeMux` (method patterns), `log/slog`, `flag`, `embed`. No web framework, no ORM, no DI container. Dependencies are passed explicitly through `New…` constructors.
- **Handlers**: thin — parse input → `requireAuth`/`requireCSRF` + role check → `If-Match` → one `store.WithTx` → `writeJSON`. Errors are `*apiError` values built with helpers in `internal/api/errors.go` (`notFound`, `forbidden`, `conflict`, `invalidState`, `validationErr`, `authRequired`, `internalError`, …); return them, never write error JSON by hand. `asAPIError` maps core/store errors and masks unexpected ones as `INTERNAL_ERROR`.
- **Error envelope**: `{"error":{"code","message","details","correlation_id"}}`. Sentinel errors live in `core` (`ErrNotFound`, `ErrConflict`, `ErrValidation`, `ErrTargetDisabled`); store returns typed/sentinel errors, API decides status.
- **Context**: `ctx context.Context` is always the first parameter; store methods accept a `store.Querier` (pool or `pgx.Tx`) so the same method works inside or outside a transaction.
- **Time**: always UTC — `time.Now().UTC()`, `timestamptz`; schedule math takes an explicit `after time.Time` parameter so it stays testable. `core.ComputeNextFire` supports `once`, `interval` (min 10 s), and cron evaluated in the schedule timezone; store `next_fire_at` in UTC.
- **Store style**: one file per resource (`definitions.go`, `runs.go`, `schedules.go`, …); scanning through `One`/`Collect` helpers by column name; JSONB via `store.JSONB`. Migrations are embedded and immutable once released — add `0002_*.sql`, never edit `0001_baseline.sql`.
- **DTO vs model**: `store` structs mirror tables; `api/dto.go` defines the wire shapes and `Role`/`Page` conventions. Handlers convert explicitly. Never serialize encrypted `parameters`, encrypted credentials, or password/token hashes — reads use `ParametersRedacted`.
- **Secrets**: JSON Schema `x-secret` / `writeOnly` fields are masked as `***`; `params.SecretValues` feeds output redaction; `SecretBox` encrypts with an `int` key version and AAD scoped to the resource field.
- **Naming**: `listX` / `getX` / `createX` / `updateX` / `deleteX` handlers; `XxxID` JSON fields; resource files and migration tables use snake_case; React files are PascalCase for pages/components (`RunDetailPage.tsx`), camelCase for `lib/`.
- **Frontend**: one hand-rolled `fetch` wrapper (`frontend/src/api.ts`) — `credentials: "include"`, auto `X-CSRF-Token` from `localStorage.tc_csrf` or the `tc_csrf` cookie on non-GET, throws `Error` with `.status`/`.body`. No React Query/Redux; `useState`/`useEffect` only, session held in `App.tsx` and passed as props. Ant Design components plus plain `styles.css`. UI copy and server-facing docs are Chinese.
- **Comments/docs language**: code identifiers and most comments are English; `README.md`, OpenAPI descriptions, and UI strings are Chinese. Match the file you edit.

## Important Files

| File | Why it matters |
|---|---|
| `cmd/task-control/main.go` | CLI, `setup()` (config → pool → migrate → `SecretBox`), role startup, graceful shutdown |
| `internal/api/server.go` | Route table and middleware order; SPA + `/assets` serving |
| `internal/api/errors.go` | Error envelope, `If-Match`, pagination, JSON body limits (4 MiB) |
| `internal/api/auth_helpers.go` | Cookie/Bearer auth, CSRF, role gates, audit helper |
| `internal/api/openapi.yaml` | Hand-maintained OpenAPI 3.1 spec, embedded and served at `/api/openapi.json`; `/api/docs` is a stub |
| `internal/store/runs.go` | Claim, lease, reaper, idempotency SQL — the core queue logic |
| `internal/store/migrations/0001_baseline.sql` | Full schema, including Alembic compatibility rows |
| `internal/core/core.go` | `ComputeNextFire`, `RetryPolicy`, `BuildTargetSnapshot`, `Checksum`/`RequestHash`, target/credential helpers |
| `internal/worker/worker.go` | Slots, heartbeat, reaper, `finish` state machine |
| `internal/scheduler/scheduler.go` | Advisory-lock tick, misfire/overlap decisions |
| `internal/params/params.go` | Schema subset, defaults, redaction, secret extraction |
| `internal/security/secrets.go` | AES-GCM key ring; `passwords.go` Argon2id; `sessions.go` hashing/TTL |
| `internal/config/config.go` | Every `TASK_CONTROL_*` default and DSN normalization |
| `frontend/src/api.ts` | Single API client; extend here instead of `fetch`-ing in pages |
| `frontend/src/types.ts` | Hand-written mirror of Go DTOs; update with API changes |

## Runtime/Tooling Preferences

- **Go 1.22+** (module `github.com/tfenng/tasks/task2go`), CGO disabled, `-trimpath`, pure-Go dependencies. Timezone data is embedded via `_ "time/tzdata"`.
- **PostgreSQL** (16 in CI) is the only service. In Compose the database is external: set `TASK_CONTROL_DATABASE_URL` (use `host.docker.internal` for a host database). DSNs in `postgresql+psycopg://` or `postgresql+asyncpg://` form are normalized to `postgresql://` for pgx.
- **pnpm 10.32.1 / Node 22** for the frontend; `build-frontend.sh` falls back to a `node:22-slim` Docker container when `pnpm` is absent. Vite 6, TypeScript 5.6 (`strict`), React 18, Ant Design 5, React Router 6.
- **Configuration** is entirely env-based, prefix `TASK_CONTROL_`, loaded by `config.Load()` with `godotenv.Load()` reading `.env` from the CWD. `TASK_CONTROL_SECRET_KEYS` is a JSON map of key version → base64 32-byte key; the active version must exist.
- Runtime image is `debian:bookworm-slim` plus `bash` and CA certificates only; task payloads must bring their own interpreters.

## Testing & QA

```sh
go test ./...                                   # unit tests; DB test skips without env
export TASK_CONTROL_TEST_DATABASE_URL='postgresql+psycopg://task_control:task_control@127.0.0.1:5432/task_control_test'
go test ./...                                   # full suite incl. Postgres e2e
go test ./internal/executor/ -run TestLocalExecutorTimeout -v
```

- Standard library `testing` only; linear `TestXxx` functions, no testify, no table-driven suites, no `t.Parallel()`.
- `internal/api/api_test.go` is black-box `package api_test`: migrates, **truncates 13 business tables**, seeds an admin, starts `httptest` + a real worker goroutine, and asserts an end-to-end local run. Never point `TASK_CONTROL_TEST_DATABASE_URL` at a database whose data you need.
- Use `t.Setenv` for config, `t.TempDir()` for script/output roots, `defer cancel/close` for cleanup. Prefer pure functions with an explicit `after time.Time` over sleeping.
- Covered: retry policy/next-fire, param validation/defaults/masking, Argon2id/AES-GCM (with Python-compat golden values), local/HTTP executors, one API e2e. **Not covered:** store SQL internals, worker claim/reaper, scheduler, ssh executor, most handler authorization/`If-Match`/rate-limit paths.
- CI (`.github/workflows/ci.yml`) runs on every push/PR with `postgres:16`, stubs `internal/web/dist/index.html`, then `gofmt -l . && go vet ./...` and `go test ./...`. `gofmt -l` only lists files; `go vet` is the real gate. CI does not build the frontend or the Docker image.

## Pitfalls

- `internal/web/dist` must exist for `//go:embed all:dist` to compile; keep `.gitkeep`. A missing `index.html` compiles but serves `503 frontend not built`. Rebuild the frontend before `go build` or the binary embeds a stale UI.
- Migrations run on **every** command that calls `setup()` (`serve`, admin commands), under an advisory lock; `healthcheck` skips the DB. Back up before switching an Alembic-managed database.
- `GET /health/ready` always returns HTTP 200; inspect `checks.database` / `checks.worker` in the body.
- `openapi.yaml` drifts from the router: it documents `/task-definitions/{id}/versions*` and `/runs/{id}/attempts*` that are not registered, while `GET /api/v1/scripts`, `/runs/{id}/live-output`, and `DELETE /schedules` exist only in code. Treat `internal/api/server.go` as truth and update the spec when adding routes.
- Attempts have no standalone list endpoint; they are returned inside `GET /runs/{id}`.
- Mutating APIs must keep the Python-compatible status/error contract; do not "clean up" cookie names (`tc_session`, `tc_csrf`), field names, or error codes.
- Worker tests and any code using `t.Setenv` must stay serial; the e2e truncates shared tables.
- `If-Match` is required and compared against the row loaded inside the transaction, but updates use `UPDATE … WHERE id=$1` and bump `version` themselves — there is no conditional `WHERE version=` and no row lock, so concurrent writers can race. Preserve the header contract; do not assume it is a hard lock.
- Definition create/update rejects `executor_type: "http"` (only `local` and `ssh`), even though the `HTTP` executor and the DB check constraint support it.
- `POST /runs` with `arguments` (instead of `parameters`) skips JSON-Schema validation and redaction; those values go straight into `__arguments__`.
- Config parse failures fall back to defaults silently (`envInt`/`envBool`/`envFloat`). A typo'd numeric env var will not fail startup — verify effective values.
- Live output is polling-based: the console hits `/runs/{id}/live-output` every 1500 ms while the run is `running` (no SSE/WebSocket). Only `local` executions write capture files; SSH/HTTP fall back to the stored excerpt.
- Graceful shutdown stops claims and waits for in-flight runs, but execution uses `context.Background()`, so long local commands can delay exit until timeout.
