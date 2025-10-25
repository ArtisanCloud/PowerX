package manager

import "errors"

var (
	ErrRouteNotFound   = errors.New("integration gateway route not found")
	ErrVersionConflict = errors.New("integration gateway version conflict")
	ErrSlugConflict    = errors.New("integration gateway slug conflict")
)
