#!/usr/bin/env bash
# Takes a consistent backup of the ForgeDeck database.
#
#   backup.sh [-d DIR] [-k KEEP]
#
#   -d DIR   where to write the backup (default ./backups)
#   -k KEEP  how many backups to keep in DIR; older ones are deleted (default 14, 0 keeps all)
#
# Reaching PostgreSQL: see lib.sh (FORGEDECK_PG_CONTAINER or FORGEDECK_DATABASE_URL).
#
# The backup holds ENCRYPTED secrets (Git tokens, registry passwords, project
# secrets), not the key that encrypts them. FORGEDECK_SECRET_KEYS is not in the
# database and not in this file: keep it, separately and safely, or the
# encrypted values cannot be read after a restore. See docs/16-backup-restore.md.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$here/lib.sh"

dir=./backups
keep=14
while getopts "d:k:h" option; do
  case $option in
    d) dir=$OPTARG ;;
    k) keep=$OPTARG ;;
    h) sed -n '2,13p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) exit 2 ;;
  esac
done
case $keep in '' | *[!0-9]*) die "-k must be a number" ;; esac
require_target

# Backups hold every credential ForgeDeck stores, even if encrypted: only the owner reads them.
umask 077
mkdir -p "$dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
file="$dir/forgedeck-$stamp.dump"

# Write to a temporary name so a failed dump never looks like a backup.
trap 'rm -f "$file.partial"' EXIT
dump_database >"$file.partial"
[ -s "$file.partial" ] || die "the dump is empty"
mv "$file.partial" "$file"
(cd "$dir" && sha256sum "$(basename "$file")" >"$(basename "$file").sha256")

if [ "$keep" -gt 0 ]; then
  # Names sort by time. Delete everything but the newest $keep backups and their checksums.
  find "$dir" -maxdepth 1 -name 'forgedeck-*.dump' | sort | head -n "-$keep" | while read -r old; do
    rm -f "$old" "$old.sha256"
  done
fi

echo "backup written: $file ($(wc -c <"$file") bytes)"
echo "REMINDER: this does not contain FORGEDECK_SECRET_KEYS. Back that up separately."
