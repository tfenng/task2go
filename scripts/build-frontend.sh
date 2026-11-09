#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ -z "${FRONTEND_DIR:-}" ]; then
  if [ -f "$ROOT/frontend/package.json" ]; then
    FRONTEND_DIR="$ROOT/frontend"
  else
    FRONTEND_DIR="$ROOT/../frontend"
  fi
fi

if [ ! -f "$FRONTEND_DIR/package.json" ]; then
  echo "frontend not found: $FRONTEND_DIR (set FRONTEND_DIR)" >&2
  exit 1
fi

if command -v pnpm >/dev/null 2>&1; then
  (cd "$FRONTEND_DIR" && pnpm install --frozen-lockfile && pnpm build)
else
  docker run --rm --user "$(id -u):$(id -g)" \
    -e HOME=/tmp -e COREPACK_HOME=/tmp/corepack \
    -v "$FRONTEND_DIR:/app" -w /app node:22-slim \
    sh -c 'corepack pnpm install --frozen-lockfile && corepack pnpm build'
fi

rm -rf "$ROOT/internal/web/dist"
mkdir -p "$ROOT/internal/web/dist"
cp -a "$FRONTEND_DIR/dist/." "$ROOT/internal/web/dist/"
touch "$ROOT/internal/web/dist/.gitkeep"
