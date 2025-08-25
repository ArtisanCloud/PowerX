package service

// internal/service/organization/errors.go

import "errors"

var (
	ErrParentNotFound   = errors.New("parent department not found")
	ErrInvalidKey       = errors.New("invalid department key")
	ErrKeyExists        = errors.New("department key already exists")
	ErrMoveCreatesCycle = errors.New("cannot move a node under its own subtree")
)

var ErrNotFound = errors.New("not found")
