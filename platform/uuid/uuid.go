package uuid

import "github.com/google/uuid"

// New returns a new UUIDv7 (time-ordered). Panics only on entropy failure,
// which is not expected in any production environment.
func New() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic("uuid: failed to generate UUIDv7: " + err.Error())
	}
	return id
}

// Parse wraps uuid.Parse for consistent import paths across the codebase.
func Parse(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
