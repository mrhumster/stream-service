package storage

import "errors"

var (
	ErrNotFound          = errors.New("file not found")
	ErrAlreadyExists     = errors.New("file already exists")
	ErrInvalidData       = errors.New("invalid data")
	ErrStorage           = errors.New("storage error")
	ErrBucketNotExist    = errors.New("bucket does not exist")
	ErrPermissionDenied  = errors.New("permission denied")
	ErrGenerateURLFailed = errors.New("failed genarte URL")
)
