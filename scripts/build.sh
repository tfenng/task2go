#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

if [ "${SKIP_FRONTEND:-0}" != "1" ]; then
  "$ROOT/scripts/build-frontend.sh"
fi

if [ ! -f "$ROOT/internal/web/dist/index.html" ]; then
  echo "frontend dist is missing; run scripts/build-frontend.sh or set SKIP_FRONTEND=1 only for API-only builds" >&2
  exit 1
fi

mkdir -p "$ROOT/dist"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$ROOT/dist/task-control" ./cmd/task-control
echo "built $ROOT/dist/task-control"
