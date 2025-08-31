package service

// internal/service/organization/errors.go

import "errors"

var (
	ErrParentNotFound   = errors.New("parent department not found")
	ErrInvalidKey       = errors.New("invalid department key")
	ErrKeyExists        = errors.New("department key already exists")
	ErrMoveCreatesCycle = errors.New("cannot move a node under its own subtree")
	ErrRoleNotFound     = errors.New("role not found")
	ErrForbidden        = errors.New("forbidden: insufficient privilege")
	ErrInvalidScope     = errors.New("invalid scope (system|tenant)")
	ErrTenantRequired   = errors.New("tenant role requires tenant_id (>0)")
	ErrRoleCodeConflict = errors.New("role code already exists in scope")
)

var ErrNotFound = errors.New("not found")
