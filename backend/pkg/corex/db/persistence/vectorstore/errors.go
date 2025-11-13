package vectorstore

import "errors"

var (
	// ErrUnknownDriver indicates the requested driver is not registered.
	ErrUnknownDriver = errors.New("vectorstore: unknown driver")
	// ErrInvalidConfig indicates the provided driver configuration cannot be parsed.
	ErrInvalidConfig = errors.New("vectorstore: invalid driver config")
	// ErrNotImplemented is returned by stub drivers that are not yet implemented.
	ErrNotImplemented = errors.New("vectorstore: driver not implemented")
)
