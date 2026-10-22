#!/usr/bin/env bash
# Restores a ForgeDeck backup, or checks that one can be restored.
#
#   restore.sh --verify FILE    restore into a temporary database, check it, drop it.
#                               Changes nothing that exists. Run this regularly.
#   restore.sh --yes FILE       REPLACE the configured database with the backup.
#
# Reaching PostgreSQL: see lib.sh (FORGEDECK_PG_CONTAINER or FORGEDECK_DATABASE_URL).
#
# Before a real restore, stop the Control Plane (and anything else using the
# database), and make sure FORGEDECK_SECRET_KEYS is the one that was in use when
# the backup was taken: without it the restored Git tokens, registry passwords
# and project secrets cannot be read. See docs/16-backup-restore.md.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$here/lib.sh"

mode=""
file=""
for argument in "$@"; do
  case $argument in
    --verify) mode=verify ;;
    --yes) mode=restore ;;
    -h | --help) sed -n '2,13p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) die "unknown option $argument" ;;
    *) file=$argument ;;
  esac
done
[ -n "$mode" ] || die "choose --verify (safe check) or --yes (replace the database); see --help"
[ -n "$file" ] || die "give the backup file"
[ -f "$file" ] || die "no such file: $file"
require_target

# The checksum written by backup.sh catches a truncated or damaged file before PostgreSQL does.
if [ -f "$file.sha256" ]; then
  (cd "$(dirname "$file")" && sha256sum --check --quiet "$(basename "$file").sha256") || die "checksum mismatch: $file is damaged"
else
  echo "warning: no $file.sha256 next to the backup, so it cannot be checked for damage" >&2
fi

target_db=$(db_name)

summarize() {
  local database=$1
  for table in users projects servers applications environments releases audit_logs; do
    printf '  %-14s %s\n' "$table" "$(sql "$database" "SELECT count(*) FROM $table")"
  done
  printf '  %-14s %s\n' "migrations" "$(sql "$database" 'SELECT max(version_id) FROM goose_db_version')"
}

if [ "$mode" = verify ]; then
  scratch="forgedeck_verify_$$_$RANDOM"
  cleanup() { sql postgres "DROP DATABASE IF EXISTS $scratch" >/dev/null 2>&1 || true; }
  trap cleanup EXIT
  echo "restoring $file into a temporary database ($scratch)..."
  sql postgres "CREATE DATABASE $scratch" >/dev/null
  restore_into "$scratch" "$file"
  echo "restored. contents:"
  summarize "$scratch"
  echo "OK: the backup restores cleanly. (The temporary database is dropped.)"
  exit 0
fi

echo "REPLACING database $target_db with $file"
restore_into "$target_db" "$file" --clean --if-exists
echo "restored. contents:"
summarize "$target_db"
echo "Start the Control Plane with the SAME FORGEDECK_SECRET_KEYS as before."
