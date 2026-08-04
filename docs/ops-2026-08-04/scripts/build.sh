#!/usr/bin/env bash
set -euo pipefail

repo_dir="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
cd "$repo_dir"

command -v bun >/dev/null || { echo "bun is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }

(
  cd web
  bun install --frozen-lockfile
  bun run build
)

version="$(cat VERSION 2>/dev/null || git rev-parse --short HEAD)"
CGO_ENABLED=0 go build \
  -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=${version}'" \
  -o new-api \
  .

echo "Built: $repo_dir/new-api"
