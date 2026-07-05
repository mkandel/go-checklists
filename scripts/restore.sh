#!/usr/bin/env bash
# Restores a checklists Postgres database from a pg_dump custom-format file
# produced by scripts/backup.sh. DESTRUCTIVE: drops and recreates the
# "checklists" database inside the docker-compose "postgres" service before
# restoring, so any data not in the dump file is lost. Requires the
# postgres service to already be running (docker compose up -d postgres),
# and the app itself should be stopped first so it doesn't write to the
# database mid-restore.
set -euo pipefail

if [ $# -ne 1 ]; then
	echo "usage: $0 <dump-file>" >&2
	exit 1
fi
DUMP_FILE="$1"
if [ ! -f "$DUMP_FILE" ]; then
	echo "no such file: $DUMP_FILE" >&2
	exit 1
fi

read -r -p "This will DROP the current 'checklists' database and restore from $DUMP_FILE. Continue? [y/N] " CONFIRM
case "$CONFIRM" in
	y|Y) ;;
	*) echo "aborted"; exit 1 ;;
esac

docker compose exec -T postgres dropdb -U checklists --if-exists checklists
docker compose exec -T postgres createdb -U checklists -O checklists checklists
docker compose exec -T postgres pg_restore -U checklists -d checklists --no-owner < "$DUMP_FILE"

echo "Restored checklists database from $DUMP_FILE"
