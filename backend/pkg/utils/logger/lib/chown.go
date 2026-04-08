//go:build !linux
// +build !linux

package lumberjack

import (
	"os"
)

// chown changes the user and group owners of the named file.
func chown(name string, info os.FileInfo) error {
	// This is a no-op on non-Unix systems
	return nil
}
