//go:build darwin || linux

package main

import (
	"errors"
	"os"
	"syscall"
)

// The OS releases this lock on exit, including SIGKILL. Never unlink a locked
// journal: competing processes must keep locking the same inode.
func lockDerivativeProgress(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("MEDIA_BACKFILL_PROGRESS_LOCKED")
	}
	return nil
}
