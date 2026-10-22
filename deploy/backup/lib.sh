# Shared by backup.sh and restore.sh (sourced). FORGEDECK_PG_CONTAINER (Docker) or FORGEDECK_DATABASE_URL (host tools).

pg_user="${POSTGRES_USER:-forgedeck}"
pg_db="${POSTGRES_DB:-forgedeck}"

die() { echo "error: $*" >&2; exit 1; }

require_target() {
  if [ -z "${FORGEDECK_PG_CONTAINER:-}" ] && [ -z "${FORGEDECK_DATABASE_URL:-}" ]; then
    die "set FORGEDECK_PG_CONTAINER (PostgreSQL in Docker) or FORGEDECK_DATABASE_URL"
  fi
}

# in_pg <database> <tool> [args...]: run a PostgreSQL tool, passing stdin through
in_pg() {
  local database=$1 tool=$2
  shift 2
  if [ -n "${FORGEDECK_PG_CONTAINER:-}" ]; then
    docker exec -i "$FORGEDECK_PG_CONTAINER" "$tool" -U "$pg_user" -d "$database" "$@"
  else
    "$tool" "$(url_for "$database")" "$@"
  fi
}

url_for() {
  local database=$1 url="${FORGEDECK_DATABASE_URL:?}"
  if [ "$database" = "$(basename "${url%%\?*}")" ]; then
    printf '%s' "$url"
  elif [ "$database" = postgres ] && [ -n "${FORGEDECK_PG_ADMIN_URL:-}" ]; then
    printf '%s' "$FORGEDECK_PG_ADMIN_URL"
  else
    # same server, other database: replace the path component
    printf '%s' "$url" | sed -E "s#^([^/]+//[^/]+)/[^?]*#\1/${database}#"
  fi
}

db_name() {
  if [ -n "${FORGEDECK_PG_CONTAINER:-}" ]; then
    printf '%s' "$pg_db"
  else
    basename "${FORGEDECK_DATABASE_URL%%\?*}"
  fi
}

# pg_dump and pg_restore address the database differently from psql, so they get own helpers
dump_database() {
  if [ -n "${FORGEDECK_PG_CONTAINER:-}" ]; then
    docker exec -i "$FORGEDECK_PG_CONTAINER" pg_dump --format=custom --no-owner --no-acl -U "$pg_user" "$pg_db"
  else
    pg_dump --format=custom --no-owner --no-acl "$FORGEDECK_DATABASE_URL"
  fi
}

restore_into() { # restore_into <database> <dumpfile> [extra pg_restore args...]
  local database=$1 file=$2
  shift 2
  if [ -n "${FORGEDECK_PG_CONTAINER:-}" ]; then
    docker exec -i "$FORGEDECK_PG_CONTAINER" pg_restore --no-owner --no-acl --exit-on-error -U "$pg_user" -d "$database" "$@" <"$file"
  else
    pg_restore --no-owner --no-acl --exit-on-error --dbname="$(url_for "$database")" "$@" "$file"
  fi
}

sql() {
  local database=$1 statement=$2
  in_pg "$database" psql --no-psqlrc --quiet --tuples-only --no-align --set ON_ERROR_STOP=1 --command "$statement"
}
