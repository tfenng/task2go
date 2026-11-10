# task2go

FastAPI + React 任务中控中心的 Go 重写。一个静态二进制同时启动 API、worker 和 scheduler，前端构建产物通过 `go:embed` 打入二进制；运行时只依赖 PostgreSQL。

## 兼容目标

- 保持 `/api/v1` 路径、请求/响应 JSON、状态码、Cookie、CSRF、`If-Match`、幂等键和错误结构。
- 前端源码维护在本仓库 `frontend/`，构建产物通过 `go:embed` 打进二进制；持续保持原有交互习惯。
- 接管现有 Alembic 数据库；迁移启动时自动执行。
- 现有 Python 后端 `backend/` 保留不动，验收后再拆独立 repo。

## 构建

需要 Go 1.22+；前端使用 pnpm 10 / Node 22 构建，本机没有 pnpm 时脚本自动回退到 `node:22-slim` Docker 镜像。

```sh
cd task2go
make build
```

或者分开执行：

```sh
./scripts/build-frontend.sh
SKIP_FRONTEND=1 ./scripts/build.sh
```

产物：

```text
task2go/dist/task-control
```

`scripts/build-frontend.sh` 默认读取 `task2go/frontend`，找不到时回退到 `../frontend`；可用 `FRONTEND_DIR` 指定。前端构建完成后复制到 `internal/web/dist`，再由 `go:embed` 打入二进制。

`task2go/` 已包含一份前端源码副本，验收后可以整个目录拆成独立仓库，不需要依赖父仓库的 `frontend/`。

## 运行

```sh
cp .env.example .env   # 二进制启动时会自动读取当前目录的 .env
export TASK_CONTROL_DATABASE_URL='postgresql+psycopg://user:pass@127.0.0.1:5432/task_control'
export TASK_CONTROL_SECRET_KEYS='{"1":"<32-byte-base64>"}'
export TASK_CONTROL_SECRET_ACTIVE_VERSION=1
./dist/task-control
```

默认监听 `0.0.0.0:8000`，可用 `TASK_CONTROL_HTTP_ADDR` 覆盖（例如 `127.0.0.1:8010`）。

首次创建管理员：

```sh
./dist/task-control create-admin --username admin
```

其他维护命令：

```sh
./dist/task-control reset-password --username admin
./dist/task-control migrate
./dist/task-control retention
./dist/task-control healthcheck
```

排障或拆分部署：

```sh
./dist/task-control serve --role api
./dist/task-control serve --role api,worker   # 逗号分隔，可任意组合
./dist/task-control serve --role worker
./dist/task-control serve --role scheduler
```

## 配置

所有变量沿用 `TASK_CONTROL_` 前缀，默认值与 Python 版一致。主要变量：

| 变量 | 默认值 |
|---|---|
| `TASK_CONTROL_DATABASE_URL` | `postgresql+psycopg://localhost:5432/task_control` |
| `TASK_CONTROL_SECRET_KEYS` | 内置开发密钥 |
| `TASK_CONTROL_SECRET_ACTIVE_VERSION` | `1` |
| `TASK_CONTROL_COOKIE_SECURE` | `false` |
| `TASK_CONTROL_SESSION_IDLE_MINUTES` | `30` |
| `TASK_CONTROL_SESSION_REMEMBER_DAYS` | `7` |
| `TASK_CONTROL_SCRIPT_ROOT` | `/tmp/task-control-scripts` |
| `TASK_CONTROL_DEFAULT_SHELL` | `/bin/bash` |
| `TASK_CONTROL_OUTPUT_ROOT` | `/tmp/task-control-output` |
| `TASK_CONTROL_OUTPUT_LIMIT_BYTES` | `1048576` |
| `TASK_CONTROL_OUTPUT_CAPTURE_LIMIT_BYTES` | `67108864` |
| `TASK_CONTROL_LIVE_OUTPUT_MAX_BYTES` | `262144` |
| `TASK_CONTROL_RETENTION_DAYS` | `395` |
| `TASK_CONTROL_ATTEMPT_RETENTION_DAYS` | `90` |
| `TASK_CONTROL_WORKER_CONCURRENCY` | `4` |
| `TASK_CONTROL_WORKER_POLL_SECONDS` | `1` |
| `TASK_CONTROL_LEASE_SECONDS` | `60` |
| `TASK_CONTROL_CORS_ORIGINS` | `http://localhost:5173` |
| `TASK_CONTROL_LOG_LEVEL` | `INFO` |
| `TASK_CONTROL_HTTP_ADDR` | `0.0.0.0:8000` |

