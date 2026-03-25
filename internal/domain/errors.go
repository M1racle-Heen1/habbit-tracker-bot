package domain

import "errors"

var (
	ErrNotFound    = errors.New("not found")
	ErrAlreadyDone = errors.New("already done today")
	ErrForbidden   = errors.New("forbidden")
)
