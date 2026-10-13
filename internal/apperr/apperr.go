// Package apperr defines transport-neutral errors that services return and
// the HTTP layer translates into status codes.
package apperr

// Kind classifies an error for clients.
type Kind int

const (
	KindNotFound Kind = iota + 1
	KindForbidden
	KindConflict
)

// Error is a client-safe error: Message may be shown to users.
type Error struct {
	Kind    Kind
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// NotFound reports a missing resource, or one the caller may not know exists.
func NotFound(message string) *Error {
	return &Error{Kind: KindNotFound, Code: "not_found", Message: message}
}

// Forbidden reports that the caller lacks permission for an action.
func Forbidden(message string) *Error {
	return &Error{Kind: KindForbidden, Code: "forbidden", Message: message}
}

// Conflict reports that the action clashes with current state.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}
