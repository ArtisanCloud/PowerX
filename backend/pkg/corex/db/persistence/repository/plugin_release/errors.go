package plugin_release

import "errors"

var (
	// ErrNotFound is returned when the target record does not exist.
	ErrNotFound = errors.New("plugin release record not found")

	// ErrDuplicateCandidate indicates a candidate already exists for the tenant/plugin/version combination.
	ErrDuplicateCandidate = errors.New("plugin release candidate already exists")

	// ErrVersionConflict indicates optimistic lock mismatches.
	ErrVersionConflict = errors.New("version conflict")
)
