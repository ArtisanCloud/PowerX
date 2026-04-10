package config

import (
	"os"
	"path/filepath"
	"strings"
)

func defaultBackendConfigPath(rel string) string {
	rel = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rel)), "/")
	if root := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT")); root != "" {
		parts := append([]string{filepath.Clean(root), "backend", "config"}, strings.Split(rel, "/")...)
		return filepath.Join(parts...)
	}
	return filepath.ToSlash(filepath.Join(".", "config", filepath.FromSlash(rel)))
}

