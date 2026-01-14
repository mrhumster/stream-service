package repository

import "errors"

var (
	ErrNotFound         = errors.New("stream not found")
	ErrAlreadyExists    = errors.New("stream already exists")
	ErrPermissionDenied = errors.New("permission denied")
)
