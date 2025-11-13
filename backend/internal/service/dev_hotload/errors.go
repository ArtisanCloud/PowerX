package devhotload

import "errors"

var (
	ErrFeatureDisabled = errors.New("dev hotload gateway disabled")
	ErrSessionConflict = errors.New("dev hotload session already active")
	ErrSessionNotFound = errors.New("dev hotload session not found")
	ErrReloadToken     = errors.New("dev hotload reload token mismatch")
	ErrCapacityReached = errors.New("dev hotload concurrent session limit reached")
)
