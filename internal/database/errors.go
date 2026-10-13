package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	return hasCode(err, "23505")
}

// IsUniqueViolationOf reports whether err violates the named unique
// constraint or unique index, so callers can tell which field clashed.
func IsUniqueViolationOf(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// IsForeignKeyViolation reports whether err is a PostgreSQL foreign key
// violation (SQLSTATE 23503), such as deleting a row that is still referenced.
func IsForeignKeyViolation(err error) bool {
	return hasCode(err, "23503")
}

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
