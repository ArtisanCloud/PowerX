package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// CopyLocalProgram reuses package path/link validation for an explicitly rebuilt
// local program. It cannot replace an existing directory and is not an HTTP API.
func CopyLocalProgram(ctx context.Context, source, target string) error {
	if !filepath.IsAbs(source) || !filepath.IsAbs(target) {
		return errors.New("local program paths must be absolute")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return errors.New("local program target already exists")
	}
	return copyTree(ctx, source, target)
}
