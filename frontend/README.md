# Task Control Console

React 18 + TypeScript + Vite + Ant Design web console for the task control backend.

## Development

```sh
pnpm install
pnpm dev
```

Vite proxies `/api` to `http://127.0.0.1:8000`. Start the backend and worker first.

## Build

```sh
pnpm build
```

The backend serves `frontend/dist` at `/` when the directory exists.

## Pages

- Login
- Dashboard (run counts and recent runs)
- Task definitions (list/create local/SSH/HTTP definitions)
- Schedules (list/create interval/cron/once, pause/resume/cancel)
- Runs (submit immediate run, list, detail, cancel, retry)
- Statistics (daily buckets, success rate, P95)
- Targets, credentials, users and audit (admin)
- Profile: password, sessions, API tokens

The full RJSF form builder, schedule editing/detail, advanced run filters and E2E tests remain pending.
