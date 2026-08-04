#!/usr/bin/env bash
set -euo pipefail

host="${NEW_API_HOST:-127.0.0.1}"
port="${NEW_API_PORT:-9025}"
base_url="http://${host}:${port}"

echo "Checking $base_url"
curl --noproxy '*' --fail --silent --show-error \
  "$base_url/api/status" >/dev/null

status="$(curl --noproxy '*' --silent --output /dev/null --write-out '%{http_code}' \
  "$base_url/v1/models")"
if [[ "$status" != "401" ]]; then
  echo "Expected unauthenticated /v1/models to return 401, got $status" >&2
  exit 1
fi

echo "Health check passed: /api/status=2xx, /v1/models(no token)=401"
