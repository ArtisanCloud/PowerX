package devhotload

import (
	"errors"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
)

var (
	ErrFeatureDisabled = errors.New("dev hotload gateway disabled")
	ErrSessionConflict = errors.New("dev hotload session already active")
	ErrSessionNotFound = errors.New("dev hotload session not found")
	ErrReloadToken     = errors.New("dev hotload reload token mismatch")
	ErrCapacityReached = errors.New("dev hotload concurrent session limit reached")
	ErrForceRequired   = errors.New("force required to delete active sessions")
	ErrForceConfirm    = errors.New("force delete requires confirmation")
	ErrTenantUUIDReq   = errors.New("tenant_uuid is required")
)

// SessionConflictError surfaces information about the conflicting session.
type SessionConflictError struct {
	Session *model.DevHotloadSession
}

func (e *SessionConflictError) Error() string {
	return ErrSessionConflict.Error()
}

func (e *SessionConflictError) Unwrap() error { return ErrSessionConflict }

func newSessionConflictError(session *model.DevHotloadSession) error {
	if session == nil {
		return ErrSessionConflict
	}
	return &SessionConflictError{Session: session}
}
