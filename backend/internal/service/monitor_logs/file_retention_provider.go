package monitorlogs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileRetentionProvider struct {
	paths []string
}

func NewFileRetentionProvider(paths []string) *FileRetentionProvider {
	out := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for i := range paths {
		p := resolvePath(paths[i])
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return &FileRetentionProvider{paths: out}
}

func (p *FileRetentionProvider) Cleanup(ctx context.Context, cutoff time.Time) (int64, []string) {
	if p == nil || len(p.paths) == 0 {
		return 0, nil
	}
	var deleted int64
	errs := make([]string, 0, 4)
	for i := range p.paths {
		select {
		case <-ctx.Done():
			errs = append(errs, "context canceled")
			return deleted, errs
		default:
		}
		root := p.paths[i]
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if removeOldFile(root, cutoff) {
				deleted++
			}
			continue
		}
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				errs = append(errs, walkErr.Error())
				return nil
			}
			if d == nil || d.IsDir() {
				return nil
			}
			if removeOldFile(path, cutoff) {
				deleted++
			}
			return nil
		})
		if walkErr != nil {
			errs = append(errs, walkErr.Error())
		}
	}
	return deleted, errs
}

func removeOldFile(path string, cutoff time.Time) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if !info.ModTime().Before(cutoff) {
		return false
	}
	if err := os.Remove(path); err != nil {
		return false
	}
	return true
}
