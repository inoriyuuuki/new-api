#!/usr/bin/env bash
set -euo pipefail
umask 077

data_dir="${NEW_API_DATA_DIR:-$HOME/new-api_data}"
backup_dir="${NEW_API_BACKUP_DIR:-$data_dir/backups}"
timestamp="$(date +%Y%m%d-%H%M%S)"

mkdir -p "$backup_dir"

for file in .env one-api.db; do
  if [[ ! -f "$data_dir/$file" ]]; then
    echo "Missing required file: $data_dir/$file" >&2
    exit 1
  fi
done

cp -a "$data_dir/.env" "$backup_dir/env-$timestamp"
cp -a "$data_dir/one-api.db" "$backup_dir/one-api-$timestamp.db"

printf 'Backup complete:\n  %s\n  %s\n' \
  "$backup_dir/env-$timestamp" \
  "$backup_dir/one-api-$timestamp.db"

context_db="$data_dir/conversation-context.db"
if [[ -f "$context_db" ]]; then
  context_backup="$backup_dir/conversation-context-$timestamp.db"
  cp -a "$context_db" "$context_backup"
  printf '  %s\n' "$context_backup"
fi
