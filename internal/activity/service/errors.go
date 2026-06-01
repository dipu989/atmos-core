package service

import "errors"

var (
	ErrDuplicate = errors.New("activity already ingested (duplicate idempotency key)")
	ErrNotFound  = errors.New("activity not found")
)
