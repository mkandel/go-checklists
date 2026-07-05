#!/usr/bin/env bash
# Dumps the checklists Postgres database to a timestamped, gzip-compressed
# custom-format file (pg_dump -Fc) suitable for pg_restore. Run against the
# docker-compose "postgres" service; requires it to already be running
# (docker compose up -d postgres).
set -euo pipefail

OUT_DIR="${1:-./backups}"
mkdir -p "$OUT_DIR"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="$OUT_DIR/checklists-$STAMP.dump"

docker compose exec -T postgres pg_dump -U checklists -d checklists -Fc > "$OUT_FILE"

echo "Wrote $OUT_FILE"