## 数据库接管

- 全新数据库：启动时自动建表，写入 `alembic_version=9f1c2d3e4a5b`，并按顺序记录 `schema_migrations`（当前 `0001_baseline`、`0002_run_timeout`）。
- 现有 Alembic head 数据库：幂等基线 SQL 不会删除或覆盖数据，只补齐缺失对象。
- 迁移使用 `pg_advisory_lock(hashtext('task_control_migrations'))`，多实例启动不会并发执行。
- 如果现有库低于当前 Alembic head，建议先执行一次 Python 版 `alembic upgrade head`，再做切换。
- 切换前请备份数据库。

## Docker Compose

```sh
cd task2go
cp .env.example .env
# 编辑 .env：填入真实的 TASK_CONTROL_SECRET_KEYS，并把 TASK_CONTROL_DATABASE_URL 指向外部 PostgreSQL
docker compose up -d --build
```

Compose 只包含 `app`，数据库需要事先准备好并写入 `TASK_CONTROL_DATABASE_URL`；`host.docker.internal` 表示 Docker 宿主机上的 PostgreSQL（容器内不能用 `127.0.0.1`）。`app` 启动时自动迁移。脚本目录挂载到 `/opt/task-scripts`，运行输出保存在命名卷。宿主机 8000 端口被占用时，在 `.env` 中修改 `TASK_CONTROL_API_PORT`。

创建管理员：

```sh
docker compose exec app task-control create-admin --username admin
```

## 执行器与运行时

二进制本身不依赖 Python/Node/Redis。镜像只安装 `bash` 和 CA 证书。

本地执行器会启动任务定义里的命令，因此任务所需的解释器或二进制必须在运行环境中存在。例如任务命令是 `python3 /opt/task-scripts/foo.py`，运行环境就必须有 `python3`。这是任务负载依赖，不是控制面依赖。

`timeout_seconds` 是单次执行的总时长上限（不是无输出超时）。实时日志查看只是读取输出文件，不会延长或重置该上限；长任务需要按实际耗时调大版本的 `timeout_seconds`（最大 86400 秒）。发起运行时也可以在「本次超时」里选择预设（1 分钟 / 5 分钟 / 30 分钟 / 1 小时 / 6 小时 / 24 小时），该值会固化到这次 run 上，重试沿用，不改动定义默认值。修改定义会生成新版本，只影响之后创建的运行；重试会沿用原运行固定的版本。

## 运行管理

- 仪表盘「最近运行」与运行列表显示任务定义、参数、状态和总用时；参数超过 64 个字符截断，悬停展示完整内容（敏感字段仍为 `***`）。
- 状态带颜色（成功绿、失败红、运行中蓝、重试等待橙）；列表在存在活动运行时每 2 秒自动刷新，总用时按秒更新。
- 运行详情提供返回、取消、重试、实时输出；实时输出每 1.5 秒轮询，最多显示最近 256 KB。
- 已结束的运行（`succeeded` / `failed` / `cancelled` / `skipped` / `missed`）可在列表删除，对应 `DELETE /api/v1/runs/{run_id}`；运行中的运行返回 `409`，执行记录随外键级联删除。
- 用户密码最短 6 个字符（新建用户、修改密码、CLI `create-admin` / `reset-password` 一致）。

## 测试

单元测试：

```sh
go test ./...
```

Postgres 集成测试：

```sh
export TASK_CONTROL_TEST_DATABASE_URL='postgresql+psycopg://task_control:task_control@127.0.0.1:5432/task_control_test'
go test ./...
```

集成测试会清空测试库中的业务表，请使用独立测试数据库。

## 目录

```text
cmd/task-control      入口与 CLI
internal/api          HTTP 路由、认证、DTO、错误映射
internal/core         调度时间、重试、目标快照、加密参数辅助
internal/executor     local / ssh / http 执行器与输出捕获
internal/params       JSON Schema、默认值、脱敏
internal/security     Argon2id、AES-GCM、会话、CSRF、Token
internal/store        pgx、SQL、事务、迁移
internal/worker       队列认领、执行、租约、reaper
internal/scheduler    到期调度、重叠/错过、retry_wait 提升
internal/web          go:embed 前端产物
```
