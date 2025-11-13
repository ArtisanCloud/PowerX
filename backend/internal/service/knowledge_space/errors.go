package knowledge_space

import "errors"

var (
	// ErrInvalidInput indicates request validation failure.
	ErrInvalidInput = errors.New("invalid provisioning input")
	// ErrSpaceConflict indicates tenant/name uniqueness violation.
	ErrSpaceConflict = errors.New("knowledge space already exists")
	// ErrSpaceNotFound indicates the requested space is missing.
	ErrSpaceNotFound = errors.New("knowledge space not found")
	// ErrProvisioningBusy indicates another provisioning flow holds the lock.
	ErrProvisioningBusy = errors.New("tenant provisioning in progress")
	// ErrInvalidStatusTransition indicates unsupported status change.
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	// ErrFusionConflict indicates conflicting fusion publish policy.
	ErrFusionConflict = errors.New("fusion strategy conflict")
	// ErrFusionStrategyNotFound indicates requested strategy does not exist.
	ErrFusionStrategyNotFound = errors.New("fusion strategy not found")
)

// IsConflictError reports whether err is caused by a duplicate space.
func IsConflictError(err error) bool {
	return errors.Is(err, ErrSpaceConflict)
}
