//go:build !darwin && !linux

package main

import (
	"errors"
	"os"
)

func lockDerivativeProgress(_ *os.File) error {
	return errors.New("MEDIA_BACKFILL_PROGRESS_PLATFORM_UNSUPPORTED")
}
